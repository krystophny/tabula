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
	defaultAssistantLLMTimeout   = 2 * time.Minute
	assistantLLMFastMaxTokens    = 512
	assistantLLMDirectMaxTokens  = 1024
	assistantLLMToolMaxTokens    = 1024
	assistantLLMResponseLimit    = 256 * 1024
	assistantLLMMaxToolRounds    = 6
	assistantLLMMalformedRetries = 2
	localAssistantDialoguePrompt = "You are Tabura's local assistant. Available tools are declared in the request. Use native tool calls only when needed; otherwise answer directly. Default to plain text, not markdown, unless the user explicitly asks for structure, code, a table, or a list. Keep replies brief: for normal questions, answer in one short paragraph with 2-5 sentences. If the user asks a simple factual question, answer in 1-3 sentences. No markdown fences. No <think> tags."
)

func assistantLLMRequestTimeout() time.Duration {
	return parseEnvDurationDefault("TABURA_ASSISTANT_LLM_TIMEOUT", defaultAssistantLLMTimeout)
}

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
	var workspaceRef *store.Workspace
	if workspace, err := a.effectiveWorkspaceForChatSession(session); err == nil {
		workspaceRef = &workspace
	}
	canvasCtx := a.resolveCanvasContext(session.WorkspacePath)
	companionCtx := a.loadCompanionPromptContext(session.WorkspacePath)
	prompt := buildLeanLocalAssistantPrompt(workspaceRef, messages, canvasCtx, companionCtx, outputMode)
	prompt = appendChatCursorPrompt(prompt, cursorCtx)
	prompt = appendCanvasInkPrompt(prompt, inkCtx)
	prompt = appendCanvasPositionPrompt(prompt, positionCtx)
	if strings.TrimSpace(prompt) == "" {
		return "", errors.New("empty prompt")
	}
	prompt, err := a.applyPreAssistantPromptHook(context.Background(), sessionID, session.WorkspacePath, outputMode, session.Mode, prompt)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(prompt) == "" {
		return "", errors.New("empty prompt")
	}
	return prompt, nil
}

func (a *App) runLocalAssistantTurn(req *assistantTurnRequest) {
	if a == nil || req == nil {
		return
	}
	if strings.TrimSpace(a.assistantLLMBaseURL()) == "" {
		if a.tryRunLocalSystemActionTurn(req.sessionID, req.session, req.userText, req.cursorCtx, req.captureMode, req.outputMode, req.localOnly) {
			return
		}
		errText := errLocalAssistantNotConfigured.Error()
		_, _ = a.store.AddChatMessage(req.sessionID, "system", errText, errText, "text")
		a.finishCompanionPendingTurn(req.sessionID, "assistant_turn_failed")
		a.broadcastChatEvent(req.sessionID, map[string]interface{}{"type": "error", "error": errText})
		return
	}
	turnStartedAt := time.Now()

	prompt := strings.TrimSpace(req.promptText)
	if !req.fastMode {
		promptMessages := withQueuedUserMessage(req.messages, req.messageID, req.promptText)
		var err error
		prompt, err = a.buildLocalAssistantPrompt(req.sessionID, req.session, promptMessages, req.cursorCtx, req.inkCtx, req.positionCtx, req.outputMode)
		if err != nil {
			errText := err.Error()
			_, _ = a.store.AddChatMessage(req.sessionID, "system", errText, errText, "text")
			a.finishCompanionPendingTurn(req.sessionID, "assistant_turn_failed")
			a.broadcastChatEvent(req.sessionID, map[string]interface{}{"type": "error", "error": errText})
			return
		}
	}
	if !req.fastMode {
		if compactedPrompt, compacted := compactLocalAssistantPrompt(prompt); compacted {
			prompt = compactedPrompt
			a.broadcastChatEvent(req.sessionID, map[string]any{
				"type": "context_compact",
			})
		}
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

	reply, err := a.runLocalAssistantToolLoop(
		ctx,
		req,
		prompt,
		latestCanvasPositionVisualAttachment(req.positionCtx),
		func(fullText string, delta string) {
			fullText = strings.TrimSpace(fullText)
			delta = strings.TrimSpace(delta)
			if fullText == "" || delta == "" {
				return
			}
			a.broadcastChatEvent(req.sessionID, map[string]any{
				"type":        "assistant_message",
				"turn_id":     runID,
				"output_mode": req.outputMode,
				"message":     fullText,
				"delta":       delta,
			})
		},
	)
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
