package web

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestHandleChatSessionMessageNaturalLanguageNotCommand(t *testing.T) {
	app := newAuthedTestApp(t)
	project, err := app.ensureDefaultProjectRecord()
	if err != nil {
		t.Fatalf("ensure default project: %v", err)
	}
	session, err := app.store.GetOrCreateChatSession(project.ProjectKey)
	if err != nil {
		t.Fatalf("get or create chat session: %v", err)
	}

	rr := doAuthedJSONRequest(
		t,
		app.Router(),
		http.MethodPost,
		"/api/chat/sessions/"+session.ID+"/messages",
		map[string]any{
			"text": "show me pr review",
		},
	)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := strFromAny(payload["kind"]); got != "turn_queued" {
		t.Fatalf("response kind = %q, want %q", got, "turn_queued")
	}
	messages, err := app.store.ListChatMessages(session.ID, 10)
	if err != nil {
		t.Fatalf("list chat messages: %v", err)
	}
	foundUserMessage := false
	for _, msg := range messages {
		if msg.Role == "user" && msg.ContentPlain == "show me pr review" {
			foundUserMessage = true
			break
		}
	}
	if !foundUserMessage {
		t.Fatalf("expected natural-language message to be stored as user text")
	}
}

func strFromAny(v any) string {
	switch typed := v.(type) {
	case string:
		return typed
	default:
		return ""
	}
}
