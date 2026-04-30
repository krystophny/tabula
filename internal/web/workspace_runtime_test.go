package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sloppy-org/slopshell/internal/store"
)

type workspacesListResponse struct {
	OK                 bool                 `json:"ok"`
	DefaultWorkspaceID string               `json:"default_workspace_id"`
	ActiveWorkspaceID  string               `json:"active_workspace_id"`
	Projects           []workspaceListEntry `json:"workspaces"`
}

type workspaceListEntry struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Kind            string `json:"kind"`
	Sphere          string `json:"sphere"`
	WorkspacePath   string `json:"workspace_path"`
	ChatSessionID   string `json:"chat_session_id"`
	ChatModel       string `json:"chat_model"`
	ReasoningEffort string `json:"chat_model_reasoning_effort"`
	CanvasSessionID string `json:"canvas_session_id"`
	Unread          bool   `json:"unread"`
	ReviewPending   bool   `json:"review_pending"`
	RunState        struct {
		ActiveTurns  int    `json:"active_turns"`
		QueuedTurns  int    `json:"queued_turns"`
		IsWorking    bool   `json:"is_working"`
		Status       string `json:"status"`
		ActiveTurnID string `json:"active_turn_id"`
	} `json:"run_state"`
}

type workspacesActivityResponse struct {
	OK       bool `json:"ok"`
	Projects []struct {
		WorkspaceID   string `json:"workspace_id"`
		ChatSessionID string `json:"chat_session_id"`
		ChatMode      string `json:"chat_mode"`
		Unread        bool   `json:"unread"`
		ReviewPending bool   `json:"review_pending"`
		RunState      struct {
			ActiveTurns int    `json:"active_turns"`
			QueuedTurns int    `json:"queued_turns"`
			IsWorking   bool   `json:"is_working"`
			Status      string `json:"status"`
		} `json:"run_state"`
	} `json:"workspaces"`
}

type workspaceFilesListResponse struct {
	OK          bool   `json:"ok"`
	WorkspaceID int64  `json:"workspace_id"`
	Path        string `json:"path"`
	IsRoot      bool   `json:"is_root"`
	Entries     []struct {
		Name  string `json:"name"`
		Path  string `json:"path"`
		IsDir bool   `json:"is_dir"`
	} `json:"entries"`
}

func configureWorkPersonalGuardrail(t *testing.T) (string, string) {
	t.Helper()
	vaultRoot := t.TempDir()
	brainRoot := filepath.Join(vaultRoot, "brain")
	personalRoot := filepath.Join(vaultRoot, "personal")
	if err := os.MkdirAll(brainRoot, 0o755); err != nil {
		t.Fatalf("mkdir brain root: %v", err)
	}
	if err := os.MkdirAll(personalRoot, 0o755); err != nil {
		t.Fatalf("mkdir personal root: %v", err)
	}
	t.Setenv("SLOPSHELL_BRAIN_WORK_ROOT", brainRoot)
	return vaultRoot, personalRoot
}

func TestProjectsListIncludesActiveAndSessions(t *testing.T) {
	app := newAuthedTestApp(t)

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/runtime/workspaces", map[string]any{})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var payload workspacesListResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.OK {
		t.Fatalf("expected ok=true")
	}
	if payload.DefaultWorkspaceID == "" {
		t.Fatalf("expected default workspace id")
	}
	if payload.ActiveWorkspaceID == "" {
		t.Fatalf("expected active workspace id")
	}
	if len(payload.Projects) == 0 {
		t.Fatalf("expected at least one workspace")
	}
	first := payload.Projects[0]
	if first.ChatSessionID == "" {
		t.Fatalf("expected workspace chat session id")
	}
	if first.CanvasSessionID == "" {
		t.Fatalf("expected workspace canvas session id")
	}
	if first.ChatModel == "" {
		t.Fatalf("expected workspace chat model")
	}
	if first.RunState.Status == "" {
		t.Fatalf("expected workspace run state status")
	}
}

func TestNewAppPrefersLocalProjectWorkspaceOnStartup(t *testing.T) {
	dataDir := t.TempDir()
	localProjectDir := filepath.Join(t.TempDir(), "tabula")
	if err := os.MkdirAll(localProjectDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(localProjectDir) error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localProjectDir, "go.mod"), []byte("module github.com/sloppy-org/slopshell\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(go.mod) error: %v", err)
	}

	app, err := New(dataDir, localProjectDir, "", "", "", "", "", false)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	t.Cleanup(func() {
		_ = app.Shutdown(context.Background())
	})

	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensureDefaultWorkspace() error: %v", err)
	}
	if project.Name != "Slopshell" {
		t.Fatalf("default project name = %q, want %q", project.Name, "Slopshell")
	}
	if err := app.ensureStartupProjectWithWorkspace(); err != nil {
		t.Fatalf("ensureStartupProjectWithWorkspace() error: %v", err)
	}
	workspace, err := app.store.ActiveWorkspace()
	if err != nil {
		t.Fatalf("ActiveWorkspace() error: %v", err)
	}
	if workspace.IsDaily {
		t.Fatal("active workspace is_daily = true, want false for local workspace startup")
	}
	if workspace.DirPath != localProjectDir {
		t.Fatalf("active workspace dir_path = %q, want %q", workspace.DirPath, localProjectDir)
	}
	activeWorkspaceID, err := app.store.ActiveWorkspaceID()
	if err != nil {
		t.Fatalf("ActiveWorkspaceID() error: %v", err)
	}
	if activeWorkspaceID != workspaceIDStr(project.ID) {
		t.Fatalf("active project id = %q, want %q", activeWorkspaceID, workspaceIDStr(project.ID))
	}
	workspace, err = app.resolveChatSessionTarget("", nil)
	if err != nil {
		t.Fatalf("resolveChatSessionTarget() error: %v", err)
	}
	if workspace.DirPath != localProjectDir {
		t.Fatalf("resolved workspace dir_path = %q, want %q", workspace.DirPath, localProjectDir)
	}
}

func TestStartupWorkspacePrefersBrainPresetOverLocalProject(t *testing.T) {
	rootDir := t.TempDir()
	dataDir := filepath.Join(rootDir, "data")
	localProjectDir := filepath.Join(rootDir, "tabula")
	workVault := filepath.Join(rootDir, "work-vault")
	brainRoot := filepath.Join(workVault, "brain")
	if err := os.MkdirAll(localProjectDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(localProjectDir) error: %v", err)
	}
	if err := os.MkdirAll(brainRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(brainRoot) error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localProjectDir, "go.mod"), []byte("module github.com/sloppy-org/slopshell\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(go.mod) error: %v", err)
	}
	configPath := filepath.Join(rootDir, "vaults.toml")
	config := `[[vault]]
sphere = "work"
root = "` + workVault + `"
brain = "brain"
`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("WriteFile(vault config) error: %v", err)
	}
	t.Setenv("SLOPTOOLS_VAULT_CONFIG", configPath)

	app, err := New(dataDir, localProjectDir, "", "", "", "", "", false)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if err := app.store.AddAuthSession(testAuthToken); err != nil {
		t.Fatalf("add auth session: %v", err)
	}
	t.Cleanup(func() {
		_ = app.Shutdown(context.Background())
	})

	startupWorkspace, err := app.ensureStartupWorkspace()
	if err != nil {
		t.Fatalf("ensureStartupWorkspace() error: %v", err)
	}
	if startupWorkspace.DirPath != brainRoot {
		t.Fatalf("startup dir_path = %q, want %q", startupWorkspace.DirPath, brainRoot)
	}
	if startupWorkspace.Name != "Work brain" {
		t.Fatalf("startup name = %q, want %q", startupWorkspace.Name, "Work brain")
	}
}

func TestProjectAPIModelIncludesWorkspaceChatSession(t *testing.T) {
	app := newAuthedTestApp(t)

	defaultProject, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensure default project: %v", err)
	}
	if _, err := app.ensureWorkspaceReady(defaultProject, false); err != nil {
		t.Fatalf("ensure workspace for default project: %v", err)
	}
	defaultItem, err := app.buildWorkspaceAPIModel(defaultProject)
	if err != nil {
		t.Fatalf("build default project API model: %v", err)
	}
	if defaultItem.ChatSessionID == "" {
		t.Fatalf("expected project chat session id")
	}
}

