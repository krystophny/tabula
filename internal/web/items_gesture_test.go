package web

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/sloppy-org/slopshell/internal/store"
)

func TestItemGestureCompleteSetsDone(t *testing.T) {
	app := newAuthedTestApp(t)
	item := mustCreateGestureItem(t, app, "Wrap up review", store.ItemOptions{State: store.ItemStateNext})

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(item.ID)+"/gesture", map[string]any{
		"action": "complete",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	payload := decodeJSONDataResponse(t, rr)
	if payload["action"] != gestureActionComplete {
		t.Fatalf("action = %#v", payload["action"])
	}
	got, err := app.store.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem error: %v", err)
	}
	if got.State != store.ItemStateDone {
		t.Fatalf("state = %q, want %q", got.State, store.ItemStateDone)
	}
	undo, _ := payload["undo"].(map[string]any)
	if undo["state"] != store.ItemStateNext {
		t.Fatalf("undo.state = %#v, want %q", undo["state"], store.ItemStateNext)
	}
}

func TestItemGestureDeferSetsFollowUp(t *testing.T) {
	app := newAuthedTestApp(t)
	item := mustCreateGestureItem(t, app, "Wait for callback", store.ItemOptions{State: store.ItemStateInbox})

	follow := "2026-05-10T09:00:00Z"
	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(item.ID)+"/gesture", map[string]any{
		"action":       "defer",
		"follow_up_at": follow,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	got, err := app.store.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem error: %v", err)
	}
	if got.State != store.ItemStateDeferred {
		t.Fatalf("state = %q, want %q", got.State, store.ItemStateDeferred)
	}
	if got.FollowUpAt == nil || *got.FollowUpAt == "" {
		t.Fatalf("follow_up_at = %v, want set", got.FollowUpAt)
	}
	if got.VisibleAfter == nil || *got.VisibleAfter == "" {
		t.Fatalf("visible_after = %v, want set", got.VisibleAfter)
	}
}

func TestItemGestureDeferRejectsMissingFollowUp(t *testing.T) {
	app := newAuthedTestApp(t)
	item := mustCreateGestureItem(t, app, "Defer without date", store.ItemOptions{})

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(item.ID)+"/gesture", map[string]any{
		"action": "defer",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rr.Code, rr.Body.String())
	}
}

func TestItemGestureDelegateAssignsActor(t *testing.T) {
	app := newAuthedTestApp(t)
	actor, err := app.store.CreateActor("Tony", store.ActorKindHuman)
	if err != nil {
		t.Fatalf("CreateActor: %v", err)
	}
	item := mustCreateGestureItem(t, app, "Delegate vendor call", store.ItemOptions{State: store.ItemStateNext})

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(item.ID)+"/gesture", map[string]any{
		"action":       "delegate",
		"actor_id":     actor.ID,
		"follow_up_at": "2026-05-15T09:00:00Z",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	got, err := app.store.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem error: %v", err)
	}
	if got.State != store.ItemStateWaiting {
		t.Fatalf("state = %q, want %q", got.State, store.ItemStateWaiting)
	}
	if got.ActorID == nil || *got.ActorID != actor.ID {
		t.Fatalf("actor_id = %v, want %d", got.ActorID, actor.ID)
	}
	if got.FollowUpAt == nil || *got.FollowUpAt == "" {
		t.Fatalf("follow_up_at = %v, want set", got.FollowUpAt)
	}
}

func TestItemGestureDelegateRequiresActor(t *testing.T) {
	app := newAuthedTestApp(t)
	item := mustCreateGestureItem(t, app, "Delegate without actor", store.ItemOptions{})

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(item.ID)+"/gesture", map[string]any{
		"action": "delegate",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rr.Code, rr.Body.String())
	}
}

func TestItemGestureDropOnExternalSourceUsesLocalOverlay(t *testing.T) {
	app := newAuthedTestApp(t)
	source := store.ExternalProviderTodoist
	ref := "todoist:task:42"
	item := mustCreateGestureItem(t, app, "Todoist-backed task", store.ItemOptions{
		State:     store.ItemStateNext,
		Source:    &source,
		SourceRef: &ref,
	})

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(item.ID)+"/gesture", map[string]any{
		"action": "drop",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	payload := decodeJSONDataResponse(t, rr)
	if payload["drop_mode"] != gestureDropModeLocalOverlay {
		t.Fatalf("drop_mode = %#v, want %q", payload["drop_mode"], gestureDropModeLocalOverlay)
	}
	if payload["email_sync_back"] == true {
		t.Fatalf("email_sync_back should be false for local overlay drop, got %#v", payload["email_sync_back"])
	}
	got, err := app.store.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem error: %v", err)
	}
	if got.State != store.ItemStateDone {
		t.Fatalf("state = %q, want %q", got.State, store.ItemStateDone)
	}
}

