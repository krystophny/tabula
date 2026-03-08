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

type itemTriageRequest struct {
	Action       string `json:"action"`
	ActorID      int64  `json:"actor_id"`
	VisibleAfter string `json:"visible_after"`
}

var (
	errItemActorRequired = errors.New("actor_id is required")
	errItemActorNotFound = errors.New("actor not found")
)

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

func (a *App) ensureActorExists(actorID int64) error {
	if actorID <= 0 {
		return errItemActorRequired
	}
	if _, err := a.store.GetActor(actorID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errItemActorNotFound
		}
		return err
	}
	return nil
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
	if err := a.ensureActorExists(req.ActorID); err != nil {
		if errors.Is(err, errItemActorNotFound) || errors.Is(err, errItemActorRequired) {
			http.Error(w, err.Error(), http.StatusBadRequest)
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
	if err := a.ensureActorExists(req.ActorID); err != nil {
		if errors.Is(err, errItemActorNotFound) || errors.Is(err, errItemActorRequired) {
			http.Error(w, err.Error(), http.StatusBadRequest)
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

func (a *App) handleItemTriage(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	itemID, err := parseItemIDParam(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req itemTriageRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	switch strings.ToLower(strings.TrimSpace(req.Action)) {
	case "done":
		err = a.store.TriageItemDone(itemID)
	case "later":
		if strings.TrimSpace(req.VisibleAfter) == "" {
			http.Error(w, "visible_after is required", http.StatusBadRequest)
			return
		}
		err = a.store.TriageItemLater(itemID, req.VisibleAfter)
	case "delegate":
		if err := a.ensureActorExists(req.ActorID); err != nil {
			if errors.Is(err, errItemActorNotFound) || errors.Is(err, errItemActorRequired) {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		err = a.store.TriageItemDelegate(itemID, req.ActorID)
	case "delete":
		err = a.store.TriageItemDelete(itemID)
	case "someday":
		err = a.store.TriageItemSomeday(itemID)
	default:
		http.Error(w, "action must be one of done, later, delegate, delete, someday", http.StatusBadRequest)
		return
	}
	if err != nil {
		writeItemStoreError(w, err)
		return
	}
	if strings.EqualFold(req.Action, "delete") {
		writeJSON(w, map[string]interface{}{
			"ok":      true,
			"deleted": true,
			"item_id": itemID,
		})
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