func TestProjectsListIncludesWorkspaceSphere(t *testing.T) {
	app := newAuthedTestApp(t)

	workspaceRoot := filepath.Join(t.TempDir(), "work-root")
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(workspaceRoot) error: %v", err)
	}
	if _, err := app.store.CreateWorkspace("Work Root", workspaceRoot, store.SphereWork); err != nil {
		t.Fatalf("CreateWorkspace() error: %v", err)
	}
	projectPath := filepath.Join(workspaceRoot, "linked-project")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(projectPath) error: %v", err)
	}

	rrCreate := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/runtime/workspaces", map[string]any{
		"name": "linked-work-project",
		"kind": "linked",
		"path": projectPath,
	})
	if rrCreate.Code != http.StatusOK {
		t.Fatalf("create project status = %d, want 200: %s", rrCreate.Code, rrCreate.Body.String())
	}

	rrList := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/runtime/workspaces", map[string]any{})
	if rrList.Code != http.StatusOK {
		t.Fatalf("list projects status = %d, want 200: %s", rrList.Code, rrList.Body.String())
	}
	var payload workspacesListResponse
	if err := json.Unmarshal(rrList.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	project := findWorkspaceByName(payload.Projects, "linked-work-project")
	if project == nil {
		t.Fatalf("linked project not found in payload: %#v", payload.Projects)
	}
	if project.Sphere != store.SphereWork {
		t.Fatalf("project sphere = %q, want %q", project.Sphere, store.SphereWork)
	}
}

func TestRuntimeWorkspaceCreateRejectsWorkPersonalPath(t *testing.T) {
	_, personalRoot := configureWorkPersonalGuardrail(t)
	app := newAuthedTestApp(t)

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/runtime/workspaces", map[string]any{
		"name": "Personal",
		"kind": "linked",
		"path": personalRoot,
	})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("create personal workspace status = %d, want 403: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "work personal subtree is blocked") {
		t.Fatalf("guardrail response = %q", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), personalRoot) {
		t.Fatalf("guardrail response leaked protected path: %q", rr.Body.String())
	}
	if _, err := app.store.GetWorkspaceByPath(personalRoot); !isNoRows(err) {
		t.Fatalf("GetWorkspaceByPath(personal) error = %v, want no rows", err)
	}
}

func TestProjectsListPrefersLastUsedWorkspaceProject(t *testing.T) {
	app := newAuthedTestApp(t)

	alphaPath := filepath.Join(t.TempDir(), "alpha")
	betaPath := filepath.Join(t.TempDir(), "beta")
	if err := os.MkdirAll(alphaPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(alpha) error: %v", err)
	}
	if err := os.MkdirAll(betaPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(beta) error: %v", err)
	}
	alphaProject, _, err := app.createWorkspace2(runtimeWorkspaceCreateRequest{Name: "Alpha", Kind: "linked", Path: alphaPath})
	if err != nil {
		t.Fatalf("createWorkspace2(alpha) error: %v", err)
	}
	betaProject, _, err := app.createWorkspace2(runtimeWorkspaceCreateRequest{Name: "Beta", Kind: "linked", Path: betaPath})
	if err != nil {
		t.Fatalf("createWorkspace2(beta) error: %v", err)
	}
	alphaWorkspace, err := app.ensureWorkspaceReady(alphaProject, false)
	if err != nil {
		t.Fatalf("ensureWorkspaceReady(alpha) error: %v", err)
	}
	betaWorkspace, err := app.ensureWorkspaceReady(betaProject, false)
	if err != nil {
		t.Fatalf("ensureWorkspaceReady(beta) error: %v", err)
	}
	if _, err := app.store.SetWorkspaceSphere(betaWorkspace.ID, store.SphereWork); err != nil {
		t.Fatalf("SetWorkspaceSphere(beta) error: %v", err)
	}
	defaultProject, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensureDefaultWorkspace() error: %v", err)
	}
	if err := app.store.SetActiveWorkspaceID(workspaceIDStr(defaultProject.ID)); err != nil {
		t.Fatalf("SetActiveWorkspaceID(default) error: %v", err)
	}
	if err := app.setActiveWorkspaceTracked(alphaWorkspace.ID, "workspace_switch"); err != nil {
		t.Fatalf("setActiveWorkspaceTracked(alpha) error: %v", err)
	}
	if err := app.setActiveWorkspaceTracked(betaWorkspace.ID, "workspace_switch"); err != nil {
		t.Fatalf("setActiveWorkspaceTracked(beta) error: %v", err)
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/runtime/workspaces", map[string]any{})
	if rr.Code != http.StatusOK {
		t.Fatalf("projects list status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var payload workspacesListResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.ActiveWorkspaceID != workspaceIDStr(betaProject.ID) {
		t.Fatalf("active_workspace_id = %q, want %q", payload.ActiveWorkspaceID, workspaceIDStr(betaProject.ID))
	}
}

func findWorkspaceByName(projects []workspaceListEntry, name string) *workspaceListEntry {
	for i := range projects {
		if projects[i].Name == name {
			return &projects[i]
		}
	}
	return nil
}

func TestCreateActivateProjectAffectsChatSessionCreation(t *testing.T) {
	app := newAuthedTestApp(t)

	linkedDir := t.TempDir()
	rrCreate := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/runtime/workspaces", map[string]any{
		"name":     "linked-repo",
		"kind":     "linked",
		"path":     filepath.Clean(linkedDir),
		"activate": false,
	})
	if rrCreate.Code != http.StatusOK {
		t.Fatalf("expected create 200, got %d: %s", rrCreate.Code, rrCreate.Body.String())
	}
	var createPayload struct {
		OK        bool `json:"ok"`
		Workspace struct {
			ID string `json:"id"`
		} `json:"workspace"`
	}
	if err := json.Unmarshal(rrCreate.Body.Bytes(), &createPayload); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if !createPayload.OK || createPayload.Workspace.ID == "" {
		t.Fatalf("expected created workspace id")
	}

	rrActivate := doAuthedJSONRequest(
		t,
		app.Router(),
		http.MethodPost,
		"/api/runtime/workspaces/"+createPayload.Workspace.ID+"/activate",
		map[string]any{},
	)
	if rrActivate.Code != http.StatusOK {
		t.Fatalf("expected activate 200, got %d: %s", rrActivate.Code, rrActivate.Body.String())
	}
	var activatePayload struct {
		OK                bool   `json:"ok"`
		ActiveWorkspaceID string `json:"active_workspace_id"`
		Project           struct {
			ID string `json:"id"`
		} `json:"workspace"`
	}
	if err := json.Unmarshal(rrActivate.Body.Bytes(), &activatePayload); err != nil {
		t.Fatalf("decode activate response: %v", err)
	}
	if !activatePayload.OK {
		t.Fatalf("expected activate ok=true")
	}
	if activatePayload.ActiveWorkspaceID != createPayload.Workspace.ID {
		t.Fatalf("expected active project %q, got %q", createPayload.Workspace.ID, activatePayload.ActiveWorkspaceID)
	}

	rrSession := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/chat/sessions", map[string]any{})
	if rrSession.Code != http.StatusOK {
		t.Fatalf("expected chat session create 200, got %d: %s", rrSession.Code, rrSession.Body.String())
	}
	var sessionPayload struct {
		OK          bool   `json:"ok"`
		SessionID   string `json:"session_id"`
		WorkspaceID int64  `json:"workspace_id"`
	}
	if err := json.Unmarshal(rrSession.Body.Bytes(), &sessionPayload); err != nil {
		t.Fatalf("decode chat session response: %v", err)
	}
	if !sessionPayload.OK {
		t.Fatalf("expected chat session create ok=true")
	}
	expectedWorkspaceID := runtimeWorkspaceIDInt64FromString(t, createPayload.Workspace.ID)
	if sessionPayload.WorkspaceID != expectedWorkspaceID {
		t.Fatalf("expected chat session workspace %d, got %d", expectedWorkspaceID, sessionPayload.WorkspaceID)
	}
	if sessionPayload.WorkspaceID <= 0 {
		t.Fatalf("expected workspace-backed chat session, got workspace_id=%d", sessionPayload.WorkspaceID)
	}
	session, err := app.store.GetChatSession(sessionPayload.SessionID)
	if err != nil {
		t.Fatalf("GetChatSession() error: %v", err)
	}
	if session.WorkspaceID != sessionPayload.WorkspaceID {
		t.Fatalf("session workspace_id = %d, want %d", session.WorkspaceID, sessionPayload.WorkspaceID)
	}
}

