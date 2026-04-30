package web

import (
	"database/sql"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/sloppy-org/slopshell/internal/store"
)

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}

func TestItemAssignmentLifecycleAPI(t *testing.T) {
	app := newAuthedTestApp(t)

	actor, err := app.store.CreateActor("Codex", store.ActorKindAgent)
	if err != nil {
		t.Fatalf("CreateActor() error: %v", err)
	}
	item, err := app.store.CreateItem("Delegate this", store.ItemOptions{})
	if err != nil {
		t.Fatalf("CreateItem() error: %v", err)
	}

	rrAssign := doAuthedJSONRequest(t, app.Router(), http.MethodPut, "/api/items/"+itoa(item.ID)+"/assign", map[string]any{
		"actor_id": actor.ID,
	})
	if rrAssign.Code != http.StatusOK {
		t.Fatalf("assign status = %d, want 200: %s", rrAssign.Code, rrAssign.Body.String())
	}
	gotItem, err := app.store.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem(assigned) error: %v", err)
	}
	if gotItem.ActorID == nil || *gotItem.ActorID != actor.ID {
		t.Fatalf("assigned ActorID = %v, want %d", gotItem.ActorID, actor.ID)
	}
	if gotItem.State != store.ItemStateWaiting {
		t.Fatalf("assigned State = %q, want %q", gotItem.State, store.ItemStateWaiting)
	}

	rrUnassign := doAuthedJSONRequest(t, app.Router(), http.MethodPut, "/api/items/"+itoa(item.ID)+"/unassign", map[string]any{})
	if rrUnassign.Code != http.StatusOK {
		t.Fatalf("unassign status = %d, want 200: %s", rrUnassign.Code, rrUnassign.Body.String())
	}
	gotItem, err = app.store.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem(unassigned) error: %v", err)
	}
	if gotItem.ActorID != nil {
		t.Fatalf("unassigned ActorID = %v, want nil", gotItem.ActorID)
	}
	if gotItem.State != store.ItemStateInbox {
		t.Fatalf("unassigned State = %q, want %q", gotItem.State, store.ItemStateInbox)
	}

	rrReassign := doAuthedJSONRequest(t, app.Router(), http.MethodPut, "/api/items/"+itoa(item.ID)+"/assign", map[string]any{
		"actor_id": actor.ID,
	})
	if rrReassign.Code != http.StatusOK {
		t.Fatalf("reassign status = %d, want 200: %s", rrReassign.Code, rrReassign.Body.String())
	}
	rrComplete := doAuthedJSONRequest(t, app.Router(), http.MethodPut, "/api/items/"+itoa(item.ID)+"/complete", map[string]any{
		"actor_id": actor.ID,
	})
	if rrComplete.Code != http.StatusOK {
		t.Fatalf("complete status = %d, want 200: %s", rrComplete.Code, rrComplete.Body.String())
	}
	gotItem, err = app.store.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem(completed) error: %v", err)
	}
	if gotItem.State != store.ItemStateDone {
		t.Fatalf("completed State = %q, want %q", gotItem.State, store.ItemStateDone)
	}
}

func TestItemCRUDAndStateAPI(t *testing.T) {
	app := newAuthedTestApp(t)

	workspace, err := app.store.CreateWorkspace("Default", filepath.Join(t.TempDir(), "workspace"))
	if err != nil {
		t.Fatalf("CreateWorkspace() error: %v", err)
	}
	if _, err := app.store.SetWorkspaceSphere(workspace.ID, store.SphereWork); err != nil {
		t.Fatalf("SetWorkspaceSphere() error: %v", err)
	}
	artifactTitle := "Plan"
	artifact, err := app.store.CreateArtifact(store.ArtifactKindMarkdown, nil, nil, &artifactTitle, nil)
	if err != nil {
		t.Fatalf("CreateArtifact() error: %v", err)
	}
	actor, err := app.store.CreateActor("Codex", store.ActorKindAgent)
	if err != nil {
		t.Fatalf("CreateActor() error: %v", err)
	}

	rrCreate := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items", map[string]any{
		"title":        "Wire REST endpoints",
		"workspace_id": workspace.ID,
		"artifact_id":  artifact.ID,
		"source":       "github",
		"source_ref":   "owner/repo#175",
	})
	if rrCreate.Code != http.StatusCreated {
		t.Fatalf("create item status = %d, want 201: %s", rrCreate.Code, rrCreate.Body.String())
	}
	createPayload := decodeJSONDataResponse(t, rrCreate)
	itemPayload, ok := createPayload["item"].(map[string]any)
	if !ok {
		t.Fatalf("create item payload = %#v", createPayload)
	}
	itemID := int64(itemPayload["id"].(float64))
	if got := itemPayload["sphere"]; got != store.SphereWork {
		t.Fatalf("create item sphere = %#v, want %q", got, store.SphereWork)
	}

	rrList := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items?state=inbox", nil)
	if rrList.Code != http.StatusOK {
		t.Fatalf("list items status = %d, want 200: %s", rrList.Code, rrList.Body.String())
	}
	listPayload := decodeJSONDataResponse(t, rrList)
	items, ok := listPayload["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("list items payload = %#v", listPayload)
	}

	visibleAfter := "2026-03-10T09:00:00Z"
	rrUpdate := doAuthedJSONRequest(t, app.Router(), http.MethodPut, "/api/items/"+itoa(itemID), map[string]any{
		"title":         "Wire core REST endpoints",
		"visible_after": visibleAfter,
	})
	if rrUpdate.Code != http.StatusOK {
		t.Fatalf("update item status = %d, want 200: %s", rrUpdate.Code, rrUpdate.Body.String())
	}
	updated, err := app.store.GetItem(itemID)
	if err != nil {
		t.Fatalf("GetItem(updated) error: %v", err)
	}
	if updated.Title != "Wire core REST endpoints" {
		t.Fatalf("updated title = %q, want %q", updated.Title, "Wire core REST endpoints")
	}
	if updated.VisibleAfter == nil || *updated.VisibleAfter != visibleAfter {
		t.Fatalf("updated visible_after = %v, want %q", updated.VisibleAfter, visibleAfter)
	}

	rrAssign := doAuthedJSONRequest(t, app.Router(), http.MethodPut, "/api/items/"+itoa(itemID)+"/assign", map[string]any{
		"actor_id": actor.ID,
	})
	if rrAssign.Code != http.StatusOK {
		t.Fatalf("assign item status = %d, want 200: %s", rrAssign.Code, rrAssign.Body.String())
	}

	rrState := doAuthedJSONRequest(t, app.Router(), http.MethodPut, "/api/items/"+itoa(itemID)+"/state", map[string]any{
		"state": store.ItemStateDone,
	})
	if rrState.Code != http.StatusOK {
		t.Fatalf("item state status = %d, want 200: %s", rrState.Code, rrState.Body.String())
	}
	doneItem, err := app.store.GetItem(itemID)
	if err != nil {
		t.Fatalf("GetItem(done) error: %v", err)
	}
	if doneItem.State != store.ItemStateDone {
		t.Fatalf("done state = %q, want %q", doneItem.State, store.ItemStateDone)
	}
	if doneItem.Sphere != store.SphereWork {
		t.Fatalf("done item sphere = %q, want %q", doneItem.Sphere, store.SphereWork)
	}

	rrReopen := doAuthedJSONRequest(t, app.Router(), http.MethodPut, "/api/items/"+itoa(itemID)+"/state", map[string]any{
		"state": store.ItemStateInbox,
	})
	if rrReopen.Code != http.StatusOK {
		t.Fatalf("item reopen status = %d, want 200: %s", rrReopen.Code, rrReopen.Body.String())
	}
	reopenedItem, err := app.store.GetItem(itemID)
	if err != nil {
		t.Fatalf("GetItem(reopened) error: %v", err)
	}
	if reopenedItem.State != store.ItemStateInbox {
		t.Fatalf("reopened state = %q, want %q", reopenedItem.State, store.ItemStateInbox)
	}

	rrGet := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/"+itoa(itemID), nil)
	if rrGet.Code != http.StatusOK {
		t.Fatalf("get item status = %d, want 200: %s", rrGet.Code, rrGet.Body.String())
	}

	rrDelete := doAuthedJSONRequest(t, app.Router(), http.MethodDelete, "/api/items/"+itoa(itemID), nil)
	if rrDelete.Code != http.StatusNoContent {
		t.Fatalf("delete item status = %d, want 204: %s", rrDelete.Code, rrDelete.Body.String())
	}
	if rrDelete.Body.Len() != 0 {
		t.Fatalf("delete item body = %q, want empty", rrDelete.Body.String())
	}

	rrMissing := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/"+itoa(itemID), nil)
	if rrMissing.Code != http.StatusNotFound {
		t.Fatalf("deleted item status = %d, want 404: %s", rrMissing.Code, rrMissing.Body.String())
	}
}

