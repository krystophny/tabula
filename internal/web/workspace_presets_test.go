package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/sloppy-org/slopshell/internal/store"
)

type workspacePresetEntry struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Sphere    string `json:"sphere"`
	RootPath  string `json:"root_path"`
	Available bool   `json:"available"`
}

type workspacePresetsListResponse struct {
	OK      bool                   `json:"ok"`
	Presets []workspacePresetEntry `json:"presets"`
}

type workspacePresetActivateResponse struct {
	OK                bool                 `json:"ok"`
	ActiveWorkspaceID string               `json:"active_workspace_id"`
	ActiveSphere      string               `json:"active_sphere"`
	Preset            workspacePresetEntry `json:"preset"`
	Workspace         workspaceListEntry   `json:"workspace"`
}

func findPreset(presets []workspacePresetEntry, id string) *workspacePresetEntry {
	for i := range presets {
		if presets[i].ID == id {
			return &presets[i]
		}
	}
	return nil
}

func TestBrainPresetsListResolvesFromEnv(t *testing.T) {
	workRoot := filepath.Join(t.TempDir(), "nextcloud-brain")
	privateRoot := filepath.Join(t.TempDir(), "dropbox-brain")
	if err := os.MkdirAll(workRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(workRoot): %v", err)
	}
	if err := os.MkdirAll(privateRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(privateRoot): %v", err)
	}
	t.Setenv("SLOPSHELL_BRAIN_WORK_ROOT", workRoot)
	t.Setenv("SLOPSHELL_BRAIN_PRIVATE_ROOT", privateRoot)

	app := newAuthedTestApp(t)

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/runtime/workspace-presets", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var payload workspacePresetsListResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !payload.OK {
		t.Fatalf("ok=false")
	}
	work := findPreset(payload.Presets, brainPresetIDWork)
	if work == nil {
		t.Fatalf("work preset missing: %#v", payload.Presets)
	}
	if work.RootPath != workRoot {
		t.Fatalf("work root = %q, want %q", work.RootPath, workRoot)
	}
	if !work.Available {
		t.Fatalf("work preset not marked available")
	}
	if work.Sphere != store.SphereWork {
		t.Fatalf("work sphere = %q, want %q", work.Sphere, store.SphereWork)
	}
	priv := findPreset(payload.Presets, brainPresetIDPrivate)
	if priv == nil {
		t.Fatalf("private preset missing: %#v", payload.Presets)
	}
	if priv.RootPath != privateRoot {
		t.Fatalf("private root = %q, want %q", priv.RootPath, privateRoot)
	}
	if !priv.Available {
		t.Fatalf("private preset not marked available")
	}
	if priv.Sphere != store.SpherePrivate {
		t.Fatalf("private sphere = %q, want %q", priv.Sphere, store.SpherePrivate)
	}
}

func TestBrainPresetsPreferSloptoolsVaultConfig(t *testing.T) {
	rootDir := t.TempDir()
	workVault := filepath.Join(rootDir, "work-vault")
	privateVault := filepath.Join(rootDir, "private-vault")
	if err := os.MkdirAll(filepath.Join(workVault, "brain"), 0o755); err != nil {
		t.Fatalf("MkdirAll(work brain): %v", err)
	}
	if err := os.MkdirAll(filepath.Join(privateVault, "brain"), 0o755); err != nil {
		t.Fatalf("MkdirAll(private brain): %v", err)
	}
	configPath := filepath.Join(rootDir, "vaults.toml")
	config := `[[vault]]
sphere = "work"
root = "` + workVault + `"
brain = "brain"

[[vault]]
sphere = "private"
root = "` + privateVault + `"
brain = "brain"
`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("write vault config: %v", err)
	}
	t.Setenv("SLOPTOOLS_VAULT_CONFIG", configPath)
	t.Setenv("SLOPSHELL_BRAIN_WORK_ROOT", filepath.Join(rootDir, "env-work"))
	t.Setenv("SLOPSHELL_BRAIN_PRIVATE_ROOT", filepath.Join(rootDir, "env-private"))

	app := newAuthedTestApp(t)
	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/runtime/workspace-presets", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var payload workspacePresetsListResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	work := findPreset(payload.Presets, brainPresetIDWork)
	if work == nil || !work.Available {
		t.Fatalf("work preset missing or unavailable: %#v", work)
	}
	if work.RootPath != filepath.Join(workVault, "brain") {
		t.Fatalf("work root = %q, want %q", work.RootPath, filepath.Join(workVault, "brain"))
	}
	priv := findPreset(payload.Presets, brainPresetIDPrivate)
	if priv == nil || !priv.Available {
		t.Fatalf("private preset missing or unavailable: %#v", priv)
	}
	if priv.RootPath != filepath.Join(privateVault, "brain") {
		t.Fatalf("private root = %q, want %q", priv.RootPath, filepath.Join(privateVault, "brain"))
	}
}