func TestCreateChatSessionWithoutSelectionStaysOnActiveWorkspace(t *testing.T) {
	app := newAuthedTestApp(t)

	anchor, err := app.store.ActiveWorkspace()
	if err != nil {
		t.Fatalf("ActiveWorkspace() error: %v", err)
	}
	_ = anchor

	linkedDir := filepath.Join(t.TempDir(), "linked-repo")
	if err := os.MkdirAll(linkedDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(linkedDir) error: %v", err)
	}
	project, err := app.store.CreateEnrichedWorkspace("linked-repo", "linked-repo", linkedDir, "linked", "", "", false)
	if err != nil {
		t.Fatalf("CreateProject() error: %v", err)
	}
	if err := app.store.SetActiveWorkspaceID(workspaceIDStr(project.ID)); err != nil {
		t.Fatalf("SetActiveWorkspaceID() error: %v", err)
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/chat/sessions", map[string]any{})
	if rr.Code != http.StatusOK {
		t.Fatalf("chat session create status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var payload struct {
		OK          bool   `json:"ok"`
		SessionID   string `json:"session_id"`
		WorkspaceID int64  `json:"workspace_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.OK {
		t.Fatal("expected ok=true")
	}
	if payload.WorkspaceID != anchor.ID {
		t.Fatalf("workspace_id = %d, want anchor %d", payload.WorkspaceID, anchor.ID)
	}
}

func TestProjectsListRehomesActiveProjectIntoActiveSphere(t *testing.T) {
	app := newAuthedTestApp(t)

	privateRoot := filepath.Join(t.TempDir(), "private-root")
	workRoot := filepath.Join(t.TempDir(), "work-root")
	for _, dir := range []string{privateRoot, workRoot} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error: %v", dir, err)
		}
	}
	if _, err := app.store.CreateWorkspace("Private Root", privateRoot, store.SpherePrivate); err != nil {
		t.Fatalf("CreateWorkspace(private) error: %v", err)
	}
	if _, err := app.store.CreateWorkspace("Work Root", workRoot, store.SphereWork); err != nil {
		t.Fatalf("CreateWorkspace(work) error: %v", err)
	}
	privateProjectPath := filepath.Join(privateRoot, "notes")
	workProjectPath := filepath.Join(workRoot, "tracker")
	for _, dir := range []string{privateProjectPath, workProjectPath} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error: %v", dir, err)
		}
	}
	_, err := app.store.CreateEnrichedWorkspace("Private Notes", "private-notes", privateProjectPath, "linked", "", "", false)
	if err != nil {
		t.Fatalf("CreateProject(private) error: %v", err)
	}
	workProject, err := app.store.CreateEnrichedWorkspace("Work Tracker", "work-tracker", workProjectPath, "linked", "", "", false)
	if err != nil {
		t.Fatalf("CreateProject(work) error: %v", err)
	}
	if err := app.store.SetActiveWorkspace(workProject.ID); err != nil {
		t.Fatalf("SetActiveWorkspace(work) error: %v", err)
	}
	if err := app.store.SetActiveSphere(store.SpherePrivate); err != nil {
		t.Fatalf("SetActiveSphere(private) error: %v", err)
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/runtime/workspaces", map[string]any{})
	if rr.Code != http.StatusOK {
		t.Fatalf("list projects status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var payload workspacesListResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.ActiveWorkspaceID == "" {
		t.Fatal("active_workspace_id is empty")
	}
}

func TestProjectActivateUpdatesActiveSphere(t *testing.T) {
	app := newAuthedTestApp(t)

	workRoot := filepath.Join(t.TempDir(), "work-root")
	if err := os.MkdirAll(workRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(workRoot) error: %v", err)
	}
	if _, err := app.store.CreateWorkspace("Work Root", workRoot, store.SphereWork); err != nil {
		t.Fatalf("CreateWorkspace(work) error: %v", err)
	}
	projectPath := filepath.Join(workRoot, "tracker")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(projectPath) error: %v", err)
	}
	project, err := app.store.CreateEnrichedWorkspace("Work Tracker", "work-tracker", projectPath, "linked", "", "", false)
	if err != nil {
		t.Fatalf("CreateProject(work) error: %v", err)
	}
	if err := app.store.SetActiveSphere(store.SpherePrivate); err != nil {
		t.Fatalf("SetActiveSphere(private) error: %v", err)
	}

	rr := doAuthedJSONRequest(
		t,
		app.Router(),
		http.MethodPost,
		"/api/runtime/workspaces/"+workspaceIDStr(project.ID)+"/activate",
		map[string]any{},
	)
	if rr.Code != http.StatusOK {
		t.Fatalf("activate status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var payload struct {
		OK                bool   `json:"ok"`
		ActiveWorkspaceID string `json:"active_workspace_id"`
		ActiveSphere      string `json:"active_sphere"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode activate response: %v", err)
	}
	if !payload.OK {
		t.Fatal("expected ok=true")
	}
	if payload.ActiveWorkspaceID != workspaceIDStr(project.ID) {
		t.Fatalf("active_workspace_id = %q, want %q", payload.ActiveWorkspaceID, project.ID)
	}
	if payload.ActiveSphere != store.SphereWork {
		t.Fatalf("active_sphere = %q, want %q", payload.ActiveSphere, store.SphereWork)
	}
	activeSphere, err := app.store.ActiveSphere()
	if err != nil {
		t.Fatalf("ActiveSphere() error: %v", err)
	}
	if activeSphere != store.SphereWork {
		t.Fatalf("stored active sphere = %q, want %q", activeSphere, store.SphereWork)
	}
}

func TestWorkspaceSnapshotRejectsWorkPersonalActivation(t *testing.T) {
	_, personalRoot := configureWorkPersonalGuardrail(t)
	app := newAuthedTestApp(t)
	project, err := app.store.CreateEnrichedWorkspace("Personal", "personal", personalRoot, "linked", "", "", false)
	if err != nil {
		t.Fatalf("CreateEnrichedWorkspace(personal) error: %v", err)
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/runtime/workspaces/"+workspaceIDStr(project.ID)+"/snapshot", nil)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("snapshot status = %d, want 403: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "work personal subtree is blocked") {
		t.Fatalf("snapshot response = %q", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), personalRoot) {
		t.Fatalf("snapshot response leaked protected path: %q", rr.Body.String())
	}
}

func TestProjectChatModelUpdate(t *testing.T) {
	app := newAuthedTestApp(t)

	rrList := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/runtime/workspaces", map[string]any{})
	if rrList.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d: %s", rrList.Code, rrList.Body.String())
	}
	var listPayload workspacesListResponse
	if err := json.Unmarshal(rrList.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode projects response: %v", err)
	}
	if len(listPayload.Projects) == 0 {
		t.Fatalf("expected at least one project")
	}
	workspaceID := listPayload.Projects[0].ID
	if workspaceID == "" {
		t.Fatalf("expected project id")
	}

	rrUpdate := doAuthedJSONRequest(
		t,
		app.Router(),
		http.MethodPost,
		"/api/runtime/workspaces/"+workspaceID+"/chat-model",
		map[string]any{"model": "gpt"},
	)
	if rrUpdate.Code != http.StatusBadRequest {
		t.Fatalf("expected update 400, got %d: %s", rrUpdate.Code, rrUpdate.Body.String())
	}
	if !strings.Contains(rrUpdate.Body.String(), "local is the only default dialogue model") {
		t.Fatalf("expected local-only error, got %q", rrUpdate.Body.String())
	}

	rrLocal := doAuthedJSONRequest(
		t,
		app.Router(),
		http.MethodPost,
		"/api/runtime/workspaces/"+workspaceID+"/chat-model",
		map[string]any{"model": "local"},
	)
	if rrLocal.Code != http.StatusOK {
		t.Fatalf("expected local update 200, got %d: %s", rrLocal.Code, rrLocal.Body.String())
	}
	var updatePayload struct {
		OK        bool `json:"ok"`
		Workspace struct {
			ID                       string `json:"id"`
			ChatModel                string `json:"chat_model"`
			ChatModelReasoningEffort string `json:"chat_model_reasoning_effort"`
		} `json:"workspace"`
	}
	if err := json.Unmarshal(rrLocal.Body.Bytes(), &updatePayload); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if !updatePayload.OK {
		t.Fatalf("expected update ok=true")
	}
	if updatePayload.Workspace.ID != workspaceID {
		t.Fatalf("expected updated workspace id %q, got %q", workspaceID, updatePayload.Workspace.ID)
	}
	if updatePayload.Workspace.ChatModel != "local" {
		t.Fatalf("expected chat model local, got %q", updatePayload.Workspace.ChatModel)
	}
	if updatePayload.Workspace.ChatModelReasoningEffort != "none" {
		t.Fatalf("expected local reasoning effort none, got %q", updatePayload.Workspace.ChatModelReasoningEffort)
	}

	rrEffortUpdate := doAuthedJSONRequest(
		t,
		app.Router(),
		http.MethodPost,
		"/api/runtime/workspaces/"+workspaceID+"/chat-model",
		map[string]any{
			"model":            "local",
			"reasoning_effort": "extra_high",
		},
	)
	if rrEffortUpdate.Code != http.StatusOK {
		t.Fatalf("expected effort update 200, got %d: %s", rrEffortUpdate.Code, rrEffortUpdate.Body.String())
	}
	var effortPayload struct {
		OK        bool `json:"ok"`
		Workspace struct {
			ID                       string `json:"id"`
			ChatModelReasoningEffort string `json:"chat_model_reasoning_effort"`
		} `json:"workspace"`
	}
	if err := json.Unmarshal(rrEffortUpdate.Body.Bytes(), &effortPayload); err != nil {
		t.Fatalf("decode effort update response: %v", err)
	}
	if !effortPayload.OK {
		t.Fatalf("expected effort update ok=true")
	}
	if effortPayload.Workspace.ChatModelReasoningEffort != "none" {
		t.Fatalf("expected effort none, got %q", effortPayload.Workspace.ChatModelReasoningEffort)
	}

	rrInvalid := doAuthedJSONRequest(
		t,
		app.Router(),
		http.MethodPost,
		"/api/runtime/workspaces/"+workspaceID+"/chat-model",
		map[string]any{"model": "invalid"},
	)
	if rrInvalid.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid model 400, got %d: %s", rrInvalid.Code, rrInvalid.Body.String())
	}
}

