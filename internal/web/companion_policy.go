package web

import (
	"fmt"
	"strings"
	"sync"
	"unicode"

	"github.com/krystophny/tabura/internal/store"
)

const (
	companionInteractionDecisionDisabled         = "disabled"
	companionInteractionDecisionAwaitingAddress  = "awaiting_address"
	companionInteractionDecisionSuppressed       = "suppressed"
	companionInteractionDecisionCooldown         = "cooldown"
	companionInteractionDecisionRespond          = "respond"
	companionInteractionDecisionInterrupt        = "interrupt"
	companionInteractionDecisionAlreadyTriggered = "already_triggered"
	companionInteractionCooldownSeconds          = 8
)

type companionInteractionPolicyState struct {
	Enabled          bool   `json:"enabled"`
	Decision         string `json:"decision"`
	Reason           string `json:"reason"`
	SessionID        string `json:"session_id,omitempty"`
	SegmentID        int64  `json:"segment_id,omitempty"`
	EvaluatedText    string `json:"evaluated_text,omitempty"`
	EvaluatedAt      int64  `json:"evaluated_at,omitempty"`
	PendingSegmentID int64  `json:"pending_segment_id,omitempty"`
	PendingSince     int64  `json:"pending_since,omitempty"`
	CooldownUntil    int64  `json:"cooldown_until,omitempty"`
}

type companionPendingTurn struct {
	participantSessionID string
	segmentID            int64
}

type companionPendingTurnTracker struct {
	mu      sync.Mutex
	pending map[string]companionPendingTurn
}

func newCompanionPendingTurnTracker() *companionPendingTurnTracker {
	return &companionPendingTurnTracker{
		pending: map[string]companionPendingTurn{},
	}
}

func (t *companionPendingTurnTracker) set(chatSessionID, participantSessionID string, segmentID int64) {
	chatSessionID = strings.TrimSpace(chatSessionID)
	participantSessionID = strings.TrimSpace(participantSessionID)
	if chatSessionID == "" || participantSessionID == "" || segmentID == 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pending[chatSessionID] = companionPendingTurn{
		participantSessionID: participantSessionID,
		segmentID:            segmentID,
	}
}

func (t *companionPendingTurnTracker) clear(chatSessionID string) (companionPendingTurn, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	pending, ok := t.pending[strings.TrimSpace(chatSessionID)]
	if !ok {
		return companionPendingTurn{}, false
	}
	delete(t.pending, strings.TrimSpace(chatSessionID))
	return pending, true
}

func (t *companionPendingTurnTracker) get(chatSessionID string) (companionPendingTurn, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	pending, ok := t.pending[strings.TrimSpace(chatSessionID)]
	return pending, ok
}

func (a *App) loadCompanionInteractionPolicy(cfg companionConfig, session *store.ParticipantSession) companionInteractionPolicyState {
	if !cfg.CompanionEnabled || !cfg.DirectedSpeechGateEnabled {
		return evaluateCompanionInteractionPolicy(cfg, session, nil, nil)
	}
	if session == nil {
		return evaluateCompanionInteractionPolicy(cfg, nil, nil, nil)
	}
	segments, err := a.store.ListParticipantSegments(session.ID, 0, 0)
	if err != nil {
		return companionInteractionPolicyState{
			Enabled:   cfg.CompanionEnabled && cfg.DirectedSpeechGateEnabled,
			Decision:  companionInteractionDecisionAwaitingAddress,
			Reason:    "segment_lookup_failed",
			SessionID: session.ID,
		}
	}
	events, err := a.store.ListParticipantEvents(session.ID)
	if err != nil {
		return companionInteractionPolicyState{
			Enabled:   cfg.CompanionEnabled && cfg.DirectedSpeechGateEnabled,
			Decision:  companionInteractionDecisionAwaitingAddress,
			Reason:    "event_lookup_failed",
			SessionID: session.ID,
		}
	}
	return evaluateCompanionInteractionPolicy(cfg, session, segments, events)
}

