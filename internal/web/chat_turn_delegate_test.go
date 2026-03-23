package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/krystophny/tabura/internal/modelprofile"
)

type mockDelegateAppServerState struct {
	mu           sync.Mutex
	threadStarts int
	turnModels   []string
}

func (s *mockDelegateAppServerState) recordThreadStart() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.threadStarts++
}

func (s *mockDelegateAppServerState) recordTurnModel(model string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.turnModels = append(s.turnModels, strings.TrimSpace(model))
}

func (s *mockDelegateAppServerState) snapshot() (int, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]string(nil), s.turnModels...)
	return s.threadStarts, out
}

func setupMockDelegateAppServer(t *testing.T) (*httptest.Server, *mockDelegateAppServerState) {
	t.Helper()
	state := &mockDelegateAppServerState{}
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade websocket: %v", err)
		}
		defer conn.Close()

		turnSeq := 0
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var msg map[string]interface{}
			if err := json.Unmarshal(data, &msg); err != nil {
				t.Fatalf("decode app-server message: %v", err)
			}
			method := strings.TrimSpace(strFromAny(msg["method"]))
			switch method {
			case "initialize":
				_ = conn.WriteJSON(map[string]interface{}{
					"id":     msg["id"],
					"result": map[string]interface{}{"userAgent": "delegate-test"},
				})
			case "thread/start":
				state.recordThreadStart()
				_ = conn.WriteJSON(map[string]interface{}{
					"id": msg["id"],
					"result": map[string]interface{}{
						"thread": map[string]interface{}{"id": "thread-delegate"},
					},
				})
			case "turn/start":
				turnSeq++
				params, _ := msg["params"].(map[string]interface{})
				model := strFromAny(params["model"])
				state.recordTurnModel(model)
				turnID := "turn-delegate-" + string(rune('0'+turnSeq))
				_ = conn.WriteJSON(map[string]interface{}{
					"id": msg["id"],
					"result": map[string]interface{}{
						"turn": map[string]interface{}{"id": turnID},
					},
				})
				_ = conn.WriteJSON(map[string]interface{}{
					"method": "item/completed",
					"params": map[string]interface{}{
						"item": map[string]interface{}{
							"type": "agentMessage",
							"text": "reply via " + model,
						},
					},
				})
				_ = conn.WriteJSON(map[string]interface{}{
					"method": "turn/completed",
					"params": map[string]interface{}{
						"turn": map[string]interface{}{"id": turnID, "status": "completed"},
					},
				})
			}
		}
	}))
	return server, state
}

func setupMockLocalAssistantLLM(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": "reply via local",
					},
				},
			},
		})
	}))
}

func newAuthedAppWithServer(t *testing.T, appServerURL string) *App {
	t.Helper()
	app, err := New(t.TempDir(), "", "", appServerURL, "", "", "", false)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	t.Cleanup(func() {
		_ = app.Shutdown(context.Background())
	})
	return app
}

func TestRunAssistantTurnExplicitGPTRequestUsesTurnOverrideAndReturnsToLocal(t *testing.T) {
	appServer, serverState := setupMockDelegateAppServer(t)
	defer appServer.Close()
	localLLM := setupMockLocalAssistantLLM(t)
	defer localLLM.Close()

	app := newAuthedAppWithServer(t, "ws"+strings.TrimPrefix(appServer.URL, "http"))
	app.assistantLLMURL = localLLM.URL
	app.intentLLMURL = localLLM.URL
	app.assistantLLMModel = "qwen-test"
	app.assistantLLMExplicit = true

	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensureDefaultWorkspace: %v", err)
	}
	session, err := app.store.GetOrCreateChatSession(project.WorkspacePath)
	if err != nil {
		t.Fatalf("GetOrCreateChatSession: %v", err)
	}

	firstUser, err := app.store.AddChatMessage(session.ID, "user", "Use GPT for this: explain the failure.", "Use GPT for this: explain the failure.", "text")
	if err != nil {
		t.Fatalf("AddChatMessage first user: %v", err)
	}
	app.runAssistantTurn(session.ID, dequeuedTurn{messageID: firstUser.ID, outputMode: turnOutputModeSilent})

	secondUser, err := app.store.AddChatMessage(session.ID, "user", "Continue with the normal dialogue.", "Continue with the normal dialogue.", "text")
	if err != nil {
		t.Fatalf("AddChatMessage second user: %v", err)
	}
	app.runAssistantTurn(session.ID, dequeuedTurn{messageID: secondUser.ID, outputMode: turnOutputModeSilent})

	threadStarts, turnModels := serverState.snapshot()
	if threadStarts != 1 {
		t.Fatalf("thread starts = %d, want 1 explicit remote turn", threadStarts)
	}
	if len(turnModels) != 1 {
		t.Fatalf("turn model count = %d, want 1", len(turnModels))
	}
	if turnModels[0] != modelprofile.ModelGPT {
		t.Fatalf("first turn model = %q, want %q", turnModels[0], modelprofile.ModelGPT)
	}

	messages, err := app.store.ListChatMessages(session.ID, 10)
	if err != nil {
		t.Fatalf("ListChatMessages: %v", err)
	}
	assistantModels := []string{}
	for _, msg := range messages {
		if msg.Role != "assistant" {
			continue
		}
		assistantModels = append(assistantModels, msg.ProviderModel)
	}
	if len(assistantModels) < 2 {
		t.Fatalf("assistant messages = %d, want at least 2", len(assistantModels))
	}
	if assistantModels[0] != modelprofile.ModelGPT {
		t.Fatalf("first assistant provider model = %q, want %q", assistantModels[0], modelprofile.ModelGPT)
	}
	if assistantModels[1] != "qwen-test" {
		t.Fatalf("second assistant provider model = %q, want %q", assistantModels[1], "qwen-test")
	}
}
