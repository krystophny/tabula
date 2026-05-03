package store

import (
	"testing"
	"time"
)

func TestOverdueNextItemIgnoresFutureStartGate(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().UTC()
	future := now.Add(48 * time.Hour).Format(time.RFC3339)
	overdue := now.Add(-24 * time.Hour).Format(time.RFC3339)

	item, err := s.CreateItem("Overdue scheduled action", ItemOptions{
		State:        ItemStateNext,
		VisibleAfter: &future,
		FollowUpAt:   &future,
		DueAt:        &overdue,
	})
	if err != nil {
		t.Fatalf("CreateItem() error: %v", err)
	}

	items, err := s.ListNextItemsFiltered(ItemListFilter{})
	if err != nil {
		t.Fatalf("ListNextItemsFiltered() error: %v", err)
	}
	if len(items) != 1 || items[0].ID != item.ID {
		t.Fatalf("next items = %+v, want overdue item %d", items, item.ID)
	}

	counts, err := s.CountItemsByStateFiltered(now, ItemListFilter{})
	if err != nil {
		t.Fatalf("CountItemsByStateFiltered() error: %v", err)
	}
	if counts[ItemStateNext] != 1 {
		t.Fatalf("next count = %d, want 1", counts[ItemStateNext])
	}
}

func TestFutureStartedNextItemStaysHiddenUntilStart(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().UTC()
	future := now.Add(48 * time.Hour).Format(time.RFC3339)

	if _, err := s.CreateItem("Scheduled action", ItemOptions{
		State:        ItemStateNext,
		VisibleAfter: &future,
		FollowUpAt:   &future,
	}); err != nil {
		t.Fatalf("CreateItem() error: %v", err)
	}

	items, err := s.ListNextItemsFiltered(ItemListFilter{})
	if err != nil {
		t.Fatalf("ListNextItemsFiltered() error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("next items len = %d, want 0", len(items))
	}

	counts, err := s.CountItemsByStateFiltered(now, ItemListFilter{})
	if err != nil {
		t.Fatalf("CountItemsByStateFiltered() error: %v", err)
	}
	if counts[ItemStateNext] != 0 {
		t.Fatalf("next count = %d, want 0", counts[ItemStateNext])
	}
}

func TestActiveProjectCountRequiresActionOrDeadlinePressure(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().UTC()
	future := now.Add(48 * time.Hour).Format(time.RFC3339)
	soon := now.Add(48 * time.Hour).Format(time.RFC3339)

	stalled, err := s.CreateItem("Project without action", ItemOptions{
		Kind:  ItemKindProject,
		State: ItemStateNext,
	})
	if err != nil {
		t.Fatalf("CreateItem(stalled) error: %v", err)
	}
	scheduled, err := s.CreateItem("Project with scheduled action", ItemOptions{
		Kind:  ItemKindProject,
		State: ItemStateNext,
	})
	if err != nil {
		t.Fatalf("CreateItem(scheduled) error: %v", err)
	}
	scheduledChild, err := s.CreateItem("Scheduled child", ItemOptions{
		State:        ItemStateNext,
		VisibleAfter: &future,
	})
	if err != nil {
		t.Fatalf("CreateItem(scheduled child) error: %v", err)
	}
	if err := s.LinkItemChild(scheduled.ID, scheduledChild.ID, ItemLinkRoleNextAction); err != nil {
		t.Fatalf("LinkItemChild(scheduled) error: %v", err)
	}
	active, err := s.CreateItem("Project with next action", ItemOptions{
		Kind:  ItemKindProject,
		State: ItemStateNext,
	})
	if err != nil {
		t.Fatalf("CreateItem(active) error: %v", err)
	}
	activeChild, err := s.CreateItem("Visible child", ItemOptions{State: ItemStateNext})
	if err != nil {
		t.Fatalf("CreateItem(active child) error: %v", err)
	}
	if err := s.LinkItemChild(active.ID, activeChild.ID, ItemLinkRoleNextAction); err != nil {
		t.Fatalf("LinkItemChild(active) error: %v", err)
	}
	deadlined, err := s.CreateItem("Project with imminent deadline", ItemOptions{
		Kind:  ItemKindProject,
		State: ItemStateNext,
	})
	if err != nil {
		t.Fatalf("CreateItem(deadlined) error: %v", err)
	}
	deadlineChild, err := s.CreateItem("Deadline child", ItemOptions{
		State:        ItemStateNext,
		VisibleAfter: &future,
		DueAt:        &soon,
	})
	if err != nil {
		t.Fatalf("CreateItem(deadline child) error: %v", err)
	}
	if err := s.LinkItemChild(deadlined.ID, deadlineChild.ID, ItemLinkRoleNextAction); err != nil {
		t.Fatalf("LinkItemChild(deadlined) error: %v", err)
	}

	got, err := s.CountSidebarSectionsFiltered(now, ItemListFilter{})
	if err != nil {
		t.Fatalf("CountSidebarSectionsFiltered() error: %v", err)
	}
	if got.ProjectItemsOpen != 2 {
		t.Fatalf("ProjectItemsOpen = %d, want 2 active projects", got.ProjectItemsOpen)
	}
	if stalled.ID == scheduled.ID {
		t.Fatalf("fixture sanity failed: distinct projects share id")
	}
}
