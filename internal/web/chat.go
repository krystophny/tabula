package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/krystophny/tabura/internal/appserver"
	"github.com/krystophny/tabura/internal/store"
)

const assistantTurnTimeout = 20 * time.Minute

const (
	turnOutputModeVoice  = "voice"
	turnOutputModeCanvas = "canvas"
)

const (
	assistantLongResponseRuneThreshold = 1400
	assistantListLineThreshold         = 3
	assistantListDensityThreshold      = 40
)

var assistantListLineRe = regexp.MustCompile(`(?m)^\s*(?:[-*+]\s+|\d+[.)]\s+)`)
var codeFenceRe = regexp.MustCompile("(?s)```[\\s\\S]*?```|~~~[\\s\\S]*?~~~")

type chatMessageRequest struct {
	Text       string `json:"text"`
	OutputMode string `json:"output_mode"`
}

type chatCommandRequest struct {
	Command string `json:"command"`
}

func (a *App) handleChatSessionCreate(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	var req struct {
		ProjectKey string `json:"project_key"`
		ProjectID  string `json:"project_id"`
	}
	if r.ContentLength > 0 {
		if err := decodeJSON(r, &req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
	}
	projectKey, err := a.resolveProjectKey(req.ProjectID, req.ProjectKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var resolvedProject store.Project
	projectResolved := false
	if strings.TrimSpace(req.ProjectID) != "" {
		if resolvedProject, err = a.store.GetProject(strings.TrimSpace(req.ProjectID)); err == nil {
			projectResolved = true
		} else if !isNoRows(err) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if !projectResolved {
		if resolvedProject, err = a.store.GetProjectByProjectKey(projectKey); err == nil {
			projectResolved = true
		} else if !isNoRows(err) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	session, err := a.store.GetOrCreateChatSession(projectKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	canvasSessionID := LocalSessionID
	projectID := ""
	if projectResolved {
		projectID = resolvedProject.ID
		canvasSessionID = a.canvasSessionIDForProject(resolvedProject)
		if err := a.store.SetActiveProjectID(resolvedProject.ID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = a.store.TouchProject(resolvedProject.ID)
		if err := a.ensureProjectCanvasReady(resolvedProject); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
	}
	writeJSON(w, map[string]interface{}{
		"ok":                true,
		"session_id":        session.ID,
		"project_key":       session.ProjectKey,
		"project_id":        projectID,
		"mode":              session.Mode,
		"canvas_session_id": canvasSessionID,
	})
}

func (a *App) handleChatSessionHistory(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	sessionID := strings.TrimSpace(chi.URLParam(r, "session_id"))
	if sessionID == "" {
		http.Error(w, "missing session_id", http.StatusBadRequest)
		return
	}
	session, err := a.store.GetChatSession(sessionID)
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	messages, err := a.store.ListChatMessages(sessionID, 1000)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{
		"ok":       true,
		"session":  session,
		"messages": messages,
	})
}

func (a *App) handleChatSessionActivity(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	sessionID := strings.TrimSpace(chi.URLParam(r, "session_id"))
	if sessionID == "" {
		http.Error(w, "missing session_id", http.StatusBadRequest)
		return
	}
	if _, err := a.store.GetChatSession(sessionID); err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	activeTurns := a.activeChatTurnCount(sessionID)
	queuedTurns := a.queuedChatTurnCount(sessionID)
	writeJSON(w, map[string]interface{}{
		"ok":           true,
		"active_turns": activeTurns,
		"queued_turns": queuedTurns,
		"is_working":   activeTurns > 0 || queuedTurns > 0,
	})
}

func (a *App) handleChatSessionCommand(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	sessionID := strings.TrimSpace(chi.URLParam(r, "session_id"))
	if sessionID == "" {
		http.Error(w, "missing session_id", http.StatusBadRequest)
		return
	}
	var req chatCommandRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	result, err := a.executeChatCommand(sessionID, req.Command)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]interface{}{
		"ok":     true,
		"kind":   "command",
		"result": result,
	})
}

func (a *App) handleChatSessionCancel(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	sessionID := strings.TrimSpace(chi.URLParam(r, "session_id"))
	if sessionID == "" {
		http.Error(w, "missing session_id", http.StatusBadRequest)
		return
	}
	if _, err := a.store.GetChatSession(sessionID); err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	activeCanceled, queuedCanceled := a.cancelChatWork(sessionID)
	writeJSON(w, map[string]interface{}{
		"ok":              true,
		"canceled":        activeCanceled + queuedCanceled,
		"active_canceled": activeCanceled,
		"queued_canceled": queuedCanceled,
	})
}

func (a *App) handleChatSessionMessage(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	sessionID := strings.TrimSpace(chi.URLParam(r, "session_id"))
	if sessionID == "" {
		http.Error(w, "missing session_id", http.StatusBadRequest)
		return
	}
	if _, err := a.store.GetChatSession(sessionID); err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	var req chatMessageRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	text := strings.TrimSpace(req.Text)
	outputMode := normalizeTurnOutputMode(req.OutputMode)
	if text == "" {
		http.Error(w, "text is required", http.StatusBadRequest)
		return
	}
	if strings.HasPrefix(text, "/") {
		result, err := a.executeChatCommand(sessionID, text)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]interface{}{
			"ok":     true,
			"kind":   "command",
			"result": result,
		})
		return
	}
	storedUser, err := a.store.AddChatMessage(sessionID, "user", text, text, "text")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.broadcastChatEvent(sessionID, map[string]interface{}{
		"type":    "message_accepted",
		"role":    "user",
		"content": text,
		"id":      storedUser.ID,
	})
	queuedTurns := a.enqueueAssistantTurn(sessionID, outputMode)
	writeJSON(w, map[string]interface{}{
		"ok":         true,
		"kind":       "turn_queued",
		"message_id": storedUser.ID,
		"queued":     queuedTurns,
	})
}

func (a *App) executeChatCommand(sessionID, raw string) (map[string]interface{}, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, errors.New("command is required")
	}
	if strings.HasPrefix(trimmed, "/") {
		trimmed = strings.TrimPrefix(trimmed, "/")
	}
	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return nil, errors.New("command is required")
	}
	name := strings.ToLower(parts[0])
	session, err := a.store.GetChatSession(sessionID)
	if err != nil {
		return nil, err
	}

	switch name {
	case "plan":
		targetMode := session.Mode
		arg := ""
		if len(parts) > 1 {
			arg = strings.ToLower(parts[1])
		}
		switch arg {
		case "", "toggle":
			if targetMode == "plan" {
				targetMode = "chat"
			} else {
				targetMode = "plan"
			}
		case "on":
			targetMode = "plan"
		case "off":
			targetMode = "chat"
		default:
			return nil, errors.New("usage: /plan [on|off]")
		}
		updated, err := a.store.UpdateChatSessionMode(sessionID, targetMode)
		if err != nil {
			return nil, err
		}
		message := fmt.Sprintf("Plan mode %s.", map[bool]string{true: "enabled", false: "disabled"}[updated.Mode == "plan"])
		_, _ = a.store.AddChatMessage(sessionID, "system", message, message, "text")
		a.broadcastChatEvent(sessionID, map[string]interface{}{
			"type":    "mode_changed",
			"mode":    updated.Mode,
			"message": message,
		})
		return map[string]interface{}{
			"name":    "plan",
			"mode":    updated.Mode,
			"message": message,
		}, nil
	case "stop", "cancel":
		activeCanceled, queuedCanceled := a.cancelChatWork(sessionID)
		canceled := activeCanceled + queuedCanceled
		message := "No assistant turn is currently running."
		if canceled > 0 {
			message = "Stopping assistant work and clearing queued prompts."
		}
		return map[string]interface{}{
			"name":            name,
			"canceled":        canceled,
			"active_canceled": activeCanceled,
			"queued_canceled": queuedCanceled,
			"message":         message,
		}, nil
	case "clear":
		if err := a.store.ClearChatMessages(sessionID); err != nil {
			return nil, err
		}
		if err := a.store.ResetChatSessionThread(sessionID); err != nil {
			return nil, err
		}
		a.closeAppSession(sessionID)
		a.broadcastChatEvent(sessionID, map[string]interface{}{
			"type": "chat_cleared",
		})
		return map[string]interface{}{
			"name":    "clear",
			"message": "Chat history cleared.",
		}, nil
	case "compact":
		// Close the current app-server session, forcing a fresh thread on
		// the next turn. The new thread gets only recent local history as
		// initial context, which is equivalent to a context compaction.
		a.closeAppSession(sessionID)
		if err := a.store.ResetChatSessionThread(sessionID); err != nil {
			return nil, err
		}
		message := "Context compacted. Next message starts a fresh app-server thread."
		a.broadcastChatEvent(sessionID, map[string]interface{}{
			"type":    "chat_compacted",
			"message": message,
		})
		return map[string]interface{}{
			"name":    "compact",
			"message": message,
		}, nil
	default:
		return nil, fmt.Errorf("unknown command: /%s", name)
	}
}

