package web

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/krystophny/tabura/internal/store"
)

func writeProjectLookupError(w http.ResponseWriter, err error) {
	switch {
	case err == nil:
		return
	case errors.Is(err, errItemProjectNotFound), errors.Is(err, sql.ErrNoRows):
		writeAPIError(w, http.StatusNotFound, "project not found")
	default:
		writeDomainStoreError(w, err)
	}
}

func (a *App) handleProjectWorkspacesList(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	projectID := chi.URLParam(r, "project_id")
	if err := a.ensureProjectExists(projectID); err != nil {
		writeProjectLookupError(w, err)
		return
	}
	workspaces, err := a.store.ListWorkspacesForProject(projectID)
	if err != nil {
		writeDomainStoreError(w, err)
		return
	}
	writeAPIData(w, http.StatusOK, map[string]any{
		"workspaces": workspaces,
	})
}

func (a *App) handleProjectItemsList(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	projectID := chi.URLParam(r, "project_id")
	if err := a.ensureProjectExists(projectID); err != nil {
		writeProjectLookupError(w, err)
		return
	}
	items, err := a.store.ListItemsFiltered(store.ItemListFilter{ProjectID: &projectID})
	if err != nil {
		writeDomainStoreError(w, err)
		return
	}
	writeAPIData(w, http.StatusOK, map[string]any{
		"items": items,
	})
}
