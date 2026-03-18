package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestTryDeterministicFastPathRegistry(t *testing.T) {
	now := time.Date(2026, time.March, 9, 8, 0, 0, 0, time.UTC)
	cursor := &chatCursorContext{ItemID: 42, View: "inbox"}
	tests := []struct {
		name       string
		text       string
		ctx        deterministicFastPathContext
		wantMatch  string
		wantAction string
		wantTitle  string
	}{
		{name: "source sync", text: "sync all sources", wantMatch: "source_sync", wantAction: "sync_sources"},
		{name: "calendar", text: "show calendar", ctx: deterministicFastPathContext{Now: now}, wantMatch: "calendar", wantAction: "show_calendar"},
		{name: "briefing", text: "show briefing", ctx: deterministicFastPathContext{Now: now}, wantMatch: "briefing", wantAction: "show_briefing"},
		{name: "todoist", text: "sync todoist", wantMatch: "todoist", wantAction: "sync_todoist"},
		{name: "evernote", text: "sync evernote", wantMatch: "evernote", wantAction: "sync_evernote"},
		{name: "bear", text: "sync bear", wantMatch: "bear", wantAction: "sync_bear"},
		{name: "zotero", text: "sync zotero", wantMatch: "zotero", wantAction: "sync_zotero"},
		{name: "cursor", text: "open this", ctx: deterministicFastPathContext{Cursor: cursor}, wantMatch: "cursor", wantAction: "cursor_open_item"},
		{name: "titled item", text: `Move the item "Budget" back to the inbox.`, wantMatch: "titled_item", wantTitle: "Budget"},
		{name: "item", text: "idea: better swipe triage", ctx: deterministicFastPathContext{Now: now, CaptureMode: chatCaptureModeVoice}, wantMatch: "item", wantAction: canonicalActionAnnotateCapture},
		{name: "github", text: "Create a GitHub issue from this and label it bug, parser.", wantMatch: "github_issue", wantAction: canonicalActionDispatchExecute},
		{name: "artifact", text: "show linked artifacts", wantMatch: "artifact_link", wantAction: "list_linked_artifacts"},
		{name: "batch", text: "show me progress", wantMatch: "batch", wantAction: "batch_status"},
		{name: "workspace", text: "list workspaces", wantMatch: "workspace", wantAction: "list_workspaces"},
		{name: "workspace focus", text: "open the plasma workspace", wantMatch: "workspace", wantAction: "focus_workspace"},
		{name: "project", text: "what project is this?", wantMatch: "project", wantAction: "show_workspace_project"},
		{name: "runtime silent", text: "be quiet", wantMatch: "runtime_control", wantAction: "toggle_silent"},
		{name: "runtime status", text: "status?", wantMatch: "runtime_control", wantAction: "show_status"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			match := tryDeterministicFastPath(tc.text, tc.ctx)
			if match == nil {
				t.Fatal("expected fast-path match")
			}
			if match.Name != tc.wantMatch {
				t.Fatalf("match name = %q, want %q", match.Name, tc.wantMatch)
			}
			if tc.wantTitle != "" {
				if match.TitledItem == nil {
					t.Fatal("expected titled item intent")
				}
				if match.TitledItem.Title != tc.wantTitle {
					t.Fatalf("title = %q, want %q", match.TitledItem.Title, tc.wantTitle)
				}
				return
			}
			if len(match.Actions) != 1 {
				t.Fatalf("action count = %d, want 1", len(match.Actions))
			}
			if match.Actions[0].Action != tc.wantAction {
				t.Fatalf("action = %q, want %q", match.Actions[0].Action, tc.wantAction)
			}
		})
	}
}

func TestTryDeterministicFastPathNoMatch(t *testing.T) {
	if match := tryDeterministicFastPath("help me with the plasma analysis", deterministicFastPathContext{}); match != nil {
		t.Fatalf("match = %#v, want nil", match)
	}
}