func (a *App) registerActiveChatTurn(sessionID, runID string, cancel context.CancelFunc) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.chatTurnCancel[sessionID] == nil {
		a.chatTurnCancel[sessionID] = map[string]context.CancelFunc{}
	}
	a.chatTurnCancel[sessionID][runID] = cancel
}

func (a *App) unregisterActiveChatTurn(sessionID, runID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	runs := a.chatTurnCancel[sessionID]
	if runs == nil {
		return
	}
	delete(runs, runID)
	if len(runs) == 0 {
		delete(a.chatTurnCancel, sessionID)
	}
}

func (a *App) cancelActiveChatTurns(sessionID string) int {
	a.mu.Lock()
	runs := a.chatTurnCancel[sessionID]
	if len(runs) == 0 {
		a.mu.Unlock()
		return 0
	}
	cancels := make([]context.CancelFunc, 0, len(runs))
	for _, cancel := range runs {
		cancels = append(cancels, cancel)
	}
	delete(a.chatTurnCancel, sessionID)
	a.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	return len(cancels)
}

func (a *App) clearQueuedChatTurns(sessionID string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	queued := a.chatTurnQueue[sessionID]
	delete(a.chatTurnQueue, sessionID)
	delete(a.chatTurnOutputMode, sessionID)
	return queued
}

func (a *App) cancelChatWork(sessionID string) (int, int) {
	activeCanceled := a.cancelActiveChatTurns(sessionID)
	queuedCanceled := a.clearQueuedChatTurns(sessionID)
	if queuedCanceled > 0 {
		a.broadcastChatEvent(sessionID, map[string]interface{}{
			"type":  "turn_queue_cleared",
			"count": queuedCanceled,
		})
	}
	return activeCanceled, queuedCanceled
}