func TestBrainPresetsPreferBrainConfigGetOverEnv(t *testing.T) {
	workRoot := filepath.Join(t.TempDir(), "mcp-work", "brain")
	privateRoot := filepath.Join(t.TempDir(), "mcp-private", "brain")
	if err := os.MkdirAll(workRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(workRoot): %v", err)
	}
	if err := os.MkdirAll(privateRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(privateRoot): %v", err)
	}

	mcpCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mcpCalls++
		if req["method"] != "tools/call" {
			http.Error(w, "unexpected method", http.StatusBadRequest)
			return
		}
		params, _ := req["params"].(map[string]any)
		if got := params["name"]; got != "brain.config.get" {
			http.Error(w, "unexpected tool", http.StatusBadRequest)
			return
		}
		resp := map[string]any{
			"jsonrpc": "2.0",
			"id":      req["id"],
			"result": map[string]any{
				"structuredContent": map[string]any{
					"vaults": []any{
						map[string]any{"sphere": "work", "root": filepath.Dir(workRoot), "brain": "brain"},
						map[string]any{"sphere": "private", "root": filepath.Dir(privateRoot), "brain": "brain"},
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	t.Setenv("SLOPSHELL_BRAIN_WORK_ROOT", filepath.Join(t.TempDir(), "env-work"))
	t.Setenv("SLOPSHELL_BRAIN_PRIVATE_ROOT", filepath.Join(t.TempDir(), "env-private"))

	app := newAuthedTestApp(t)
	app.tunnels.setEndpoint(LocalSessionID, mcpEndpoint{httpURL: server.URL})

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/runtime/workspace-presets", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	if mcpCalls == 0 {
		t.Fatal("brain.config.get was not called")
	}
	var payload workspacePresetsListResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	work := findPreset(payload.Presets, brainPresetIDWork)
	if work == nil || !work.Available {
		t.Fatalf("work preset missing or unavailable: %#v", work)
	}
	if work.RootPath != workRoot {
		t.Fatalf("work root = %q, want %q", work.RootPath, workRoot)
	}
	priv := findPreset(payload.Presets, brainPresetIDPrivate)
	if priv == nil || !priv.Available {
		t.Fatalf("private preset missing or unavailable: %#v", priv)
	}
	if priv.RootPath != privateRoot {
		t.Fatalf("private root = %q, want %q", priv.RootPath, privateRoot)
	}
}

func TestBrainPresetsUnavailableWhenPathMissing(t *testing.T) {
	t.Setenv("SLOPSHELL_BRAIN_WORK_ROOT", "")
	missingPath := filepath.Join(t.TempDir(), "does-not-exist")
	t.Setenv("SLOPSHELL_BRAIN_PRIVATE_ROOT", missingPath)

	app := newAuthedTestApp(t)
	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/runtime/workspace-presets", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var payload workspacePresetsListResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	work := findPreset(payload.Presets, brainPresetIDWork)
	if work == nil || work.Available {
		t.Fatalf("expected work preset listed but unavailable; got %#v", work)
	}
	if work.RootPath != "" {
		t.Fatalf("work root_path = %q, want empty when env is unset", work.RootPath)
	}
	priv := findPreset(payload.Presets, brainPresetIDPrivate)
	if priv == nil || priv.Available {
		t.Fatalf("expected private preset listed but unavailable; got %#v", priv)
	}
}

func TestWorkspaceListIncludesPresetsAndActiveSphere(t *testing.T) {
	workRoot := filepath.Join(t.TempDir(), "nextcloud-brain")
	if err := os.MkdirAll(workRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(workRoot): %v", err)
	}
	t.Setenv("SLOPSHELL_BRAIN_WORK_ROOT", workRoot)

	app := newAuthedTestApp(t)
	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/runtime/workspaces", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var raw map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := raw["active_sphere"]; !ok {
		t.Fatalf("active_sphere missing from workspaces list response: %s", rr.Body.String())
	}
	presetsAny, ok := raw["presets"].([]any)
	if !ok {
		t.Fatalf("presets missing or wrong type in workspaces list response: %s", rr.Body.String())
	}
	foundWork := false
	for _, p := range presetsAny {
		entry, ok := p.(map[string]any)
		if !ok {
			continue
		}
		if entry["id"] == brainPresetIDWork && entry["available"] == true {
			foundWork = true
		}
	}
	if !foundWork {
		t.Fatalf("workspaces list missing available work preset: %s", rr.Body.String())
	}
}

func TestBrainPresetActivateCreatesAndActivatesWorkspace(t *testing.T) {
	workRoot := filepath.Join(t.TempDir(), "nextcloud-brain")
	if err := os.MkdirAll(workRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(workRoot): %v", err)
	}
	t.Setenv("SLOPSHELL_BRAIN_WORK_ROOT", workRoot)

	app := newAuthedTestApp(t)
	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/runtime/workspace-presets/brain.work/activate", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("activate status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var payload workspacePresetActivateResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !payload.OK {
		t.Fatalf("ok=false: %s", rr.Body.String())
	}
	if payload.ActiveSphere != store.SphereWork {
		t.Fatalf("active_sphere = %q, want %q", payload.ActiveSphere, store.SphereWork)
	}
	if payload.Workspace.ID == "" {
		t.Fatalf("workspace id missing")
	}
	if payload.Workspace.Sphere != store.SphereWork {
		t.Fatalf("workspace sphere = %q, want %q", payload.Workspace.Sphere, store.SphereWork)
	}
	if payload.Workspace.WorkspacePath != workRoot {
		t.Fatalf("workspace path = %q, want %q", payload.Workspace.WorkspacePath, workRoot)
	}
	if payload.Preset.ID != brainPresetIDWork {
		t.Fatalf("preset id = %q, want %q", payload.Preset.ID, brainPresetIDWork)
	}

	activeID, err := app.store.ActiveWorkspaceID()
	if err != nil {
		t.Fatalf("ActiveWorkspaceID: %v", err)
	}
	if activeID != payload.ActiveWorkspaceID {
		t.Fatalf("ActiveWorkspaceID = %q, want %q", activeID, payload.ActiveWorkspaceID)
	}
	currentSphere, err := app.store.ActiveSphere()
	if err != nil {
		t.Fatalf("ActiveSphere: %v", err)
	}
	if currentSphere != store.SphereWork {
		t.Fatalf("ActiveSphere = %q, want %q", currentSphere, store.SphereWork)
	}
}

func TestBrainPresetActivateReusesExistingWorkspace(t *testing.T) {
	privateRoot := filepath.Join(t.TempDir(), "dropbox-brain")
	if err := os.MkdirAll(privateRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(privateRoot): %v", err)
	}
	t.Setenv("SLOPSHELL_BRAIN_PRIVATE_ROOT", privateRoot)

	app := newAuthedTestApp(t)
	rrFirst := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/runtime/workspace-presets/brain.private/activate", nil)
	if rrFirst.Code != http.StatusOK {
		t.Fatalf("first activate status = %d: %s", rrFirst.Code, rrFirst.Body.String())
	}
	var first workspacePresetActivateResponse
	if err := json.Unmarshal(rrFirst.Body.Bytes(), &first); err != nil {
		t.Fatalf("decode first: %v", err)
	}

	rrSecond := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/runtime/workspace-presets/brain.private/activate", nil)
	if rrSecond.Code != http.StatusOK {
		t.Fatalf("second activate status = %d: %s", rrSecond.Code, rrSecond.Body.String())
	}
	var second workspacePresetActivateResponse
	if err := json.Unmarshal(rrSecond.Body.Bytes(), &second); err != nil {
		t.Fatalf("decode second: %v", err)
	}
	if first.Workspace.ID != second.Workspace.ID {
		t.Fatalf("preset re-activate created new workspace: first=%q second=%q", first.Workspace.ID, second.Workspace.ID)
	}
}

func TestBrainPresetActivateFailsWhenUnavailable(t *testing.T) {
	t.Setenv("SLOPSHELL_BRAIN_WORK_ROOT", "")
	app := newAuthedTestApp(t)
	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/runtime/workspace-presets/brain.work/activate", nil)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 when preset unavailable: %s", rr.Code, rr.Body.String())
	}
}

