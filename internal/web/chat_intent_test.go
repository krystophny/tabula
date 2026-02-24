package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func setupMockCanvasArtifactServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result": map[string]any{
				"structuredContent": map[string]any{},
			},
		})
	}))
}

func TestInferImplicitChatCommand(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "show me pr review", want: "/pr"},
		{input: "open PR 123", want: "/pr 123"},
		{input: "refresh pr review", want: "/pr refresh"},
		{input: "reload pull request #44 diff view", want: "/pr 44"},
		{input: "switch to pull request review mode", want: "/pr"},
		{input: "how do i review a pr?", want: ""},
		{input: "ask codex to review this pr", want: ""},
	}
	for _, tc := range tests {
		got := inferImplicitChatCommand(tc.input)
		if got != tc.want {
			t.Errorf("inferImplicitChatCommand(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestHandleChatSessionMessageImplicitPRCommand(t *testing.T) {
	app := newAuthedTestApp(t)
	project, err := app.ensureDefaultProjectRecord()
	if err != nil {
		t.Fatalf("ensure default project: %v", err)
	}
	session, err := app.store.GetOrCreateChatSession(project.ProjectKey)
	if err != nil {
		t.Fatalf("get or create chat session: %v", err)
	}

	app.ghCommandRunner = func(_ context.Context, _ string, args ...string) (string, error) {
		if len(args) >= 2 && args[0] == "pr" && args[1] == "view" {
			return `{"number":17,"title":"Fix parser","url":"https://example.invalid/pr/17","headRefName":"feat/fix","baseRefName":"main"}`, nil
		}
		if len(args) >= 2 && args[0] == "pr" && args[1] == "diff" {
			return strings.Join([]string{
				"diff --git a/main.go b/main.go",
				"--- a/main.go",
				"+++ b/main.go",
				"@@ -1 +1 @@",
				"-old",
				"+new",
			}, "\n"), nil
		}
		return "", errors.New("unexpected command")
	}

	canvasServer := setupMockCanvasArtifactServer(t)
	defer canvasServer.Close()
	parsedURL, err := url.Parse(canvasServer.URL)
	if err != nil {
		t.Fatalf("parse mock canvas URL: %v", err)
	}
	port, err := strconv.Atoi(parsedURL.Port())
	if err != nil {
		t.Fatalf("parse mock canvas port: %v", err)
	}
	app.mu.Lock()
	app.tunnelPorts[project.CanvasSessionID] = port
	app.mu.Unlock()

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
	if got := strings.TrimSpace(strFromAny(payload["kind"])); got != "command" {
		t.Fatalf("response kind = %q, want %q", got, "command")
	}
	result, _ := payload["result"].(map[string]any)
	if got := strings.TrimSpace(strFromAny(result["name"])); got != "pr" {
		t.Fatalf("command name = %q, want %q", got, "pr")
	}
	messages, err := app.store.ListChatMessages(session.ID, 10)
	if err != nil {
		t.Fatalf("list chat messages: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("expected no stored user message for implicit command, got %d", len(messages))
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
