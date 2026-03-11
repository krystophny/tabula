package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseIntentPlanClassificationReadsAddressedField(t *testing.T) {
	classification, err := parseIntentPlanClassification(`{"addressed":false,"action":"toggle_silent"}`)
	if err != nil {
		t.Fatalf("parseIntentPlanClassification returned error: %v", err)
	}
	if classification.Addressed == nil {
		t.Fatal("expected addressed classification")
	}
	if *classification.Addressed {
		t.Fatal("addressed = true, want false")
	}
	if len(classification.Actions) != 1 {
		t.Fatalf("actions length = %d, want 1", len(classification.Actions))
	}
	if classification.Actions[0].Action != "toggle_silent" {
		t.Fatalf("action = %q, want toggle_silent", classification.Actions[0].Action)
	}
}

func TestClassifyIntentPlanWithLLMMeetingPromptRequestsAddressedness(t *testing.T) {
	var systemPrompt string
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode llm request: %v", err)
		}
		messages, _ := payload["messages"].([]interface{})
		if len(messages) == 0 {
			t.Fatal("missing llm messages")
		}
		first, _ := messages[0].(map[string]interface{})
		systemPrompt = strings.TrimSpace(strFromAny(first["content"]))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"content": `{"addressed":true,"kind":"dialogue"}`,
					},
				},
			},
		})
	}))
	defer llm.Close()

	app := newAuthedTestApp(t)
	app.intentLLMURL = llm.URL
	setLivePolicyForTest(t, app, LivePolicyMeeting)

	classification, err := app.classifyIntentPlanWithLLMResult(context.Background(), "what changed?")
	if err != nil {
		t.Fatalf("classifyIntentPlanWithLLMResult returned error: %v", err)
	}
	if classification.Addressed == nil || !*classification.Addressed {
		t.Fatalf("addressed = %#v, want true", classification.Addressed)
	}
	if !strings.Contains(systemPrompt, `include an "addressed" boolean`) {
		t.Fatalf("system prompt = %q, want addressedness instruction", systemPrompt)
	}
}

func TestRunAssistantTurnSuppressesUnaddressedMeetingTurn(t *testing.T) {
	llm := setupMockIntentLLMServer(t, http.StatusOK, `{"addressed":false,"action":"toggle_silent"}`)
	defer llm.Close()

	app := newAuthedTestApp(t)
	app.intentLLMURL = llm.URL
	setLivePolicyForTest(t, app, LivePolicyMeeting)

	project, err := app.ensureDefaultProjectRecord()
	if err != nil {
		t.Fatalf("ensure default project: %v", err)
	}
	session, err := app.chatSessionForProject(project)
	if err != nil {
		t.Fatalf("project session: %v", err)
	}
	if _, err := app.store.AddChatMessage(session.ID, "user", "please summarize the budget discussion", "please summarize the budget discussion", "text"); err != nil {
		t.Fatalf("add user message: %v", err)
	}

	app.runAssistantTurn(session.ID, dequeuedTurn{outputMode: turnOutputModeSilent})

	if app.silentModeEnabled() {
		t.Fatal("silent mode toggled for unaddressed meeting turn")
	}
	if got := latestAssistantMessage(t, app, session.ID); got != "" {
		t.Fatalf("assistant message = %q, want empty", got)
	}
}

func TestRunAssistantTurnMeetingDirectAddressOverridesFalseAddressedClassification(t *testing.T) {
	llm := setupMockIntentLLMServer(t, http.StatusOK, `{"addressed":false,"action":"toggle_silent"}`)
	defer llm.Close()

	app := newAuthedTestApp(t)
	app.intentLLMURL = llm.URL
	setLivePolicyForTest(t, app, LivePolicyMeeting)

	project, err := app.ensureDefaultProjectRecord()
	if err != nil {
		t.Fatalf("ensure default project: %v", err)
	}
	session, err := app.chatSessionForProject(project)
	if err != nil {
		t.Fatalf("project session: %v", err)
	}
	if _, err := app.store.AddChatMessage(session.ID, "user", "Tabura, be quiet", "Tabura, be quiet", "text"); err != nil {
		t.Fatalf("add user message: %v", err)
	}

	message, payloads, handled := app.classifyAndExecuteSystemActionForTurn(context.Background(), session.ID, session, "Tabura, be quiet", nil, "")
	if !handled {
		t.Fatal("expected explicit direct address to be handled")
	}
	if message != "Toggled silent mode." {
		t.Fatalf("message = %q, want %q", message, "Toggled silent mode.")
	}
	if len(payloads) != 1 || strFromAny(payloads[0]["type"]) != "toggle_silent" {
		t.Fatalf("payloads = %#v, want toggle_silent payload", payloads)
	}
}

func TestRunAssistantTurnDialogueIgnoresAddressedFlag(t *testing.T) {
	llm := setupMockIntentLLMServer(t, http.StatusOK, `{"addressed":false,"action":"toggle_silent"}`)
	defer llm.Close()

	app := newAuthedTestApp(t)
	app.intentLLMURL = llm.URL
	setLivePolicyForTest(t, app, LivePolicyDialogue)

	project, err := app.ensureDefaultProjectRecord()
	if err != nil {
		t.Fatalf("ensure default project: %v", err)
	}
	session, err := app.chatSessionForProject(project)
	if err != nil {
		t.Fatalf("project session: %v", err)
	}
	if _, err := app.store.AddChatMessage(session.ID, "user", "be quiet", "be quiet", "text"); err != nil {
		t.Fatalf("add user message: %v", err)
	}

	message, payloads, handled := app.classifyAndExecuteSystemActionForTurn(context.Background(), session.ID, session, "be quiet", nil, "")
	if !handled {
		t.Fatal("expected dialogue mode to ignore addressed flag")
	}
	if message != "Toggled silent mode." {
		t.Fatalf("message = %q, want %q", message, "Toggled silent mode.")
	}
	if len(payloads) != 1 || strFromAny(payloads[0]["type"]) != "toggle_silent" {
		t.Fatalf("payloads = %#v, want toggle_silent payload", payloads)
	}
}
