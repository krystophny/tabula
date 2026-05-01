package store

import (
	"testing"
)

func TestRestoreItemFromGestureUndoBypassesForwardOnlyTransition(t *testing.T) {
	s := newTestStore(t)
	item, err := s.CreateItem("Reopen me", ItemOptions{State: ItemStateNext})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	if err := s.UpdateItemState(item.ID, ItemStateDone); err != nil {
		t.Fatalf("UpdateItemState(done): %v", err)
	}
	if err := s.UpdateItemState(item.ID, ItemStateNext); err == nil {
		t.Fatalf("UpdateItemState(done -> next) should reject forward-only transition")
	}
	if err := s.RestoreItemFromGestureUndo(item.ID, ItemGestureUndo{State: ItemStateNext}); err != nil {
		t.Fatalf("RestoreItemFromGestureUndo: %v", err)
	}
	got, err := s.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if got.State != ItemStateNext {
		t.Fatalf("state = %q, want %q", got.State, ItemStateNext)
	}
}

func TestRestoreItemFromGestureUndoRestoresActorAndSchedule(t *testing.T) {
	s := newTestStore(t)
	actor, err := s.CreateActor("Pat", ActorKindHuman)
	if err != nil {
		t.Fatalf("CreateActor: %v", err)
	}
	item, err := s.CreateItem("Schedule revert", ItemOptions{State: ItemStateInbox})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	follow := "2026-05-15T09:00:00Z"
	visible := "2026-05-15T09:00:00Z"
	if err := s.UpdateItem(item.ID, ItemUpdate{
		State:        stringPtr(ItemStateWaiting),
		ActorID:      int64Ptr(actor.ID),
		FollowUpAt:   stringPtr(follow),
		VisibleAfter: stringPtr(visible),
	}); err != nil {
		t.Fatalf("UpdateItem: %v", err)
	}

	prevActor := actor.ID
	if err := s.RestoreItemFromGestureUndo(item.ID, ItemGestureUndo{
		State:        ItemStateInbox,
		ActorID:      &prevActor,
		FollowUpAt:   stringPtr(""),
		VisibleAfter: stringPtr(""),
	}); err != nil {
		t.Fatalf("RestoreItemFromGestureUndo: %v", err)
	}
	got, err := s.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if got.State != ItemStateInbox {
		t.Fatalf("state = %q, want %q", got.State, ItemStateInbox)
	}
	if got.ActorID == nil || *got.ActorID != prevActor {
		t.Fatalf("actor_id = %v, want %d", got.ActorID, prevActor)
	}
	if got.FollowUpAt != nil && *got.FollowUpAt != "" {
		t.Fatalf("follow_up_at = %v, want cleared", got.FollowUpAt)
	}
	if got.VisibleAfter != nil && *got.VisibleAfter != "" {
		t.Fatalf("visible_after = %v, want cleared", got.VisibleAfter)
	}
}

func TestRestoreItemFromGestureUndoRequiresState(t *testing.T) {
	s := newTestStore(t)
	item, err := s.CreateItem("Empty undo", ItemOptions{})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	if err := s.RestoreItemFromGestureUndo(item.ID, ItemGestureUndo{}); err == nil {
		t.Fatalf("expected error when state is missing")
	}
}

func TestRestoreItemFromGestureUndoUnknownItem(t *testing.T) {
	s := newTestStore(t)
	if err := s.RestoreItemFromGestureUndo(999_999, ItemGestureUndo{State: ItemStateInbox}); err == nil {
		t.Fatalf("expected error for unknown item")
	}
}

func stringPtr(s string) *string { return &s }
func int64Ptr(v int64) *int64    { return &v }
