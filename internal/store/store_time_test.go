package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestTimeEntrySwitchAndSummaryLifecycle(t *testing.T) {
	s := newTestStore(t)

	workspace, err := s.CreateWorkspace("Tabura", filepath.Join(t.TempDir(), "tabura"), SphereWork)
	if err != nil {
		t.Fatalf("CreateWorkspace() error: %v", err)
	}
	project, err := s.CreateProject("Tabura", "tabura", filepath.Join(t.TempDir(), "project"), "managed", "", "", false)
	if err != nil {
		t.Fatalf("CreateProject() error: %v", err)
	}

	start := time.Date(2026, 3, 9, 8, 0, 0, 0, time.UTC)
	middle := start.Add(90 * time.Minute)
	end := middle.Add(30 * time.Minute)

	first, changed, err := s.SwitchActiveTimeEntry(start, &workspace.ID, &project.ID, SphereWork, "workspace_switch", nil)
	if err != nil {
		t.Fatalf("SwitchActiveTimeEntry(first) error: %v", err)
	}
	if !changed {
		t.Fatal("expected first switch to create an entry")
	}
	second, changed, err := s.SwitchActiveTimeEntry(middle, nil, &project.ID, SphereWork, "project_switch", nil)
	if err != nil {
		t.Fatalf("SwitchActiveTimeEntry(second) error: %v", err)
	}
	if !changed {
		t.Fatal("expected second switch to create a new entry")
	}
	if _, changed, err := s.SwitchActiveTimeEntry(middle.Add(10*time.Minute), nil, &project.ID, SphereWork, "project_switch", nil); err != nil {
		t.Fatalf("SwitchActiveTimeEntry(no-op) error: %v", err)
	} else if changed {
		t.Fatal("expected identical context switch to be a no-op")
	}
	if stopped, err := s.StopActiveTimeEntries(end); err != nil {
		t.Fatalf("StopActiveTimeEntries() error: %v", err)
	} else if stopped != 1 {
		t.Fatalf("StopActiveTimeEntries() = %d, want 1", stopped)
	}

	entries, err := s.ListTimeEntries(TimeEntryListFilter{})
	if err != nil {
		t.Fatalf("ListTimeEntries() error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("ListTimeEntries() len = %d, want 2", len(entries))
	}
	if entries[0].ID != first.ID {
		t.Fatalf("first entry id = %d, want %d", entries[0].ID, first.ID)
	}
	if entries[0].EndedAt == nil || *entries[0].EndedAt != middle.Format(time.RFC3339) {
		t.Fatalf("first entry ended_at = %v, want %s", entries[0].EndedAt, middle.Format(time.RFC3339))
	}
	if entries[1].ID != second.ID {
		t.Fatalf("second entry id = %d, want %d", entries[1].ID, second.ID)
	}
	if entries[1].EndedAt == nil || *entries[1].EndedAt != end.Format(time.RFC3339) {
		t.Fatalf("second entry ended_at = %v, want %s", entries[1].EndedAt, end.Format(time.RFC3339))
	}

	projectSummary, err := s.SummarizeTimeEntries(TimeEntryListFilter{
		From: &start,
		To:   &end,
	}, "project", end)
	if err != nil {
		t.Fatalf("SummarizeTimeEntries(project) error: %v", err)
	}
	if len(projectSummary) != 1 {
		t.Fatalf("project summary len = %d, want 1", len(projectSummary))
	}
	if got := projectSummary[0].Label; got != project.Name {
		t.Fatalf("project summary label = %q, want %q", got, project.Name)
	}
	if got := projectSummary[0].Seconds; got != 2*60*60 {
		t.Fatalf("project summary seconds = %d, want %d", got, 2*60*60)
	}
	if got := projectSummary[0].Duration; got != "2h" {
		t.Fatalf("project summary duration = %q, want %q", got, "2h")
	}

	workspaceSummary, err := s.SummarizeTimeEntries(TimeEntryListFilter{
		From: &start,
		To:   &end,
	}, "workspace", end)
	if err != nil {
		t.Fatalf("SummarizeTimeEntries(workspace) error: %v", err)
	}
	if len(workspaceSummary) != 2 {
		t.Fatalf("workspace summary len = %d, want 2", len(workspaceSummary))
	}
	if got := workspaceSummary[0].Label; got != workspace.Name {
		t.Fatalf("workspace summary[0] label = %q, want %q", got, workspace.Name)
	}
	if got := workspaceSummary[0].Seconds; got != 90*60 {
		t.Fatalf("workspace summary[0] seconds = %d, want %d", got, 90*60)
	}
	if got := workspaceSummary[1].Label; got != "No workspace" {
		t.Fatalf("workspace summary[1] label = %q, want %q", got, "No workspace")
	}
	if got := workspaceSummary[1].Seconds; got != 30*60 {
		t.Fatalf("workspace summary[1] seconds = %d, want %d", got, 30*60)
	}
}

func TestActiveWorkspaceReturnsCurrentSelection(t *testing.T) {
	s := newTestStore(t)

	alpha, err := s.CreateWorkspace("Alpha", filepath.Join(t.TempDir(), "alpha"), SpherePrivate)
	if err != nil {
		t.Fatalf("CreateWorkspace(alpha) error: %v", err)
	}
	beta, err := s.CreateWorkspace("Beta", filepath.Join(t.TempDir(), "beta"), SphereWork)
	if err != nil {
		t.Fatalf("CreateWorkspace(beta) error: %v", err)
	}
	if err := s.SetActiveWorkspace(beta.ID); err != nil {
		t.Fatalf("SetActiveWorkspace(beta) error: %v", err)
	}

	active, err := s.ActiveWorkspace()
	if err != nil {
		t.Fatalf("ActiveWorkspace() error: %v", err)
	}
	if active.ID != beta.ID {
		t.Fatalf("ActiveWorkspace() = %d, want %d", active.ID, beta.ID)
	}

	if err := s.SetActiveWorkspace(alpha.ID); err != nil {
		t.Fatalf("SetActiveWorkspace(alpha) error: %v", err)
	}
	active, err = s.ActiveWorkspace()
	if err != nil {
		t.Fatalf("ActiveWorkspace() second error: %v", err)
	}
	if active.ID != alpha.ID {
		t.Fatalf("ActiveWorkspace() second = %d, want %d", active.ID, alpha.ID)
	}
}