func (a *App) activeChatTurnCount(sessionID string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.chatTurnCancel[sessionID])
}

func (a *App) queuedChatTurnCount(sessionID string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.chatTurnQueue[sessionID]
}

func (a *App) enqueueAssistantTurn(sessionID, outputMode string) int {
	mode := normalizeTurnOutputMode(outputMode)
	a.mu.Lock()
	a.chatTurnOutputMode[sessionID] = append(a.chatTurnOutputMode[sessionID], mode)
	a.chatTurnQueue[sessionID] = a.chatTurnQueue[sessionID] + 1
	queued := a.chatTurnQueue[sessionID]
	workerRunning := a.chatTurnWorker[sessionID]
	if !workerRunning {
		a.chatTurnWorker[sessionID] = true
	}
	a.mu.Unlock()
	if !workerRunning {
		go a.runAssistantTurnQueue(sessionID)
	}
	return queued
}

func (a *App) dequeueAssistantTurn(sessionID string) (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	queued := a.chatTurnQueue[sessionID]
	if queued <= 0 {
		return "", false
	}
	modes := a.chatTurnOutputMode[sessionID]
	mode := turnOutputModeVoice
	if len(modes) > 0 {
		mode = normalizeTurnOutputMode(modes[0])
		modes = modes[1:]
		if len(modes) == 0 {
			delete(a.chatTurnOutputMode, sessionID)
		} else {
			a.chatTurnOutputMode[sessionID] = modes
		}
	}
	queued--
	if queued <= 0 {
		delete(a.chatTurnQueue, sessionID)
		delete(a.chatTurnOutputMode, sessionID)
		return mode, true
	}
	a.chatTurnQueue[sessionID] = queued
	return mode, true
}

func (a *App) markAssistantWorkerIdleIfQueueEmpty(sessionID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.chatTurnQueue[sessionID] > 0 {
		return false
	}
	delete(a.chatTurnWorker, sessionID)
	return true
}

func (a *App) runAssistantTurnQueue(sessionID string) {
	for {
		outputMode, ok := a.dequeueAssistantTurn(sessionID)
		if !ok {
			if a.markAssistantWorkerIdleIfQueueEmpty(sessionID) {
				return
			}
			continue
		}
		a.runAssistantTurn(sessionID, outputMode)
	}
}

