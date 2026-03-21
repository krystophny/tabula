package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/krystophny/tabura/internal/store"
)

const (
	assistantModeAuto            = "auto"
	assistantModeLocal           = "local"
	assistantModeCodex           = "codex"
	DefaultAssistantMode         = assistantModeAuto
	assistantLLMRequestTimeout   = 20 * time.Second
	assistantLLMResponseLimit    = 256 * 1024
	assistantLLMMaxTokens        = 2048
	localAssistantDialoguePrompt = "You are Tabura's local assistant. Reply in plain text only. Do not emit JSON, code fences, or tool calls. The request has already passed through local command routing, so answer conversationally and concisely."
)

func normalizeAssistantMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case assistantModeLocal:
		return assistantModeLocal
	case assistantModeCodex:
		return assistantModeCodex
	default:
		return assistantModeAuto
	}
}

func (a *App) assistantRoutingMode() string {
	if a == nil {
		return DefaultAssistantMode
	}
	return normalizeAssistantMode(a.assistantMode)
}

func (a *App) assistantTurnMode(localOnly bool) string {
	if localOnly {
		return assistantModeLocal
	}
	switch a.assistantRoutingMode() {
	case assistantModeLocal:
		return assistantModeLocal
	case assistantModeCodex:
		return assistantModeCodex
	default:
		if a == nil || a.appServerClient == nil {
			return assistantModeLocal
		}
		return assistantModeCodex
	}
}

func (a *App) assistantLLMBaseURL() string {
	if a == nil {
		return ""
	}
	baseURL := strings.TrimSpace(a.assistantLLMURL)
	if baseURL == "" {
		baseURL = strings.TrimSpace(a.intentLLMURL)
	}
	return strings.TrimRight(baseURL, "/")
}

func (a *App) localAssistantLLMModel() string {
	if a == nil {
		return DefaultIntentLLMModel
	}
	if model := strings.TrimSpace(a.assistantLLMModel); model != "" {
		return model
	}
	return a.localIntentLLMModel()
}

func (a *App) requestLocalAssistantMessage(ctx context.Context, prompt string) (string, error) {
	baseURL := a.assistantLLMBaseURL()
	if baseURL == "" {
		return "", errors.New("local assistant is not configured")
	}
	requestBody, _ := json.Marshal(map[string]any{
		"model":       a.localAssistantLLMModel(),
		"temperature": 0,
		"max_tokens":  assistantLLMMaxTokens,
		"chat_template_kwargs": map[string]any{
			"enable_thinking": false,
		},
		"messages": []map[string]string{
			{"role": "system", "content": localAssistantDialoguePrompt},
			{"role": "user", "content": prompt},
		},
	})
	requestCtx, cancel := context.WithTimeout(ctx, assistantLLMRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(
		requestCtx,
		http.MethodPost,
		baseURL+"/v1/chat/completions",
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, assistantLLMResponseLimit))
		return "", fmt.Errorf("assistant llm HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload localIntentLLMChatCompletionResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, assistantLLMResponseLimit)).Decode(&payload); err != nil {
		return "", err
	}
	if len(payload.Choices) == 0 {
		return "", errors.New("assistant llm returned no choices")
	}
	content := strings.TrimSpace(stripCodeFence(payload.Choices[0].Message.Content))
	if content == "" {
		return "", errors.New("assistant llm returned empty content")
	}
	if classification, err := parseIntentPlanClassification(content); err == nil && classification.LocalAnswer != nil {
		if text := strings.TrimSpace(classification.LocalAnswer.Text); text != "" {
			return text, nil
		}
	}
	return content, nil
}

func (a *App) buildLocalAssistantPrompt(sessionID string, session store.ChatSession, messages []store.ChatMessage, cursorCtx *chatCursorContext, inkCtx []*chatCanvasInkEvent, positionCtx []*chatCanvasPositionEvent, outputMode string) (string, error) {
	canvasCtx := a.resolveCanvasContext(session.WorkspacePath)
	companionCtx := a.loadCompanionPromptContext(session.WorkspacePath)
	prompt := buildPromptFromHistoryForSessionWithCompanionPolicy(session.Mode, a.yoloModeEnabled(), sessionID, messages, canvasCtx, companionCtx, outputMode, "")
	prompt = appendChatCursorPrompt(prompt, cursorCtx)
	prompt = appendCanvasInkPrompt(prompt, inkCtx)
	prompt = appendCanvasPositionPrompt(prompt, positionCtx)
	if strings.TrimSpace(prompt) == "" {
		return "", errors.New("empty prompt")
	}
	prompt = a.applyWorkspacePromptContext(session.WorkspacePath, prompt)
	prompt, err := a.applyPreAssistantPromptHook(context.Background(), sessionID, session.WorkspacePath, outputMode, session.Mode, prompt)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(prompt) == "" {
		return "", errors.New("empty prompt")
	}
	return prompt, nil
}