func evaluateCompanionInteractionPolicy(cfg companionConfig, session *store.ParticipantSession, segments []store.ParticipantSegment, events []store.ParticipantEvent) companionInteractionPolicyState {
	state := companionInteractionPolicyState{
		Enabled:  cfg.CompanionEnabled && cfg.DirectedSpeechGateEnabled,
		Decision: companionInteractionDecisionAwaitingAddress,
		Reason:   "awaiting_direct_address",
	}
	if !cfg.CompanionEnabled {
		state.Decision = companionInteractionDecisionDisabled
		state.Reason = "companion_disabled"
		return state
	}
	if !cfg.DirectedSpeechGateEnabled {
		state.Decision = companionInteractionDecisionDisabled
		state.Reason = "gate_disabled"
		return state
	}
	if session == nil {
		state.Reason = "awaiting_session"
		return state
	}
	state.SessionID = session.ID
	latest := latestMeaningfulParticipantSegment(segments)
	if latest == nil {
		state.Reason = "awaiting_transcript"
		return state
	}
	state.SegmentID = latest.ID
	state.EvaluatedText = strings.TrimSpace(latest.Text)
	state.EvaluatedAt = latest.CommittedAt
	if state.EvaluatedAt == 0 {
		state.EvaluatedAt = latest.EndTS
	}
	pendingSegmentID, pendingSince := latestPendingCompanionSegment(events)
	state.PendingSegmentID = pendingSegmentID
	state.PendingSince = pendingSince
	cooldownUntil := latestCompanionCooldownUntil(events)
	state.CooldownUntil = cooldownUntil

	if participantSegmentAlreadyTriggered(events, latest.ID) {
		state.Decision = companionInteractionDecisionAlreadyTriggered
		state.Reason = "segment_already_triggered"
		return state
	}
	if !isCompanionDirectAddress(latest.Text) {
		state.Decision = companionInteractionDecisionAwaitingAddress
		state.Reason = "not_direct_address"
		return state
	}
	if isCompanionNoiseSuppressed(latest.Text) {
		state.Decision = companionInteractionDecisionSuppressed
		state.Reason = "noise_suppressed"
		return state
	}
	if pendingSegmentID != 0 && pendingSegmentID != latest.ID {
		state.Decision = companionInteractionDecisionInterrupt
		state.Reason = "response_pending"
		return state
	}
	if cooldownUntil > 0 && state.EvaluatedAt > 0 && state.EvaluatedAt < cooldownUntil {
		state.Decision = companionInteractionDecisionCooldown
		state.Reason = "response_cooldown_active"
		return state
	}
	state.Decision = companionInteractionDecisionRespond
	state.Reason = "direct_address_ready"
	return state
}

func latestPendingCompanionSegment(events []store.ParticipantEvent) (int64, int64) {
	pendingSegmentID := int64(0)
	pendingSince := int64(0)
	for _, event := range events {
		switch event.EventType {
		case "assistant_triggered":
			pendingSegmentID = event.SegmentID
			pendingSince = event.CreatedAt
		case "assistant_turn_completed", "assistant_turn_failed", "assistant_turn_cancelled", "assistant_interrupted":
			if pendingSegmentID != 0 && event.SegmentID == pendingSegmentID {
				pendingSegmentID = 0
				pendingSince = 0
			}
		}
	}
	return pendingSegmentID, pendingSince
}

func latestCompanionCooldownUntil(events []store.ParticipantEvent) int64 {
	latest := int64(0)
	for _, event := range events {
		switch event.EventType {
		case "assistant_turn_completed", "assistant_turn_failed", "assistant_turn_cancelled", "assistant_interrupted":
			if event.CreatedAt > latest {
				latest = event.CreatedAt
			}
		}
	}
	if latest == 0 {
		return 0
	}
	return latest + companionInteractionCooldownSeconds
}