func TestProjectsListMatchesStoredProjects(t *testing.T) {
	app := newAuthedTestApp(t)

	rrList := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/runtime/workspaces", map[string]any{})
	if rrList.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d: %s", rrList.Code, rrList.Body.String())
	}
	var payload workspacesListResponse
	if err := json.Unmarshal(rrList.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode projects response: %v", err)
	}
	storedProjects, err := app.store.ListEnrichedWorkspaces()
	if err != nil {
		t.Fatalf("ListProjects() error: %v", err)
	}
	if len(payload.Projects) != len(storedProjects) {
		t.Fatalf("payload project count = %d, want %d", len(payload.Projects), len(storedProjects))
	}
	storedByID := make(map[string]store.Workspace, len(storedProjects))
	for _, project := range storedProjects {
		storedByID[workspaceIDStr(project.ID)] = project
	}
	for _, project := range payload.Projects {
		stored, ok := storedByID[project.ID]
		if !ok {
			t.Fatalf("unexpected project in payload: %#v", project)
		}
		if project.Name != stored.Name {
			t.Fatalf("project %q name = %q, want %q", project.ID, project.Name, stored.Name)
		}
	}
}

func TestProjectsListIncludesRunState(t *testing.T) {
	app := newAuthedTestApp(t)
	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("default project: %v", err)
	}
	session, err := app.store.GetOrCreateChatSession(project.WorkspacePath)
	if err != nil {
		t.Fatalf("chat session: %v", err)
	}
	app.registerActiveChatTurn(session.ID, "run-projects", func() {})
	app.turns.mu.Lock()
	app.turns.queue[session.ID] = 2
	app.turns.mu.Unlock()

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/runtime/workspaces", map[string]any{})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var payload workspacesListResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	for _, item := range payload.Projects {
		if item.ID != workspaceIDStr(project.ID) {
			continue
		}
		if item.RunState.ActiveTurns != 1 {
			t.Fatalf("active_turns = %d, want 1", item.RunState.ActiveTurns)
		}
		if item.RunState.QueuedTurns != 2 {
			t.Fatalf("queued_turns = %d, want 2", item.RunState.QueuedTurns)
		}
		if !item.RunState.IsWorking {
			t.Fatalf("expected workspace to be working")
		}
		if item.RunState.Status != "running" {
			t.Fatalf("status = %q, want running", item.RunState.Status)
		}
		if item.RunState.ActiveTurnID != "run-projects" {
			t.Fatalf("active_turn_id = %q, want run-projects", item.RunState.ActiveTurnID)
		}
		return
	}
	t.Fatalf("expected workspace %d in list response", project.ID)
}

func TestProjectsActivityListsPerProjectRunState(t *testing.T) {
	app := newAuthedTestApp(t)
	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("default project: %v", err)
	}
	session, err := app.store.GetOrCreateChatSession(project.WorkspacePath)
	if err != nil {
		t.Fatalf("chat session: %v", err)
	}
	app.turns.mu.Lock()
	app.turns.queue[session.ID] = 3
	app.turns.mu.Unlock()

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/runtime/workspaces/activity", map[string]any{})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected activity 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var payload workspacesActivityResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode activity response: %v", err)
	}
	for _, item := range payload.Projects {
		if item.WorkspaceID != workspaceIDStr(project.ID) {
			continue
		}
		if item.ChatSessionID != session.ID {
			t.Fatalf("chat_session_id = %q, want %q", item.ChatSessionID, session.ID)
		}
		if item.RunState.ActiveTurns != 0 {
			t.Fatalf("active_turns = %d, want 0", item.RunState.ActiveTurns)
		}
		if item.RunState.QueuedTurns != 3 {
			t.Fatalf("queued_turns = %d, want 3", item.RunState.QueuedTurns)
		}
		if !item.RunState.IsWorking {
			t.Fatalf("expected workspace to be working")
		}
		if item.RunState.Status != "queued" {
			t.Fatalf("status = %q, want queued", item.RunState.Status)
		}
		return
	}
	t.Fatalf("expected workspace %d in activity response", project.ID)
}

func TestProjectsActivityUnreadClearsOnActivate(t *testing.T) {
	app := newAuthedTestApp(t)

	linkedDir := t.TempDir()
	rrCreate := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/runtime/workspaces", map[string]any{
		"name":     "Unread Test",
		"kind":     "linked",
		"path":     filepath.Clean(linkedDir),
		"activate": false,
	})
	if rrCreate.Code != http.StatusOK {
		t.Fatalf("expected create 200, got %d: %s", rrCreate.Code, rrCreate.Body.String())
	}
	var createPayload struct {
		Workspace struct {
			ID string `json:"id"`
		} `json:"workspace"`
	}
	if err := json.Unmarshal(rrCreate.Body.Bytes(), &createPayload); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	project, err := app.store.GetEnrichedWorkspace(createPayload.Workspace.ID)
	if err != nil {
		t.Fatalf("get project: %v", err)
	}

	app.markWorkspaceOutput(project.WorkspacePath)

	findActivity := func() workspacesActivityResponse {
		t.Helper()
		rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/runtime/workspaces/activity", map[string]any{})
		if rr.Code != http.StatusOK {
			t.Fatalf("expected activity 200, got %d: %s", rr.Code, rr.Body.String())
		}
		var payload workspacesActivityResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode activity response: %v", err)
		}
		return payload
	}

	initial := findActivity()
	foundUnread := false
	for _, item := range initial.Projects {
		if item.WorkspaceID != workspaceIDStr(project.ID) {
			continue
		}
		foundUnread = true
		if !item.Unread {
			t.Fatalf("expected unread=true before activation")
		}
		if item.ReviewPending {
			t.Fatalf("expected review_pending=false before activation")
		}
	}
	if !foundUnread {
		t.Fatalf("expected project %q in activity response", project.ID)
	}

	rrActivate := doAuthedJSONRequest(
		t,
		app.Router(),
		http.MethodPost,
		"/api/runtime/workspaces/"+workspaceIDStr(project.ID)+"/activate",
		map[string]any{},
	)
	if rrActivate.Code != http.StatusOK {
		t.Fatalf("expected activate 200, got %d: %s", rrActivate.Code, rrActivate.Body.String())
	}

	afterActivate := findActivity()
	for _, item := range afterActivate.Projects {
		if item.WorkspaceID != workspaceIDStr(project.ID) {
			continue
		}
		if item.Unread {
			t.Fatalf("expected unread=false after activation")
		}
		if item.ReviewPending {
			t.Fatalf("expected review_pending=false after activation")
		}
		return
	}
	t.Fatalf("expected workspace %d in activity response after activation", project.ID)
}

