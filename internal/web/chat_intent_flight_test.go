package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewCommandEnvelopeUsesStableIdempotencyKey(t *testing.T) {
	first := newCommandEnvelope(&SystemAction{
		Action: "switch_workspace",
		Params: map[string]interface{}{
			"workspace": "Alpha",
			"options": map[string]interface{}{
				"focus": true,
				"mode":  "work",
			},
		},
	})
	second := newCommandEnvelope(&SystemAction{
		Action: "switch_workspace",
		Params: map[string]interface{}{
			"options": map[string]interface{}{
				"mode":  "work",
				"focus": true,
			},
			"workspace": "Alpha",
		},
	})
	if first.IdempotencyKey != second.IdempotencyKey {
		t.Fatalf("idempotency key mismatch: %q != %q", first.IdempotencyKey, second.IdempotencyKey)
	}
	if !first.Idempotent {
		t.Fatal("switch_workspace should be idempotent")
	}
}

func TestCommandFlightTrackerSuppressesInflightAndCooldown(t *testing.T) {
	base := time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC)
	now := base
	tracker := newCommandFlightTracker(2 * time.Second)
	tracker.now = func() time.Time { return now }
	env := newCommandEnvelope(&SystemAction{
		Action: "make_item",
		Params: map[string]interface{}{"title": "Call Bob"},
	})

	if status, remaining, ok := tracker.TryAcquire(env); !ok || status != "" || remaining != 0 {
		t.Fatalf("first acquire = (%q, %v, %v), want success", status, remaining, ok)
	}
	if status, _, ok := tracker.TryAcquire(env); ok || status != "already_executed" {
		t.Fatalf("second acquire while inflight = (%q, %v), want already_executed", status, ok)
	}

	tracker.Release(env, true)
	if status, remaining, ok := tracker.TryAcquire(env); ok || status != "cooldown_suppressed" || remaining <= 0 {
		t.Fatalf("acquire during cooldown = (%q, %v, %v), want cooldown_suppressed", status, remaining, ok)
	}

	now = now.Add(3 * time.Second)
	if status, remaining, ok := tracker.TryAcquire(env); !ok || status != "" || remaining != 0 {
		t.Fatalf("acquire after cooldown = (%q, %v, %v), want success", status, remaining, ok)
	}
}

func TestCommandFlightTrackerIdempotentActionBypassesCooldown(t *testing.T) {
	base := time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC)
	now := base
	tracker := newCommandFlightTracker(2 * time.Second)
	tracker.now = func() time.Time { return now }
	env := newCommandEnvelope(&SystemAction{
		Action: "open_file_canvas",
		Params: map[string]interface{}{"path": "README.md"},
	})

	if !env.Idempotent {
		t.Fatal("open_file_canvas should be idempotent")
	}
	if status, _, ok := tracker.TryAcquire(env); !ok || status != "" {
		t.Fatalf("first acquire = (%q, %v), want success", status, ok)
	}
	tracker.Release(env, true)
	if status, remaining, ok := tracker.TryAcquire(env); !ok || status != "" || remaining != 0 {
		t.Fatalf("idempotent re-acquire = (%q, %v, %v), want success", status, remaining, ok)
	}
}

func TestCommandFlightTrackerSyncActionBypassesCooldown(t *testing.T) {
	base := time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC)
	now := base
	tracker := newCommandFlightTracker(2 * time.Second)
	tracker.now = func() time.Time { return now }
	env := newCommandEnvelope(&SystemAction{
		Action: "sync_todoist",
	})

	if !env.Idempotent {
		t.Fatal("sync_todoist should be idempotent")
	}
	if status, _, ok := tracker.TryAcquire(env); !ok || status != "" {
		t.Fatalf("first acquire = (%q, %v), want success", status, ok)
	}
	if status, _, ok := tracker.TryAcquire(env); ok || status != "already_executed" {
		t.Fatalf("second acquire while inflight = (%q, %v), want already_executed", status, ok)
	}

	tracker.Release(env, true)
	if status, remaining, ok := tracker.TryAcquire(env); !ok || status != "" || remaining != 0 {
		t.Fatalf("sync re-acquire = (%q, %v, %v), want success", status, remaining, ok)
	}
}