func (a *App) getOrCreateAppSession(sessionID string, cwd string) (*appserver.Session, bool, error) {
	a.mu.Lock()
	s := a.chatAppSessions[sessionID]
	a.mu.Unlock()
	if s != nil && s.IsOpen() {
		return s, true, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	s, err := a.appServerClient.OpenSessionWithParams(ctx, cwd, a.appServerModel, appServerReasoningParamsForModel(a.appServerModel, a.appServerSparkReasoningEffort))
	if err != nil {
		return nil, false, err
	}
	a.mu.Lock()
	if old := a.chatAppSessions[sessionID]; old != nil {
		_ = old.Close()
	}
	a.chatAppSessions[sessionID] = s
	a.mu.Unlock()
	return s, false, nil
}

func (a *App) closeAppSession(sessionID string) {
	a.mu.Lock()
	s := a.chatAppSessions[sessionID]
	delete(a.chatAppSessions, sessionID)
	a.mu.Unlock()
	if s != nil {
		_ = s.Close()
	}
}

func (a *App) runAssistantTurn(sessionID string, outputMode string) {
	session, err := a.store.GetChatSession(sessionID)
	if err != nil {
		a.broadcastChatEvent(sessionID, map[string]interface{}{"type": "error", "error": err.Error()})
		return
	}
	messages, err := a.store.ListChatMessages(sessionID, 200)
	if err != nil {
		a.broadcastChatEvent(sessionID, map[string]interface{}{"type": "error", "error": err.Error()})
		return
	}
	if a.appServerClient == nil {
		errText := "app-server is not configured"
		_, _ = a.store.AddChatMessage(sessionID, "system", errText, errText, "text")
		a.broadcastChatEvent(sessionID, map[string]interface{}{"type": "error", "error": errText})
		return
	}

	cwd := a.cwdForProjectKey(session.ProjectKey)
	appSess, resumed, sessErr := a.getOrCreateAppSession(sessionID, cwd)
	if sessErr != nil {
		a.runAssistantTurnLegacy(sessionID, session, messages, outputMode)
		return
	}

	canvasCtx := a.resolveCanvasContext(session.ProjectKey)
	var prompt string
	if resumed {
		prompt = buildTurnPrompt(messages, canvasCtx)
	} else {
		prompt = buildPromptFromHistory(session.Mode, messages, canvasCtx)
		_ = a.store.UpdateChatSessionThread(sessionID, appSess.ThreadID())
	}
	if strings.TrimSpace(prompt) == "" {
		a.broadcastChatEvent(sessionID, map[string]interface{}{"type": "error", "error": "empty prompt"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), assistantTurnTimeout)
	runID := randomToken()
	a.registerActiveChatTurn(sessionID, runID, cancel)
	defer func() {
		cancel()
		a.unregisterActiveChatTurn(sessionID, runID)
	}()

	go a.watchCanvasFile(ctx, session.ProjectKey)

	latestMessage := ""
	latestTurnID := ""
	persistedAssistantID := int64(0)
	persistedAssistantText := ""
	persistedAssistantPlain := ""
	persistedAssistantFormat := ""
	persistWriteFailed := false

	persistAssistantSnapshot := func(text string, renderOnCanvas bool) {
		candidateMarkdown, candidatePlain, candidateFormat := assistantSnapshotContent(text, renderOnCanvas)
		if candidateMarkdown == "" && candidatePlain == "" {
			return
		}
		if persistedAssistantID == 0 {
			storedAssistant, storeErr := a.store.AddChatMessage(sessionID, "assistant", candidateMarkdown, candidatePlain, candidateFormat)
			if storeErr != nil {
				if !persistWriteFailed {
					persistWriteFailed = true
					a.broadcastChatEvent(sessionID, map[string]interface{}{
						"type":  "error",
						"error": storeErr.Error(),
					})
				}
				return
			}
			persistedAssistantID = storedAssistant.ID
			persistedAssistantText = candidateMarkdown
			persistedAssistantPlain = candidatePlain
			persistedAssistantFormat = candidateFormat
			return
		}
		if candidateMarkdown == persistedAssistantText &&
			candidatePlain == persistedAssistantPlain &&
			candidateFormat == persistedAssistantFormat {
			return
		}
		if storeErr := a.store.UpdateChatMessageContent(persistedAssistantID, candidateMarkdown, candidatePlain, candidateFormat); storeErr != nil {
			if !persistWriteFailed {
				persistWriteFailed = true
				a.broadcastChatEvent(sessionID, map[string]interface{}{
					"type":  "error",
					"error": storeErr.Error(),
				})
			}
			return
		}
		persistedAssistantText = candidateMarkdown
		persistedAssistantPlain = candidatePlain
		persistedAssistantFormat = candidateFormat
	}

	appResp, err := appSess.SendTurnWithParams(ctx, prompt, "", appServerReasoningParamsForModel(a.appServerModel, a.appServerSparkReasoningEffort), func(ev appserver.StreamEvent) {
		payload := map[string]interface{}{
			"type":      ev.Type,
			"thread_id": ev.ThreadID,
			"turn_id":   ev.TurnID,
		}
		shouldBroadcast := true
		switch ev.Type {
		case "thread_started":
			// Thread ID already stored on session open.
		case "turn_started":
			if strings.TrimSpace(ev.TurnID) != "" {
				latestTurnID = ev.TurnID
			}
		case "assistant_message":
			latestMessage = ev.Message
			latestTurnID = ev.TurnID
			renderOnCanvas := shouldRenderAssistantOutputOnCanvas(outputMode, ev.Message)
			persistAssistantSnapshot(ev.Message, renderOnCanvas)
			payload["message"] = ev.Message
			payload["delta"] = ev.Delta
			if renderOnCanvas {
				payload["render_on_canvas"] = true
			}
		case "turn_completed":
			if strings.TrimSpace(ev.Message) != "" {
				latestMessage = ev.Message
			}
			latestTurnID = ev.TurnID
			renderOnCanvas := shouldRenderAssistantOutputOnCanvas(outputMode, latestMessage)
			persistAssistantSnapshot(latestMessage, renderOnCanvas)
			payload["message"] = latestMessage
			if renderOnCanvas {
				payload["render_on_canvas"] = true
			}
		case "context_usage":
			payload["context_used"] = ev.ContextUsed
			payload["context_max"] = ev.ContextMax
		case "context_compact":
			// pass through to frontend
		case "error":
			if strings.TrimSpace(ev.TurnID) != "" {
				latestTurnID = ev.TurnID
			}
			shouldBroadcast = false
		}
		if shouldBroadcast {
			a.broadcastChatEvent(sessionID, payload)
		}
	})
	if err != nil {
		a.closeAppSession(sessionID)
		if errors.Is(err, context.Canceled) {
			a.broadcastChatEvent(sessionID, map[string]interface{}{
				"type":    "turn_cancelled",
				"turn_id": latestTurnID,
			})
			return
		}
		if errors.Is(err, context.DeadlineExceeded) {
			errText := "assistant request timed out"
			_, _ = a.store.AddChatMessage(sessionID, "system", errText, errText, "text")
			payload := map[string]interface{}{
				"type":  "error",
				"error": errText,
			}
			if strings.TrimSpace(latestTurnID) != "" {
				payload["turn_id"] = latestTurnID
			}
			a.broadcastChatEvent(sessionID, payload)
			return
		}
		errText := normalizeAssistantError(err)
		_, _ = a.store.AddChatMessage(sessionID, "system", errText, errText, "text")
		payload := map[string]interface{}{
			"type":  "error",
			"error": errText,
		}
		if strings.TrimSpace(latestTurnID) != "" {
			payload["turn_id"] = latestTurnID
		}
		a.broadcastChatEvent(sessionID, payload)
		return
	}

	assistantText := strings.TrimSpace(appResp.Message)
	if assistantText == "" {
		assistantText = strings.TrimSpace(latestMessage)
	}
	if assistantText == "" {
		assistantText = "(assistant returned no content)"
	}

	assistantText = a.finalizeAssistantResponse(sessionID, session.ProjectKey, assistantText,
		&persistedAssistantID, &persistedAssistantText, appResp.TurnID, latestTurnID, appResp.ThreadID, outputMode)
	_ = assistantText
}

// runAssistantTurnLegacy is the single-shot fallback when persistent session
// fails to connect. Each call creates a new WS + thread.
func (a *App) runAssistantTurnLegacy(sessionID string, session store.ChatSession, messages []store.ChatMessage, outputMode string) {
	canvasCtx := a.resolveCanvasContext(session.ProjectKey)
	prompt := buildPromptFromHistory(session.Mode, messages, canvasCtx)
	if strings.TrimSpace(prompt) == "" {
		a.broadcastChatEvent(sessionID, map[string]interface{}{"type": "error", "error": "empty prompt"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), assistantTurnTimeout)
	runID := randomToken()
	a.registerActiveChatTurn(sessionID, runID, cancel)
	defer func() {
		cancel()
		a.unregisterActiveChatTurn(sessionID, runID)
	}()

	go a.watchCanvasFile(ctx, session.ProjectKey)

	latestMessage := ""
	latestTurnID := ""
	persistedAssistantID := int64(0)
	persistedAssistantText := ""
	persistedAssistantPlain := ""
	persistedAssistantFormat := ""
	persistWriteFailed := false
	persistAssistantSnapshot := func(text string, renderOnCanvas bool) {
		candidateMarkdown, candidatePlain, candidateFormat := assistantSnapshotContent(text, renderOnCanvas)
		if candidateMarkdown == "" && candidatePlain == "" {
			return
		}
		if persistedAssistantID == 0 {
			storedAssistant, storeErr := a.store.AddChatMessage(sessionID, "assistant", candidateMarkdown, candidatePlain, candidateFormat)
			if storeErr != nil {
				if !persistWriteFailed {
					persistWriteFailed = true
					a.broadcastChatEvent(sessionID, map[string]interface{}{
						"type":  "error",
						"error": storeErr.Error(),
					})
				}
				return
			}
			persistedAssistantID = storedAssistant.ID
			persistedAssistantText = candidateMarkdown
			persistedAssistantPlain = candidatePlain
			persistedAssistantFormat = candidateFormat
			return
		}
		if candidateMarkdown == persistedAssistantText &&
			candidatePlain == persistedAssistantPlain &&
			candidateFormat == persistedAssistantFormat {
			return
		}
		if storeErr := a.store.UpdateChatMessageContent(persistedAssistantID, candidateMarkdown, candidatePlain, candidateFormat); storeErr != nil {
			if !persistWriteFailed {
				persistWriteFailed = true
				a.broadcastChatEvent(sessionID, map[string]interface{}{
					"type":  "error",
					"error": storeErr.Error(),
				})
			}
			return
		}
		persistedAssistantText = candidateMarkdown
		persistedAssistantPlain = candidatePlain
		persistedAssistantFormat = candidateFormat
	}

	appResp, err := a.appServerClient.SendPromptStream(ctx, appserver.PromptRequest{
		CWD:          a.cwdForProjectKey(session.ProjectKey),
		Prompt:       prompt,
		Model:        a.appServerModel,
		ThreadParams: appServerReasoningParamsForModel(a.appServerModel, a.appServerSparkReasoningEffort),
		TurnParams:   appServerReasoningParamsForModel(a.appServerModel, a.appServerSparkReasoningEffort),
		Timeout:      assistantTurnTimeout,
	}, func(ev appserver.StreamEvent) {
		payload := map[string]interface{}{
			"type":      ev.Type,
			"thread_id": ev.ThreadID,
			"turn_id":   ev.TurnID,
		}
		shouldBroadcast := true
		switch ev.Type {
		case "thread_started":
			if strings.TrimSpace(ev.ThreadID) != "" {
				_ = a.store.UpdateChatSessionThread(sessionID, ev.ThreadID)
			}
		case "turn_started":
			if strings.TrimSpace(ev.TurnID) != "" {
				latestTurnID = ev.TurnID
			}
		case "assistant_message":
			latestMessage = ev.Message
			latestTurnID = ev.TurnID
			renderOnCanvas := shouldRenderAssistantOutputOnCanvas(outputMode, ev.Message)
			persistAssistantSnapshot(ev.Message, renderOnCanvas)
			payload["message"] = ev.Message
			payload["delta"] = ev.Delta
			if renderOnCanvas {
				payload["render_on_canvas"] = true
			}
		case "turn_completed":
			if strings.TrimSpace(ev.Message) != "" {
				latestMessage = ev.Message
			}
			latestTurnID = ev.TurnID
			renderOnCanvas := shouldRenderAssistantOutputOnCanvas(outputMode, latestMessage)
			persistAssistantSnapshot(latestMessage, renderOnCanvas)
			payload["message"] = latestMessage
			if renderOnCanvas {
				payload["render_on_canvas"] = true
			}
		case "error":
			if strings.TrimSpace(ev.TurnID) != "" {
				latestTurnID = ev.TurnID
			}
			shouldBroadcast = false
		}
		if shouldBroadcast {
			a.broadcastChatEvent(sessionID, payload)
		}
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			a.broadcastChatEvent(sessionID, map[string]interface{}{
				"type":    "turn_cancelled",
				"turn_id": latestTurnID,
			})
			return
		}
		errText := normalizeAssistantError(err)
		_, _ = a.store.AddChatMessage(sessionID, "system", errText, errText, "text")
		payload := map[string]interface{}{
			"type":  "error",
			"error": errText,
		}
		if strings.TrimSpace(latestTurnID) != "" {
			payload["turn_id"] = latestTurnID
		}
		a.broadcastChatEvent(sessionID, payload)
		return
	}

	assistantText := strings.TrimSpace(appResp.Message)
	if assistantText == "" {
		assistantText = strings.TrimSpace(latestMessage)
	}
	if assistantText == "" {
		assistantText = "(assistant returned no content)"
	}

	assistantText = a.finalizeAssistantResponse(sessionID, session.ProjectKey, assistantText,
		&persistedAssistantID, &persistedAssistantText, appResp.TurnID, latestTurnID, appResp.ThreadID, outputMode)
	_ = assistantText
}

// finalizeAssistantResponse handles post-processing shared by both turn paths:
// parse and execute canvas/file blocks, strip lang tags, route text to chat/canvas,
// persist final text, and broadcast assistant_output.
func (a *App) finalizeAssistantResponse(
	sessionID, projectKey, text string,
	persistedID *int64, persistedText *string,
	turnID, fallbackTurnID, threadID string,
	outputMode string,
) string {
	outputMode = normalizeTurnOutputMode(outputMode)
	renderOnCanvas := false
	canvasSessionID := a.resolveCanvasSessionID(projectKey)
	hasCanvasBlocks := false
	hasFileBlocks := false
	if cBlocks, cleaned := parseCanvasBlocks(text); len(cBlocks) > 0 {
		hasCanvasBlocks = true
		if canvasSessionID != "" {
			a.executeCanvasBlocks(canvasSessionID, cBlocks)
		}
		text = cleaned
	}
	if fBlocks, cleaned := parseFileBlocks(text); len(fBlocks) > 0 {
		hasFileBlocks = true
		if canvasSessionID != "" {
			a.executeFileBlocks(canvasSessionID, fBlocks)
		}
		text = cleaned
	}
	text = stripLangTags(text)
	renderOnCanvas = hasCanvasBlocks || hasFileBlocks
	if !renderOnCanvas && canvasSessionID != "" && shouldRenderAssistantOutputOnCanvas(outputMode, text) {
		renderOnCanvas = true
	}

	chatMarkdown := text
	chatPlain := text
	renderFormat := "markdown"
	assistantOutputTitle := ""
	if outputMode == turnOutputModeCanvas || renderOnCanvas {
		canvasText := strings.TrimSpace(stripCanvasFileMarkers(text))
		if canvasSessionID != "" {
			if canvasText != "" {
				a.executeAssistantTextBlock(canvasSessionID, "Assistant Output", canvasText)
				assistantOutputTitle = "Assistant Output"
			}
			// Keep plain text for prompt continuity, but suppress assistant markdown in chat UI.
			chatMarkdown = ""
			chatPlain = canvasText
			renderFormat = "text"
			renderOnCanvas = true
		}
	}

	a.refreshCanvasFromDisk(projectKey)

	if *persistedID == 0 {
		stored, err := a.store.AddChatMessage(sessionID, "assistant", chatMarkdown, chatPlain, renderFormat)
		if err != nil {
			a.broadcastChatEvent(sessionID, map[string]interface{}{"type": "error", "error": err.Error()})
			return chatMarkdown
		}
		*persistedID = stored.ID
		*persistedText = chatMarkdown
	} else {
		if err := a.store.UpdateChatMessageContent(*persistedID, chatMarkdown, chatPlain, renderFormat); err != nil {
			a.broadcastChatEvent(sessionID, map[string]interface{}{"type": "error", "error": err.Error()})
			return chatMarkdown
		}
		*persistedText = chatMarkdown
	}
	tid := strings.TrimSpace(turnID)
	if tid == "" {
		tid = fallbackTurnID
	}
	payload := map[string]interface{}{
		"type":             "assistant_output",
		"role":             "assistant",
		"id":               *persistedID,
		"turn_id":          tid,
		"thread_id":        threadID,
		"message":          chatMarkdown,
		"render_on_canvas": renderOnCanvas,
	}
	if assistantOutputTitle != "" {
		payload["title"] = assistantOutputTitle
	}
	a.broadcastChatEvent(sessionID, payload)
	return chatMarkdown
}

func shouldRenderAssistantOutputOnCanvas(outputMode, text string) bool {
	if normalizeTurnOutputMode(outputMode) == turnOutputModeCanvas {
		return true
	}
	clean := strings.TrimSpace(stripCanvasFileMarkers(stripLangTags(text)))
	if clean == "" {
		return false
	}
	return shouldRenderAssistantTextInCanvas(clean)
}

func assistantSnapshotContent(text string, renderOnCanvas bool) (string, string, string) {
	candidate := strings.TrimSpace(text)
	if candidate == "" {
		return "", "", "markdown"
	}
	if !renderOnCanvas {
		return candidate, candidate, "markdown"
	}
	plain := strings.TrimSpace(stripCanvasFileMarkers(stripLangTags(candidate)))
	if plain == "" {
		plain = candidate
	}
	return "", plain, "text"
}

func shouldRenderAssistantTextInCanvas(text string) bool {
	clean := strings.TrimSpace(text)
	if clean == "" {
		return false
	}
	if utf8.RuneCountInString(clean) >= assistantLongResponseRuneThreshold {
		return true
	}
	return looksListHeavy(stripCodeFences(clean))
}

func stripCodeFences(text string) string {
	if text == "" {
		return ""
	}
	return strings.TrimSpace(codeFenceRe.ReplaceAllString(text, "\n"))
}

func looksListHeavy(text string) bool {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		return false
	}
	listLines := 0
	totalLines := 0
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		totalLines++
		if assistantListLineRe.MatchString(line) {
			listLines++
		}
	}
	if totalLines == 0 {
		return false
	}
	if listLines >= assistantListLineThreshold {
		return true
	}
	return listLines*100/totalLines >= assistantListDensityThreshold
}

func normalizeTurnOutputMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case turnOutputModeCanvas:
		return turnOutputModeCanvas
	default:
		return turnOutputModeVoice
	}
}

