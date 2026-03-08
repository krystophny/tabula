package web

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/krystophny/tabura/internal/store"
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