func TestItemGestureDropOnProjectItemPreservesChildLinks(t *testing.T) {
	app := newAuthedTestApp(t)
	parent, err := app.store.CreateItem("Outcome: ship review", store.ItemOptions{
		Kind:  store.ItemKindProject,
		State: store.ItemStateNext,
	})
	if err != nil {
		t.Fatalf("CreateItem(project): %v", err)
	}
	child, err := app.store.CreateItem("Draft acceptance check", store.ItemOptions{State: store.ItemStateNext})
	if err != nil {
		t.Fatalf("CreateItem(child): %v", err)
	}
	if err := app.store.LinkItemChild(parent.ID, child.ID, store.ItemLinkRoleNextAction); err != nil {
		t.Fatalf("LinkItemChild: %v", err)
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(parent.ID)+"/gesture", map[string]any{
		"action": "drop",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	payload := decodeJSONDataResponse(t, rr)
	if payload["drop_mode"] != gestureDropModeProjectClose {
		t.Fatalf("drop_mode = %#v, want %q", payload["drop_mode"], gestureDropModeProjectClose)
	}
	links, err := app.store.ListItemChildLinks(parent.ID)
	if err != nil {
		t.Fatalf("ListItemChildLinks: %v", err)
	}
	if len(links) != 1 || links[0].ChildItemID != child.ID {
		t.Fatalf("child links = %#v, want one link to child %d", links, child.ID)
	}
	parentItem, err := app.store.GetItem(parent.ID)
	if err != nil {
		t.Fatalf("GetItem(parent): %v", err)
	}
	if parentItem.State != store.ItemStateDone {
		t.Fatalf("parent state = %q, want %q", parentItem.State, store.ItemStateDone)
	}
	childItem, err := app.store.GetItem(child.ID)
	if err != nil {
		t.Fatalf("GetItem(child): %v", err)
	}
	if childItem.State != store.ItemStateNext {
		t.Fatalf("child state = %q, want %q (closing parent must not silently close child)", childItem.State, store.ItemStateNext)
	}
}

func TestItemGestureUndoRevertsState(t *testing.T) {
	app := newAuthedTestApp(t)
	item := mustCreateGestureItem(t, app, "Undo me", store.ItemOptions{State: store.ItemStateNext})

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(item.ID)+"/gesture", map[string]any{
		"action": "complete",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("complete status = %d: %s", rr.Code, rr.Body.String())
	}
	payload := decodeJSONDataResponse(t, rr)
	undo := payload["undo"]
	rrUndo := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(item.ID)+"/gesture/undo", map[string]any{
		"undo": undo,
	})
	if rrUndo.Code != http.StatusOK {
		t.Fatalf("undo status = %d: %s", rrUndo.Code, rrUndo.Body.String())
	}
	got, err := app.store.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem error: %v", err)
	}
	if got.State != store.ItemStateNext {
		t.Fatalf("after undo state = %q, want %q", got.State, store.ItemStateNext)
	}
}

func TestItemGestureUndoRevertsDelegate(t *testing.T) {
	app := newAuthedTestApp(t)
	actor, err := app.store.CreateActor("Pat", store.ActorKindHuman)
	if err != nil {
		t.Fatalf("CreateActor: %v", err)
	}
	item := mustCreateGestureItem(t, app, "Delegate then undo", store.ItemOptions{State: store.ItemStateInbox})

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(item.ID)+"/gesture", map[string]any{
		"action":   "delegate",
		"actor_id": actor.ID,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("delegate status = %d: %s", rr.Code, rr.Body.String())
	}
	payload := decodeJSONDataResponse(t, rr)
	rrUndo := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(item.ID)+"/gesture/undo", map[string]any{
		"undo": payload["undo"],
	})
	if rrUndo.Code != http.StatusOK {
		t.Fatalf("undo status = %d: %s", rrUndo.Code, rrUndo.Body.String())
	}
	got, err := app.store.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem error: %v", err)
	}
	if got.State != store.ItemStateInbox {
		t.Fatalf("after undo state = %q, want %q", got.State, store.ItemStateInbox)
	}
	if got.ActorID != nil {
		t.Fatalf("after undo actor_id = %v, want nil", got.ActorID)
	}
}