func TestProjectChatModelUpdateAllowsDefaultProject(t *testing.T) {
	app := newAuthedTestApp(t)
	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensure default project: %v", err)
	}

	rr := doAuthedJSONRequest(
		t,
		app.Router(),
		http.MethodPost,
		"/api/runtime/workspaces/"+workspaceIDStr(project.ID)+"/chat-model",
		map[string]any{"model": "gpt"},
	)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "local is the only default dialogue model") {
		t.Fatalf("expected local-only error, got %q", rr.Body.String())
	}
}

func TestProjectFilesListReturnsOneLevelAndSupportsSubfolders(t *testing.T) {
	app := newAuthedTestApp(t)
	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("default project: %v", err)
	}
	workspace := requireWorkspaceForProject(t, app, project)
	root := filepath.Clean(project.RootPath)
	dirName := "zz_test_dir"
	fileName := "zz_test_file.txt"
	dirPath := filepath.Join(root, dirName)
	filePath := filepath.Join(root, fileName)
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		t.Fatalf("mkdir test dir: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("root"), 0o644); err != nil {
		t.Fatalf("write root test file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dirPath, "child.md"), []byte("child"), 0o644); err != nil {
		t.Fatalf("write child test file: %v", err)
	}

	rrRoot := doAuthedJSONRequest(
		t,
		app.Router(),
		http.MethodGet,
		"/api/workspaces/"+itoa(workspace.ID)+"/files",
		nil,
	)
	if rrRoot.Code != http.StatusOK {
		t.Fatalf("expected root list 200, got %d: %s", rrRoot.Code, rrRoot.Body.String())
	}
	var rootPayload workspaceFilesListResponse
	if err := json.Unmarshal(rrRoot.Body.Bytes(), &rootPayload); err != nil {
		t.Fatalf("decode root payload: %v", err)
	}
	if !rootPayload.OK {
		t.Fatalf("expected ok=true")
	}
	if rootPayload.WorkspaceID != workspace.ID {
		t.Fatalf("workspace_id = %d, want %d", rootPayload.WorkspaceID, workspace.ID)
	}
	if !rootPayload.IsRoot || rootPayload.Path != "" {
		t.Fatalf("expected root listing, got is_root=%v path=%q", rootPayload.IsRoot, rootPayload.Path)
	}
	dirIndex := -1
	fileIndex := -1
	for i, entry := range rootPayload.Entries {
		if entry.Path == dirName {
			if !entry.IsDir {
				t.Fatalf("expected %q to be a directory", dirName)
			}
			dirIndex = i
		}
		if entry.Path == fileName {
			if entry.IsDir {
				t.Fatalf("expected %q to be a file", fileName)
			}
			fileIndex = i
		}
	}
	if dirIndex < 0 || fileIndex < 0 {
		t.Fatalf("expected seeded entries in root listing")
	}
	if dirIndex > fileIndex {
		t.Fatalf("expected directories before files in listing")
	}

	rrSub := doAuthedJSONRequest(
		t,
		app.Router(),
		http.MethodGet,
		"/api/workspaces/"+itoa(workspace.ID)+"/files?path="+dirName,
		nil,
	)
	if rrSub.Code != http.StatusOK {
		t.Fatalf("expected subdirectory list 200, got %d: %s", rrSub.Code, rrSub.Body.String())
	}
	var subPayload workspaceFilesListResponse
	if err := json.Unmarshal(rrSub.Body.Bytes(), &subPayload); err != nil {
		t.Fatalf("decode sub payload: %v", err)
	}
	if subPayload.IsRoot || subPayload.Path != dirName {
		t.Fatalf("expected subdirectory payload for %q, got is_root=%v path=%q", dirName, subPayload.IsRoot, subPayload.Path)
	}
	if len(subPayload.Entries) == 0 || subPayload.Entries[0].Path != dirName+"/child.md" {
		t.Fatalf("expected child file path %q in subdirectory listing", dirName+"/child.md")
	}
}

func TestProjectFilesListBlocksWorkPersonalSubtree(t *testing.T) {
	vaultRoot, personalRoot := configureWorkPersonalGuardrail(t)
	app := newAuthedTestApp(t)
	if err := os.WriteFile(filepath.Join(personalRoot, "diary.md"), []byte("secret"), 0o644); err != nil {
		t.Fatalf("write protected file: %v", err)
	}
	workspace, err := app.store.CreateWorkspace("Work Vault", vaultRoot, store.SphereWork)
	if err != nil {
		t.Fatalf("CreateWorkspace(vault) error: %v", err)
	}

	rrRoot := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/workspaces/"+itoa(workspace.ID)+"/files", nil)
	if rrRoot.Code != http.StatusOK {
		t.Fatalf("root list status = %d, want 200: %s", rrRoot.Code, rrRoot.Body.String())
	}
	body := rrRoot.Body.String()
	if strings.Contains(body, "personal") || strings.Contains(body, "diary.md") {
		t.Fatalf("root listing leaked protected subtree metadata: %s", body)
	}

	rrPersonal := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/workspaces/"+itoa(workspace.ID)+"/files?path=personal", nil)
	if rrPersonal.Code != http.StatusForbidden {
		t.Fatalf("personal list status = %d, want 403: %s", rrPersonal.Code, rrPersonal.Body.String())
	}
	body = rrPersonal.Body.String()
	if !strings.Contains(body, "work personal subtree is blocked") {
		t.Fatalf("personal list response = %q", body)
	}
	if strings.Contains(body, "diary.md") || strings.Contains(body, personalRoot) {
		t.Fatalf("personal list response leaked protected metadata: %q", body)
	}
}

