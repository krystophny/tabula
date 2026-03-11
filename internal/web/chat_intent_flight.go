package web

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

type CommandEnvelope struct {
	Action         string
	Params         map[string]interface{}
	IdempotencyKey string
	Idempotent     bool
	Timestamp      time.Time
}

type commandFlightTracker struct {
	mu       sync.Mutex
	inflight map[string]time.Time
	recent   map[string]time.Time
	cooldown time.Duration
	now      func() time.Time
}

func newCommandEnvelope(action *SystemAction) CommandEnvelope {
	cleanAction := normalizeSystemActionName(strings.TrimSpace(action.Action))
	params := map[string]interface{}{}
	if action != nil && action.Params != nil {
		for key, value := range action.Params {
			params[key] = value
		}
	}
	canonical, err := json.Marshal(params)
	if err != nil {
		canonical = []byte("{}")
	}
	sum := sha256.Sum256([]byte(cleanAction + "\n" + string(canonical)))
	return CommandEnvelope{
		Action:         cleanAction,
		Params:         params,
		IdempotencyKey: hex.EncodeToString(sum[:]),
		Idempotent:     isSystemActionIdempotent(cleanAction),
		Timestamp:      time.Now().UTC(),
	}
}

func isSystemActionIdempotent(action string) bool {
	switch normalizeSystemActionName(strings.TrimSpace(action)) {
	case "shell",
		"create_workspace",
		"create_workspace_from_git",
		"rename_workspace",
		"delete_workspace",
		"batch_work",
		"batch_configure",
		"review_policy",
		"batch_limit",
		"assign_workspace_project",
		"create_project",
		"make_item",
		"delegate_item",
		"snooze_item",
		"split_items",
		"reassign_workspace",
		"reassign_project",
		"clear_workspace",
		"clear_project",
		"capture_idea",
		"refine_idea_note",
		"apply_idea_promotion",
		"link_workspace_artifact",
		"triage_someday",
		"promote_someday",
		"toggle_someday_review_nudge",
		"map_todoist_project",
		"create_todoist_task",
		"promote_bear_checklist",
		"create_github_issue",
		"create_github_issue_split",
		"cursor_triage_item":
		return false
	default:
		return true
	}
}

func newCommandFlightTracker(cooldown time.Duration) *commandFlightTracker {
	if cooldown <= 0 {
		cooldown = 2 * time.Second
	}
	return &commandFlightTracker{
		inflight: map[string]time.Time{},
		recent:   map[string]time.Time{},
		cooldown: cooldown,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (t *commandFlightTracker) TryAcquire(env CommandEnvelope) (string, time.Duration, bool) {
	if t == nil || strings.TrimSpace(env.IdempotencyKey) == "" {
		return "", 0, true
	}
	now := t.now()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pruneRecentLocked(now)
	if _, ok := t.inflight[env.IdempotencyKey]; ok {
		return "already_executed", 0, false
	}
	if !env.Idempotent {
		if completedAt, ok := t.recent[env.IdempotencyKey]; ok {
			remaining := t.cooldown - now.Sub(completedAt)
			if remaining > 0 {
				return "cooldown_suppressed", remaining, false
			}
			delete(t.recent, env.IdempotencyKey)
		}
	}
	t.inflight[env.IdempotencyKey] = now
	return "", 0, true
}

func (t *commandFlightTracker) Release(env CommandEnvelope, success bool) {
	if t == nil || strings.TrimSpace(env.IdempotencyKey) == "" {
		return
	}
	now := t.now()
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.inflight, env.IdempotencyKey)
	t.pruneRecentLocked(now)
	if success && !env.Idempotent {
		t.recent[env.IdempotencyKey] = now
	}
}

func (t *commandFlightTracker) pruneRecentLocked(now time.Time) {
	for key, completedAt := range t.recent {
		if now.Sub(completedAt) >= t.cooldown {
			delete(t.recent, key)
		}
	}
}

func suppressedSystemActionMessage(env CommandEnvelope, status string) string {
	action := strings.TrimSpace(env.Action)
	if action == "" {
		action = "system action"
	}
	switch strings.TrimSpace(status) {
	case "already_executed":
		return fmt.Sprintf("Suppressed duplicate %s command: already executing.", action)
	case "cooldown_suppressed":
		return fmt.Sprintf("Suppressed duplicate %s command during cooldown.", action)
	default:
		return fmt.Sprintf("Suppressed duplicate %s command.", action)
	}
}

func suppressedSystemActionPayload(env CommandEnvelope, status string, cooldownRemaining time.Duration) map[string]interface{} {
	payload := map[string]interface{}{
		"type":            "system_action_suppressed",
		"action_type":     env.Action,
		"status":          status,
		"idempotent":      env.Idempotent,
		"idempotency_key": env.IdempotencyKey,
	}
	if cooldownRemaining > 0 {
		payload["cooldown_ms"] = cooldownRemaining.Milliseconds()
	}
	return payload
}