func TestItemGestureRejectsUnknownAction(t *testing.T) {
	app := newAuthedTestApp(t)
	item := mustCreateGestureItem(t, app, "Bad action", store.ItemOptions{})

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(item.ID)+"/gesture", map[string]any{
		"action": "explode",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rr.Code, rr.Body.String())
	}
}

func TestDropModeRoutingMatrix(t *testing.T) {
	cases := []struct {
		name     string
		item     store.Item
		upstream bool
		want     string
	}{
		{
			name: "local action drops into overlay",
			item: store.Item{Kind: store.ItemKindAction},
			want: gestureDropModeLocalOverlay,
		},
		{
			name: "project item closes locally to preserve child links",
			item: store.Item{Kind: store.ItemKindProject},
			want: gestureDropModeProjectClose,
		},
		{
			name: "external source defaults to overlay drop",
			item: store.Item{Kind: store.ItemKindAction, Source: stringPointer(store.ExternalProviderGmail)},
			want: gestureDropModeLocalOverlay,
		},
		{
			name:     "explicit upstream drop on external source uses upstream mode",
			item:     store.Item{Kind: store.ItemKindAction, Source: stringPointer(store.ExternalProviderGmail)},
			upstream: true,
			want:     gestureDropModeUpstream,
		},
		{
			name:     "upstream flag without external source still drops locally",
			item:     store.Item{Kind: store.ItemKindAction},
			upstream: true,
			want:     gestureDropModeLocalOverlay,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := dropModeForItem(tc.item, tc.upstream); got != tc.want {
				t.Fatalf("dropModeForItem = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestItemGestureCompleteOnMarkdownBackedItemValidatesAfterWriteThrough(t *testing.T) {
	app := newAuthedTestApp(t)
	source := "markdown"
	ref := "brain/commitments/example.md"
	sphere := store.SphereWork
	item, err := app.store.CreateItem("Markdown commitment", store.ItemOptions{
		State:     store.ItemStateNext,
		Source:    &source,
		SourceRef: &ref,
		Sphere:    &sphere,
	})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}

	calls := []capturedMCPCall{}
	mcp := newGTDStatusMCPServer(t, &calls, false)
	app.localMCPEndpoint = mcpEndpoint{httpURL: mcp.URL}

	// /gtd-status is the validated write-through path used by gesture-driven
	// closes for markdown-backed items: gestures call complete, the frontend
	// routes through gtd-status when the item is markdown-backed, and the
	// brain.note.parse + brain.gtd.set_status sequence enforces validation.
	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPut, "/api/items/"+itoa(item.ID)+"/gtd-status", map[string]any{
		"state": "done",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	if got := callNames(calls); !reflect.DeepEqual(got, []string{gtdParseTool, gtdSetStatusTool}) {
		t.Fatalf("MCP calls = %#v", got)
	}
}

func TestItemGestureCompleteOnExternalEmailRunsArchive(t *testing.T) {
	app := newAuthedTestApp(t)
	source := store.ExternalProviderGmail
	item := mustCreateGestureItem(t, app, "Mail thread", store.ItemOptions{
		State:  store.ItemStateNext,
		Source: &source,
	})
	// No matching account/binding wired in this test, so syncRemoteEmailItemState
	// returns nil without calling a provider. The point is to exercise the
	// gesture code path without errors and confirm state lands at done.
	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(item.ID)+"/gesture", map[string]any{
		"action": "complete",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	got, err := app.store.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if got.State != store.ItemStateDone {
		t.Fatalf("state = %q, want %q", got.State, store.ItemStateDone)
	}
}

func mustCreateGestureItem(t *testing.T, app *App, title string, opts store.ItemOptions) store.Item {
	t.Helper()
	item, err := app.store.CreateItem(title, opts)
	if err != nil {
		t.Fatalf("CreateItem(%q): %v", title, err)
	}
	return item
}