func TestWorkspaceMarkdownLinkResolveAllowsBrainAndVaultLinks(t *testing.T) {
	vaultRoot, _ := configureWorkPersonalGuardrail(t)
	brainRoot := filepath.Join(vaultRoot, "brain")
	sourceRoot := filepath.Join(vaultRoot, "sources")
	if err := os.MkdirAll(filepath.Join(brainRoot, "topics"), 0o755); err != nil {
		t.Fatalf("mkdir brain topics: %v", err)
	}
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatalf("mkdir sources: %v", err)
	}
	if err := os.WriteFile(filepath.Join(brainRoot, "topics", "active.md"), []byte("[related](related.md)"), 0o644); err != nil {
		t.Fatalf("write active note: %v", err)
	}
	if err := os.WriteFile(filepath.Join(brainRoot, "topics", "related.md"), []byte("related"), 0o644); err != nil {
		t.Fatalf("write related note: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "paper.md"), []byte("paper"), 0o644); err != nil {
		t.Fatalf("write source note: %v", err)
	}
	app := newAuthedTestApp(t)
	workspace, err := app.store.CreateWorkspace("Work brain", brainRoot, store.SphereWork)
	if err != nil {
		t.Fatalf("CreateWorkspace(brain) error: %v", err)
	}

	inBrain := resolveMarkdownLinkForTest(t, app, workspace.ID, "topics/active.md", "related.md", "")
	if !inBrain.OK || inBrain.ResolvedPath != "brain/topics/related.md" || inBrain.Kind != "text" {
		t.Fatalf("in-brain resolution = %+v", inBrain)
	}
	outToSource := resolveMarkdownLinkForTest(t, app, workspace.ID, "topics/active.md", "../../sources/paper.md", "")
	if !outToSource.OK || outToSource.ResolvedPath != "sources/paper.md" || outToSource.Kind != "text" {
		t.Fatalf("out-to-source resolution = %+v", outToSource)
	}
	if strings.Contains(outToSource.FileURL, vaultRoot) || filepath.IsAbs(outToSource.ResolvedPath) {
		t.Fatalf("resolution leaked absolute path: %+v", outToSource)
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, outToSource.FileURL, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("linked file status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	if strings.TrimSpace(rr.Body.String()) != "paper" {
		t.Fatalf("linked file body = %q, want paper", rr.Body.String())
	}
}

func TestWorkspaceMarkdownLinkResolveSupportsWikilinks(t *testing.T) {
	vaultRoot, _ := configureWorkPersonalGuardrail(t)
	brainRoot := filepath.Join(vaultRoot, "brain")
	if err := os.MkdirAll(filepath.Join(brainRoot, "topics"), 0o755); err != nil {
		t.Fatalf("mkdir brain topics: %v", err)
	}
	if err := os.WriteFile(filepath.Join(brainRoot, "topics", "active.md"), []byte("[[Related Note]]"), 0o644); err != nil {
		t.Fatalf("write active note: %v", err)
	}
	if err := os.WriteFile(filepath.Join(brainRoot, "topics", "Related Note.md"), []byte("related"), 0o644); err != nil {
		t.Fatalf("write related note: %v", err)
	}
	app := newAuthedTestApp(t)
	workspace, err := app.store.CreateWorkspace("Work brain", brainRoot, store.SphereWork)
	if err != nil {
		t.Fatalf("CreateWorkspace(brain) error: %v", err)
	}

	resolved := resolveMarkdownLinkForTest(t, app, workspace.ID, "topics/active.md", "Related Note", "wikilink")
	if !resolved.OK || resolved.ResolvedPath != "brain/topics/Related Note.md" {
		t.Fatalf("wikilink resolution = %+v", resolved)
	}
}

func TestWorkspaceMarkdownLinkResolveRejectsOutOfVaultAndWorkPersonal(t *testing.T) {
	vaultRoot, personalRoot := configureWorkPersonalGuardrail(t)
	brainRoot := filepath.Join(vaultRoot, "brain")
	outsidePath := filepath.Join(filepath.Dir(vaultRoot), "outside.md")
	if err := os.MkdirAll(filepath.Join(brainRoot, "topics"), 0o755); err != nil {
		t.Fatalf("mkdir brain topics: %v", err)
	}
	if err := os.WriteFile(filepath.Join(brainRoot, "topics", "active.md"), []byte("active"), 0o644); err != nil {
		t.Fatalf("write active note: %v", err)
	}
	if err := os.WriteFile(outsidePath, []byte("outside"), 0o644); err != nil {
		t.Fatalf("write outside note: %v", err)
	}
	if err := os.WriteFile(filepath.Join(personalRoot, "diary.md"), []byte("secret"), 0o644); err != nil {
		t.Fatalf("write personal note: %v", err)
	}
	app := newAuthedTestApp(t)
	workspace, err := app.store.CreateWorkspace("Work brain", brainRoot, store.SphereWork)
	if err != nil {
		t.Fatalf("CreateWorkspace(brain) error: %v", err)
	}

	outside := resolveMarkdownLinkForTest(t, app, workspace.ID, "topics/active.md", "../../../outside.md", "")
	if outside.OK || !outside.Blocked || !strings.Contains(outside.Reason, "leaves the vault") {
		t.Fatalf("out-of-vault resolution = %+v", outside)
	}
	personal := resolveMarkdownLinkForTest(t, app, workspace.ID, "topics/active.md", "../../personal/diary.md", "")
	if personal.OK || !personal.Blocked || !strings.Contains(personal.Reason, "work personal subtree is blocked") {
		t.Fatalf("personal resolution = %+v", personal)
	}
	if strings.Contains(personal.Reason, personalRoot) || strings.Contains(outside.Reason, outsidePath) {
		t.Fatalf("blocked reason leaked absolute path: personal=%q outside=%q", personal.Reason, outside.Reason)
	}
}

func TestWorkspaceMarkdownLinkResolveOpensLinkedFolderWithinVault(t *testing.T) {
	vaultRoot, _ := configureWorkPersonalGuardrail(t)
	brainRoot := filepath.Join(vaultRoot, "brain")
	sourceDir := filepath.Join(brainRoot, "topics")
	targetDir := filepath.Join(vaultRoot, "project", "path")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("mkdir source dir: %v", err)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "active.md"), []byte("active"), 0o644); err != nil {
		t.Fatalf("write source note: %v", err)
	}
	app := newAuthedTestApp(t)
	workspace, err := app.store.CreateWorkspace("Work brain", brainRoot, store.SphereWork)
	if err != nil {
		t.Fatalf("CreateWorkspace(brain) error: %v", err)
	}

	resolution := resolveMarkdownLinkForTest(t, app, workspace.ID, "topics/active.md", "../../project/path/", "")
	wantRel := filepath.ToSlash(filepath.Join("project", "path"))
	if !resolution.OK {
		t.Fatalf("resolution blocked: %+v", resolution)
	}
	if resolution.Kind != "folder" {
		t.Fatalf("resolution kind = %q, want folder", resolution.Kind)
	}
	if resolution.FileURL != "" {
		t.Fatalf("folder resolution file_url = %q, want empty", resolution.FileURL)
	}
	if resolution.ResolvedPath != wantRel || resolution.VaultRelativePath != wantRel {
		t.Fatalf("resolution path = %+v, want relative %q", resolution, wantRel)
	}
	if strings.HasPrefix(resolution.ResolvedPath, string(filepath.Separator)) || strings.Contains(resolution.ResolvedPath, ":") {
		t.Fatalf("resolution leaked absolute path: %+v", resolution)
	}
}

func TestWorkspaceMarkdownLinkResolveUsesRelativeFileURLForVaultNotes(t *testing.T) {
	vaultRoot, _ := configureWorkPersonalGuardrail(t)
	brainRoot := filepath.Join(vaultRoot, "brain")
	sourceDir := filepath.Join(brainRoot, "topics")
	targetDir := filepath.Join(vaultRoot, "project", "path")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("mkdir source dir: %v", err)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "active.md"), []byte("active"), 0o644); err != nil {
		t.Fatalf("write source note: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "file.md"), []byte("target"), 0o644); err != nil {
		t.Fatalf("write target note: %v", err)
	}
	app := newAuthedTestApp(t)
	workspace, err := app.store.CreateWorkspace("Work brain", brainRoot, store.SphereWork)
	if err != nil {
		t.Fatalf("CreateWorkspace(brain) error: %v", err)
	}

	resolution := resolveMarkdownLinkForTest(t, app, workspace.ID, "topics/active.md", "../../project/path/file.md", "")
	wantRel := filepath.ToSlash(filepath.Join("project", "path", "file.md"))
	if !resolution.OK {
		t.Fatalf("resolution blocked: %+v", resolution)
	}
	if resolution.Kind != "text" {
		t.Fatalf("resolution kind = %q, want text", resolution.Kind)
	}
	if resolution.ResolvedPath != wantRel || resolution.VaultRelativePath != wantRel {
		t.Fatalf("resolution path = %+v, want relative %q", resolution, wantRel)
	}
	if resolution.FileURL == "" || !strings.Contains(resolution.FileURL, "/api/workspaces/") {
		t.Fatalf("resolution file_url = %q, want api path", resolution.FileURL)
	}
	if strings.Contains(resolution.FileURL, "file://") || strings.Contains(resolution.FileURL, vaultRoot) || strings.Contains(resolution.FileURL, string(filepath.Separator)+"home"+string(filepath.Separator)) {
		t.Fatalf("resolution leaked machine path in file_url: %+v", resolution)
	}
}