func isCompanionNoiseSuppressed(raw string) bool {
	text := normalizeCompanionGateText(raw)
	if text == "" {
		return true
	}
	if companionRequestPattern.MatchString(text) {
		return false
	}
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), unicode.IsSpace(r):
			return r
		default:
			return ' '
		}
	}, text)
	words := strings.Fields(cleaned)
	meaningful := make([]string, 0, len(words))
	for _, word := range words {
		switch word {
		case "tabura", "assistant", "hey", "please":
			continue
		default:
			meaningful = append(meaningful, word)
		}
	}
	if len(meaningful) == 0 {
		return true
	}
	for _, word := range meaningful {
		if !isCompanionNoiseToken(word) {
			return false
		}
	}
	return true
}

func isCompanionNoiseToken(word string) bool {
	switch word {
	case "ahem", "ah", "eh", "er", "hm", "hmm", "mm", "mmm", "okay", "ok", "right", "sure", "thanks", "thank", "uh", "uhh", "um", "umm", "yeah", "yes":
		return true
	default:
		return false
	}
}

func (a *App) noteCompanionPendingTurn(chatSessionID, participantSessionID string, segmentID int64) {
	if a == nil || a.companionTurns == nil {
		return
	}
	a.companionTurns.set(chatSessionID, participantSessionID, segmentID)
}

func (a *App) finishCompanionPendingTurn(chatSessionID, eventType string) {
	if a == nil || a.store == nil || a.companionTurns == nil {
		return
	}
	pending, ok := a.companionTurns.clear(chatSessionID)
	if !ok {
		return
	}
	payload := fmt.Sprintf(`{"chat_session_id":%q}`, strings.TrimSpace(chatSessionID))
	_ = a.store.AddParticipantEvent(pending.participantSessionID, pending.segmentID, strings.TrimSpace(eventType), payload)
	a.syncProjectCompanionArtifactsBySessionID(pending.participantSessionID)
	session, err := a.store.GetParticipantSession(pending.participantSessionID)
	if err != nil {
		return
	}
	project, err := a.store.GetProjectByProjectKey(session.ProjectKey)
	if err != nil {
		return
	}
	switch strings.TrimSpace(eventType) {
	case "assistant_turn_cancelled":
		a.settleCompanionRuntimeState(session.ProjectKey, a.loadCompanionConfig(project), "assistant_turn_cancelled")
	case "assistant_turn_failed":
		a.broadcastCompanionRuntimeState(session.ProjectKey, companionRuntimeSnapshot{
			State:                companionRuntimeStateError,
			Reason:               "assistant_turn_failed",
			ProjectKey:           session.ProjectKey,
			ParticipantSessionID: pending.participantSessionID,
			ParticipantSegmentID: pending.segmentID,
		})
	case "assistant_turn_completed":
		a.settleCompanionRuntimeState(session.ProjectKey, a.loadCompanionConfig(project), "assistant_turn_completed")
	}
}

func (a *App) interruptCompanionPendingTurn(chatSessionID, participantSessionID string, segmentID int64, activeCanceled, queuedCanceled int) {
	if a == nil || a.store == nil {
		return
	}
	if a.companionTurns != nil {
		if pending, ok := a.companionTurns.clear(chatSessionID); ok {
			if strings.TrimSpace(participantSessionID) == "" {
				participantSessionID = pending.participantSessionID
			}
			if segmentID == 0 {
				segmentID = pending.segmentID
			}
		}
	}
	if strings.TrimSpace(participantSessionID) == "" || segmentID == 0 {
		return
	}
	payload := fmt.Sprintf(`{"chat_session_id":%q,"active_canceled":%d,"queued_canceled":%d}`, strings.TrimSpace(chatSessionID), activeCanceled, queuedCanceled)
	_ = a.store.AddParticipantEvent(participantSessionID, segmentID, "assistant_interrupted", payload)
	a.syncProjectCompanionArtifactsBySessionID(participantSessionID)
	session, err := a.store.GetParticipantSession(participantSessionID)
	if err != nil {
		return
	}
	project, err := a.store.GetProjectByProjectKey(session.ProjectKey)
	if err != nil {
		return
	}
	a.settleCompanionRuntimeState(session.ProjectKey, a.loadCompanionConfig(project), "assistant_interrupted")
}