func (a *App) cwdForProjectKey(projectKey string) string {
	key := strings.TrimSpace(projectKey)
	if key != "" {
		if project, err := a.store.GetProjectByProjectKey(key); err == nil {
			root := strings.TrimSpace(project.RootPath)
			if root != "" {
				return root
			}
		}
		return key
	}
	if strings.TrimSpace(a.localProjectDir) != "" {
		return strings.TrimSpace(a.localProjectDir)
	}
	return "."
}

func normalizeAssistantError(err error) string {
	if err == nil {
		return "assistant request failed"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "assistant request timed out"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "assistant request timed out"
	}
	errText := strings.TrimSpace(err.Error())
	if errText == "" {
		return "assistant request failed"
	}
	if strings.Contains(strings.ToLower(errText), "i/o timeout") {
		return "assistant request timed out"
	}
	return errText
}

func (a *App) resolveCanvasSessionID(projectKey string) string {
	key := strings.TrimSpace(projectKey)
	if key == "" {
		return ""
	}
	project, err := a.store.GetProjectByProjectKey(key)
	if err != nil {
		return ""
	}
	return a.canvasSessionIDForProject(project)
}

func (a *App) resolveCanvasContext(projectKey string) *canvasContext {
	key := strings.TrimSpace(projectKey)
	if key == "" {
		return nil
	}
	project, err := a.store.GetProjectByProjectKey(key)
	if err != nil {
		return nil
	}
	sid := a.canvasSessionIDForProject(project)
	a.mu.Lock()
	port, ok := a.tunnelPorts[sid]
	a.mu.Unlock()
	if !ok {
		return nil
	}
	status, err := a.mcpToolsCall(port, "canvas_status", map[string]interface{}{"session_id": sid})
	if err != nil {
		return nil
	}
	active, _ := status["active_artifact"].(map[string]interface{})
	if active == nil {
		return nil
	}
	title := strings.TrimSpace(fmt.Sprint(active["title"]))
	if title == "<nil>" {
		title = ""
	}
	kind := strings.TrimSpace(fmt.Sprint(active["kind"]))
	if kind == "<nil>" {
		kind = ""
	}
	return &canvasContext{HasArtifact: true, ArtifactTitle: title, ArtifactKind: kind}
}