func TestItemSphereAPI(t *testing.T) {
	app := newAuthedTestApp(t)
	if err := app.store.SetActiveSphere(store.SphereWork); err != nil {
		t.Fatalf("SetActiveSphere() error: %v", err)
	}

	rrCreate := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items", map[string]any{
		"title": "Active sphere item",
	})
	if rrCreate.Code != http.StatusCreated {
		t.Fatalf("create item status = %d, want 201: %s", rrCreate.Code, rrCreate.Body.String())
	}
	createPayload := decodeJSONDataResponse(t, rrCreate)
	itemPayload, ok := createPayload["item"].(map[string]any)
	if !ok {
		t.Fatalf("create payload = %#v", createPayload)
	}
	itemID := int64(itemPayload["id"].(float64))
	if got := itemPayload["sphere"]; got != store.SphereWork {
		t.Fatalf("created sphere = %#v, want %q", got, store.SphereWork)
	}

	privateSphere := store.SpherePrivate
	rrUpdate := doAuthedJSONRequest(t, app.Router(), http.MethodPut, "/api/items/"+itoa(itemID), map[string]any{
		"sphere": privateSphere,
	})
	if rrUpdate.Code != http.StatusOK {
		t.Fatalf("update item sphere status = %d, want 200: %s", rrUpdate.Code, rrUpdate.Body.String())
	}
	updated, err := app.store.GetItem(itemID)
	if err != nil {
		t.Fatalf("GetItem(updated) error: %v", err)
	}
	if updated.Sphere != store.SpherePrivate {
		t.Fatalf("updated sphere = %q, want %q", updated.Sphere, store.SpherePrivate)
	}
}

