package web

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/krystophny/tabura/internal/store"
)

type externalAccountCreateRequest struct {
	Sphere   string         `json:"sphere"`
	Provider string         `json:"provider"`
	Label    string         `json:"label"`
	Config   map[string]any `json:"config"`
	Enabled  *bool          `json:"enabled,omitempty"`
}

type externalAccountUpdateRequest struct {
	Sphere   *string        `json:"sphere,omitempty"`
	Provider *string        `json:"provider,omitempty"`
	Label    *string        `json:"label,omitempty"`
	Config   map[string]any `json:"config,omitempty"`
	Enabled  *bool          `json:"enabled,omitempty"`
}

func normalizeExternalAccountScope(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case store.SphereWork:
		return store.SphereWork
	case store.SpherePrivate:
		return store.SpherePrivate
	default:
		return ""
	}
}

func externalAccountScope(r *http.Request) (string, error) {
	if pathScope := strings.TrimSpace(chi.URLParam(r, "sphere")); pathScope != "" {
		scope := normalizeExternalAccountScope(pathScope)
		if scope == "" {
			return "", errors.New("sphere must be work or private")
		}
		return scope, nil
	}
	queryScope := strings.TrimSpace(r.URL.Query().Get("sphere"))
	if queryScope == "" {
		return "", nil
	}
	scope := normalizeExternalAccountScope(queryScope)
	if scope == "" {
		return "", errors.New("sphere must be work or private")
	}
	return scope, nil
}

func (a *App) handleExternalAccountList(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	sphere, err := externalAccountScope(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	accounts, err := a.store.ListExternalAccounts(sphere)
	if err != nil {
		writeDomainStoreError(w, err)
		return
	}
	writeAPIData(w, http.StatusOK, map[string]any{
		"accounts": accounts,
	})
}

func (a *App) handleExternalAccountCreate(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	var req externalAccountCreateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	sphere, err := externalAccountScope(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	if sphere != "" {
		if req.Sphere != "" && normalizeExternalAccountScope(req.Sphere) != sphere {
			writeAPIError(w, http.StatusBadRequest, "sphere must match the scoped route")
			return
		}
		req.Sphere = sphere
	}
	account, err := a.store.CreateExternalAccount(req.Sphere, req.Provider, req.Label, req.Config)
	if err != nil {
		writeDomainStoreError(w, err)
		return
	}
	if req.Enabled != nil && !*req.Enabled {
		if err := a.store.UpdateExternalAccount(account.ID, store.ExternalAccountUpdate{Enabled: req.Enabled}); err != nil {
			writeDomainStoreError(w, err)
			return
		}
		account, err = a.store.GetExternalAccount(account.ID)
		if err != nil {
			writeDomainStoreError(w, err)
			return
		}
	}
	writeAPIData(w, http.StatusCreated, map[string]any{
		"account": account,
	})
}

func (a *App) handleExternalAccountUpdate(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	accountID, err := parseURLInt64Param(r, "account_id")
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req externalAccountUpdateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := a.store.UpdateExternalAccount(accountID, store.ExternalAccountUpdate{
		Sphere:   req.Sphere,
		Provider: req.Provider,
		Label:    req.Label,
		Config:   req.Config,
		Enabled:  req.Enabled,
	}); err != nil {
		writeDomainStoreError(w, err)
		return
	}
	account, err := a.store.GetExternalAccount(accountID)
	if err != nil {
		writeDomainStoreError(w, err)
		return
	}
	writeAPIData(w, http.StatusOK, map[string]any{
		"account": account,
	})
}

func (a *App) handleExternalAccountDelete(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	accountID, err := parseURLInt64Param(r, "account_id")
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	sphere, err := externalAccountScope(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	if sphere != "" {
		account, err := a.store.GetExternalAccount(accountID)
		if err != nil {
			writeDomainStoreError(w, err)
			return
		}
		if account.Sphere != sphere {
			writeAPIError(w, http.StatusNotFound, "external account not found")
			return
		}
	}
	if err := a.store.DeleteExternalAccount(accountID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeAPIError(w, http.StatusNotFound, "external account not found")
			return
		}
		writeDomainStoreError(w, err)
		return
	}
	writeNoContent(w)
}
