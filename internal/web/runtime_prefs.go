package web

import (
	"net/http"
	"strings"
)

type runtimeYoloRequest struct {
	Enabled bool `json:"enabled"`
}

type runtimeDisclaimerAckRequest struct {
	Version string `json:"version"`
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
		"ok":      true,
		"enabled": req.Enabled,
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