func TestItemStateViewAPI(t *testing.T) {
	app := newAuthedTestApp(t)

	workspace, err := app.store.CreateWorkspace("Default", filepath.Join(t.TempDir(), "workspace"))
	if err != nil {
		t.Fatalf("CreateWorkspace() error: %v", err)
	}
	artifactTitle := "Inbox plan"
	artifact, err := app.store.CreateArtifact(store.ArtifactKindIdeaNote, nil, nil, &artifactTitle, nil)
	if err != nil {
		t.Fatalf("CreateArtifact() error: %v", err)
	}
	actor, err := app.store.CreateActor("Alice", store.ActorKindHuman)
	if err != nil {
		t.Fatalf("CreateActor() error: %v", err)
	}

	past := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	future := time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339)
	source := "github"
	sourceRef := "owner/repo#177"
	visibleInbox, err := app.store.CreateItem("Visible inbox", store.ItemOptions{
		State:        store.ItemStateInbox,
		WorkspaceID:  &workspace.ID,
		ArtifactID:   &artifact.ID,
		ActorID:      &actor.ID,
		VisibleAfter: &past,
		Source:       &source,
		SourceRef:    &sourceRef,
	})
	if err != nil {
		t.Fatalf("CreateItem(visible inbox) error: %v", err)
	}
	if _, err := app.store.CreateItem("Hidden inbox", store.ItemOptions{
		State:        store.ItemStateInbox,
		VisibleAfter: &future,
	}); err != nil {
		t.Fatalf("CreateItem(hidden inbox) error: %v", err)
	}
	waitingItem, err := app.store.CreateItem("Waiting item", store.ItemOptions{State: store.ItemStateWaiting})
	if err != nil {
		t.Fatalf("CreateItem(waiting) error: %v", err)
	}
	nextItem, err := app.store.CreateItem("Next item", store.ItemOptions{State: store.ItemStateNext})
	if err != nil {
		t.Fatalf("CreateItem(next) error: %v", err)
	}
	deferredItem, err := app.store.CreateItem("Deferred item", store.ItemOptions{State: store.ItemStateDeferred})
	if err != nil {
		t.Fatalf("CreateItem(deferred) error: %v", err)
	}
	if _, err := app.store.CreateItem("Someday item", store.ItemOptions{State: store.ItemStateSomeday}); err != nil {
		t.Fatalf("CreateItem(someday) error: %v", err)
	}
	reviewItem, err := app.store.CreateItem("Review item", store.ItemOptions{State: store.ItemStateReview})
	if err != nil {
		t.Fatalf("CreateItem(review) error: %v", err)
	}
	doneItem, err := app.store.CreateItem("Done item", store.ItemOptions{State: store.ItemStateDone})
	if err != nil {
		t.Fatalf("CreateItem(done) error: %v", err)
	}

	rrInbox := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/inbox", nil)
	if rrInbox.Code != http.StatusOK {
		t.Fatalf("inbox status = %d, want 200: %s", rrInbox.Code, rrInbox.Body.String())
	}
	inboxPayload := decodeJSONResponse(t, rrInbox)
	inboxItems, ok := inboxPayload["items"].([]any)
	if !ok || len(inboxItems) != 1 {
		t.Fatalf("inbox payload = %#v", inboxPayload)
	}
	inboxRow, ok := inboxItems[0].(map[string]any)
	if !ok {
		t.Fatalf("inbox row = %#v", inboxItems[0])
	}
	if got := int64(inboxRow["id"].(float64)); got != visibleInbox.ID {
		t.Fatalf("inbox item id = %d, want %d", got, visibleInbox.ID)
	}
	if got := strFromAny(inboxRow["artifact_title"]); got != artifactTitle {
		t.Fatalf("artifact_title = %q, want %q", got, artifactTitle)
	}
	if got := strFromAny(inboxRow["artifact_kind"]); got != string(store.ArtifactKindIdeaNote) {
		t.Fatalf("artifact_kind = %q, want %q", got, store.ArtifactKindIdeaNote)
	}
	if got := strFromAny(inboxRow["actor_name"]); got != "Alice" {
		t.Fatalf("actor_name = %q, want Alice", got)
	}

	rrWaiting := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/waiting", nil)
	if rrWaiting.Code != http.StatusOK {
		t.Fatalf("waiting status = %d, want 200: %s", rrWaiting.Code, rrWaiting.Body.String())
	}
	waitingPayload := decodeJSONResponse(t, rrWaiting)
	waitingItems, ok := waitingPayload["items"].([]any)
	if !ok || len(waitingItems) != 1 {
		t.Fatalf("waiting payload = %#v", waitingPayload)
	}
	waitingRow, ok := waitingItems[0].(map[string]any)
	if !ok || int64(waitingRow["id"].(float64)) != waitingItem.ID {
		t.Fatalf("waiting row = %#v, want id %d", waitingRow, waitingItem.ID)
	}
	for _, tc := range []struct {
		path string
		id   int64
		name string
	}{
		{path: "/api/items/next", id: nextItem.ID, name: "next"},
		{path: "/api/items/deferred", id: deferredItem.ID, name: "deferred"},
		{path: "/api/items/review", id: reviewItem.ID, name: "review"},
	} {
		rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, tc.path, nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200: %s", tc.name, rr.Code, rr.Body.String())
		}
		payload := decodeJSONResponse(t, rr)
		items, ok := payload["items"].([]any)
		if !ok || len(items) != 1 {
			t.Fatalf("%s payload = %#v", tc.name, payload)
		}
		row, ok := items[0].(map[string]any)
		if !ok || int64(row["id"].(float64)) != tc.id {
			t.Fatalf("%s row = %#v, want id %d", tc.name, row, tc.id)
		}
	}

	rrDone := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/done?limit=1", nil)
	if rrDone.Code != http.StatusOK {
		t.Fatalf("done status = %d, want 200: %s", rrDone.Code, rrDone.Body.String())
	}
	donePayload := decodeJSONResponse(t, rrDone)
	doneItems, ok := donePayload["items"].([]any)
	if !ok || len(doneItems) != 1 {
		t.Fatalf("done payload = %#v", donePayload)
	}
	doneRow, ok := doneItems[0].(map[string]any)
	if !ok || int64(doneRow["id"].(float64)) != doneItem.ID {
		t.Fatalf("done row = %#v, want id %d", doneRow, doneItem.ID)
	}

	rrCounts := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/counts", nil)
	if rrCounts.Code != http.StatusOK {
		t.Fatalf("counts status = %d, want 200: %s", rrCounts.Code, rrCounts.Body.String())
	}
	countsPayload := decodeJSONResponse(t, rrCounts)
	counts, ok := countsPayload["counts"].(map[string]any)
	if !ok {
		t.Fatalf("counts payload = %#v", countsPayload)
	}
	if got := int(counts[store.ItemStateInbox].(float64)); got != 1 {
		t.Fatalf("counts[inbox] = %d, want 1", got)
	}
	if got := int(counts[store.ItemStateWaiting].(float64)); got != 1 {
		t.Fatalf("counts[waiting] = %d, want 1", got)
	}
	if got := int(counts[store.ItemStateNext].(float64)); got != 1 {
		t.Fatalf("counts[next] = %d, want 1", got)
	}
	if got := int(counts[store.ItemStateDeferred].(float64)); got != 1 {
		t.Fatalf("counts[deferred] = %d, want 1", got)
	}
	if got := int(counts[store.ItemStateSomeday].(float64)); got != 1 {
		t.Fatalf("counts[someday] = %d, want 1", got)
	}
	if got := int(counts[store.ItemStateReview].(float64)); got != 1 {
		t.Fatalf("counts[review] = %d, want 1", got)
	}
	if got := int(counts[store.ItemStateDone].(float64)); got != 1 {
		t.Fatalf("counts[done] = %d, want 1", got)
	}

	rrBadLimit := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/done?limit=bad", nil)
	if rrBadLimit.Code != http.StatusBadRequest {
		t.Fatalf("bad done limit status = %d, want 400: %s", rrBadLimit.Code, rrBadLimit.Body.String())
	}
}

