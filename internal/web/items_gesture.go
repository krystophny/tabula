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
// the frontend reverse the local overlay AND any upstream side-effect that
// ran (email archive, brain.gtd.set_status markdown write-through).
type itemGestureUndo struct {
	State               string  `json:"state"`
	ActorID             *int64  `json:"actor_id,omitempty"`
	VisibleAfter        *string `json:"visible_after,omitempty"`
	FollowUpAt          *string `json:"follow_up_at,omitempty"`
	EmailSyncBackRan    bool    `json:"email_sync_back,omitempty"`
	MarkdownSyncBackRan bool    `json:"markdown_sync_back,omitempty"`
}

// gestureSyncBack records which write-through paths ran for a single gesture
// so the undo handler can reverse exactly the side-effects that happened.
type gestureSyncBack struct {
	Markdown bool
	Email    bool
}

type itemGestureResult struct {
	Item                store.Item                  `json:"item"`
	Action              string                      `json:"action"`
	DropMode            string                      `json:"drop_mode,omitempty"`
	EmailSyncBackRan    bool                        `json:"email_sync_back,omitempty"`
	MarkdownSyncBackRan bool                        `json:"markdown_sync_back,omitempty"`
	SyncError           string                      `json:"sync_error,omitempty"`
	ParentProjectHealth []gestureParentProjectState `json:"parent_project_health,omitempty"`
	Undo                itemGestureUndo             `json:"undo"`
}

// gestureParentProjectState pairs a parent project-item's id with its newly
// recomputed health so the UI can refresh the outcome row in place after a
// child action completes. Health stays a derived view; we never mutate the
// parent state from the gesture endpoint.
type gestureParentProjectState struct {
	ProjectItemID int64                    `json:"project_item_id"`
	Health        store.ProjectItemHealth  `json:"health"`
	Counts        store.ProjectChildCounts `json:"counts"`
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
	payload := map[string]any{
		"item":               result.Item,
		"action":             result.Action,
		"drop_mode":          result.DropMode,
		"email_sync_back":    result.EmailSyncBackRan,
		"markdown_sync_back": result.MarkdownSyncBackRan,
		"undo":               result.Undo,
	}
	if result.SyncError != "" {
		payload["sync_error"] = result.SyncError
	}
	if len(result.ParentProjectHealth) > 0 {
		payload["parent_project_health"] = result.ParentProjectHealth
	}
	writeAPIData(w, http.StatusOK, payload)
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
		return a.gestureSnapshotResult(item, gestureActionComplete, "", gestureSyncBack{}, snapshot), http.StatusOK, nil
	}
	mdRan, status, err := a.gestureWriteThroughMarkdown(item, store.ItemStateDone)
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
	sync := gestureSyncBack{Markdown: mdRan}
	syncErr := ""
	if !mdRan {
		emailRan, err := a.runItemGestureUpstreamComplete(ctx, item)
		sync.Email = emailRan
		if err != nil {
			syncErr = err.Error()
		}
	}
	snapshot.EmailSyncBackRan = sync.Email
	snapshot.MarkdownSyncBackRan = sync.Markdown
	result := a.gestureSnapshotResult(updated, gestureActionComplete, "", sync, snapshot)
	result.SyncError = syncErr
	parents, err := a.parentProjectHealthForChild(item)
	if err != nil {
		return itemGestureResult{}, http.StatusInternalServerError, err
	}
	result.ParentProjectHealth = parents
	return result, http.StatusOK, nil
}

func (a *App) gestureDrop(ctx context.Context, item store.Item, snapshot itemGestureUndo, dropUpstream bool) (itemGestureResult, int, error) {
	mode := dropModeForItem(item, dropUpstream)
	var sync gestureSyncBack
	if item.State != store.ItemStateDone {
		if mode == gestureDropModeUpstream {
			ran, status, err := a.gestureWriteThroughClose(ctx, item)
			if err != nil {
				return itemGestureResult{}, status, err
			}
			sync = ran
		} else {
			mdRan, status, err := a.gestureWriteThroughMarkdown(item, store.ItemStateDone)
			if err != nil {
				return itemGestureResult{}, status, err
			}
			sync.Markdown = mdRan
		}
		if err := a.store.UpdateItemState(item.ID, store.ItemStateDone); err != nil {
			return itemGestureResult{}, itemResponseErrorStatus(err), err
		}
	}
	updated, err := a.store.GetItem(item.ID)
	if err != nil {
		return itemGestureResult{}, itemResponseErrorStatus(err), err
	}
	snapshot.EmailSyncBackRan = sync.Email
	snapshot.MarkdownSyncBackRan = sync.Markdown
	return a.gestureSnapshotResult(updated, gestureActionDrop, mode, sync, snapshot), http.StatusOK, nil
}