type canvasContext struct {
	HasArtifact   bool
	ArtifactTitle string
	ArtifactKind  string
}

func buildPromptFromHistory(mode string, messages []store.ChatMessage, canvas *canvasContext) string {
	const maxHistory = 80
	if len(messages) > maxHistory {
		messages = messages[len(messages)-maxHistory:]
	}
	var b strings.Builder

	b.WriteString("You are Tabura, an AI assistant. Everything you write is spoken aloud via TTS except content inside :::canvas{} and :::file{} blocks.\n\n")
	b.WriteString("Prefer plain-language prose for spoken responses. Use markdown only when required inside non-spoken channels (canvas/file), because markdown symbols are read literally by TTS.\n\n")
	b.WriteString("When the response is long, list-heavy, or over roughly 1.4k chars, render the full response as a canvas artifact using :::canvas{title=\"Assistant Output\"}.\n")
	b.WriteString("Short confirmations may remain as spoken chat responses.\n\n")
	b.WriteString("## Response Format\n\n")
	b.WriteString("Write naturally. Your text is read aloud, so avoid raw paths, URLs, or code in prose.\n")
	b.WriteString("Use [lang:de] at the start of your answer when responding in German. Default is English.\n\n")
	b.WriteString("Visual content (not spoken):\n")
	b.WriteString("- :::canvas{title=\"Title\"}...:::  ephemeral display (analysis, reports)\n")
	b.WriteString("- :::file{path=\"filename.go\"}...:::  file-bound artifact (code, config)\n")
	b.WriteString("- Line references: when the user mentions [Line N of \"file\"], apply at that location.\n\n")

	b.WriteString("## Delegation\n")
	b.WriteString("Use `delegate_to_model` for tasks that benefit from another model.\n")
	b.WriteString("- 'let codex do this' / 'ask codex' -> model='codex'. 'ask gpt' / 'use the big model' -> model='gpt'.\n")
	b.WriteString("- Auto-delegate complex multi-file coding or deep analysis to 'codex'.\n")
	b.WriteString("- Provide 'context' and 'system_prompt' when delegating.\n")
	b.WriteString("- Do NOT delegate simple conversational replies.\n")
	b.WriteString("- Delegates have full filesystem access and edit files directly on disk.\n")
	b.WriteString("- Do NOT parse or apply patches/diffs from the delegate response.\n")
	b.WriteString("- The delegate result includes 'files_changed' (list of modified file paths) and 'message' (summary).\n")
	b.WriteString("- Relay the delegate summary to the user (spoken or canvas as appropriate).\n\n")

	if canvas != nil && canvas.HasArtifact {
		b.WriteString("## Current Artifact\n")
		fmt.Fprintf(&b, "- Active artifact tab: %q (kind: %s)\n\n", canvas.ArtifactTitle, canvas.ArtifactKind)
	}

	if strings.EqualFold(strings.TrimSpace(mode), "plan") {
		b.WriteString("You are in plan mode. Focus on analysis, design, and specification before implementation.\n\n")
	}

	b.WriteString("Conversation transcript:\n")
	for _, msg := range messages {
		content := strings.TrimSpace(msg.ContentPlain)
		if content == "" {
			content = strings.TrimSpace(msg.ContentMarkdown)
		}
		if content == "" {
			continue
		}
		role := strings.ToUpper(strings.TrimSpace(msg.Role))
		if role == "" {
			role = "USER"
		}
		b.WriteString(role)
		b.WriteString(":\n")
		if role == "USER" {
			content = applyDelegationHints(content)
		}
		b.WriteString(content)
		b.WriteString("\n\n")
	}
	b.WriteString("Reply as ASSISTANT.")
	return b.String()
}

