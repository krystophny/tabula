package web

import (
	"strings"
	"sync"
	"time"
)

const (
	companionRuntimeStateThinking = "thinking"
	companionRuntimeStateTalking  = "talking"
	companionRuntimeStateError    = "error"

	companionEventState             = "companion_state"
	companionEventTranscriptPartial = "companion_transcript_partial"
	companionEventTranscriptFinal   = "companion_transcript_final"
)

type companionRuntimeSnapshot struct {
	State                string `json:"state"`
	Reason               string `json:"reason,omitempty"`
	Error                string `json:"error,omitempty"`
	WorkspacePath        string `json:"workspace_path,omitempty"`
	ChatSessionID        string `json:"chat_session_id,omitempty"`
	ParticipantSessionID string `json:"participant_session_id,omitempty"`
	ParticipantSegmentID int64  `json:"participant_segment_id,omitempty"`
	TurnID               string `json:"turn_id,omitempty"`
	OutputMode           string `json:"output_mode,omitempty"`
	UpdatedAt            int64  `json:"updated_at"`
}

type companionRuntimeTracker struct {
	mu     sync.Mutex
	states map[string]companionRuntimeSnapshot
}

func newCompanionRuntimeTracker() *companionRuntimeTracker {
	return &companionRuntimeTracker{
		states: map[string]companionRuntimeSnapshot{},
	}
}