func resolveMarkdownLinkForTest(t *testing.T, app *App, workspaceID int64, sourcePath, target, linkType string) workspaceMarkdownLinkResolution {
	t.Helper()
	values := url.Values{}
	values.Set("source", sourcePath)
	values.Set("target", target)
	if linkType != "" {
		values.Set("type", linkType)
	}
	rr := doAuthedJSONRequest(
		t,
		app.Router(),
		http.MethodGet,
		"/api/workspaces/"+itoa(workspaceID)+"/markdown-link/resolve?"+values.Encode(),
		nil,
	)
	if rr.Code != http.StatusOK {
		t.Fatalf("resolve status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var payload workspaceMarkdownLinkResolution
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode resolution: %v", err)
	}
	return payload
}

func TestProjectFilesListRejectsTraversal(t *testing.T) {
	app := newAuthedTestApp(t)
	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("default project: %v", err)
	}
	workspace := requireWorkspaceForProject(t, app, project)
	rr := doAuthedJSONRequest(
		t,
		app.Router(),
		http.MethodGet,
		"/api/workspaces/"+itoa(workspace.ID)+"/files?path=../secret",
		nil,
	)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected traversal request 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestProjectWelcomeListsDocsAndRecentFiles(t *testing.T) {
	app := newAuthedTestApp(t)

	rrList := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/runtime/workspaces", map[string]any{})
	if rrList.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d: %s", rrList.Code, rrList.Body.String())
	}
	var listPayload workspacesListResponse
	if err := json.Unmarshal(rrList.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode projects response: %v", err)
	}
	if len(listPayload.Projects) == 0 {
		t.Fatalf("expected at least one project")
	}
	workspaceID := listPayload.Projects[0].ID
	project, err := app.store.GetEnrichedWorkspace(workspaceID)
	if err != nil {
		t.Fatalf("get project: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project.RootPath, "README.md"), []byte("# hello"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project.RootPath, "notes.txt"), []byte("recent"), 0o644); err != nil {
		t.Fatalf("write notes.txt: %v", err)
	}

	rrWelcome := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/runtime/workspaces/"+workspaceID+"/welcome", nil)
	if rrWelcome.Code != http.StatusOK {
		t.Fatalf("expected welcome 200, got %d: %s", rrWelcome.Code, rrWelcome.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rrWelcome.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode welcome response: %v", err)
	}
	if got := strFromAny(payload["scope"]); got != "project" {
		t.Fatalf("scope = %q, want %q", got, "project")
	}
	sections, ok := payload["sections"].([]any)
	if !ok || len(sections) == 0 {
		t.Fatalf("expected welcome sections, got %v", payload["sections"])
	}
	body := rrWelcome.Body.String()
	if !strings.Contains(body, "README.md") {
		t.Fatalf("welcome body missing README.md: %s", body)
	}
	if !strings.Contains(body, "notes.txt") {
		t.Fatalf("welcome body missing notes.txt: %s", body)
	}
}

func TestProjectWelcomeIncludesRuntimeCards(t *testing.T) {
	app := newAuthedTestApp(t)

	rrList := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/runtime/workspaces", map[string]any{})
	if rrList.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d: %s", rrList.Code, rrList.Body.String())
	}
	var listPayload workspacesListResponse
	if err := json.Unmarshal(rrList.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode projects response: %v", err)
	}
	if len(listPayload.Projects) == 0 {
		t.Fatalf("expected at least one project")
	}
	workspaceID := listPayload.Projects[0].ID

	rrWelcome := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/runtime/workspaces/"+workspaceID+"/welcome", nil)
	if rrWelcome.Code != http.StatusOK {
		t.Fatalf("expected project welcome 200, got %d: %s", rrWelcome.Code, rrWelcome.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rrWelcome.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode welcome response: %v", err)
	}
	if got := strFromAny(payload["scope"]); got != "project" {
		t.Fatalf("scope = %q, want %q", got, "project")
	}
	if !strings.Contains(rrWelcome.Body.String(), "Silent mode") {
		t.Fatalf("welcome missing runtime card: %s", rrWelcome.Body.String())
	}
}

func TestProjectWelcomeIncludesStartAgentCardForLinkedWorkspaces(t *testing.T) {
	vaultRoot, _ := configureWorkPersonalGuardrail(t)
	brainRoot := filepath.Join(vaultRoot, "brain")
	linkedRoot := filepath.Join(vaultRoot, "project", "path")
	if err := os.MkdirAll(filepath.Join(brainRoot, "topics"), 0o755); err != nil {
		t.Fatalf("mkdir brain topics: %v", err)
	}
	if err := os.MkdirAll(linkedRoot, 0o755); err != nil {
		t.Fatalf("mkdir linked root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(brainRoot, "topics", "active.md"), []byte("active"), 0o644); err != nil {
		t.Fatalf("write active note: %v", err)
	}
	app := newAuthedTestApp(t)
	if _, err := app.store.CreateWorkspace("Work brain", brainRoot, store.SphereWork); err != nil {
		t.Fatalf("CreateWorkspace(brain) error: %v", err)
	}
	linked, _, err := app.createWorkspace2(runtimeWorkspaceCreateRequest{
		Name: "Linked source",
		Kind: "linked",
		Path: linkedRoot,
	})
	if err != nil {
		t.Fatalf("create linked workspace: %v", err)
	}
	workspaceID := workspaceIDStr(linked.ID)
	rrWelcome := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/runtime/workspaces/"+workspaceID+"/welcome", nil)
	if rrWelcome.Code != http.StatusOK {
		t.Fatalf("expected linked welcome 200, got %d: %s", rrWelcome.Code, rrWelcome.Body.String())
	}
	if !strings.Contains(rrWelcome.Body.String(), "Start agent here") {
		t.Fatalf("welcome missing start-agent card: %s", rrWelcome.Body.String())
	}
}

func TestTemporaryProjectCreationCopiesSourceSettingsAndPersist(t *testing.T) {
	app := newAuthedTestApp(t)
	source, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("default project: %v", err)
	}
	if err := app.store.UpdateEnrichedWorkspaceChatModel(workspaceIDStr(source.ID), "gpt"); err != nil {
		t.Fatalf("UpdateProjectChatModel() error: %v", err)
	}
	if err := app.store.UpdateEnrichedWorkspaceChatModelReasoningEffort(workspaceIDStr(source.ID), "xhigh"); err != nil {
		t.Fatalf("UpdateProjectChatModelReasoningEffort() error: %v", err)
	}
	if err := app.store.UpdateEnrichedWorkspaceCompanionConfig(workspaceIDStr(source.ID), `{"companion_enabled":true,"idle_surface":"black"}`); err != nil {
		t.Fatalf("UpdateProjectCompanionConfig() error: %v", err)
	}

	rrCreate := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/runtime/workspaces", map[string]any{
		"kind":                "meeting",
		"source_workspace_id": workspaceIDStr(source.ID),
	})
	if rrCreate.Code != http.StatusOK {
		t.Fatalf("expected create 200, got %d: %s", rrCreate.Code, rrCreate.Body.String())
	}
	var createPayload struct {
		OK        bool `json:"ok"`
		Workspace struct {
			ID        string `json:"id"`
			Kind      string `json:"kind"`
			RootPath  string `json:"root_path"`
			ChatModel string `json:"chat_model"`
		} `json:"workspace"`
	}
	if err := json.Unmarshal(rrCreate.Body.Bytes(), &createPayload); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if !createPayload.OK || createPayload.Workspace.ID == "" {
		t.Fatalf("expected created workspace payload")
	}
	if createPayload.Workspace.Kind != "meeting" {
		t.Fatalf("created kind = %q, want meeting", createPayload.Workspace.Kind)
	}
	if createPayload.Workspace.ChatModel != "gpt" {
		t.Fatalf("created chat model = %q, want gpt", createPayload.Workspace.ChatModel)
	}
	if createPayload.Workspace.RootPath == source.RootPath {
		t.Fatalf("temporary workspace root should differ from source root")
	}
	if !strings.Contains(filepath.ToSlash(createPayload.Workspace.RootPath), "/projects/temporary/meeting/") {
		t.Fatalf("temporary root = %q, want temporary meeting path", createPayload.Workspace.RootPath)
	}

	created, err := app.store.GetEnrichedWorkspace(createPayload.Workspace.ID)
	if err != nil {
		t.Fatalf("GetProject(created) error: %v", err)
	}
	if created.ChatModel != "gpt" {
		t.Fatalf("stored chat model = %q, want gpt", created.ChatModel)
	}
	if created.ChatModelReasoningEffort != "xhigh" {
		t.Fatalf("stored reasoning effort = %q, want xhigh", created.ChatModelReasoningEffort)
	}
	if got := strings.TrimSpace(created.CompanionConfigJSON); got != `{"companion_enabled":true,"idle_surface":"black"}` {
		t.Fatalf("stored companion config = %q", got)
	}
	workspace, err := app.ensureWorkspaceReady(created, false)
	if err != nil {
		t.Fatalf("ensureWorkspaceReady(created) error: %v", err)
	}
	artifactPath := filepath.Join(created.RootPath, "meeting-notes.md")
	if err := os.WriteFile(artifactPath, []byte("notes"), 0o644); err != nil {
		t.Fatalf("WriteFile(artifactPath) error: %v", err)
	}
	artifact, err := app.store.CreateArtifact(store.ArtifactKindMarkdown, &artifactPath, nil, nil, nil)
	if err != nil {
		t.Fatalf("CreateArtifact() error: %v", err)
	}
	participantSession, err := app.store.AddParticipantSession(created.WorkspacePath, "{}")
	if err != nil {
		t.Fatalf("AddParticipantSession() error: %v", err)
	}
	targetPath := filepath.Join(t.TempDir(), "persisted-meeting")

	rrPersist := doAuthedJSONRequest(
		t,
		app.Router(),
		http.MethodPost,
		"/api/runtime/workspaces/"+createPayload.Workspace.ID+"/persist",
		map[string]any{
			"name": "Focused Meeting",
			"path": targetPath,
		},
	)
	if rrPersist.Code != http.StatusOK {
		t.Fatalf("expected persist 200, got %d: %s", rrPersist.Code, rrPersist.Body.String())
	}
	var persistPayload struct {
		OK        bool `json:"ok"`
		Workspace struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		} `json:"workspace"`
	}
	if err := json.Unmarshal(rrPersist.Body.Bytes(), &persistPayload); err != nil {
		t.Fatalf("decode persist response: %v", err)
	}
	if !persistPayload.OK {
		t.Fatalf("expected persist ok=true")
	}
	if persistPayload.Workspace.Kind != "managed" {
		t.Fatalf("persisted kind = %q, want managed", persistPayload.Workspace.Kind)
	}
	persisted, err := app.store.GetEnrichedWorkspace(createPayload.Workspace.ID)
	if err != nil {
		t.Fatalf("GetProject(persisted) error: %v", err)
	}
	if persisted.Name != "Focused Meeting" {
		t.Fatalf("persisted name = %q, want Focused Meeting", persisted.Name)
	}
	if persisted.RootPath != targetPath {
		t.Fatalf("persisted root_path = %q, want %q", persisted.RootPath, targetPath)
	}
	updatedWorkspace, err := app.store.GetWorkspace(workspace.ID)
	if err != nil {
		t.Fatalf("GetWorkspace(updated) error: %v", err)
	}
	if updatedWorkspace.Name != "Focused Meeting" {
		t.Fatalf("workspace name = %q, want Focused Meeting", updatedWorkspace.Name)
	}
	if updatedWorkspace.DirPath != targetPath {
		t.Fatalf("workspace dir_path = %q, want %q", updatedWorkspace.DirPath, targetPath)
	}
	updatedArtifact, err := app.store.GetArtifact(artifact.ID)
	if err != nil {
		t.Fatalf("GetArtifact(updated) error: %v", err)
	}
	if updatedArtifact.RefPath == nil || *updatedArtifact.RefPath != filepath.Join(targetPath, "meeting-notes.md") {
		t.Fatalf("artifact ref_path = %v, want moved path", updatedArtifact.RefPath)
	}
	if _, err := os.Stat(filepath.Join(targetPath, "meeting-notes.md")); err != nil {
		t.Fatalf("Stat(moved artifact) error: %v", err)
	}
	updatedParticipantSession, err := app.store.GetParticipantSession(participantSession.ID)
	if err != nil {
		t.Fatalf("GetParticipantSession(updated) error: %v", err)
	}
	if updatedParticipantSession.WorkspacePath != targetPath {
		t.Fatalf("participant session workspace_path = %q, want %q", updatedParticipantSession.WorkspacePath, targetPath)
	}
}