// buildTurnPrompt constructs a prompt for a resumed thread: only the latest
// user message plus optional canvas context update.
func buildTurnPrompt(messages []store.ChatMessage, canvas *canvasContext) string {
	var lastUserMsg string
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.EqualFold(strings.TrimSpace(messages[i].Role), "user") {
			lastUserMsg = strings.TrimSpace(messages[i].ContentPlain)
			if lastUserMsg == "" {
				lastUserMsg = strings.TrimSpace(messages[i].ContentMarkdown)
			}
			break
		}
	}
	if lastUserMsg == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("For long list-style or >1.4k char answers, use :::canvas{title=\"Assistant Output\"}.\n\n")
	if canvas != nil && canvas.HasArtifact {
		fmt.Fprintf(&b, "[Active artifact tab: %q (kind: %s)]\n\n", canvas.ArtifactTitle, canvas.ArtifactKind)
	}
	b.WriteString(applyDelegationHints(lastUserMsg))
	return b.String()
}

type delegationHint struct {
	Detected bool
	Model    string
	Task     string
}

var delegationPatterns = regexp.MustCompile(
	`(?i)^(?:let |ask |use )(codex|gpt|spark|the big model)\b[,: ]*(.*)`,
)

func detectDelegationHint(text string) delegationHint {
	m := delegationPatterns.FindStringSubmatch(strings.TrimSpace(text))
	if m == nil {
		return delegationHint{}
	}
	model := strings.ToLower(m[1])
	if model == "the big model" {
		model = "gpt"
	}
	task := strings.TrimSpace(m[2])
	if task == "" {
		task = text
	}
	return delegationHint{Detected: true, Model: model, Task: task}
}