func (t *companionRuntimeTracker) set(workspacePath string, snapshot companionRuntimeSnapshot) companionRuntimeSnapshot {
	cleanWorkspacePath := strings.TrimSpace(workspacePath)
	if cleanWorkspacePath == "" {
		return companionRuntimeSnapshot{}
	}
	snapshot.WorkspacePath = cleanWorkspacePath
	snapshot.State = normalizeCompanionRuntimeState(snapshot.State)
	snapshot.Reason = strings.TrimSpace(snapshot.Reason)
	snapshot.Error = strings.TrimSpace(snapshot.Error)
	snapshot.ChatSessionID = strings.TrimSpace(snapshot.ChatSessionID)
	snapshot.ParticipantSessionID = strings.TrimSpace(snapshot.ParticipantSessionID)
	snapshot.TurnID = strings.TrimSpace(snapshot.TurnID)
	snapshot.OutputMode = normalizeTurnOutputMode(snapshot.OutputMode)
	if snapshot.UpdatedAt == 0 {
		snapshot.UpdatedAt = time.Now().Unix()
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.states[cleanWorkspacePath] = snapshot
	return snapshot
}

func (t *companionRuntimeTracker) get(workspacePath string) (companionRuntimeSnapshot, bool) {
	cleanWorkspacePath := strings.TrimSpace(workspacePath)
	if cleanWorkspacePath == "" {
		return companionRuntimeSnapshot{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	snapshot, ok := t.states[cleanWorkspacePath]
	return snapshot, ok
}

func normalizeCompanionRuntimeState(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case companionRuntimeStateListening:
		return companionRuntimeStateListening
	case companionRuntimeStateThinking:
		return companionRuntimeStateThinking
	case companionRuntimeStateTalking:
		return companionRuntimeStateTalking
	case companionRuntimeStateError:
		return companionRuntimeStateError
	default:
		return companionRuntimeStateIdle
	}
}

func (s companionRuntimeSnapshot) payload(eventType string) map[string]interface{} {
	payload := map[string]interface{}{
		"type":           strings.TrimSpace(eventType),
		"state":          s.State,
		"workspace_path": s.WorkspacePath,
		"updated_at":     s.UpdatedAt,
	}
	if s.Reason != "" {
		payload["reason"] = s.Reason
	}
	if s.Error != "" {
		payload["error"] = s.Error
	}
	if s.ChatSessionID != "" {
		payload["chat_session_id"] = s.ChatSessionID
	}
	if s.ParticipantSessionID != "" {
		payload["participant_session_id"] = s.ParticipantSessionID
	}
	if s.ParticipantSegmentID != 0 {
		payload["participant_segment_id"] = s.ParticipantSegmentID
	}
	if s.TurnID != "" {
		payload["turn_id"] = s.TurnID
	}
	if s.OutputMode != "" {
		payload["output_mode"] = s.OutputMode
	}
	return payload
}

func (a *App) chatSessionIDForWorkspacePath(workspacePath string) (string, bool) {
	if a == nil || a.store == nil {
		return "", false
	}
	session, err := a.chatSessionForWorkspacePath(strings.TrimSpace(workspacePath))
	if err != nil {
		return "", false
	}
	return session.ID, true
}

func (a *App) currentCompanionRuntimeState(workspacePath string, cfg companionConfig) companionRuntimeSnapshot {
	workspacePath = strings.TrimSpace(workspacePath)
	if workspacePath == "" {
		return companionRuntimeSnapshot{State: companionRuntimeStateIdle}
	}
	if a != nil && a.companionRuntime != nil {
		if snapshot, ok := a.companionRuntime.get(workspacePath); ok {
			return snapshot
		}
	}
	return a.companionSteadyState(workspacePath, cfg, "")
}

func (a *App) companionSteadyState(workspacePath string, cfg companionConfig, reason string) companionRuntimeSnapshot {
	state := companionRuntimeStateIdle
	if cfg.CompanionEnabled {
		if activeSessions := a.activeCompanionSessionCount(workspacePath); activeSessions > 0 {
			state = companionRuntimeStateListening
			if strings.TrimSpace(reason) == "" {
				reason = "participant_capture_active"
			}
		}
	}
	if strings.TrimSpace(reason) == "" {
		if state == companionRuntimeStateIdle {
			reason = "idle"
		} else {
			reason = "listening"
		}
	}
	return companionRuntimeSnapshot{
		State:         state,
		Reason:        strings.TrimSpace(reason),
		WorkspacePath: strings.TrimSpace(workspacePath),
	}
}

func (a *App) activeCompanionSessionCount(workspacePath string) int {
	if a == nil || a.store == nil {
		return 0
	}
	sessions, err := a.store.ListParticipantSessions(strings.TrimSpace(workspacePath))
	if err != nil {
		return 0
	}
	activeSessions := 0
	for _, session := range sessions {
		if session.EndedAt == 0 {
			activeSessions++
		}
	}
	return activeSessions
}

func (a *App) setCompanionRuntimeState(workspacePath string, snapshot companionRuntimeSnapshot) companionRuntimeSnapshot {
	if a == nil || a.companionRuntime == nil {
		return companionRuntimeSnapshot{}
	}
	return a.companionRuntime.set(workspacePath, snapshot)
}

func (a *App) companionPendingTurnForChatSession(chatSessionID string) (companionPendingTurn, bool) {
	if a == nil || a.companionTurns == nil {
		return companionPendingTurn{}, false
	}
	return a.companionTurns.get(chatSessionID)
}

func (a *App) broadcastCompanionRuntimeState(workspacePath string, snapshot companionRuntimeSnapshot) {
	workspacePath = strings.TrimSpace(workspacePath)
	if workspacePath == "" {
		return
	}
	sessionID, ok := a.chatSessionIDForWorkspacePath(workspacePath)
	if !ok {
		return
	}
	snapshot.ChatSessionID = sessionID
	snapshot = a.setCompanionRuntimeState(workspacePath, snapshot)
	a.broadcastChatEvent(sessionID, snapshot.payload(companionEventState))
}

func (a *App) settleCompanionRuntimeState(workspacePath string, cfg companionConfig, reason string) {
	a.broadcastCompanionRuntimeState(workspacePath, a.companionSteadyState(workspacePath, cfg, reason))
}

func (a *App) broadcastCompanionTranscriptEvent(workspacePath string, payload map[string]interface{}) {
	workspacePath = strings.TrimSpace(workspacePath)
	if workspacePath == "" {
		return
	}
	sessionID, ok := a.chatSessionIDForWorkspacePath(workspacePath)
	if !ok {
		return
	}
	payload["workspace_path"] = workspacePath
	a.broadcastChatEvent(sessionID, payload)
}

func (a *App) markCompanionThinking(sessionID, workspacePath, turnID, outputMode, reason string) {
	pending, ok := a.companionPendingTurnForChatSession(sessionID)
	if !ok {
		return
	}
	a.broadcastCompanionRuntimeState(workspacePath, companionRuntimeSnapshot{
		State:                companionRuntimeStateThinking,
		Reason:               strings.TrimSpace(reason),
		WorkspacePath:        strings.TrimSpace(workspacePath),
		ParticipantSessionID: pending.participantSessionID,
		ParticipantSegmentID: pending.segmentID,
		TurnID:               strings.TrimSpace(turnID),
		OutputMode:           outputMode,
	})
}
