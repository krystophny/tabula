package web

import (
	"context"
	"net/http"
	"net/http/httptest"
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

func TestClassifyAndExecuteSystemActionFastPathBypassesIntentLLM(t *testing.T) {
	app := newAuthedTestApp(t)
	project, err := app.ensureDefaultProjectRecord()
	if err != nil {
		t.Fatalf("ensure default project: %v", err)
	}
	session, err := app.store.GetOrCreateChatSession(project.ProjectKey)
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
