package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sloppy-org/slopshell/internal/store"
)

const (
	gestureActionComplete = "complete"
	gestureActionDrop     = "drop"
	gestureActionDefer    = "defer"
	gestureActionDelegate = "delegate"

	gestureDropModeLocalOverlay = "local_overlay"
	gestureDropModeProjectClose = "project_close"
	gestureDropModeUpstream     = "upstream"
)

// itemGestureRequest is the body shape for POST /api/items/{id}/gesture.
//
// Action is required. FollowUpAt is required for `defer` and optional for the
// other actions. ActorID is required for `delegate`. DropUpstream forces
// `drop` to issue an upstream destructive action; the default is local-only
// drop (state→done) so external-source items never trigger upstream deletes
// without an explicit ask.
type itemGestureRequest struct {
	Action       string `json:"action"`
	FollowUpAt   string `json:"follow_up_at"`
	ActorID      int64  `json:"actor_id"`
	DropUpstream bool   `json:"drop_upstream"`
}

// itemGestureUndo is the snapshot returned with every gesture and accepted by
// the undo endpoint. Capturing prior state plus any executed sync-back lets
// the frontend reverse the local overlay AND any upstream archive that ran.
type itemGestureUndo struct {
	State            string  `json:"state"`
	ActorID          *int64  `json:"actor_id,omitempty"`
	VisibleAfter     *string `json:"visible_after,omitempty"`
	FollowUpAt       *string `json:"follow_up_at,omitempty"`
	EmailSyncBackRan bool    `json:"email_sync_back,omitempty"`
}

type itemGestureResult struct {
	Item             store.Item      `json:"item"`
	Action           string          `json:"action"`
	DropMode         string          `json:"drop_mode,omitempty"`
	EmailSyncBackRan bool            `json:"email_sync_back,omitempty"`
	Undo             itemGestureUndo `json:"undo"`
}

