package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/krystophny/tabura/internal/store"
)

func TestItemCreateTodoistBackedItemCreatesRemoteTask(t *testing.T) {
	app := newAuthedTestApp(t)

	var createBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/projects":
			writeTodoistJSON(t, w, []map[string]any{{"id": "proj-1", "name": "Admin"}})
		case r.Method == http.MethodPost && r.URL.Path == "/tasks":
			defer r.Body.Close()
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read create body: %v", err)
			}
			if err := json.Unmarshal(body, &createBody); err != nil {
				t.Fatalf("unmarshal create body: %v", err)
			}
			writeTodoistJSON(t, w, map[string]any{
				"id":            "task-99",
				"content":       createBody["content"],
				"project_id":    createBody["project_id"],
				"priority":      4,
				"labels":        []string{"waiting"},
				"comment_count": 2,
				"url":           "https://todoist.test/task-99",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	createTodoistTestAccount(t, app, "Personal Todoist", server.URL)
	workspace, err := app.store.CreateWorkspace("Admin", filepath.Join(t.TempDir(), "admin"))
	if err != nil {
		t.Fatalf("CreateWorkspace() error: %v", err)
	}
	if _, err := app.store.SetContainerMapping(store.ExternalProviderTodoist, "project", "Admin", &workspace.ID, nil, nil); err != nil {
		t.Fatalf("SetContainerMapping() error: %v", err)
	}

	followUpAt := "2026-03-10T09:00:00Z"
	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items", map[string]any{
		"title":        "Review proposal",
		"workspace_id": workspace.ID,
		"source":       store.ExternalProviderTodoist,
		"follow_up_at": followUpAt,
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create todoist item status = %d, want 201: %s", rr.Code, rr.Body.String())
	}
	if got := strFromAny(createBody["content"]); got != "Review proposal" {
		t.Fatalf("create content = %q, want Review proposal", got)
	}
	if got := strFromAny(createBody["project_id"]); got != "proj-1" {
		t.Fatalf("create project_id = %q, want proj-1", got)
	}
	if got := strFromAny(createBody["due_datetime"]); got != followUpAt {
		t.Fatalf("create due_datetime = %q, want %q", got, followUpAt)
	}

	item := mustFirstItemByState(t, app, store.ItemStateInbox)
	if item.SourceRef == nil || *item.SourceRef != "task:task-99" {
		t.Fatalf("item source_ref = %v, want task:task-99", item.SourceRef)
	}
	if item.WorkspaceID == nil || *item.WorkspaceID != workspace.ID {
		t.Fatalf("item workspace_id = %v, want %d", item.WorkspaceID, workspace.ID)
	}
	if item.ArtifactID == nil {
		t.Fatal("expected todoist source artifact")
	}
	artifact, err := app.store.GetArtifact(*item.ArtifactID)
	if err != nil {
		t.Fatalf("GetArtifact() error: %v", err)
	}
	if artifact.Kind != store.ArtifactKindExternalTask {
		t.Fatalf("artifact kind = %q, want %q", artifact.Kind, store.ArtifactKindExternalTask)
	}
	if artifact.RefURL == nil || *artifact.RefURL != "https://todoist.test/task-99" {
		t.Fatalf("artifact ref_url = %v, want task URL", artifact.RefURL)
	}
}

func TestItemUpdateTodoistFollowUpSyncsRemoteTask(t *testing.T) {
	app := newAuthedTestApp(t)

	var updateBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/tasks/task-1":
			defer r.Body.Close()
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read update body: %v", err)
			}
			if err := json.Unmarshal(body, &updateBody); err != nil {
				t.Fatalf("unmarshal update body: %v", err)
			}
			writeTodoistJSON(t, w, map[string]any{"id": "task-1", "content": "Follow up"})
		case r.Method == http.MethodGet && r.URL.Path == "/tasks/task-1":
			writeTodoistJSON(t, w, map[string]any{"id": "task-1", "content": "Follow up"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	createTodoistTestAccount(t, app, "Personal Todoist", server.URL)
	source := store.ExternalProviderTodoist
	sourceRef := "task:task-1"
	item, err := app.store.CreateItem("Follow up", store.ItemOptions{
		Source:    &source,
		SourceRef: &sourceRef,
	})
	if err != nil {
		t.Fatalf("CreateItem() error: %v", err)
	}

	nextFollowUp := "2026-03-11T15:04:05Z"
	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPut, "/api/items/"+itoa(item.ID), map[string]any{
		"follow_up_at": nextFollowUp,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("update todoist item status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	if got := strFromAny(updateBody["due_datetime"]); got != nextFollowUp {
		t.Fatalf("update due_datetime = %q, want %q", got, nextFollowUp)
	}
	updated, err := app.store.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem() error: %v", err)
	}
	if updated.FollowUpAt == nil || *updated.FollowUpAt != nextFollowUp {
		t.Fatalf("updated follow_up_at = %v, want %q", updated.FollowUpAt, nextFollowUp)
	}
}

func TestItemStateDoneTodoistBackedItemCompletesRemoteTask(t *testing.T) {
	app := newAuthedTestApp(t)

	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		if r.Method != http.MethodPost || r.URL.Path != "/tasks/task-1/close" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	createTodoistTestAccount(t, app, "Personal Todoist", server.URL)
	source := store.ExternalProviderTodoist
	sourceRef := "task:task-1"
	item, err := app.store.CreateItem("Follow up", store.ItemOptions{
		Source:    &source,
		SourceRef: &sourceRef,
	})
	if err != nil {
		t.Fatalf("CreateItem() error: %v", err)
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPut, "/api/items/"+itoa(item.ID)+"/state", map[string]any{
		"state": store.ItemStateDone,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("todoist state status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	if len(calls) != 1 || calls[0] != "/tasks/task-1/close" {
		t.Fatalf("close calls = %#v, want [/tasks/task-1/close]", calls)
	}
	completed, err := app.store.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem() error: %v", err)
	}
	if completed.State != store.ItemStateDone {
		t.Fatalf("completed state = %q, want %q", completed.State, store.ItemStateDone)
	}
}

func TestItemUpdateTodoistFollowUpRejectsInvalidTimestamp(t *testing.T) {
	app := newAuthedTestApp(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	createTodoistTestAccount(t, app, "Personal Todoist", server.URL)
	source := store.ExternalProviderTodoist
	sourceRef := "task:task-1"
	item, err := app.store.CreateItem("Follow up", store.ItemOptions{
		Source:    &source,
		SourceRef: &sourceRef,
	})
	if err != nil {
		t.Fatalf("CreateItem() error: %v", err)
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPut, "/api/items/"+itoa(item.ID), map[string]any{
		"follow_up_at": "tomorrow morning",
	})
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("invalid follow_up status = %d, want 502: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "follow_up_at must be a valid RFC3339 timestamp") {
		t.Fatalf("invalid follow_up body = %q", rr.Body.String())
	}
}