func TestTemporaryProjectDiscardRemovesProjectDataAndFallsBackToDefaultProject(t *testing.T) {
	app := newAuthedTestApp(t)
	defaultProject, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensure default project: %v", err)
	}

	rrCreate := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/runtime/workspaces", map[string]any{
		"kind": "task",
	})
	if rrCreate.Code != http.StatusOK {
		t.Fatalf("expected create 200, got %d: %s", rrCreate.Code, rrCreate.Body.String())
	}
	var createPayload struct {
		Workspace struct {
			ID            string `json:"id"`
			WorkspacePath string `json:"workspace_path"`
			RootPath      string `json:"root_path"`
		} `json:"workspace"`
	}
	if err := json.Unmarshal(rrCreate.Body.Bytes(), &createPayload); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if createPayload.Workspace.ID == "" {
		t.Fatalf("expected created task project id")
	}
	if err := os.WriteFile(filepath.Join(createPayload.Workspace.RootPath, "run-output.md"), []byte("saved output"), 0o644); err != nil {
		t.Fatalf("WriteFile(run-output.md) error: %v", err)
	}
	chatSession, err := app.store.GetOrCreateChatSession(createPayload.Workspace.WorkspacePath)
	if err != nil {
		t.Fatalf("GetOrCreateChatSession() error: %v", err)
	}
	workspace, err := app.store.GetWorkspace(chatSession.WorkspaceID)
	if err != nil {
		t.Fatalf("GetWorkspace(chat workspace) error: %v", err)
	}
	if _, err := app.store.AddChatMessage(chatSession.ID, "assistant", "saved output", "saved output", "markdown"); err != nil {
		t.Fatalf("AddChatMessage() error: %v", err)
	}
	item, err := app.store.CreateItem("Temporary follow-up", store.ItemOptions{WorkspaceID: &workspace.ID})
	if err != nil {
		t.Fatalf("CreateItem() error: %v", err)
	}
	participantSession, err := app.store.AddParticipantSession(createPayload.Workspace.WorkspacePath, "{}")
	if err != nil {
		t.Fatalf("AddParticipantSession() error: %v", err)
	}
	if _, err := app.store.AddParticipantSegment(store.ParticipantSegment{
		SessionID: participantSession.ID,
		StartTS:   100,
		EndTS:     101,
		Text:      "transcript only",
		Status:    "final",
	}); err != nil {
		t.Fatalf("AddParticipantSegment() error: %v", err)
	}
	if err := app.store.UpsertParticipantRoomState(participantSession.ID, "summary", `["Acme"]`, `["Decision"]`); err != nil {
		t.Fatalf("UpsertParticipantRoomState() error: %v", err)
	}

	rrDiscard := doAuthedJSONRequest(
		t,
		app.Router(),
		http.MethodPost,
		"/api/runtime/workspaces/"+createPayload.Workspace.ID+"/discard",
		map[string]any{},
	)
	if rrDiscard.Code != http.StatusOK {
		t.Fatalf("expected discard 200, got %d: %s", rrDiscard.Code, rrDiscard.Body.String())
	}
	var discardPayload struct {
		OK                bool   `json:"ok"`
		ActiveWorkspaceID string `json:"active_workspace_id"`
		ActiveWorkspace   struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		} `json:"active_workspace"`
	}
	if err := json.Unmarshal(rrDiscard.Body.Bytes(), &discardPayload); err != nil {
		t.Fatalf("decode discard response: %v", err)
	}
	if !discardPayload.OK {
		t.Fatalf("expected discard ok=true")
	}
	if discardPayload.ActiveWorkspaceID != workspaceIDStr(defaultProject.ID) {
		t.Fatalf("active_workspace_id = %q, want %q", discardPayload.ActiveWorkspaceID, workspaceIDStr(defaultProject.ID))
	}
	if discardPayload.ActiveWorkspace.Kind != defaultProject.Kind {
		t.Fatalf("active project kind = %q, want %q", discardPayload.ActiveWorkspace.Kind, defaultProject.Kind)
	}
	if _, err := app.store.GetEnrichedWorkspace(createPayload.Workspace.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetProject(discarded) error = %v, want sql.ErrNoRows", err)
	}
	if _, err := app.store.GetChatSession(chatSession.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetChatSession(discarded) error = %v, want sql.ErrNoRows", err)
	}
	if _, err := app.store.GetWorkspaceByPath(createPayload.Workspace.RootPath); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetWorkspaceByPath(discarded root) error = %v, want sql.ErrNoRows", err)
	}
	if _, err := app.store.GetParticipantSession(participantSession.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetParticipantSession(discarded) error = %v, want sql.ErrNoRows", err)
	}
	if _, err := os.Stat(createPayload.Workspace.RootPath); !os.IsNotExist(err) {
		t.Fatalf("temporary workspace root still exists: %v", err)
	}
	survivingItem, err := app.store.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem(surviving) error: %v", err)
	}
	if survivingItem.WorkspaceID != nil {
		t.Fatalf("surviving item workspace_id = %v, want nil", survivingItem.WorkspaceID)
	}
	if survivingItem.WorkspaceID != nil {
		t.Fatalf("surviving item workspace_id = %v, want nil", survivingItem.WorkspaceID)
	}
	if survivingItem.Sphere != store.SpherePrivate {
		t.Fatalf("surviving item sphere = %q, want %q", survivingItem.Sphere, store.SpherePrivate)
	}
}