func (a *App) handleItemGesture(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	itemID, err := parseItemIDParam(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req itemGestureRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	item, err := a.store.GetItem(itemID)
	if err != nil {
		writeItemStoreError(w, err)
		return
	}
	result, status, err := a.applyItemGesture(r.Context(), item, action, req)
	if err != nil {
		writeAPIError(w, status, err.Error())
		return
	}
	writeAPIData(w, http.StatusOK, map[string]any{
		"item":            result.Item,
		"action":          result.Action,
		"drop_mode":       result.DropMode,
		"email_sync_back": result.EmailSyncBackRan,
		"undo":            result.Undo,
	})
}

// applyItemGesture mutates the item per the requested gesture and returns the
// new item plus an undo snapshot. Routing rules:
//
//   - `complete` runs upstream sync-back (todoist complete, email archive)
//     because the user is finishing the work in both places.
//   - `drop` is local-only by default (state→done) so external-source items
//     do not silently trigger upstream destruction. Pass DropUpstream to
//     force the destructive path. Project items never hard-delete; their
//     row is closed locally so child links stay queryable.
//   - `defer` writes both visible_after and follow_up_at to the same RFC3339
//     timestamp so the existing resurfacer treats the item consistently.
//   - `delegate` requires actor_id and stores follow_up_at when supplied.
//
// Markdown-backed items route every state change through brain.gtd.set_status
// (validated by brain.note.parse) before mirroring locally. That is the
// "validate after write-through" guarantee the gesture acceptance criteria
// requires; the local store row is only updated when the source markdown
// accepts the new status.
func (a *App) applyItemGesture(ctx context.Context, item store.Item, action string, req itemGestureRequest) (itemGestureResult, int, error) {
	snapshot := itemGestureUndo{
		State:        item.State,
		ActorID:      copyInt64Pointer(item.ActorID),
		VisibleAfter: copyStringPointer(item.VisibleAfter),
		FollowUpAt:   copyStringPointer(item.FollowUpAt),
	}
	switch action {
	case gestureActionComplete:
		return a.gestureComplete(ctx, item, snapshot)
	case gestureActionDrop:
		return a.gestureDrop(ctx, item, snapshot, req.DropUpstream)
	case gestureActionDefer:
		return a.gestureDefer(item, snapshot, req.FollowUpAt)
	case gestureActionDelegate:
		return a.gestureDelegate(item, snapshot, req)
	default:
		return itemGestureResult{}, http.StatusBadRequest, fmt.Errorf("action must be one of complete, drop, defer, delegate")
	}
}

func (a *App) gestureComplete(ctx context.Context, item store.Item, snapshot itemGestureUndo) (itemGestureResult, int, error) {
	if item.State == store.ItemStateDone {
		return a.gestureSnapshotResult(item, gestureActionComplete, "", false, snapshot), http.StatusOK, nil
	}
	syncRan, status, err := a.gestureWriteThroughClose(ctx, item)
	if err != nil {
		return itemGestureResult{}, status, err
	}
	if err := a.store.UpdateItemState(item.ID, store.ItemStateDone); err != nil {
		return itemGestureResult{}, itemResponseErrorStatus(err), err
	}
	updated, err := a.store.GetItem(item.ID)
	if err != nil {
		return itemGestureResult{}, itemResponseErrorStatus(err), err
	}
	snapshot.EmailSyncBackRan = syncRan
	return a.gestureSnapshotResult(updated, gestureActionComplete, "", syncRan, snapshot), http.StatusOK, nil
}

func (a *App) gestureDrop(ctx context.Context, item store.Item, snapshot itemGestureUndo, dropUpstream bool) (itemGestureResult, int, error) {
	mode := dropModeForItem(item, dropUpstream)
	syncRan := false
	if item.State != store.ItemStateDone {
		if mode == gestureDropModeUpstream {
			ran, status, err := a.gestureWriteThroughClose(ctx, item)
			if err != nil {
				return itemGestureResult{}, status, err
			}
			syncRan = ran
		} else {
			if _, status, err := a.gestureWriteThroughMarkdown(item, store.ItemStateDone); err != nil {
				return itemGestureResult{}, status, err
			}
		}
		if err := a.store.UpdateItemState(item.ID, store.ItemStateDone); err != nil {
			return itemGestureResult{}, itemResponseErrorStatus(err), err
		}
	}
	updated, err := a.store.GetItem(item.ID)
	if err != nil {
		return itemGestureResult{}, itemResponseErrorStatus(err), err
	}
	snapshot.EmailSyncBackRan = syncRan
	return a.gestureSnapshotResult(updated, gestureActionDrop, mode, syncRan, snapshot), http.StatusOK, nil
}

func (a *App) gestureDefer(item store.Item, snapshot itemGestureUndo, rawFollowUp string) (itemGestureResult, int, error) {
	follow, err := normalizeRequiredRFC3339(rawFollowUp)
	if err != nil {
		return itemGestureResult{}, http.StatusBadRequest, err
	}
	if _, status, err := a.gestureWriteThroughMarkdown(item, store.ItemStateDeferred); err != nil {
		return itemGestureResult{}, status, err
	}
	if err := a.store.UpdateItem(item.ID, store.ItemUpdate{
		State:        stringPointer(store.ItemStateDeferred),
		VisibleAfter: stringPointer(follow),
		FollowUpAt:   stringPointer(follow),
	}); err != nil {
		return itemGestureResult{}, itemResponseErrorStatus(err), err
	}
	updated, err := a.store.GetItem(item.ID)
	if err != nil {
		return itemGestureResult{}, itemResponseErrorStatus(err), err
	}
	return a.gestureSnapshotResult(updated, gestureActionDefer, "", false, snapshot), http.StatusOK, nil
}

func (a *App) gestureDelegate(item store.Item, snapshot itemGestureUndo, req itemGestureRequest) (itemGestureResult, int, error) {
	if req.ActorID <= 0 {
		return itemGestureResult{}, http.StatusBadRequest, errors.New("actor_id is required")
	}
	if err := a.ensureActorExists(req.ActorID); err != nil {
		if errors.Is(err, errItemActorNotFound) || errors.Is(err, errItemActorRequired) {
			return itemGestureResult{}, http.StatusBadRequest, err
		}
		return itemGestureResult{}, http.StatusInternalServerError, err
	}
	actorID := req.ActorID
	update := store.ItemUpdate{
		State:   stringPointer(store.ItemStateWaiting),
		ActorID: &actorID,
	}
	if strings.TrimSpace(req.FollowUpAt) != "" {
		follow, err := normalizeRequiredRFC3339(req.FollowUpAt)
		if err != nil {
			return itemGestureResult{}, http.StatusBadRequest, err
		}
		update.FollowUpAt = stringPointer(follow)
	}
	if _, status, err := a.gestureWriteThroughMarkdown(item, store.ItemStateWaiting); err != nil {
		return itemGestureResult{}, status, err
	}
	if err := a.store.UpdateItem(item.ID, update); err != nil {
		return itemGestureResult{}, itemResponseErrorStatus(err), err
	}
	updated, err := a.store.GetItem(item.ID)
	if err != nil {
		return itemGestureResult{}, itemResponseErrorStatus(err), err
	}
	return a.gestureSnapshotResult(updated, gestureActionDelegate, "", false, snapshot), http.StatusOK, nil
}

// gestureWriteThroughClose handles the close path for both markdown-backed
// items (validated brain.gtd.set_status) and external-backed items (todoist
// complete + email archive). It returns whether email archive ran so undo
// can move the message back, the HTTP status to return on error, and any
// error that should abort the gesture.
func (a *App) gestureWriteThroughClose(ctx context.Context, item store.Item) (bool, int, error) {
	if ran, status, err := a.gestureWriteThroughMarkdown(item, store.ItemStateDone); err != nil || ran {
		return false, status, err
	}
	syncRan, err := a.runItemGestureUpstreamComplete(ctx, item)
	if err != nil {
		return false, http.StatusBadGateway, err
	}
	return syncRan, http.StatusOK, nil
}

// gestureWriteThroughMarkdown writes a state change through to the source
// markdown via brain.gtd.set_status (validated by brain.note.parse) when the
// item resolves to a markdown-backed GTD target. Returns (true, ...) when the
// markdown write-through actually ran. Non-markdown items short-circuit so the
// caller can fall back to its existing path.
func (a *App) gestureWriteThroughMarkdown(item store.Item, targetState string) (bool, int, error) {
	target, ok, err := a.gtdStatusTarget(item)
	if err != nil {
		return false, http.StatusInternalServerError, err
	}
	if !ok {
		return false, http.StatusOK, nil
	}
	status, err := gtdStatusForGestureState(targetState)
	if err != nil {
		return false, http.StatusBadRequest, err
	}
	if _, _, err := a.setBrainGTDStatus(target, itemGTDStatusRequest{}, status); err != nil {
		return false, http.StatusBadGateway, err
	}
	return true, http.StatusOK, nil
}

// gtdStatusForGestureState maps the local item state a gesture is about to
// commit into the brain.gtd.set_status status vocabulary. The set is
// deliberately small: gestures only ever transition into closed, deferred, or
// waiting on the brain side; the local store retains the richer state ladder.
func gtdStatusForGestureState(targetState string) (string, error) {
	switch targetState {
	case store.ItemStateDone:
		return "closed", nil
	case store.ItemStateDeferred:
		return store.ItemStateDeferred, nil
	case store.ItemStateWaiting:
		return store.ItemStateWaiting, nil
	default:
		return "", fmt.Errorf("gesture cannot route state %q through brain.gtd.set_status", targetState)
	}
}

func (a *App) gestureSnapshotResult(item store.Item, action, dropMode string, syncRan bool, snapshot itemGestureUndo) itemGestureResult {
	return itemGestureResult{
		Item:             item,
		Action:           action,
		DropMode:         dropMode,
		EmailSyncBackRan: syncRan,
		Undo:             snapshot,
	}
}

// runItemGestureUpstreamComplete fires backend-specific sync-back when the
// gesture closes the item. Returns whether email archive ran so undo can
// move the message back to the inbox.
func (a *App) runItemGestureUpstreamComplete(ctx context.Context, item store.Item) (bool, error) {
	if todoistBackedItem(item) && item.State != store.ItemStateDone {
		if err := a.syncTodoistItemCompletion(item); err != nil {
			return false, err
		}
	}
	if !emailBackedItem(item) || item.State == store.ItemStateDone {
		return false, nil
	}
	if err := a.syncRemoteEmailItemState(ctx, item, store.ItemStateDone); err != nil {
		return false, err
	}
	return true, nil
}

// dropModeForItem chooses the routing for a `drop` gesture. Project items
// never hard-delete because that would cascade away child links and break
// the GTD outcome review. External-source items default to a local overlay
// drop so we never trigger an unintended remote destruction. Local items
// also drop into the local overlay (state→done) so undo stays cheap.
func dropModeForItem(item store.Item, dropUpstream bool) string {
	if item.Kind == store.ItemKindProject {
		return gestureDropModeProjectClose
	}
	if dropUpstream && hasExternalSource(item) {
		return gestureDropModeUpstream
	}
	return gestureDropModeLocalOverlay
}

func hasExternalSource(item store.Item) bool {
	source := strings.ToLower(strings.TrimSpace(stringFromPointer(item.Source)))
	if source == "" {
		return false
	}
	if isBrainGTDSource(source) {
		return true
	}
	if store.IsEmailProvider(source) || store.IsTaskProvider(source) {
		return true
	}
	switch source {
	case "github", "gitlab":
		return true
	}
	return false
}

// itemGestureUndoRequest is the body for POST /api/items/{id}/gesture/undo.
type itemGestureUndoRequest struct {
	Undo itemGestureUndo `json:"undo"`
}

func (a *App) handleItemGestureUndo(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	itemID, err := parseItemIDParam(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req itemGestureUndoRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	prev := strings.TrimSpace(req.Undo.State)
	if prev == "" {
		writeAPIError(w, http.StatusBadRequest, "undo.state is required")
		return
	}
	item, err := a.store.GetItem(itemID)
	if err != nil {
		writeItemStoreError(w, err)
		return
	}
	if req.Undo.EmailSyncBackRan {
		if err := a.syncRemoteEmailItemState(r.Context(), item, store.ItemStateInbox); err != nil {
			writeAPIError(w, http.StatusBadGateway, err.Error())
			return
		}
	}
	if err := a.store.RestoreItemFromGestureUndo(itemID, store.ItemGestureUndo{
		State:        prev,
		ActorID:      req.Undo.ActorID,
		VisibleAfter: req.Undo.VisibleAfter,
		FollowUpAt:   req.Undo.FollowUpAt,
	}); err != nil {
		writeItemStoreError(w, err)
		return
	}
	updated, err := a.store.GetItem(itemID)
	if err != nil {
		writeItemStoreError(w, err)
		return
	}
	writeAPIData(w, http.StatusOK, map[string]any{
		"item": updated,
	})
}

func normalizeRequiredRFC3339(value string) (string, error) {
	clean := strings.TrimSpace(value)
	if clean == "" {
		return "", errors.New("follow_up_at is required")
	}
	parsed, err := time.Parse(time.RFC3339Nano, clean)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339, clean)
		if err != nil {
			return "", errors.New("follow_up_at must be a valid RFC3339 timestamp")
		}
	}
	return parsed.UTC().Format(time.RFC3339Nano), nil
}

func stringPointer(value string) *string {
	v := value
	return &v
}

func copyStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	v := *value
	return &v
}
