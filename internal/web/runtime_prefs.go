package web

import (
	"net/http"
	"strings"

	"github.com/krystophny/tabura/internal/store"
)

type runtimeYoloRequest struct {
	Enabled bool `json:"enabled"`
}

type runtimeDisclaimerAckRequest struct {
	Version string `json:"version"`
}

type runtimePreferencesRequest struct {
	SilentMode      *bool  `json:"silent_mode"`
	Tool            string `json:"tool"`
	StartupBehavior string `json:"startup_behavior"`
	ActiveSphere    string `json:"active_sphere"`
}

func normalizeRuntimeTool(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "highlight", "select":
		return "highlight"
	case "ink", "draw", "pen", "handwrite":
		return "ink"
	case "text_note", "text-note", "text", "note", "keyboard", "typing", "type":
		return "text_note"
	case "prompt", "voice", "talk", "mic", "audio":
		return "prompt"
	case "pointer", "point":
		return "pointer"
	default:
		return "pointer"
	}
}

func normalizeRuntimeStartupBehavior(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "hub_first":
		return "hub_first"
	default:
		return "hub_first"
	}
}

func normalizeRuntimeActiveSphere(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case store.SphereWork:
		return store.SphereWork
	case store.SpherePrivate:
		return store.SpherePrivate
	default:
		return ""
	}
}

func (a *App) silentModeEnabled() bool {
	if a == nil || a.store == nil {
		return false
	}
	value, err := a.store.AppState(appStateSilentModeKey)
	if err != nil {
		return false
	}
	return parseBoolString(value, false)
}

func (a *App) runtimeTool() string {
	if a == nil || a.store == nil {
		return "pointer"
	}
	value, err := a.store.AppState(appStateToolKey)
	if err != nil {
		return "pointer"
	}
	if strings.TrimSpace(value) == "" {
		return "pointer"
	}
	return normalizeRuntimeTool(value)
}

func (a *App) runtimeStartupBehavior() string {
	if a == nil || a.store == nil {
		return "hub_first"
	}
	value, err := a.store.AppState(appStateStartupBehaviorKey)
	if err != nil {
		return "hub_first"
	}
	return normalizeRuntimeStartupBehavior(value)
}

func (a *App) runtimeActiveSphere() string {
	if a == nil || a.store == nil {
		return store.SpherePrivate
	}
	value, err := a.store.ActiveSphere()
	if err != nil {
		return store.SpherePrivate
	}
	sphere := normalizeRuntimeActiveSphere(value)
	if sphere == "" {
		return store.SpherePrivate
	}
	return sphere
}

func (a *App) setSilentModeEnabled(enabled bool) error {
	if a == nil || a.store == nil {
		return nil
	}
	if enabled {
		return a.store.SetAppState(appStateSilentModeKey, "true")
	}
	return a.store.SetAppState(appStateSilentModeKey, "false")
}

func (a *App) setRuntimeTool(tool string) error {
	if a == nil || a.store == nil {
		return nil
	}
	return a.store.SetAppState(appStateToolKey, normalizeRuntimeTool(tool))
}

func (a *App) setRuntimeStartupBehavior(behavior string) error {
	if a == nil || a.store == nil {
		return nil
	}
	return a.store.SetAppState(appStateStartupBehaviorKey, normalizeRuntimeStartupBehavior(behavior))
}

func (a *App) setRuntimeActiveSphere(sphere string) error {
	if a == nil || a.store == nil {
		return nil
	}
	return a.setActiveSphereTracked(sphere, "sphere_switch")
}

func (a *App) handleRuntimePreferencesUpdate(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	var req runtimePreferencesRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.SilentMode != nil {
		if err := a.setSilentModeEnabled(*req.SilentMode); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if strings.TrimSpace(req.Tool) != "" {
		if err := a.setRuntimeTool(req.Tool); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if strings.TrimSpace(req.StartupBehavior) != "" {
		if err := a.setRuntimeStartupBehavior(req.StartupBehavior); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if strings.TrimSpace(req.ActiveSphere) != "" {
		if err := a.setRuntimeActiveSphere(req.ActiveSphere); err != nil {
			http.Error(w, "active sphere must be work or private", http.StatusBadRequest)
			return
		}
	}
	writeJSON(w, map[string]interface{}{
		"ok":               true,
		"silent_mode":      a.silentModeEnabled(),
		"tool":             a.runtimeTool(),
		"startup_behavior": a.runtimeStartupBehavior(),
		"active_sphere":    a.runtimeActiveSphere(),
	})
}

func (a *App) handleRuntimeYoloModeUpdate(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	var req runtimeYoloRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if err := a.setYoloModeEnabled(req.Enabled); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{
		"ok":               true,
		"enabled":          req.Enabled,
		"execution_policy": executionPolicyForSession("chat", req.Enabled).Name,
	})
}

func (a *App) handleRuntimeDisclaimerAck(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	var req runtimeDisclaimerAckRequest
	if r.ContentLength > 0 {
		if err := decodeJSON(r, &req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
	}
	version := strings.TrimSpace(req.Version)
	if version == "" {
		version = disclaimerVersionCurrent
	}
	if err := a.setDisclaimerAckVersion(version); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{
		"ok":                     true,
		"disclaimer_ack_version": version,
	})
}