func (a *App) gestureDefer(item store.Item, snapshot itemGestureUndo, rawFollowUp string) (itemGestureResult, int, error) {
	follow, err := normalizeRequiredRFC3339(rawFollowUp)
	if err != nil {
		return itemGestureResult{}, http.StatusBadRequest, err
	}
	mdRan, status, err := a.gestureWriteThroughMarkdown(item, store.ItemStateDeferred)
	if err != nil {
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
	snapshot.MarkdownSyncBackRan = mdRan
	return a.gestureSnapshotResult(updated, gestureActionDefer, "", gestureSyncBack{Markdown: mdRan}, snapshot), http.StatusOK, nil
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
	mdRan, status, err := a.gestureWriteThroughMarkdown(item, store.ItemStateWaiting)
	if err != nil {
		return itemGestureResult{}, status, err
	}
	if err := a.store.UpdateItem(item.ID, update); err != nil {
		return itemGestureResult{}, itemResponseErrorStatus(err), err
	}
	updated, err := a.store.GetItem(item.ID)
	if err != nil {
		return itemGestureResult{}, itemResponseErrorStatus(err), err
	}
	snapshot.MarkdownSyncBackRan = mdRan
	return a.gestureSnapshotResult(updated, gestureActionDelegate, "", gestureSyncBack{Markdown: mdRan}, snapshot), http.StatusOK, nil
}

// gestureWriteThroughClose handles the close path for both markdown-backed
// items (validated brain.gtd.set_status) and external-backed items (todoist
// complete + email archive). It returns which write-through paths ran so undo
// can reverse exactly the side-effects that happened, the HTTP status to
// return on error, and any error that should abort the gesture.
func (a *App) gestureWriteThroughClose(ctx context.Context, item store.Item) (gestureSyncBack, int, error) {
	mdRan, status, err := a.gestureWriteThroughMarkdown(item, store.ItemStateDone)
	if err != nil {
		return gestureSyncBack{}, status, err
	}
	if mdRan {
		return gestureSyncBack{Markdown: true}, http.StatusOK, nil
	}
	emailRan, err := a.runItemGestureUpstreamComplete(ctx, item)
	if err != nil {
		return gestureSyncBack{}, http.StatusBadGateway, err
	}
	return gestureSyncBack{Email: emailRan}, http.StatusOK, nil
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
	status, err := gtdStatusForLocalState(targetState)
	if err != nil {
		return false, http.StatusBadRequest, err
	}
	if _, _, err := a.setBrainGTDStatus(target, itemGTDStatusRequest{}, status); err != nil {
		return false, http.StatusBadGateway, err
	}
	return true, http.StatusOK, nil
}

// gtdStatusForLocalState maps an item's local state into the brain.gtd
// status vocabulary used by brain.gtd.set_status. Forward gestures only ever
// pass done/deferred/waiting; undo passes any prior state. The local store
// retains a richer state ladder than brain.gtd, hence the mapping.
func gtdStatusForLocalState(localState string) (string, error) {
	switch localState {
	case store.ItemStateDone:
		return "closed", nil
	case store.ItemStateInbox, store.ItemStateNext, store.ItemStateWaiting,
		store.ItemStateDeferred, store.ItemStateSomeday, store.ItemStateReview:
		return localState, nil
	default:
		return "", fmt.Errorf("cannot map state %q to brain.gtd status", localState)
	}
}

func (a *App) gestureSnapshotResult(item store.Item, action, dropMode string, sync gestureSyncBack, snapshot itemGestureUndo) itemGestureResult {
	return itemGestureResult{
		Item:                item,
		Action:              action,
		DropMode:            dropMode,
		EmailSyncBackRan:    sync.Email,
		MarkdownSyncBackRan: sync.Markdown,
		Undo:                snapshot,
	}
}

// parentProjectHealthForChild returns refreshed health for every project item
// that lists `child` as a child. Action items can have project parents; project
// items never do. The endpoint surfaces the recomputed health so a UI showing
// the project row can refresh in place after a child action completes, without
// auto-closing the parent project (the local store rules already preserve the
// child links and parent state).
func (a *App) parentProjectHealthForChild(child store.Item) ([]gestureParentProjectState, error) {
	if child.Kind != store.ItemKindAction {
		return nil, nil
	}
	links, err := a.store.ListItemParentLinks(child.ID)
	if err != nil {
		return nil, err
	}
	if len(links) == 0 {
		return nil, nil
	}
	out := make([]gestureParentProjectState, 0, len(links))
	for _, link := range links {
		review, err := a.store.GetProjectItemReview(link.ParentItemID)
		if err != nil {
			return nil, err
		}
		out = append(out, gestureParentProjectState{
			ProjectItemID: link.ParentItemID,
			Health:        review.Health,
			Counts:        review.Children,
		})
	}
	return out, nil
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
	if req.Undo.MarkdownSyncBackRan {
		if status, err := a.revertGestureMarkdownSyncBack(item, prev); err != nil {
			writeAPIError(w, status, err.Error())
			return
		}
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

// revertGestureMarkdownSyncBack reverses the brain.gtd.set_status write-through
// the forward gesture executed, so undo restores the source markdown alongside
// the local overlay row. The target is recomputed from the current item rather
// than stashed in the snapshot because gtdStatusTarget is deterministic in the
// item's source/sphere/artifact, none of which the gesture mutates.
func (a *App) revertGestureMarkdownSyncBack(item store.Item, priorState string) (int, error) {
	target, ok, err := a.gtdStatusTarget(item)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	if !ok {
		return http.StatusOK, nil
	}
	status, err := gtdStatusForLocalState(priorState)
	if err != nil {
		return http.StatusBadRequest, err
	}
	if _, _, err := a.setBrainGTDStatus(target, itemGTDStatusRequest{}, status); err != nil {
		return http.StatusBadGateway, err
	}
	return http.StatusOK, nil
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