func applyDelegationHints(text string) string {
	hint := detectDelegationHint(text)
	if !hint.Detected {
		return text
	}
	return fmt.Sprintf("[Delegation hint: user wants model=%q] %s", hint.Model, text)
}

func (a *App) handleChatWS(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	sessionID := strings.TrimSpace(chi.URLParam(r, "session_id"))
	if sessionID == "" {
		http.Error(w, "missing session_id", http.StatusBadRequest)
		return
	}
	if _, err := a.store.GetChatSession(sessionID); err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	ws, err := a.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	conn := newChatWSConn(ws)
	a.mu.Lock()
	if a.chatWS[sessionID] == nil {
		a.chatWS[sessionID] = map[*chatWSConn]struct{}{}
	}
	a.chatWS[sessionID][conn] = struct{}{}
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		if set := a.chatWS[sessionID]; set != nil {
			delete(set, conn)
		}
		a.mu.Unlock()
		_ = ws.Close()
	}()

	if session, err := a.store.GetChatSession(sessionID); err == nil {
		_ = conn.writeJSON(map[string]interface{}{
			"type": "mode_changed",
			"mode": session.Mode,
		})
	}
	for {
		mt, data, err := ws.ReadMessage()
		if err != nil {
			return
		}
		switch mt {
		case websocket.BinaryMessage:
			handleSTTBinaryChunk(conn, data)
		case websocket.TextMessage:
			handleChatWSTextMessage(a, conn, sessionID, data)
		}
	}
}

func (a *App) broadcastChatEvent(sessionID string, payload map[string]interface{}) {
	payload["session_id"] = sessionID
	encoded, _ := json.Marshal(payload)
	turnID, _ := payload["turn_id"].(string)
	eventType, _ := payload["type"].(string)
	_ = a.store.AddChatEvent(sessionID, turnID, eventType, string(encoded))

	a.mu.Lock()
	clients := make([]*chatWSConn, 0)
	for conn := range a.chatWS[sessionID] {
		clients = append(clients, conn)
	}
	a.mu.Unlock()
	for _, conn := range clients {
		_ = conn.writeText(encoded)
	}
}