func TestItemStateViewAPIResurfacesDueDeferredItemsImmediately(t *testing.T) {
	app := newAuthedTestApp(t)

	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	future := time.Now().UTC().Add(time.Minute).Format(time.RFC3339)

	dueVisible, err := app.store.CreateItem("Due visible_after", store.ItemOptions{
		State:        store.ItemStateDeferred,
		VisibleAfter: &past,
	})
	if err != nil {
		t.Fatalf("CreateItem(due visible_after) error: %v", err)
	}
	dueFollowUp, err := app.store.CreateItem("Due follow_up_at", store.ItemOptions{
		State:      store.ItemStateDeferred,
		FollowUpAt: &past,
	})
	if err != nil {
		t.Fatalf("CreateItem(due follow_up_at) error: %v", err)
	}
	futureWaiting, err := app.store.CreateItem("Future waiting", store.ItemOptions{
		State:        store.ItemStateWaiting,
		VisibleAfter: &future,
	})
	if err != nil {
		t.Fatalf("CreateItem(future waiting) error: %v", err)
	}

	rrNext := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/next", nil)
	if rrNext.Code != http.StatusOK {
		t.Fatalf("next status = %d, want 200: %s", rrNext.Code, rrNext.Body.String())
	}
	nextItems, ok := decodeJSONResponse(t, rrNext)["items"].([]any)
	if !ok || len(nextItems) != 2 {
		t.Fatalf("next payload = %#v", decodeJSONResponse(t, rrNext))
	}

	nextIDs := map[int64]bool{}
	for _, raw := range nextItems {
		row, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("next row = %#v", raw)
		}
		nextIDs[int64(row["id"].(float64))] = true
	}
	if !nextIDs[dueVisible.ID] || !nextIDs[dueFollowUp.ID] {
		t.Fatalf("next ids = %#v, want %d and %d", nextIDs, dueVisible.ID, dueFollowUp.ID)
	}

	rrList := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items?state=next", nil)
	if rrList.Code != http.StatusOK {
		t.Fatalf("list next status = %d, want 200: %s", rrList.Code, rrList.Body.String())
	}
	listItems, ok := decodeJSONDataResponse(t, rrList)["items"].([]any)
	if !ok || len(listItems) != 2 {
		t.Fatalf("list next payload = %#v", decodeJSONDataResponse(t, rrList))
	}

	rrWaiting := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/waiting", nil)
	if rrWaiting.Code != http.StatusOK {
		t.Fatalf("waiting status = %d, want 200: %s", rrWaiting.Code, rrWaiting.Body.String())
	}
	waitingItems, ok := decodeJSONResponse(t, rrWaiting)["items"].([]any)
	if !ok || len(waitingItems) != 1 {
		t.Fatalf("waiting payload = %#v", decodeJSONResponse(t, rrWaiting))
	}
	waitingRow, ok := waitingItems[0].(map[string]any)
	if !ok || int64(waitingRow["id"].(float64)) != futureWaiting.ID {
		t.Fatalf("waiting row = %#v, want id %d", waitingRow, futureWaiting.ID)
	}

	rrCounts := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/counts", nil)
	if rrCounts.Code != http.StatusOK {
		t.Fatalf("counts status = %d, want 200: %s", rrCounts.Code, rrCounts.Body.String())
	}
	counts, ok := decodeJSONResponse(t, rrCounts)["counts"].(map[string]any)
	if !ok {
		t.Fatalf("counts payload = %#v", decodeJSONResponse(t, rrCounts))
	}
	if got := int(counts[store.ItemStateNext].(float64)); got != 2 {
		t.Fatalf("counts[next] = %d, want 2", got)
	}
	if got := int(counts[store.ItemStateWaiting].(float64)); got != 1 {
		t.Fatalf("counts[waiting] = %d, want 1", got)
	}

	for _, tc := range []struct {
		name string
		id   int64
		want string
	}{
		{name: "due visible_after", id: dueVisible.ID, want: store.ItemStateNext},
		{name: "due follow_up_at", id: dueFollowUp.ID, want: store.ItemStateNext},
		{name: "future waiting", id: futureWaiting.ID, want: store.ItemStateWaiting},
	} {
		item, err := app.store.GetItem(tc.id)
		if err != nil {
			t.Fatalf("GetItem(%s) error: %v", tc.name, err)
		}
		if item.State != tc.want {
			t.Fatalf("%s state = %q, want %q", tc.name, item.State, tc.want)
		}
	}
}

