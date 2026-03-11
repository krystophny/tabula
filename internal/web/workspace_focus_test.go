package web

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWorkspaceFocusAPI(t *testing.T) {
	app := newAuthedTestApp(t)

	anchor, err := app.ensureTodayDailyWorkspace()
	if err != nil {
		t.Fatalf("ensureTodayDailyWorkspace: %v", err)
	}
	focusPath := filepath.Join(t.TempDir(), "plasma")
	if err := os.MkdirAll(focusPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(focusPath): %v", err)
	}
	focus, err := app.store.CreateWorkspace("Plasma", focusPath)
	if err != nil {
		t.Fatalf("CreateWorkspace(focus): %v", err)
	}

	rrPost := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/workspace/focus", map[string]any{
		"workspace_id": focus.ID,
	})
	if rrPost.Code != http.StatusOK {
		t.Fatalf("POST /api/workspace/focus status = %d, want 200: %s", rrPost.Code, rrPost.Body.String())
	}
	postPayload := decodeJSONDataResponse(t, rrPost)
	if got := int64(postPayload["anchor"].(map[string]any)["id"].(float64)); got != anchor.ID {
		t.Fatalf("anchor id = %d, want %d", got, anchor.ID)
	}
	if got := int64(postPayload["focus"].(map[string]any)["id"].(float64)); got != focus.ID {
		t.Fatalf("focus id = %d, want %d", got, focus.ID)
	}
	if explicit, _ := postPayload["explicit"].(bool); !explicit {
		t.Fatalf("explicit = %#v, want true", postPayload["explicit"])
	}

	rrDelete := doAuthedJSONRequest(t, app.Router(), http.MethodDelete, "/api/workspace/focus", nil)
	if rrDelete.Code != http.StatusOK {
		t.Fatalf("DELETE /api/workspace/focus status = %d, want 200: %s", rrDelete.Code, rrDelete.Body.String())
	}
	deletePayload := decodeJSONDataResponse(t, rrDelete)
	if got := int64(deletePayload["focus"].(map[string]any)["id"].(float64)); got != anchor.ID {
		t.Fatalf("cleared focus id = %d, want %d", got, anchor.ID)
	}
	if explicit, _ := deletePayload["explicit"].(bool); explicit {
		t.Fatalf("explicit after clear = %#v, want false", deletePayload["explicit"])
	}
}

func TestWorkspaceFocusBroadcastsWebsocketChanges(t *testing.T) {
	app := newAuthedTestApp(t)

	anchor, err := app.ensureTodayDailyWorkspace()
	if err != nil {
		t.Fatalf("ensureTodayDailyWorkspace: %v", err)
	}
	session, err := app.store.GetOrCreateChatSessionForWorkspace(anchor.ID)
	if err != nil {
		t.Fatalf("GetOrCreateChatSessionForWorkspace: %v", err)
	}
	focus, err := app.store.CreateWorkspace("Plasma", filepath.Join(t.TempDir(), "plasma"))
	if err != nil {
		t.Fatalf("CreateWorkspace(focus): %v", err)
	}

	conn, clientConn, cleanup := newParticipantTestWSConn(t)
	defer cleanup()
	app.hub.registerChat(session.ID, conn)
	defer app.hub.unregisterChat(session.ID, conn)

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/workspace/focus", map[string]any{
		"workspace_id": focus.ID,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/workspace/focus status = %d, want 200", rr.Code)
	}

	payload := waitForWSJSONMessageType(t, clientConn, 2*time.Second, "workspace_focus_changed")
	if got := int64(payload["anchor"].(map[string]any)["id"].(float64)); got != anchor.ID {
		t.Fatalf("ws anchor id = %d, want %d", got, anchor.ID)
	}
	if got := int64(payload["focus"].(map[string]any)["id"].(float64)); got != focus.ID {
		t.Fatalf("ws focus id = %d, want %d", got, focus.ID)
	}
}

func TestFocusedWorkspaceShellCommandUsesFocusCWD(t *testing.T) {
	app := newAuthedTestApp(t)

	anchor, err := app.ensureTodayDailyWorkspace()
	if err != nil {
		t.Fatalf("ensureTodayDailyWorkspace: %v", err)
	}
	session, err := app.store.GetOrCreateChatSessionForWorkspace(anchor.ID)
	if err != nil {
		t.Fatalf("GetOrCreateChatSessionForWorkspace: %v", err)
	}
	focusPath := filepath.Join(t.TempDir(), "focused")
	if err := os.MkdirAll(focusPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(focusPath): %v", err)
	}
	focus, err := app.store.CreateWorkspace("Focused", focusPath)
	if err != nil {
		t.Fatalf("CreateWorkspace(focus): %v", err)
	}
	if err := app.setFocusedWorkspace(focus.ID); err != nil {
		t.Fatalf("setFocusedWorkspace: %v", err)
	}

	message, payload, err := app.executeSystemAction(session.ID, session, &SystemAction{
		Action: "shell",
		Params: map[string]interface{}{"command": "pwd"},
	})
	if err != nil {
		t.Fatalf("executeSystemAction(shell): %v", err)
	}
	if got := strFromAny(payload["cwd"]); got != focusPath {
		t.Fatalf("shell cwd = %q, want %q", got, focusPath)
	}
	if !containsLine(message, focusPath) {
		t.Fatalf("shell output = %q, want line %q", message, focusPath)
	}
}

func containsLine(text, want string) bool {
	for _, line := range splitLines(text) {
		if line == want {
			return true
		}
	}
	return false
}

func splitLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.Split(text, "\n")
}
