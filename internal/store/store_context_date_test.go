package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestEnsureDateContextHierarchyRepairsParentChain(t *testing.T) {
	s := newTestStore(t)

	year, err := s.CreateContext("2026", nil)
	if err != nil {
		t.Fatalf("CreateContext(year) error: %v", err)
	}
	month, err := s.CreateContext("2026/03", nil)
	if err != nil {
		t.Fatalf("CreateContext(month) error: %v", err)
	}
	day, err := s.CreateContext("2026/03/11", nil)
	if err != nil {
		t.Fatalf("CreateContext(day) error: %v", err)
	}

	dayID, err := s.EnsureDateContextHierarchy(time.Date(2026, time.March, 11, 16, 4, 0, 0, time.FixedZone("CET", 3600)))
	if err != nil {
		t.Fatalf("EnsureDateContextHierarchy() error: %v", err)
	}
	if dayID != day.ID {
		t.Fatalf("EnsureDateContextHierarchy() = %d, want existing day context %d", dayID, day.ID)
	}

	updatedYear := mustGetContextByName(t, s, "2026")
	updatedMonth := mustGetContextByName(t, s, "2026/03")
	updatedDay := mustGetContextByName(t, s, "2026/03/11")
	if updatedYear.ParentID != nil {
		t.Fatalf("year parent_id = %v, want nil", updatedYear.ParentID)
	}
	if updatedMonth.ParentID == nil || *updatedMonth.ParentID != year.ID {
		t.Fatalf("month parent_id = %v, want %d", updatedMonth.ParentID, year.ID)
	}
	if updatedDay.ParentID == nil || *updatedDay.ParentID != month.ID {
		t.Fatalf("day parent_id = %v, want %d", updatedDay.ParentID, month.ID)
	}
}

func TestEnsureDailyWorkspaceLinksDateContextHierarchy(t *testing.T) {
	s := newTestStore(t)

	dirPath := filepath.Join(t.TempDir(), "daily", "2026", "03", "11")
	workspace, err := s.EnsureDailyWorkspace("2026-03-11", dirPath)
	if err != nil {
		t.Fatalf("EnsureDailyWorkspace() error: %v", err)
	}

	year := mustGetContextByName(t, s, "2026")
	month := mustGetContextByName(t, s, "2026/03")
	day := mustGetContextByName(t, s, "2026/03/11")
	if month.ParentID == nil || *month.ParentID != year.ID {
		t.Fatalf("month parent_id = %v, want %d", month.ParentID, year.ID)
	}
	if day.ParentID == nil || *day.ParentID != month.ID {
		t.Fatalf("day parent_id = %v, want %d", day.ParentID, month.ID)
	}
	if !hasContextLink(t, s, "context_workspaces", "workspace_id", workspace.ID, day.ID) {
		t.Fatalf("workspace %d missing date context link %d", workspace.ID, day.ID)
	}

	workspaces, err := s.ListWorkspacesByContextPrefix("2026/03")
	if err != nil {
		t.Fatalf("ListWorkspacesByContextPrefix() error: %v", err)
	}
	if len(workspaces) != 1 || workspaces[0].ID != workspace.ID {
		t.Fatalf("ListWorkspacesByContextPrefix(2026/03) = %+v, want workspace %d", workspaces, workspace.ID)
	}
}

func TestDailyWorkspaceItemsInheritAndRefreshDateContexts(t *testing.T) {
	s := newTestStore(t)

	marchWorkspace, err := s.EnsureDailyWorkspace("2026-03-11", filepath.Join(t.TempDir(), "daily", "2026", "03", "11"))
	if err != nil {
		t.Fatalf("EnsureDailyWorkspace(march) error: %v", err)
	}
	aprilWorkspace, err := s.EnsureDailyWorkspace("2026-04-01", filepath.Join(t.TempDir(), "daily", "2026", "04", "01"))
	if err != nil {
		t.Fatalf("EnsureDailyWorkspace(april) error: %v", err)
	}

	past := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	item, err := s.CreateItem("Daily item", ItemOptions{
		State:        ItemStateInbox,
		WorkspaceID:  &marchWorkspace.ID,
		VisibleAfter: &past,
	})
	if err != nil {
		t.Fatalf("CreateItem() error: %v", err)
	}

	marchItems, err := s.ListItemsByContextPrefix("2026/03/11")
	if err != nil {
		t.Fatalf("ListItemsByContextPrefix(march) error: %v", err)
	}
	if len(marchItems) != 1 || marchItems[0].ID != item.ID {
		t.Fatalf("ListItemsByContextPrefix(2026/03/11) = %+v, want item %d", marchItems, item.ID)
	}

	if err := s.SetItemWorkspace(item.ID, &aprilWorkspace.ID); err != nil {
		t.Fatalf("SetItemWorkspace(april) error: %v", err)
	}

	marchItems, err = s.ListItemsByContextPrefix("2026/03/11")
	if err != nil {
		t.Fatalf("ListItemsByContextPrefix(march after move) error: %v", err)
	}
	if len(marchItems) != 0 {
		t.Fatalf("ListItemsByContextPrefix(2026/03/11) len = %d, want 0 after move", len(marchItems))
	}

	aprilItems, err := s.ListItemsByContextPrefix("2026/04")
	if err != nil {
		t.Fatalf("ListItemsByContextPrefix(april) error: %v", err)
	}
	if len(aprilItems) != 1 || aprilItems[0].ID != item.ID {
		t.Fatalf("ListItemsByContextPrefix(2026/04) = %+v, want item %d", aprilItems, item.ID)
	}

	if err := s.SetItemWorkspace(item.ID, nil); err != nil {
		t.Fatalf("SetItemWorkspace(nil) error: %v", err)
	}
	aprilItems, err = s.ListItemsByContextPrefix("2026/04")
	if err != nil {
		t.Fatalf("ListItemsByContextPrefix(april after clear) error: %v", err)
	}
	if len(aprilItems) != 0 {
		t.Fatalf("ListItemsByContextPrefix(2026/04) len = %d, want 0 after clear", len(aprilItems))
	}
}

func mustGetContextByName(t *testing.T, s *Store, name string) Context {
	t.Helper()
	contextID := contextIDByNameForTest(t, s, name)
	context, err := s.GetContext(contextID)
	if err != nil {
		t.Fatalf("GetContext(%q) error: %v", name, err)
	}
	return context
}

func hasContextLink(t *testing.T, s *Store, linkTable, entityColumn string, entityID, contextID int64) bool {
	t.Helper()
	var count int
	query := `SELECT COUNT(1) FROM ` + linkTable + ` WHERE ` + entityColumn + ` = ? AND context_id = ?`
	if err := s.db.QueryRow(query, entityID, contextID).Scan(&count); err != nil {
		t.Fatalf("count context link %s error: %v", linkTable, err)
	}
	return count > 0
}
