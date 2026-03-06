package web

import (
	"testing"

	"github.com/krystophny/tabura/internal/store"
)

func TestEvaluateCompanionInteractionPolicyRespondsToDirectAddress(t *testing.T) {
	cfg := defaultCompanionConfig()
	cfg.CompanionEnabled = true
	cfg.DirectedSpeechGateEnabled = true

	session := &store.ParticipantSession{ID: "psess-1", ProjectKey: "proj"}
	segments := []store.ParticipantSegment{
		{ID: 1, SessionID: session.ID, Text: "Tabura, draft the summary.", CommittedAt: 100},
	}

	policy := evaluateCompanionInteractionPolicy(cfg, session, segments, nil)
	if policy.Decision != companionInteractionDecisionRespond {
		t.Fatalf("decision = %q, want %q", policy.Decision, companionInteractionDecisionRespond)
	}
	if policy.Reason != "direct_address_ready" {
		t.Fatalf("reason = %q, want direct_address_ready", policy.Reason)
	}
}

func TestEvaluateCompanionInteractionPolicySuppressesNoise(t *testing.T) {
	cfg := defaultCompanionConfig()
	cfg.CompanionEnabled = true
	cfg.DirectedSpeechGateEnabled = true

	session := &store.ParticipantSession{ID: "psess-1", ProjectKey: "proj"}
	segments := []store.ParticipantSegment{
		{ID: 2, SessionID: session.ID, Text: "Tabura, okay", CommittedAt: 100},
	}

	policy := evaluateCompanionInteractionPolicy(cfg, session, segments, nil)
	if policy.Decision != companionInteractionDecisionSuppressed {
		t.Fatalf("decision = %q, want %q", policy.Decision, companionInteractionDecisionSuppressed)
	}
	if policy.Reason != "noise_suppressed" {
		t.Fatalf("reason = %q, want noise_suppressed", policy.Reason)
	}
}

func TestEvaluateCompanionInteractionPolicyAppliesCooldown(t *testing.T) {
	cfg := defaultCompanionConfig()
	cfg.CompanionEnabled = true
	cfg.DirectedSpeechGateEnabled = true

	session := &store.ParticipantSession{ID: "psess-1", ProjectKey: "proj"}
	segments := []store.ParticipantSegment{
		{ID: 3, SessionID: session.ID, Text: "Tabura, summarize that.", CommittedAt: 203},
	}
	events := []store.ParticipantEvent{
		{SessionID: session.ID, SegmentID: 1, EventType: "assistant_triggered", CreatedAt: 195},
		{SessionID: session.ID, SegmentID: 1, EventType: "assistant_turn_completed", CreatedAt: 200},
	}

	policy := evaluateCompanionInteractionPolicy(cfg, session, segments, events)
	if policy.Decision != companionInteractionDecisionCooldown {
		t.Fatalf("decision = %q, want %q", policy.Decision, companionInteractionDecisionCooldown)
	}
	if policy.CooldownUntil != 200+companionInteractionCooldownSeconds {
		t.Fatalf("cooldown_until = %d, want %d", policy.CooldownUntil, 200+companionInteractionCooldownSeconds)
	}
}

func TestEvaluateCompanionInteractionPolicyInterruptsPendingResponse(t *testing.T) {
	cfg := defaultCompanionConfig()
	cfg.CompanionEnabled = true
	cfg.DirectedSpeechGateEnabled = true

	session := &store.ParticipantSession{ID: "psess-1", ProjectKey: "proj"}
	segments := []store.ParticipantSegment{
		{ID: 1, SessionID: session.ID, Text: "Tabura, summarize that.", CommittedAt: 100},
		{ID: 2, SessionID: session.ID, Text: "Tabura, stop and open the transcript.", CommittedAt: 102},
	}
	events := []store.ParticipantEvent{
		{SessionID: session.ID, SegmentID: 1, EventType: "assistant_triggered", CreatedAt: 101},
	}

	policy := evaluateCompanionInteractionPolicy(cfg, session, segments, events)
	if policy.Decision != companionInteractionDecisionInterrupt {
		t.Fatalf("decision = %q, want %q", policy.Decision, companionInteractionDecisionInterrupt)
	}
	if policy.PendingSegmentID != 1 {
		t.Fatalf("pending_segment_id = %d, want 1", policy.PendingSegmentID)
	}
}