func TestItemStateViewAPIFiltersBySphere(t *testing.T) {
	app := newAuthedTestApp(t)

	workWorkspace, err := app.store.CreateWorkspace("Work", filepath.Join(t.TempDir(), "work"))
	if err != nil {
		t.Fatalf("CreateWorkspace(work) error: %v", err)
	}
	if _, err := app.store.SetWorkspaceSphere(workWorkspace.ID, store.SphereWork); err != nil {
		t.Fatalf("SetWorkspaceSphere(work) error: %v", err)
	}

	if _, err := app.store.CreateItem("Private inbox", store.ItemOptions{
		State: store.ItemStateInbox,
	}); err != nil {
		t.Fatalf("CreateItem(private) error: %v", err)
	}
	workItem, err := app.store.CreateItem("Work inbox", store.ItemOptions{
		State:       store.ItemStateInbox,
		WorkspaceID: &workWorkspace.ID,
	})
	if err != nil {
		t.Fatalf("CreateItem(work) error: %v", err)
	}
	if _, err := app.store.CreateItem("Private done", store.ItemOptions{
		State: store.ItemStateDone,
	}); err != nil {
		t.Fatalf("CreateItem(private done) error: %v", err)
	}
	if _, err := app.store.CreateItem("Work done", store.ItemOptions{
		State:       store.ItemStateDone,
		WorkspaceID: &workWorkspace.ID,
	}); err != nil {
		t.Fatalf("CreateItem(work done) error: %v", err)
	}

	rrInbox := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/inbox?sphere=work", nil)
	if rrInbox.Code != http.StatusOK {
		t.Fatalf("work inbox status = %d, want 200: %s", rrInbox.Code, rrInbox.Body.String())
	}
	inboxPayload := decodeJSONResponse(t, rrInbox)
	inboxItems, ok := inboxPayload["items"].([]any)
	if !ok || len(inboxItems) != 1 {
		t.Fatalf("work inbox payload = %#v", inboxPayload)
	}
	inboxRow, ok := inboxItems[0].(map[string]any)
	if !ok {
		t.Fatalf("work inbox row = %#v", inboxItems[0])
	}
	if got := int64(inboxRow["id"].(float64)); got != workItem.ID {
		t.Fatalf("work inbox id = %d, want %d", got, workItem.ID)
	}
	if got := strFromAny(inboxRow["sphere"]); got != store.SphereWork {
		t.Fatalf("work inbox sphere = %q, want %q", got, store.SphereWork)
	}

	rrCounts := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/counts?sphere=work", nil)
	if rrCounts.Code != http.StatusOK {
		t.Fatalf("work counts status = %d, want 200: %s", rrCounts.Code, rrCounts.Body.String())
	}
	countsPayload := decodeJSONResponse(t, rrCounts)
	counts, ok := countsPayload["counts"].(map[string]any)
	if !ok {
		t.Fatalf("work counts payload = %#v", countsPayload)
	}
	if got := int(counts[store.ItemStateInbox].(float64)); got != 1 {
		t.Fatalf("work counts[inbox] = %d, want 1", got)
	}
	if got := int(counts[store.ItemStateDone].(float64)); got != 1 {
		t.Fatalf("work counts[done] = %d, want 1", got)
	}

	rrBad := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/inbox?sphere=office", nil)
	if rrBad.Code != http.StatusBadRequest {
		t.Fatalf("invalid sphere status = %d, want 400: %s", rrBad.Code, rrBad.Body.String())
	}
}

func TestItemStateViewAPIFiltersBySourceWorkspaceAndProject(t *testing.T) {
	app := newAuthedTestApp(t)

	workspace, err := app.store.CreateWorkspace("Inbox Workspace", filepath.Join(t.TempDir(), "workspace"))
	if err != nil {
		t.Fatalf("CreateWorkspace() error: %v", err)
	}
	past := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	sourceTodoist := store.ExternalProviderTodoist
	sourceExchange := store.ExternalProviderExchange

	if _, err := app.store.CreateItem("Unassigned todoist", store.ItemOptions{
		State:        store.ItemStateInbox,
		VisibleAfter: &past,
		Source:       &sourceTodoist,
	}); err != nil {
		t.Fatalf("CreateItem(unassigned todoist) error: %v", err)
	}
	if _, err := app.store.CreateItem("Workspace todoist", store.ItemOptions{
		State:        store.ItemStateInbox,
		WorkspaceID:  &workspace.ID,
		VisibleAfter: &past,
		Source:       &sourceTodoist,
	}); err != nil {
		t.Fatalf("CreateItem(workspace todoist) error: %v", err)
	}
	if _, err := app.store.CreateItem("Workspace exchange", store.ItemOptions{
		State:        store.ItemStateInbox,
		WorkspaceID:  &workspace.ID,
		VisibleAfter: &past,
		Source:       &sourceExchange,
	}); err != nil {
		t.Fatalf("CreateItem(workspace exchange) error: %v", err)
	}

	rrSource := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/inbox?source=todoist", nil)
	if rrSource.Code != http.StatusOK {
		t.Fatalf("todoist inbox status = %d, want 200: %s", rrSource.Code, rrSource.Body.String())
	}
	sourceItems, ok := decodeJSONResponse(t, rrSource)["items"].([]any)
	if !ok || len(sourceItems) != 2 {
		t.Fatalf("todoist inbox payload = %#v", decodeJSONResponse(t, rrSource))
	}

	rrUnassigned := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/inbox?workspace_id=null", nil)
	if rrUnassigned.Code != http.StatusOK {
		t.Fatalf("unassigned inbox status = %d, want 200: %s", rrUnassigned.Code, rrUnassigned.Body.String())
	}
	unassignedItems, ok := decodeJSONResponse(t, rrUnassigned)["items"].([]any)
	if !ok || len(unassignedItems) != 1 {
		t.Fatalf("unassigned inbox payload = %#v", decodeJSONResponse(t, rrUnassigned))
	}
	if got := strFromAny(unassignedItems[0].(map[string]any)["title"]); got != "Unassigned todoist" {
		t.Fatalf("unassigned title = %q, want %q", got, "Unassigned todoist")
	}

	rrProject := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/inbox?workspace_id="+strconv.FormatInt(workspace.ID, 10), nil)
	if rrProject.Code != http.StatusOK {
		t.Fatalf("project inbox status = %d, want 200: %s", rrProject.Code, rrProject.Body.String())
	}
	projectItems, ok := decodeJSONResponse(t, rrProject)["items"].([]any)
	if !ok || len(projectItems) != 2 {
		t.Fatalf("project inbox payload = %#v", decodeJSONResponse(t, rrProject))
	}
	titles := []string{
		strFromAny(projectItems[0].(map[string]any)["title"]),
		strFromAny(projectItems[1].(map[string]any)["title"]),
	}
	if !(containsString(titles, "Workspace todoist") && containsString(titles, "Workspace exchange")) {
		t.Fatalf("project titles = %#v, want workspace todoist and exchange", titles)
	}

	rrCounts := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/counts?source=todoist", nil)
	if rrCounts.Code != http.StatusOK {
		t.Fatalf("todoist counts status = %d, want 200: %s", rrCounts.Code, rrCounts.Body.String())
	}
	counts, ok := decodeJSONResponse(t, rrCounts)["counts"].(map[string]any)
	if !ok {
		t.Fatalf("todoist counts payload = %#v", decodeJSONResponse(t, rrCounts))
	}
	if got := int(counts[store.ItemStateInbox].(float64)); got != 2 {
		t.Fatalf("todoist counts[inbox] = %d, want 2", got)
	}

	rrList := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items?state=inbox&source=exchange&workspace_id="+itoa(workspace.ID), nil)
	if rrList.Code != http.StatusOK {
		t.Fatalf("filtered item list status = %d, want 200: %s", rrList.Code, rrList.Body.String())
	}
	listItems, ok := decodeJSONDataResponse(t, rrList)["items"].([]any)
	if !ok || len(listItems) != 1 {
		t.Fatalf("filtered item list payload = %#v", decodeJSONDataResponse(t, rrList))
	}
	if got := strFromAny(listItems[0].(map[string]any)["title"]); got != "Workspace exchange" {
		t.Fatalf("filtered item list title = %q, want %q", got, "Workspace exchange")
	}

	rrBadWorkspace := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/inbox?workspace_id=bad", nil)
	if rrBadWorkspace.Code != http.StatusBadRequest {
		t.Fatalf("bad workspace filter status = %d, want 400: %s", rrBadWorkspace.Code, rrBadWorkspace.Body.String())
	}
}

