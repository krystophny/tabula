package web

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/sloppy-org/slopshell/internal/store"
)

type brainPreset struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Sphere    string `json:"sphere"`
	RootPath  string `json:"root_path"`
	Available bool   `json:"available"`
}

const (
	brainPresetIDWork    = "brain.work"
	brainPresetIDPrivate = "brain.private"
)

func (a *App) brainPresetRootEnv(presetID string) string {
	switch presetID {
	case brainPresetIDWork:
		return strings.TrimSpace(os.Getenv("SLOPSHELL_BRAIN_WORK_ROOT"))
	case brainPresetIDPrivate:
		return strings.TrimSpace(os.Getenv("SLOPSHELL_BRAIN_PRIVATE_ROOT"))
	}
	return ""
}

func (a *App) brainPresets() []brainPreset {
	defs := []struct {
		id     string
		label  string
		sphere string
	}{
		{brainPresetIDWork, "Work brain", store.SphereWork},
		{brainPresetIDPrivate, "Private brain", store.SpherePrivate},
	}
	configRoots := a.brainPresetRoots()
	out := make([]brainPreset, 0, len(defs))
	for _, def := range defs {
		preset := brainPreset{
			ID:     def.id,
			Label:  def.label,
			Sphere: def.sphere,
		}
		root := strings.TrimSpace(configRoots[def.sphere])
		if root == "" {
			root = a.brainPresetRootEnv(def.id)
		}
		if root != "" {
			preset.RootPath = filepath.Clean(root)
			preset.Available = presetRootAvailable(preset.RootPath)
		}
		out = append(out, preset)
	}
	return out
}

func (a *App) findBrainPreset(presetID string) (brainPreset, bool) {
	id := strings.TrimSpace(presetID)
	for _, p := range a.brainPresets() {
		if p.ID == id {
			return p, true
		}
	}
	return brainPreset{}, false
}

func (a *App) handleWorkspacePresetsList(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	writeJSON(w, map[string]interface{}{
		"ok":      true,
		"presets": a.brainPresets(),
	})
}

func (a *App) activateBrainPreset(presetID string) (store.Workspace, brainPreset, error) {
	preset, ok := a.findBrainPreset(presetID)
	if !ok {
		return store.Workspace{}, brainPreset{}, errors.New("unknown workspace preset")
	}
	if !preset.Available {
		return store.Workspace{}, preset, errors.New("preset path is not configured or does not exist")
	}
	project, _, err := a.createWorkspace2(runtimeWorkspaceCreateRequest{
		Name: preset.Label,
		Kind: "linked",
		Path: preset.RootPath,
	})
	if err != nil {
		return store.Workspace{}, preset, err
	}
	if err := a.applyBrainPresetSphere(project, preset.Sphere); err != nil {
		return store.Workspace{}, preset, err
	}
	if err := a.store.SetActiveSphere(preset.Sphere); err != nil {
		return store.Workspace{}, preset, err
	}
	activated, err := a.activateWorkspace(workspaceIDStr(project.ID))
	if err != nil {
		return store.Workspace{}, preset, err
	}
	return activated, preset, nil
}

func (a *App) applyBrainPresetSphere(project store.Workspace, sphere string) error {
	cleanSphere := strings.TrimSpace(sphere)
	if cleanSphere == "" {
		return nil
	}
	workspace, err := a.ensureWorkspaceReady(project, false)
	if err != nil {
		return err
	}
	if strings.EqualFold(workspace.Sphere, cleanSphere) {
		return nil
	}
	if _, err := a.store.SetWorkspaceSphere(workspace.ID, cleanSphere); err != nil {
		return err
	}
	return nil
}

func (a *App) handleWorkspacePresetActivate(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	presetID := strings.TrimSpace(chi.URLParam(r, "preset_id"))
	if presetID == "" {
		http.Error(w, "preset_id is required", http.StatusBadRequest)
		return
	}
	project, preset, err := a.activateBrainPreset(presetID)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "unknown workspace preset"):
			http.Error(w, err.Error(), http.StatusNotFound)
		case strings.Contains(err.Error(), "not configured"):
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		default:
			http.Error(w, err.Error(), http.StatusBadGateway)
		}
		return
	}
	item, err := a.buildWorkspaceAPIModel(project)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{
		"ok":                  true,
		"preset":              preset,
		"active_workspace_id": workspaceIDStr(project.ID),
		"active_sphere":       a.runtimeActiveSphere(),
		"workspace":           item,
	})
}
