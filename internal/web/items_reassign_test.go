package web

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/krystophny/tabura/internal/store"
)

func TestItemWorkspaceReassignmentAPI(t *testing.T) {
	app := newAuthedTestApp(t)

	oldWorkspace, err := app.store.CreateWorkspace("Alpha", filepath.Join(t.TempDir(), "alpha"))
	if err != nil {
		t.Fatalf("CreateWorkspace(alpha) error: %v", err)
	}
	newWorkspace, err := app.store.CreateWorkspace("Beta", filepath.Join(t.TempDir(), "beta"))
	if err != nil {
		t.Fatalf("CreateWorkspace(beta) error: %v", err)
	}
	refPath := filepath.Join(oldWorkspace.DirPath, "README.md")
	title := "README.md"
	artifact, err := app.store.CreateArtifact(store.ArtifactKindMarkdown, &refPath, nil, &title, nil)
	if err != nil {
		t.Fatalf("CreateArtifact() error: %v", err)
	}
	item, err := app.store.CreateItem("Review workspace assignment", store.ItemOptions{
		WorkspaceID: &oldWorkspace.ID,
		ArtifactID:  &artifact.ID,
	})
	if err != nil {
		t.Fatalf("CreateItem() error: %v", err)
	}

	rrWorkspace := doAuthedJSONRequest(t, app.Router(), http.MethodPut, "/api/items/"+itoa(item.ID)+"/workspace", map[string]any{
		"workspace_id": newWorkspace.ID,
	})
	if rrWorkspace.Code != http.StatusOK {
		t.Fatalf("workspace reassignment status = %d, want 200: %s", rrWorkspace.Code, rrWorkspace.Body.String())
	}
	workspacePayload := decodeJSONResponse(t, rrWorkspace)
	if got := strFromAny(workspacePayload["warning"]); got == "" {
		t.Fatalf("workspace warning = %q, want artifact link warning", got)
	}
	updatedItem, err := app.store.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem(reassigned workspace) error: %v", err)
	}
	if updatedItem.WorkspaceID == nil || *updatedItem.WorkspaceID != newWorkspace.ID {
		t.Fatalf("workspace_id = %v, want %d", updatedItem.WorkspaceID, newWorkspace.ID)
	}
	updatedArtifact, err := app.store.GetArtifact(artifact.ID)
	if err != nil {
		t.Fatalf("GetArtifact(updated) error: %v", err)
	}
	if updatedArtifact.RefPath == nil || *updatedArtifact.RefPath != refPath {
		t.Fatalf("artifact ref_path = %v, want %q", updatedArtifact.RefPath, refPath)
	}

	rrClearWorkspace := doAuthedJSONRequest(t, app.Router(), http.MethodPut, "/api/items/"+itoa(item.ID)+"/workspace", map[string]any{
		"workspace_id": nil,
	})
	if rrClearWorkspace.Code != http.StatusOK {
		t.Fatalf("clear workspace status = %d, want 200: %s", rrClearWorkspace.Code, rrClearWorkspace.Body.String())
	}
	updatedItem, err = app.store.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem(cleared workspace) error: %v", err)
	}
	if updatedItem.WorkspaceID != nil {
		t.Fatalf("cleared workspace_id = %v, want nil", updatedItem.WorkspaceID)
	}
}

func TestLegacyItemProjectReassignmentRouteRemoved(t *testing.T) {
	app := newAuthedTestApp(t)
	item, err := app.store.CreateItem("Reassign me", store.ItemOptions{})
	if err != nil {
		t.Fatalf("CreateItem() error: %v", err)
	}

	rrProject := doAuthedJSONRequest(t, app.Router(), http.MethodPut, "/api/items/"+itoa(item.ID)+"/project", map[string]any{
		"workspace_id": "missing-project",
	})
	if rrProject.Code != http.StatusNotFound {
		t.Fatalf("legacy project route status = %d, want 404: %s", rrProject.Code, rrProject.Body.String())
	}
}

func TestItemWorkspaceReassignmentAPIRejectsUnknownWorkspace(t *testing.T) {
	app := newAuthedTestApp(t)
	item, err := app.store.CreateItem("Reassign me", store.ItemOptions{})
	if err != nil {
		t.Fatalf("CreateItem() error: %v", err)
	}

	rrWorkspace := doAuthedJSONRequest(t, app.Router(), http.MethodPut, "/api/items/"+itoa(item.ID)+"/workspace", map[string]any{
		"workspace_id": 9999,
	})
	if rrWorkspace.Code != http.StatusBadRequest {
		t.Fatalf("unknown workspace status = %d, want 400: %s", rrWorkspace.Code, rrWorkspace.Body.String())
	}
}