func TestItemStateViewAPIFiltersByContextIncludingDescendants(t *testing.T) {
	app := newAuthedTestApp(t)

	parent, err := app.store.CreateLabel("Work", nil)
	if err != nil {
		t.Fatalf("CreateLabel(parent) error: %v", err)
	}
	child, err := app.store.CreateLabel("W7x", &parent.ID)
	if err != nil {
		t.Fatalf("CreateLabel(child) error: %v", err)
	}
	privateCtx, err := app.store.CreateLabel("Private", nil)
	if err != nil {
		t.Fatalf("CreateLabel(private) error: %v", err)
	}

	workspace, err := app.store.CreateWorkspace("Inbox Workspace", filepath.Join(t.TempDir(), "workspace"))
	if err != nil {
		t.Fatalf("CreateWorkspace() error: %v", err)
	}
	if err := app.store.LinkLabelToWorkspace(child.ID, workspace.ID); err != nil {
		t.Fatalf("LinkLabelToWorkspace() error: %v", err)
	}

	past := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	workItem, err := app.store.CreateItem("Workspace W7x", store.ItemOptions{
		State:        store.ItemStateInbox,
		WorkspaceID:  &workspace.ID,
		VisibleAfter: &past,
	})
	if err != nil {
		t.Fatalf("CreateItem(work item) error: %v", err)
	}
	privateItem, err := app.store.CreateItem("Private inbox", store.ItemOptions{
		State:        store.ItemStateInbox,
		VisibleAfter: &past,
	})
	if err != nil {
		t.Fatalf("CreateItem(private item) error: %v", err)
	}
	if err := app.store.LinkLabelToItem(privateCtx.ID, privateItem.ID); err != nil {
		t.Fatalf("LinkLabelToItem() error: %v", err)
	}
	directChildItem, err := app.store.CreateItem("Direct child", store.ItemOptions{
		State:        store.ItemStateInbox,
		VisibleAfter: &past,
	})
	if err != nil {
		t.Fatalf("CreateItem(direct child) error: %v", err)
	}
	if err := app.store.LinkLabelToItem(child.ID, directChildItem.ID); err != nil {
		t.Fatalf("LinkLabelToItem(child) error: %v", err)
	}

	rrInbox := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/inbox?label_id="+itoa(parent.ID), nil)
	if rrInbox.Code != http.StatusOK {
		t.Fatalf("label inbox status = %d, want 200: %s", rrInbox.Code, rrInbox.Body.String())
	}
	items, ok := decodeJSONResponse(t, rrInbox)["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("label inbox payload = %#v", decodeJSONResponse(t, rrInbox))
	}
	gotTitles := map[string]bool{}
	gotIDs := map[int64]bool{}
	for _, raw := range items {
		entry, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("label inbox entry = %#v", raw)
		}
		gotTitles[strFromAny(entry["title"])] = true
		gotIDs[int64(entry["id"].(float64))] = true
	}
	if !gotTitles["Workspace W7x"] || !gotTitles["Direct child"] {
		t.Fatalf("label inbox titles = %#v, want Workspace W7x and Direct child", gotTitles)
	}
	if !gotIDs[workItem.ID] || !gotIDs[directChildItem.ID] {
		t.Fatalf("label inbox ids = %#v, want %d and %d", gotIDs, workItem.ID, directChildItem.ID)
	}

	rrCounts := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/counts?label_id="+itoa(parent.ID), nil)
	if rrCounts.Code != http.StatusOK {
		t.Fatalf("label counts status = %d, want 200: %s", rrCounts.Code, rrCounts.Body.String())
	}
	counts, ok := decodeJSONResponse(t, rrCounts)["counts"].(map[string]any)
	if !ok {
		t.Fatalf("label counts payload = %#v", decodeJSONResponse(t, rrCounts))
	}
	if got := int(counts[store.ItemStateInbox].(float64)); got != 2 {
		t.Fatalf("label counts[inbox] = %d, want 2", got)
	}

	rrBad := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/inbox?label_id=bad", nil)
	if rrBad.Code != http.StatusBadRequest {
		t.Fatalf("bad label filter status = %d, want 400: %s", rrBad.Code, rrBad.Body.String())
	}
}

