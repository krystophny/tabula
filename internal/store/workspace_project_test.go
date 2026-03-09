package store

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestWorkspaceProjectInferenceFromPathAndRemote(t *testing.T) {
	s := newTestStore(t)

	project, err := s.CreateProject("EUROfusion", "project-eurofusion", filepath.Join(t.TempDir(), "projects", "eurofusion"), "managed", "", "", false)
	if err != nil {
		t.Fatalf("CreateProject() error: %v", err)
	}

	pathWorkspaceDir := filepath.Join(t.TempDir(), "write", "eurofusion-proposal")
	if err := os.MkdirAll(pathWorkspaceDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(path workspace) error: %v", err)
	}
	pathWorkspace, err := s.CreateWorkspace("Proposal", pathWorkspaceDir, SphereWork)
	if err != nil {
		t.Fatalf("CreateWorkspace(path) error: %v", err)
	}
	if pathWorkspace.ProjectID == nil || *pathWorkspace.ProjectID != project.ID {
		t.Fatalf("path workspace project_id = %v, want %q", pathWorkspace.ProjectID, project.ID)
	}

	repoDir := filepath.Join(t.TempDir(), "code", "solver")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(repoDir) error: %v", err)
	}
	if err := exec.Command("git", "init", repoDir).Run(); err != nil {
		t.Fatalf("git init %s: %v", repoDir, err)
	}
	if err := exec.Command("git", "-C", repoDir, "remote", "add", "origin", "git@github.com:EUROfusion/solver.git").Run(); err != nil {
		t.Fatalf("git remote add origin %s: %v", repoDir, err)
	}
	repoWorkspace, err := s.CreateWorkspace("Solver", repoDir, SphereWork)
	if err != nil {
		t.Fatalf("CreateWorkspace(repo) error: %v", err)
	}
	if repoWorkspace.ProjectID == nil || *repoWorkspace.ProjectID != project.ID {
		t.Fatalf("repo workspace project_id = %v, want %q", repoWorkspace.ProjectID, project.ID)
	}
}

func TestWorkspaceProjectInferenceSkipsAmbiguousMatches(t *testing.T) {
	s := newTestStore(t)

	for _, name := range []string{"Alpha", "Beta"} {
		if _, err := s.CreateProject(name, "project-"+name, filepath.Join(t.TempDir(), name), "managed", "", "", false); err != nil {
			t.Fatalf("CreateProject(%s) error: %v", name, err)
		}
	}

	workspaceDir := filepath.Join(t.TempDir(), "alpha-beta")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(workspaceDir) error: %v", err)
	}
	workspace, err := s.CreateWorkspace("Ambiguous", workspaceDir, SpherePrivate)
	if err != nil {
		t.Fatalf("CreateWorkspace() error: %v", err)
	}
	if workspace.ProjectID != nil {
		t.Fatalf("workspace project_id = %v, want nil", workspace.ProjectID)
	}
}

func TestProjectAwareItemFilteringUsesWorkspaceProjectFallback(t *testing.T) {
	s := newTestStore(t)

	project, err := s.CreateProject("Tabura", "project-tabura", filepath.Join(t.TempDir(), "projects", "tabura"), "managed", "", "", false)
	if err != nil {
		t.Fatalf("CreateProject() error: %v", err)
	}
	workspaceDir := filepath.Join(t.TempDir(), "code", "tabura")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(workspaceDir) error: %v", err)
	}
	workspace, err := s.CreateWorkspace("Tabura", workspaceDir, SphereWork)
	if err != nil {
		t.Fatalf("CreateWorkspace() error: %v", err)
	}
	if _, err := s.SetWorkspaceProject(workspace.ID, &project.ID); err != nil {
		t.Fatalf("SetWorkspaceProject() error: %v", err)
	}

	item, err := s.CreateItem("Review parser", ItemOptions{WorkspaceID: &workspace.ID})
	if err != nil {
		t.Fatalf("CreateItem() error: %v", err)
	}
	if err := s.SetItemProject(item.ID, nil); err != nil {
		t.Fatalf("SetItemProject(nil) error: %v", err)
	}

	filtered, err := s.ListItemsFiltered(ItemListFilter{ProjectID: &project.ID})
	if err != nil {
		t.Fatalf("ListItemsFiltered() error: %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != item.ID {
		t.Fatalf("filtered items = %#v, want item %d", filtered, item.ID)
	}
}