func TestBrainPresetActivateUnknownReturnsNotFound(t *testing.T) {
	app := newAuthedTestApp(t)
	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/runtime/workspace-presets/brain.unknown/activate", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for unknown preset: %s", rr.Code, rr.Body.String())
	}
}

func TestExplicitWorkspaceCreationStillWorksAlongsidePresets(t *testing.T) {
	workRoot := filepath.Join(t.TempDir(), "nextcloud-brain")
	if err := os.MkdirAll(workRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(workRoot): %v", err)
	}
	t.Setenv("SLOPSHELL_BRAIN_WORK_ROOT", workRoot)

	app := newAuthedTestApp(t)
	explicitPath := filepath.Join(t.TempDir(), "explicit-project")
	if err := os.MkdirAll(explicitPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(explicitPath): %v", err)
	}
	rrCreate := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/runtime/workspaces", map[string]any{
		"name": "explicit-project",
		"kind": "linked",
		"path": explicitPath,
	})
	if rrCreate.Code != http.StatusOK {
		t.Fatalf("explicit create status = %d: %s", rrCreate.Code, rrCreate.Body.String())
	}

	rrPresets := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/runtime/workspace-presets", nil)
	if rrPresets.Code != http.StatusOK {
		t.Fatalf("preset list status = %d", rrPresets.Code)
	}
	var presets workspacePresetsListResponse
	if err := json.Unmarshal(rrPresets.Body.Bytes(), &presets); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if work := findPreset(presets.Presets, brainPresetIDWork); work == nil || !work.Available {
		t.Fatalf("expected work preset available; got %#v", work)
	}
}
