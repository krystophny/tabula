package web

import (
	"context"
	"errors"
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
	assistantLLMMaxTokens        = 4096
	assistantLLMMaxToolRounds    = 6
	assistantLLMMalformedRetries = 2
	localAssistantDialoguePrompt = "You are Tabura's local assistant. Use tools when the task depends on workspace state, shell inspection, or MCP capabilities. Prefer native tool calls when the model supports them. If native tool calls are unavailable, respond with JSON only using either {\"tool_calls\":[...]} or {\"final\":\"...\"}. Available tools: shell with command and optional cwd, and mcp with name, arguments, and optional mcp_url. When a tool fails, recover with another tool call or explain the failure clearly. Do not emit markdown fences around JSON."
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

func (a *App) runLocalAssistantTurn(req *assistantTurnRequest, evaluation *localTurnEvaluation) {
	if a == nil || req == nil {
		return
	}
	turnStartedAt := time.Now()
	eval := evaluation
	if eval == nil {
		computed := a.evaluateLocalTurn(
			context.Background(),
			req.sessionID,
			req.session,
			req.userText,
			req.cursorCtx,
			req.captureMode,
		)
		eval = &computed
	}
	if eval != nil && eval.handled {
		if suppressLocalAssistantResponse(eval.payloads) {
			a.finishCompanionPendingTurn(req.sessionID, "assistant_turn_suppressed")
			return
		}
		runID := randomToken()
		a.broadcastChatEvent(req.sessionID, map[string]interface{}{
			"type":    "turn_started",
			"turn_id": runID,
		})
		assistantText := strings.TrimSpace(eval.text)
		if assistantText == "" {
			assistantText = "Done."
		}
		for _, actionPayload := range eval.payloads {
			if actionPayload == nil {
				continue
			}
			a.broadcastSystemActionEvent(req.sessionID, actionPayload)
		}
		persistedAssistantID := int64(0)
		persistedAssistantText := ""
		a.finalizeAssistantResponseWithMetadata(
			req.sessionID,
			req.session.WorkspacePath,
			assistantText,
			&persistedAssistantID,
			&persistedAssistantText,
			"",
			runID,
			"",
			req.outputMode,
			newAssistantResponseMetadata(a.localAssistantProvider(), a.localAssistantModelLabel(), time.Since(turnStartedAt)),
		)
		return
	}

	prompt, err := a.buildLocalAssistantPrompt(req.sessionID, req.session, req.messages, req.cursorCtx, req.inkCtx, req.positionCtx, req.outputMode)
	if err != nil {
		errText := err.Error()
		_, _ = a.store.AddChatMessage(req.sessionID, "system", errText, errText, "text")
		a.finishCompanionPendingTurn(req.sessionID, "assistant_turn_failed")
		a.broadcastChatEvent(req.sessionID, map[string]interface{}{"type": "error", "error": errText})
		return
	}
	if compactedPrompt, compacted := compactLocalAssistantPrompt(prompt); compacted {
		prompt = compactedPrompt
		a.broadcastChatEvent(req.sessionID, map[string]any{
			"type": "context_compact",
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	runID := randomToken()
	a.registerActiveChatTurn(req.sessionID, runID, cancel)
	defer func() {
		cancel()
		a.unregisterActiveChatTurn(req.sessionID, runID)
	}()

	go a.watchCanvasFile(ctx, req.session.WorkspacePath)
	a.broadcastChatEvent(req.sessionID, map[string]interface{}{
		"type":    "turn_started",
		"turn_id": runID,
	})

	reply, err := a.runLocalAssistantToolLoop(ctx, req, prompt, latestCanvasPositionVisualAttachment(req.positionCtx))
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			a.finishCompanionPendingTurn(req.sessionID, "assistant_turn_cancelled")
			a.broadcastChatEvent(req.sessionID, map[string]interface{}{
				"type":    "turn_cancelled",
				"turn_id": runID,
			})
			return
		}
		errText := normalizeAssistantError(err)
		_, _ = a.store.AddChatMessage(req.sessionID, "system", errText, errText, "text")
		a.finishCompanionPendingTurn(req.sessionID, "assistant_turn_failed")
		a.broadcastChatEvent(req.sessionID, map[string]interface{}{"type": "error", "error": errText})
		return
	}

	assistantText := strings.TrimSpace(reply)
	if assistantText == "" {
		assistantText = "(assistant returned no content)"
	}
	persistedAssistantID := int64(0)
	persistedAssistantText := ""
	a.finalizeAssistantResponseWithMetadata(
		req.sessionID,
		req.session.WorkspacePath,
		assistantText,
		&persistedAssistantID,
		&persistedAssistantText,
		"",
		runID,
		"",
		req.outputMode,
		newAssistantResponseMetadata(a.localAssistantProvider(), a.localAssistantModelLabel(), time.Since(turnStartedAt)),
	)
}