func TestExecuteSystemActionSuppressesShellCooldownDuplicate(t *testing.T) {
	app, err := New(t.TempDir(), t.TempDir(), "", "", "", "", "", false)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	t.Cleanup(func() {
		_ = app.Shutdown(t.Context())
	})
	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensure default project: %v", err)
	}
	session, err := app.store.GetOrCreateChatSession(project.WorkspacePath)
	if err != nil {
		t.Fatalf("chat session: %v", err)
	}
	markerPath := filepath.Join(project.RootPath, "flight-marker.txt")
	command := "printf x >> flight-marker.txt"

	msg, payload, err := app.executeSystemAction(session.ID, session, &SystemAction{
		Action: "shell",
		Params: map[string]interface{}{"command": command},
	})
	if err != nil {
		t.Fatalf("first executeSystemAction() error: %v", err)
	}
	if strings.TrimSpace(msg) == "" {
		t.Fatal("expected shell output message")
	}
	if got := strFromAny(payload["type"]); got != "shell" {
		t.Fatalf("first payload type = %q, want shell", got)
	}

	msg, payload, err = app.executeSystemAction(session.ID, session, &SystemAction{
		Action: "shell",
		Params: map[string]interface{}{"command": command},
	})
	if err != nil {
		t.Fatalf("second executeSystemAction() error: %v", err)
	}
	if !strings.Contains(strings.ToLower(msg), "cooldown") {
		t.Fatalf("suppression message = %q, want cooldown hint", msg)
	}
	if got := strFromAny(payload["type"]); got != "system_action_suppressed" {
		t.Fatalf("suppression payload type = %q, want system_action_suppressed", got)
	}
	if got := strFromAny(payload["status"]); got != "cooldown_suppressed" {
		t.Fatalf("suppression status = %q, want cooldown_suppressed", got)
	}
	data, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("read marker file: %v", err)
	}
	if got := string(data); got != "x" {
		t.Fatalf("marker file = %q, want single execution", got)
	}
}

func TestBroadcastSystemActionEventEmitsSuppressedEventType(t *testing.T) {
	app := newAuthedTestApp(t)
	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensure default project: %v", err)
	}
	session, err := app.store.GetOrCreateChatSession(project.WorkspacePath)
	if err != nil {
		t.Fatalf("chat session: %v", err)
	}

	conn, clientConn, cleanup := newParticipantTestWSConn(t)
	defer cleanup()
	app.hub.registerChat(session.ID, conn)
	defer app.hub.unregisterChat(session.ID, conn)

	env := newCommandEnvelope(&SystemAction{
		Action: "shell",
		Params: map[string]interface{}{"command": "printf 'hello-shell'"},
	})
	app.broadcastSystemActionEvent(session.ID, suppressedSystemActionPayload(env, "cooldown_suppressed", 1500*time.Millisecond))

	payload := waitForWSJSONMessageType(t, clientConn, 2*time.Second, "system_action_suppressed")
	action, ok := payload["action"].(map[string]interface{})
	if !ok {
		t.Fatalf("action payload = %#v, want object", payload["action"])
	}
	if got := strFromAny(action["action_type"]); got != "shell" {
		t.Fatalf("action_type = %q, want shell", got)
	}
	if got := strFromAny(action["status"]); got != "cooldown_suppressed" {
		t.Fatalf("status = %q, want cooldown_suppressed", got)
	}
	if got := intFromAny(action["cooldown_ms"], 0); got != 1500 {
		t.Fatalf("cooldown_ms = %d, want 1500", got)
	}
}