func TestItemDomainAPIRejectsConflictAndInvalidState(t *testing.T) {
	app := newAuthedTestApp(t)

	rrConflict := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items", map[string]any{
		"title":        "Bad foreign key",
		"workspace_id": 999999,
	})
	if rrConflict.Code != http.StatusConflict {
		t.Fatalf("create item conflict status = %d, want 409: %s", rrConflict.Code, rrConflict.Body.String())
	}

	item, err := app.store.CreateItem("Stateful item", store.ItemOptions{})
	if err != nil {
		t.Fatalf("CreateItem() error: %v", err)
	}

	rrBadState := doAuthedJSONRequest(t, app.Router(), http.MethodPut, "/api/items/"+itoa(item.ID)+"/state", map[string]any{
		"state": "paused",
	})
	if rrBadState.Code != http.StatusBadRequest {
		t.Fatalf("bad state status = %d, want 400: %s", rrBadState.Code, rrBadState.Body.String())
	}
}

func TestItemAssignmentAPIRejectsMissingActor(t *testing.T) {
	app := newAuthedTestApp(t)

	item, err := app.store.CreateItem("Delegate this", store.ItemOptions{})
	if err != nil {
		t.Fatalf("CreateItem() error: %v", err)
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPut, "/api/items/"+itoa(item.ID)+"/assign", map[string]any{
		"actor_id": 999,
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("assign missing actor status = %d, want 400: %s", rr.Code, rr.Body.String())
	}
}

func TestItemAssignmentAPIRejectsDoneItems(t *testing.T) {
	app := newAuthedTestApp(t)

	actor, err := app.store.CreateActor("Codex", store.ActorKindAgent)
	if err != nil {
		t.Fatalf("CreateActor() error: %v", err)
	}
	item, err := app.store.CreateItem("Done already", store.ItemOptions{
		State:   store.ItemStateWaiting,
		ActorID: &actor.ID,
	})
	if err != nil {
		t.Fatalf("CreateItem() error: %v", err)
	}
	if err := app.store.CompleteItemByActor(item.ID, actor.ID); err != nil {
		t.Fatalf("CompleteItemByActor() error: %v", err)
	}

	rrAssign := doAuthedJSONRequest(t, app.Router(), http.MethodPut, "/api/items/"+itoa(item.ID)+"/assign", map[string]any{
		"actor_id": actor.ID,
	})
	if rrAssign.Code != http.StatusBadRequest {
		t.Fatalf("assign done item status = %d, want 400: %s", rrAssign.Code, rrAssign.Body.String())
	}

	rrUnassign := doAuthedJSONRequest(t, app.Router(), http.MethodPut, "/api/items/"+itoa(item.ID)+"/unassign", map[string]any{})
	if rrUnassign.Code != http.StatusBadRequest {
		t.Fatalf("unassign done item status = %d, want 400: %s", rrUnassign.Code, rrUnassign.Body.String())
	}
}

func TestItemCompletionAPIRejectsWrongActorAndMissingItem(t *testing.T) {
	app := newAuthedTestApp(t)

	owner, err := app.store.CreateActor("Owner", store.ActorKindHuman)
	if err != nil {
		t.Fatalf("CreateActor(owner) error: %v", err)
	}
	other, err := app.store.CreateActor("Other", store.ActorKindAgent)
	if err != nil {
		t.Fatalf("CreateActor(other) error: %v", err)
	}
	item, err := app.store.CreateItem("Assigned task", store.ItemOptions{
		State:   store.ItemStateWaiting,
		ActorID: &owner.ID,
	})
	if err != nil {
		t.Fatalf("CreateItem() error: %v", err)
	}

	rrWrongActor := doAuthedJSONRequest(t, app.Router(), http.MethodPut, "/api/items/"+itoa(item.ID)+"/complete", map[string]any{
		"actor_id": other.ID,
	})
	if rrWrongActor.Code != http.StatusBadRequest {
		t.Fatalf("complete wrong actor status = %d, want 400: %s", rrWrongActor.Code, rrWrongActor.Body.String())
	}

	rrMissingItem := doAuthedJSONRequest(t, app.Router(), http.MethodPut, "/api/items/999999/complete", map[string]any{
		"actor_id": owner.ID,
	})
	if rrMissingItem.Code != http.StatusNotFound {
		t.Fatalf("complete missing item status = %d, want 404: %s", rrMissingItem.Code, rrMissingItem.Body.String())
	}
}

func TestItemTriageAPI(t *testing.T) {
	app := newAuthedTestApp(t)

	actor, err := app.store.CreateActor("Codex", store.ActorKindAgent)
	if err != nil {
		t.Fatalf("CreateActor() error: %v", err)
	}

	doneItem, err := app.store.CreateItem("Done me", store.ItemOptions{})
	if err != nil {
		t.Fatalf("CreateItem(done) error: %v", err)
	}
	rrDone := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(doneItem.ID)+"/triage", map[string]any{
		"action": "done",
	})
	if rrDone.Code != http.StatusOK {
		t.Fatalf("triage done status = %d, want 200: %s", rrDone.Code, rrDone.Body.String())
	}
	gotDone, err := app.store.GetItem(doneItem.ID)
	if err != nil {
		t.Fatalf("GetItem(done) error: %v", err)
	}
	if gotDone.State != store.ItemStateDone {
		t.Fatalf("done state = %q, want %q", gotDone.State, store.ItemStateDone)
	}

	laterItem, err := app.store.CreateItem("Later me", store.ItemOptions{})
	if err != nil {
		t.Fatalf("CreateItem(later) error: %v", err)
	}
	visibleAfter := "2026-03-10T09:00:00Z"
	rrLater := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(laterItem.ID)+"/triage", map[string]any{
		"action":        "later",
		"visible_after": visibleAfter,
	})
	if rrLater.Code != http.StatusOK {
		t.Fatalf("triage later status = %d, want 200: %s", rrLater.Code, rrLater.Body.String())
	}
	gotLater, err := app.store.GetItem(laterItem.ID)
	if err != nil {
		t.Fatalf("GetItem(later) error: %v", err)
	}
	if gotLater.State != store.ItemStateDeferred {
		t.Fatalf("later state = %q, want %q", gotLater.State, store.ItemStateDeferred)
	}
	if gotLater.VisibleAfter == nil || *gotLater.VisibleAfter != visibleAfter {
		t.Fatalf("later visible_after = %v, want %q", gotLater.VisibleAfter, visibleAfter)
	}

	delegateItem, err := app.store.CreateItem("Delegate me", store.ItemOptions{
		VisibleAfter: &visibleAfter,
	})
	if err != nil {
		t.Fatalf("CreateItem(delegate) error: %v", err)
	}
	rrDelegate := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(delegateItem.ID)+"/triage", map[string]any{
		"action":   "delegate",
		"actor_id": actor.ID,
	})
	if rrDelegate.Code != http.StatusOK {
		t.Fatalf("triage delegate status = %d, want 200: %s", rrDelegate.Code, rrDelegate.Body.String())
	}
	gotDelegate, err := app.store.GetItem(delegateItem.ID)
	if err != nil {
		t.Fatalf("GetItem(delegate) error: %v", err)
	}
	if gotDelegate.State != store.ItemStateWaiting {
		t.Fatalf("delegate state = %q, want %q", gotDelegate.State, store.ItemStateWaiting)
	}
	if gotDelegate.ActorID == nil || *gotDelegate.ActorID != actor.ID {
		t.Fatalf("delegate actor = %v, want %d", gotDelegate.ActorID, actor.ID)
	}
	if gotDelegate.VisibleAfter != nil {
		t.Fatalf("delegate visible_after = %v, want nil", gotDelegate.VisibleAfter)
	}

	somedayItem, err := app.store.CreateItem("Someday me", store.ItemOptions{
		VisibleAfter: &visibleAfter,
		FollowUpAt:   &visibleAfter,
	})
	if err != nil {
		t.Fatalf("CreateItem(someday) error: %v", err)
	}
	rrSomeday := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(somedayItem.ID)+"/triage", map[string]any{
		"action": "someday",
	})
	if rrSomeday.Code != http.StatusOK {
		t.Fatalf("triage someday status = %d, want 200: %s", rrSomeday.Code, rrSomeday.Body.String())
	}
	gotSomeday, err := app.store.GetItem(somedayItem.ID)
	if err != nil {
		t.Fatalf("GetItem(someday) error: %v", err)
	}
	if gotSomeday.State != store.ItemStateSomeday {
		t.Fatalf("someday state = %q, want %q", gotSomeday.State, store.ItemStateSomeday)
	}
	if gotSomeday.VisibleAfter != nil || gotSomeday.FollowUpAt != nil {
		t.Fatalf("someday timestamps = visible_after:%v follow_up_at:%v, want nil", gotSomeday.VisibleAfter, gotSomeday.FollowUpAt)
	}

	deleteItem, err := app.store.CreateItem("Delete me", store.ItemOptions{})
	if err != nil {
		t.Fatalf("CreateItem(delete) error: %v", err)
	}
	rrDelete := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(deleteItem.ID)+"/triage", map[string]any{
		"action": "delete",
	})
	if rrDelete.Code != http.StatusOK {
		t.Fatalf("triage delete status = %d, want 200: %s", rrDelete.Code, rrDelete.Body.String())
	}
	if _, err := app.store.GetItem(deleteItem.ID); err == nil {
		t.Fatal("expected deleted item to be gone")
	} else if err != sql.ErrNoRows {
		t.Fatalf("GetItem(deleted) error = %v, want sql.ErrNoRows", err)
	}
}