func TestDeterministicFastPathCatalogIncludesRuntimeAndUIControls(t *testing.T) {
	catalog := deterministicFastPathCatalog()
	find := func(name string) *deterministicFastPathSpec {
		t.Helper()
		for i := range catalog {
			if catalog[i].Name == name {
				return &catalog[i]
			}
		}
		return nil
	}

	runtime := find("runtime_control")
	if runtime == nil {
		t.Fatal("expected runtime_control catalog entry")
	}
	if runtime.Route != "text" {
		t.Fatalf("runtime route = %q, want text", runtime.Route)
	}
	for _, action := range []string{"toggle_silent", "toggle_live_dialogue", "cancel_work", "show_status"} {
		if !containsExactString(runtime.Actions, action) {
			t.Fatalf("runtime actions = %#v, missing %q", runtime.Actions, action)
		}
	}

	ui := find("ui_runtime_controls")
	if ui == nil {
		t.Fatal("expected ui_runtime_controls catalog entry")
	}
	if ui.Route != "ui" {
		t.Fatalf("ui route = %q, want ui", ui.Route)
	}
	if !containsExactString(ui.Triggers, "system_action:toggle_live_dialogue") {
		t.Fatalf("ui triggers = %#v, missing live dialogue system action", ui.Triggers)
	}

	ptt := find("ui_push_to_talk")
	if ptt == nil {
		t.Fatal("expected ui_push_to_talk catalog entry")
	}
	for _, trigger := range []string{"ctrl_long_press", "ctrl_release"} {
		if !containsExactString(ptt.Triggers, trigger) {
			t.Fatalf("ptt triggers = %#v, missing %q", ptt.Triggers, trigger)
		}
	}
}

func TestClassifyAndExecuteSystemActionFastPathBypassesIntentLLM(t *testing.T) {
	app := newAuthedTestApp(t)
	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensure default project: %v", err)
	}
	session, err := app.store.GetOrCreateChatSession(project.WorkspacePath)
	if err != nil {
		t.Fatalf("chat session: %v", err)
	}

	var llmCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		llmCalls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	app.intentLLMURL = server.URL

	message, _, handled := app.classifyAndExecuteSystemAction(context.Background(), session.ID, session, "sync evernote")
	if !handled {
		t.Fatal("expected fast path to handle sync evernote")
	}
	if llmCalls.Load() != 0 {
		t.Fatalf("intent llm calls = %d, want 0", llmCalls.Load())
	}
	if got := message; got == "" || got == "I can only handle system actions in local-only mode." {
		t.Fatalf("message = %q, want fast-path execution result", got)
	}
}

func TestClassifyAndExecuteSystemActionRuntimeControlBypassesIntentLLM(t *testing.T) {
	app := newAuthedTestApp(t)
	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensure default project: %v", err)
	}
	session, err := app.store.GetOrCreateChatSession(project.WorkspacePath)
	if err != nil {
		t.Fatalf("chat session: %v", err)
	}

	var llmCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		llmCalls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	app.intentLLMURL = server.URL

	message, payloads, handled := app.classifyAndExecuteSystemAction(context.Background(), session.ID, session, "be quiet")
	if !handled {
		t.Fatal("expected runtime control fast path to handle be quiet")
	}
	if llmCalls.Load() != 0 {
		t.Fatalf("intent llm calls = %d, want 0", llmCalls.Load())
	}
	if message != "Toggled silent mode." {
		t.Fatalf("message = %q, want %q", message, "Toggled silent mode.")
	}
	if len(payloads) != 1 || strFromAny(payloads[0]["type"]) != "toggle_silent" {
		t.Fatalf("payloads = %#v, want toggle_silent payload", payloads)
	}
}

func TestClassifyAndExecuteSystemActionStatusFastPathBypassesIntentLLM(t *testing.T) {
	wsServer := setupMockAppServerStatusServer(t, "Agent healthy.")
	defer wsServer.Close()
	wsURL := "ws" + strings.TrimPrefix(wsServer.URL, "http")

	app, err := New(t.TempDir(), "", "", wsURL, "", "", "", false)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	t.Cleanup(func() {
		_ = app.Shutdown(context.Background())
	})

	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensure default project: %v", err)
	}
	session, err := app.store.GetOrCreateChatSession(project.WorkspacePath)
	if err != nil {
		t.Fatalf("chat session: %v", err)
	}

	var llmCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		llmCalls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	app.intentLLMURL = server.URL

	message, payloads, handled := app.classifyAndExecuteSystemAction(context.Background(), session.ID, session, "status?")
	if !handled {
		t.Fatal("expected runtime control fast path to handle status")
	}
	if llmCalls.Load() != 0 {
		t.Fatalf("intent llm calls = %d, want 0", llmCalls.Load())
	}
	if message != "Agent healthy." {
		t.Fatalf("message = %q, want %q", message, "Agent healthy.")
	}
	if len(payloads) != 0 {
		t.Fatalf("payloads = %#v, want none", payloads)
	}
}

func containsExactString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
