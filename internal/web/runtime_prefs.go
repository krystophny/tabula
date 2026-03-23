package web

import (
	"net/http"
	"strings"

	"github.com/krystophny/tabura/internal/store"
	"github.com/krystophny/tabura/internal/turn"
)

type runtimeYoloRequest struct {
	Enabled bool `json:"enabled"`
}

type runtimeDisclaimerAckRequest struct {
	Version string `json:"version"`
}

type runtimePreferencesRequest struct {
	SilentMode             *bool  `json:"silent_mode"`
	FastMode               *bool  `json:"fast_mode"`
	Tool                   string `json:"tool"`
	StartupBehavior        string `json:"startup_behavior"`
	ActiveSphere           string `json:"active_sphere"`
	TurnPolicyProfile      string `json:"turn_policy_profile"`
	TurnEvalLoggingEnabled *bool  `json:"turn_eval_logging_enabled"`
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
	case "resume_active", "workspace_first", "project_first":
		return "resume_active"
	default:
		return "resume_active"
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

func normalizeRuntimeTurnPolicyProfile(raw string) string {
	switch turn.Profile(strings.ToLower(strings.TrimSpace(raw))) {
	case turn.ProfilePatient:
		return string(turn.ProfilePatient)
	case turn.ProfileAssertive:
		return string(turn.ProfileAssertive)
	default:
		return string(turn.ProfileBalanced)
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

func (a *App) fastModeEnabled() bool {
	if a == nil || a.store == nil {
		return false
	}
	value, err := a.store.AppState(appStateFastModeKey)
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
		return "resume_active"
	}
	value, err := a.store.AppState(appStateStartupBehaviorKey)
	if err != nil {
		return "resume_active"
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

func (a *App) runtimeTurnPolicyProfile() string {
	if a == nil || a.store == nil {
		return string(turn.ProfileBalanced)
	}
	value, err := a.store.AppState(appStateTurnPolicyProfileKey)
	if err != nil {
		return string(turn.ProfileBalanced)
	}
	return normalizeRuntimeTurnPolicyProfile(value)
}

func (a *App) runtimeTurnEvalLoggingEnabled() bool {
	if a == nil || a.store == nil {
		return true
	}
	value, err := a.store.AppState(appStateTurnEvalLoggingKey)
	if err != nil {
		return true
	}
	if strings.TrimSpace(value) == "" {
		return true
	}
	return parseBoolString(value, true)
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

func (a *App) setFastModeEnabled(enabled bool) error {
	if a == nil || a.store == nil {
		return nil
	}
	if enabled {
		return a.store.SetAppState(appStateFastModeKey, "true")
	}
	return a.store.SetAppState(appStateFastModeKey, "false")
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

func (a *App) setRuntimeTurnPolicyProfile(profile string) error {
	if a == nil || a.store == nil {
		return nil
	}
	return a.store.SetAppState(appStateTurnPolicyProfileKey, normalizeRuntimeTurnPolicyProfile(profile))
}

func (a *App) setRuntimeTurnEvalLoggingEnabled(enabled bool) error {
	if a == nil || a.store == nil {
		return nil
	}
	if enabled {
		return a.store.SetAppState(appStateTurnEvalLoggingKey, "true")
	}
	return a.store.SetAppState(appStateTurnEvalLoggingKey, "false")
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
	if req.FastMode != nil {
		if err := a.setFastModeEnabled(*req.FastMode); err != nil {
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
	if strings.TrimSpace(req.TurnPolicyProfile) != "" {
		if err := a.setRuntimeTurnPolicyProfile(req.TurnPolicyProfile); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if req.TurnEvalLoggingEnabled != nil {
		if err := a.setRuntimeTurnEvalLoggingEnabled(*req.TurnEvalLoggingEnabled); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, map[string]interface{}{
		"ok":                        true,
		"silent_mode":               a.silentModeEnabled(),
		"fast_mode":                 a.fastModeEnabled(),
		"tool":                      a.runtimeTool(),
		"startup_behavior":          a.runtimeStartupBehavior(),
		"active_sphere":             a.runtimeActiveSphere(),
		"turn_policy_profile":       a.runtimeTurnPolicyProfile(),
		"turn_eval_logging_enabled": a.runtimeTurnEvalLoggingEnabled(),
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