func TestItemTriageAPIRejectsInvalidRequests(t *testing.T) {
	app := newAuthedTestApp(t)

	actor, err := app.store.CreateActor("Codex", store.ActorKindAgent)
	if err != nil {
		t.Fatalf("CreateActor() error: %v", err)
	}
	doneItem, err := app.store.CreateItem("Already done", store.ItemOptions{})
	if err != nil {
		t.Fatalf("CreateItem(done) error: %v", err)
	}
	if err := app.store.TriageItemDone(doneItem.ID); err != nil {
		t.Fatalf("TriageItemDone() setup error: %v", err)
	}
	inboxItem, err := app.store.CreateItem("Inbox item", store.ItemOptions{})
	if err != nil {
		t.Fatalf("CreateItem(inbox) error: %v", err)
	}

	for _, tc := range []struct {
		name   string
		path   string
		body   map[string]any
		status int
	}{
		{
			name:   "missing visible_after",
			path:   "/api/items/" + itoa(inboxItem.ID) + "/triage",
			body:   map[string]any{"action": "later"},
			status: http.StatusBadRequest,
		},
		{
			name:   "invalid visible_after",
			path:   "/api/items/" + itoa(inboxItem.ID) + "/triage",
			body:   map[string]any{"action": "later", "visible_after": "tomorrow"},
			status: http.StatusBadRequest,
		},
		{
			name:   "missing actor",
			path:   "/api/items/" + itoa(inboxItem.ID) + "/triage",
			body:   map[string]any{"action": "delegate"},
			status: http.StatusBadRequest,
		},
		{
			name:   "unknown actor",
			path:   "/api/items/" + itoa(inboxItem.ID) + "/triage",
			body:   map[string]any{"action": "delegate", "actor_id": actor.ID + 999},
			status: http.StatusBadRequest,
		},
		{
			name:   "unknown action",
			path:   "/api/items/" + itoa(inboxItem.ID) + "/triage",
			body:   map[string]any{"action": "archive"},
			status: http.StatusBadRequest,
		},
		{
			name:   "missing item",
			path:   "/api/items/999999/triage",
			body:   map[string]any{"action": "done"},
			status: http.StatusNotFound,
		},
		{
			name:   "done item",
			path:   "/api/items/" + itoa(doneItem.ID) + "/triage",
			body:   map[string]any{"action": "someday"},
			status: http.StatusBadRequest,
		},
	} {
		rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, tc.path, tc.body)
		if rr.Code != tc.status {
			t.Fatalf("%s status = %d, want %d: %s", tc.name, rr.Code, tc.status, rr.Body.String())
		}
	}
}