func (a *App) runLocalAssistantTurn(sessionID string, session store.ChatSession, messages []store.ChatMessage, userText string, cursorCtx *chatCursorContext, inkCtx []*chatCanvasInkEvent, positionCtx []*chatCanvasPositionEvent, captureMode string, outputMode string) {
	turnStartedAt := time.Now()
	actionMessage, actionPayloads, handled := a.classifyAndExecuteSystemActionForTurn(context.Background(), sessionID, session, userText, cursorCtx, captureMode)
	if handled {
		if suppressLocalAssistantResponse(actionPayloads) {
			a.finishCompanionPendingTurn(sessionID, "assistant_turn_suppressed")
			return
		}
		runID := randomToken()
		a.broadcastChatEvent(sessionID, map[string]interface{}{
			"type":    "turn_started",
			"turn_id": runID,
		})
		assistantText := strings.TrimSpace(actionMessage)
		if assistantText == "" {
			assistantText = "Done."
		}
		for _, actionPayload := range actionPayloads {
			if actionPayload == nil {
				continue
			}
			a.broadcastSystemActionEvent(sessionID, actionPayload)
		}
		persistedAssistantID := int64(0)
		persistedAssistantText := ""
		a.finalizeAssistantResponseWithMetadata(
			sessionID,
			session.WorkspacePath,
			assistantText,
			&persistedAssistantID,
			&persistedAssistantText,
			"",
			runID,
			"",
			outputMode,
			newAssistantResponseMetadata(a.localAssistantProvider(), a.localAssistantModelLabel(), time.Since(turnStartedAt)),
		)
		return
	}

	prompt, err := a.buildLocalAssistantPrompt(sessionID, session, messages, cursorCtx, inkCtx, positionCtx, outputMode)
	if err != nil {
		errText := err.Error()
		_, _ = a.store.AddChatMessage(sessionID, "system", errText, errText, "text")
		a.finishCompanionPendingTurn(sessionID, "assistant_turn_failed")
		a.broadcastChatEvent(sessionID, map[string]interface{}{"type": "error", "error": errText})
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	runID := randomToken()
	a.registerActiveChatTurn(sessionID, runID, cancel)
	defer func() {
		cancel()
		a.unregisterActiveChatTurn(sessionID, runID)
	}()

	go a.watchCanvasFile(ctx, session.WorkspacePath)
	a.broadcastChatEvent(sessionID, map[string]interface{}{
		"type":    "turn_started",
		"turn_id": runID,
	})

	reply, err := a.requestLocalAssistantMessage(ctx, prompt)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			a.finishCompanionPendingTurn(sessionID, "assistant_turn_cancelled")
			a.broadcastChatEvent(sessionID, map[string]interface{}{
				"type":    "turn_cancelled",
				"turn_id": runID,
			})
			return
		}
		errText := normalizeAssistantError(err)
		_, _ = a.store.AddChatMessage(sessionID, "system", errText, errText, "text")
		a.finishCompanionPendingTurn(sessionID, "assistant_turn_failed")
		a.broadcastChatEvent(sessionID, map[string]interface{}{"type": "error", "error": errText})
		return
	}

	assistantText := strings.TrimSpace(reply)
	if assistantText == "" {
		assistantText = "(assistant returned no content)"
	}
	persistedAssistantID := int64(0)
	persistedAssistantText := ""
	a.finalizeAssistantResponseWithMetadata(
		sessionID,
		session.WorkspacePath,
		assistantText,
		&persistedAssistantID,
		&persistedAssistantText,
		"",
		runID,
		"",
		outputMode,
		newAssistantResponseMetadata(a.localAssistantProvider(), a.localAssistantModelLabel(), time.Since(turnStartedAt)),
	)
}
