package web

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

type itemAssignRequest struct {
	ActorID int64 `json:"actor_id"`
}

type itemCompleteRequest struct {
	ActorID int64 `json:"actor_id"`
}

func parseItemIDParam(r *http.Request) (int64, error) {
	itemID := strings.TrimSpace(chi.URLParam(r, "item_id"))
	if itemID == "" {
		return 0, errors.New("missing item_id")
	}
	return strconv.ParseInt(itemID, 10, 64)
}

func itemResponseErrorStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}
	if errors.Is(err, sql.ErrNoRows) {
		return http.StatusNotFound
	}
	return http.StatusBadRequest
}

func writeItemStoreError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	http.Error(w, err.Error(), itemResponseErrorStatus(err))
}

func (a *App) handleItemAssign(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	itemID, err := parseItemIDParam(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req itemAssignRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.ActorID <= 0 {
		http.Error(w, "actor_id is required", http.StatusBadRequest)
		return
	}
	if _, err := a.store.GetActor(req.ActorID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "actor not found", http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := a.store.AssignItem(itemID, req.ActorID); err != nil {
		writeItemStoreError(w, err)
		return
	}
	item, err := a.store.GetItem(itemID)
	if err != nil {
		writeItemStoreError(w, err)
		return
	}
	writeJSON(w, map[string]interface{}{
		"ok":   true,
		"item": item,
	})
}

func (a *App) handleItemUnassign(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	itemID, err := parseItemIDParam(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.store.UnassignItem(itemID); err != nil {
		writeItemStoreError(w, err)
		return
	}
	item, err := a.store.GetItem(itemID)
	if err != nil {
		writeItemStoreError(w, err)
		return
	}
	writeJSON(w, map[string]interface{}{
		"ok":   true,
		"item": item,
	})
}

func (a *App) handleItemComplete(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	itemID, err := parseItemIDParam(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req itemCompleteRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.ActorID <= 0 {
		http.Error(w, "actor_id is required", http.StatusBadRequest)
		return
	}
	if _, err := a.store.GetActor(req.ActorID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "actor not found", http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := a.store.CompleteItemByActor(itemID, req.ActorID); err != nil {
		writeItemStoreError(w, err)
		return
	}
	item, err := a.store.GetItem(itemID)
	if err != nil {
		writeItemStoreError(w, err)
		return
	}
	writeJSON(w, map[string]interface{}{
		"ok":   true,
		"item": item,
	})
}
