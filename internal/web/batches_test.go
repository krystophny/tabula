package web

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/krystophny/tabura/internal/store"
)

func TestBatchAPIListDetailAndArtifact(t *testing.T) {
	app := newAuthedTestApp(t)

	workspace, err := app.store.CreateWorkspace("Batch Workspace", filepath.Join(t.TempDir(), "workspace"))
	if err != nil {
		t.Fatalf("CreateWorkspace() error: %v", err)
	}
	otherWorkspace, err := app.store.CreateWorkspace("Other Workspace", filepath.Join(t.TempDir(), "other"))
	if err != nil {
		t.Fatalf("CreateWorkspace(other) error: %v", err)
	}
	itemDone, err := app.store.CreateItem("Fix login flow", store.ItemOptions{WorkspaceID: &workspace.ID})
	if err != nil {
		t.Fatalf("CreateItem(done) error: %v", err)
	}
	itemFailed, err := app.store.CreateItem("Repair flaky spec", store.ItemOptions{WorkspaceID: &workspace.ID})
	if err != nil {
		t.Fatalf("CreateItem(failed) error: %v", err)
	}

	run, err := app.store.CreateBatchRun(workspace.ID, `{"worker":"codex"}`, "running")
	if err != nil {
		t.Fatalf("CreateBatchRun() error: %v", err)
	}
	if _, err := app.store.CreateBatchRun(otherWorkspace.ID, `{"worker":"other"}`, "running"); err != nil {
		t.Fatalf("CreateBatchRun(other) error: %v", err)
	}

	prNumber := int64(87)
	prURL := "https://example.test/pr/87"
	if _, err := app.store.UpsertBatchRunItem(run.ID, itemDone.ID, store.BatchRunItemUpdate{
		Status:   "done",
		PRNumber: &prNumber,
		PRURL:    &prURL,
	}); err != nil {
		t.Fatalf("UpsertBatchRunItem(done) error: %v", err)
	}
	errorMsg := "review queue stalled"
	if _, err := app.store.UpsertBatchRunItem(run.ID, itemFailed.ID, store.BatchRunItemUpdate{
		Status:   "failed",
		ErrorMsg: &errorMsg,
	}); err != nil {
		t.Fatalf("UpsertBatchRunItem(failed) error: %v", err)
	}

	rrList := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/batches?workspace_id="+itoa(workspace.ID), nil)
	if rrList.Code != http.StatusOK {
		t.Fatalf("batch list status = %d, want 200: %s", rrList.Code, rrList.Body.String())
	}
	listPayload := decodeJSONDataResponse(t, rrList)
	runs, ok := listPayload["batch_runs"].([]any)
	if !ok || len(runs) != 1 {
		t.Fatalf("batch list payload = %#v", listPayload)
	}

	rrGet := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/batches/"+itoa(run.ID), nil)
	if rrGet.Code != http.StatusOK {
		t.Fatalf("batch get status = %d, want 200: %s", rrGet.Code, rrGet.Body.String())
	}
	getPayload := decodeJSONDataResponse(t, rrGet)
	items, ok := getPayload["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("batch detail payload = %#v", getPayload)
	}

	rrArtifact := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/batches/"+itoa(run.ID)+"/artifact", nil)
	if rrArtifact.Code != http.StatusOK {
		t.Fatalf("batch artifact status = %d, want 200: %s", rrArtifact.Code, rrArtifact.Body.String())
	}
	artifactPayload := decodeJSONDataResponse(t, rrArtifact)
	markdown := strings.TrimSpace(strFromAny(artifactPayload["markdown"]))
	if !strings.Contains(markdown, "| Fix login flow | done | [#87](https://example.test/pr/87) |  |") {
		t.Fatalf("artifact markdown missing done row: %s", markdown)
	}
	if !strings.Contains(markdown, "| Repair flaky spec | failed |  | review queue stalled |") {
		t.Fatalf("artifact markdown missing failed row: %s", markdown)
	}
}

func TestBatchArtifactEmptyBatch(t *testing.T) {
	app := newAuthedTestApp(t)

	workspace, err := app.store.CreateWorkspace("Batch Workspace", filepath.Join(t.TempDir(), "workspace"))
	if err != nil {
		t.Fatalf("CreateWorkspace() error: %v", err)
	}
	run, err := app.store.CreateBatchRun(workspace.ID, "", "")
	if err != nil {
		t.Fatalf("CreateBatchRun() error: %v", err)
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/batches/"+itoa(run.ID)+"/artifact", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("empty batch artifact status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	payload := decodeJSONDataResponse(t, rr)
	markdown := strFromAny(payload["markdown"])
	if !strings.Contains(markdown, "_No items recorded for this batch._") {
		t.Fatalf("empty batch artifact markdown = %q, want no-items note", markdown)
	}
}

func TestBatchProgressBroadcastsToConnectedClients(t *testing.T) {
	app := newAuthedTestApp(t)

	workspace, err := app.store.CreateWorkspace("Batch Workspace", filepath.Join(t.TempDir(), "workspace"))
	if err != nil {
		t.Fatalf("CreateWorkspace() error: %v", err)
	}
	item, err := app.store.CreateItem("Fix login flow", store.ItemOptions{WorkspaceID: &workspace.ID})
	if err != nil {
		t.Fatalf("CreateItem() error: %v", err)
	}
	run, err := app.store.CreateBatchRun(workspace.ID, `{"worker":"codex"}`, "running")
	if err != nil {
		t.Fatalf("CreateBatchRun() error: %v", err)
	}

	conn, clientConn, cleanup := newParticipantTestWSConn(t)
	defer cleanup()
	app.hub.registerChat("batch-test-session", conn)
	defer app.hub.unregisterChat("batch-test-session", conn)

	errorMsg := "lint failed"
	if _, err := app.recordBatchRunItemUpdate(run.ID, item.ID, store.BatchRunItemUpdate{
		Status:   "failed",
		ErrorMsg: &errorMsg,
	}); err != nil {
		t.Fatalf("recordBatchRunItemUpdate() error: %v", err)
	}

	itemPayload := waitForWSJSONMessageType(t, clientConn, 2*time.Second, batchProgressEventType)
	if got := int64FromAny(itemPayload["batch_id"]); got != run.ID {
		t.Fatalf("batch_progress batch_id = %d, want %d", got, run.ID)
	}
	itemData, ok := itemPayload["item"].(map[string]any)
	if !ok {
		t.Fatalf("batch_progress item payload = %#v", itemPayload)
	}
	if got := strFromAny(itemData["status"]); got != "failed" {
		t.Fatalf("batch_progress item status = %q, want failed", got)
	}

	finishedAt := "2026-03-09T10:30:00Z"
	if _, err := app.recordBatchRunStatus(run.ID, "completed", &finishedAt); err != nil {
		t.Fatalf("recordBatchRunStatus() error: %v", err)
	}

	runPayload := waitForWSJSONMessageType(t, clientConn, 2*time.Second, batchProgressEventType)
	batchData, ok := runPayload["batch"].(map[string]any)
	if !ok {
		t.Fatalf("batch_progress batch payload = %#v", runPayload)
	}
	if got := strFromAny(batchData["status"]); got != "completed" {
		t.Fatalf("batch_progress batch status = %q, want completed", got)
	}
	if got := strFromAny(batchData["finished_at"]); got != finishedAt {
		t.Fatalf("batch_progress batch finished_at = %q, want %q", got, finishedAt)
	}
}
