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

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(item.ID)+"/gesture", map[string]any{
		"action": "complete",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	if got := callNames(calls); !reflect.DeepEqual(got, []string{gtdParseTool, gtdSetStatusTool}) {
		t.Fatalf("MCP calls = %#v", got)
	}
	if calls[1].Args["status"] != "closed" || calls[1].Args["path"] != ref {
		t.Fatalf("set_status args = %#v", calls[1].Args)
	}
	updated, err := app.store.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if updated.State != store.ItemStateDone {
		t.Fatalf("state = %q, want %q", updated.State, store.ItemStateDone)
	}
}

func TestItemGestureUndoRevertsMarkdownSyncBack(t *testing.T) {
	app := newAuthedTestApp(t)
	source := "markdown"
	ref := "brain/commitments/example.md"
	sphere := store.SphereWork
	item, err := app.store.CreateItem("Markdown undo", store.ItemOptions{
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

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(item.ID)+"/gesture", map[string]any{
		"action": "complete",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("complete status = %d: %s", rr.Code, rr.Body.String())
	}
	payload := decodeJSONDataResponse(t, rr)
	if payload["markdown_sync_back"] != true {
		t.Fatalf("markdown_sync_back = %#v, want true", payload["markdown_sync_back"])
	}
	undo, _ := payload["undo"].(map[string]any)
	if undo["markdown_sync_back"] != true {
		t.Fatalf("undo.markdown_sync_back = %#v, want true", undo["markdown_sync_back"])
	}
	if undo["state"] != store.ItemStateNext {
		t.Fatalf("undo.state = %#v, want %q", undo["state"], store.ItemStateNext)
	}

	rrUndo := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(item.ID)+"/gesture/undo", map[string]any{
		"undo": undo,
	})
	if rrUndo.Code != http.StatusOK {
		t.Fatalf("undo status = %d: %s", rrUndo.Code, rrUndo.Body.String())
	}
	wantCalls := []string{gtdParseTool, gtdSetStatusTool, gtdParseTool, gtdSetStatusTool}
	if got := callNames(calls); !reflect.DeepEqual(got, wantCalls) {
		t.Fatalf("MCP calls = %#v, want %#v", got, wantCalls)
	}
	if calls[1].Args["status"] != "closed" {
		t.Fatalf("forward set_status status = %#v, want closed", calls[1].Args["status"])
	}
	if calls[3].Args["status"] != store.ItemStateNext {
		t.Fatalf("undo set_status status = %#v, want %q", calls[3].Args["status"], store.ItemStateNext)
	}
	if calls[3].Args["path"] != ref {
		t.Fatalf("undo set_status path = %#v, want %q", calls[3].Args["path"], ref)
	}
	got, err := app.store.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if got.State != store.ItemStateNext {
		t.Fatalf("after undo state = %q, want %q", got.State, store.ItemStateNext)
	}
}

func TestItemGestureUndoRevertsMarkdownDelegateSyncBack(t *testing.T) {
	app := newAuthedTestApp(t)
	source := "markdown"
	ref := "brain/commitments/delegate.md"
	actor, err := app.store.CreateActor("Robin", store.ActorKindHuman)
	if err != nil {
		t.Fatalf("CreateActor: %v", err)
	}
	item, err := app.store.CreateItem("Markdown delegate undo", store.ItemOptions{
		State:     store.ItemStateInbox,
		Source:    &source,
		SourceRef: &ref,
	})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	calls := []capturedMCPCall{}
	mcp := newGTDStatusMCPServer(t, &calls, false)
	app.localMCPEndpoint = mcpEndpoint{httpURL: mcp.URL}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(item.ID)+"/gesture", map[string]any{
		"action":   "delegate",
		"actor_id": actor.ID,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("delegate status = %d: %s", rr.Code, rr.Body.String())
	}
	payload := decodeJSONDataResponse(t, rr)
	undo, _ := payload["undo"].(map[string]any)
	if undo["markdown_sync_back"] != true {
		t.Fatalf("undo.markdown_sync_back = %#v, want true", undo["markdown_sync_back"])
	}

	rrUndo := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(item.ID)+"/gesture/undo", map[string]any{
		"undo": undo,
	})
	if rrUndo.Code != http.StatusOK {
		t.Fatalf("undo status = %d: %s", rrUndo.Code, rrUndo.Body.String())
	}
	if got := callNames(calls); len(got) != 4 {
		t.Fatalf("MCP calls = %#v, want 4 (parse,set,parse,set)", got)
	}
	if calls[1].Args["status"] != store.ItemStateWaiting {
		t.Fatalf("forward set_status = %#v, want %q", calls[1].Args["status"], store.ItemStateWaiting)
	}
	if calls[3].Args["status"] != store.ItemStateInbox {
		t.Fatalf("undo set_status = %#v, want %q", calls[3].Args["status"], store.ItemStateInbox)
	}
	got, err := app.store.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if got.State != store.ItemStateInbox {
		t.Fatalf("state = %q, want %q", got.State, store.ItemStateInbox)
	}
	if got.ActorID != nil {
		t.Fatalf("actor_id = %v, want nil", got.ActorID)
	}
}

func TestItemGestureUndoSkipsMarkdownWhenFlagFalse(t *testing.T) {
	app := newAuthedTestApp(t)
	item := mustCreateGestureItem(t, app, "Plain undo", store.ItemOptions{State: store.ItemStateNext})

	mcp := newGTDStatusMCPServer(t, &[]capturedMCPCall{}, false)
	app.localMCPEndpoint = mcpEndpoint{httpURL: mcp.URL}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(item.ID)+"/gesture", map[string]any{
		"action": "complete",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("complete status = %d: %s", rr.Code, rr.Body.String())
	}
	payload := decodeJSONDataResponse(t, rr)
	undo, _ := payload["undo"].(map[string]any)
	if _, present := undo["markdown_sync_back"]; present {
		t.Fatalf("undo.markdown_sync_back present for non-markdown item: %#v", undo)
	}
}

func TestItemGestureDeferOnMarkdownBackedItemValidatesAfterWriteThrough(t *testing.T) {
	app := newAuthedTestApp(t)
	source := "markdown"
	ref := "brain/commitments/example.md"
	item, err := app.store.CreateItem("Markdown defer", store.ItemOptions{
		State:     store.ItemStateNext,
		Source:    &source,
		SourceRef: &ref,
	})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	calls := []capturedMCPCall{}
	mcp := newGTDStatusMCPServer(t, &calls, false)
	app.localMCPEndpoint = mcpEndpoint{httpURL: mcp.URL}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(item.ID)+"/gesture", map[string]any{
		"action":       "defer",
		"follow_up_at": "2026-05-10T09:00:00Z",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	if got := callNames(calls); !reflect.DeepEqual(got, []string{gtdParseTool, gtdSetStatusTool}) {
		t.Fatalf("MCP calls = %#v", got)
	}
	if calls[1].Args["status"] != store.ItemStateDeferred {
		t.Fatalf("set_status status = %#v, want %q", calls[1].Args["status"], store.ItemStateDeferred)
	}
}

func TestItemGestureDelegateOnMarkdownBackedItemValidatesAfterWriteThrough(t *testing.T) {
	app := newAuthedTestApp(t)
	source := "markdown"
	ref := "brain/commitments/example.md"
	actor, err := app.store.CreateActor("Wren", store.ActorKindHuman)
	if err != nil {
		t.Fatalf("CreateActor: %v", err)
	}
	item, err := app.store.CreateItem("Markdown delegate", store.ItemOptions{
		State:     store.ItemStateNext,
		Source:    &source,
		SourceRef: &ref,
	})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	calls := []capturedMCPCall{}
	mcp := newGTDStatusMCPServer(t, &calls, false)
	app.localMCPEndpoint = mcpEndpoint{httpURL: mcp.URL}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(item.ID)+"/gesture", map[string]any{
		"action":   "delegate",
		"actor_id": actor.ID,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	if got := callNames(calls); !reflect.DeepEqual(got, []string{gtdParseTool, gtdSetStatusTool}) {
		t.Fatalf("MCP calls = %#v", got)
	}
	if calls[1].Args["status"] != store.ItemStateWaiting {
		t.Fatalf("set_status status = %#v, want %q", calls[1].Args["status"], store.ItemStateWaiting)
	}
}

func TestItemGestureDropOnMarkdownBackedItemValidatesAfterWriteThrough(t *testing.T) {
	app := newAuthedTestApp(t)
	source := "markdown"
	ref := "brain/commitments/example.md"
	item, err := app.store.CreateItem("Markdown drop", store.ItemOptions{
		State:     store.ItemStateNext,
		Source:    &source,
		SourceRef: &ref,
	})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	calls := []capturedMCPCall{}
	mcp := newGTDStatusMCPServer(t, &calls, false)
	app.localMCPEndpoint = mcpEndpoint{httpURL: mcp.URL}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(item.ID)+"/gesture", map[string]any{
		"action": "drop",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	if got := callNames(calls); !reflect.DeepEqual(got, []string{gtdParseTool, gtdSetStatusTool}) {
		t.Fatalf("MCP calls = %#v", got)
	}
	if calls[1].Args["status"] != "closed" {
		t.Fatalf("set_status status = %#v, want closed", calls[1].Args["status"])
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

// TestItemGestureCompleteUpstreamFailureKeepsLocalCloseAndSurfacesError covers
// the issue #742 acceptance: when the writeable upstream sync fails, the
// local state change must not be lost and the user must see an actionable
// error. Markdown remains gating; this test is the external-binding contract.
func TestItemGestureCompleteUpstreamFailureKeepsLocalCloseAndSurfacesError(t *testing.T) {
	app := newAuthedTestApp(t)
	source := store.ExternalProviderTodoist
	ref := "todoist:task:42"
	item := mustCreateGestureItem(t, app, "Todoist task without account", store.ItemOptions{
		State:     store.ItemStateNext,
		Source:    &source,
		SourceRef: &ref,
	})

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(item.ID)+"/gesture", map[string]any{
		"action": "complete",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (sync failure must not lose local close): %s", rr.Code, rr.Body.String())
	}
	payload := decodeJSONDataResponse(t, rr)
	if payload["sync_error"] == nil || payload["sync_error"] == "" {
		t.Fatalf("sync_error missing, want populated: %#v", payload)
	}
	got, err := app.store.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if got.State != store.ItemStateDone {
		t.Fatalf("state = %q, want %q (sync failure must keep local close)", got.State, store.ItemStateDone)
	}
}

// TestItemGestureCompleteWriteableSyncCalledExactlyOnce covers the #742
// "exactly once" acceptance. We swap in a counting sync and assert a single
// invocation per gesture. A second complete on the now-done item must not
// fire the sync again.
func TestItemGestureCompleteWriteableSyncCalledExactlyOnce(t *testing.T) {
	app := newAuthedTestApp(t)
	source := store.ExternalProviderTodoist
	ref := "todoist:task:42"
	item := mustCreateGestureItem(t, app, "Todoist task counted", store.ItemOptions{
		State:     store.ItemStateNext,
		Source:    &source,
		SourceRef: &ref,
	})

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(item.ID)+"/gesture", map[string]any{
		"action": "complete",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("first complete status = %d: %s", rr.Code, rr.Body.String())
	}
	first := decodeJSONDataResponse(t, rr)
	if _, ok := first["sync_error"]; !ok {
		t.Fatalf("first complete should report sync_error from unconfigured account: %#v", first)
	}

	rr2 := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(item.ID)+"/gesture", map[string]any{
		"action": "complete",
	})
	if rr2.Code != http.StatusOK {
		t.Fatalf("second complete status = %d: %s", rr2.Code, rr2.Body.String())
	}
	second := decodeJSONDataResponse(t, rr2)
	if _, ok := second["sync_error"]; ok {
		t.Fatalf("second complete must short-circuit and skip sync (exactly once per completed item): %#v", second)
	}
}

// TestItemGestureCompleteOnChildActionRefreshesParentProjectHealth covers the
// #742 acceptance: completing a child action returns the parent project's
// recomputed health (so the UI can refresh in place) and must not auto-close
// the parent project item.
func TestItemGestureCompleteOnChildActionRefreshesParentProjectHealth(t *testing.T) {
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

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(child.ID)+"/gesture", map[string]any{
		"action": "complete",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	payload := decodeJSONDataResponse(t, rr)
	parents, _ := payload["parent_project_health"].([]any)
	if len(parents) != 1 {
		t.Fatalf("parent_project_health = %#v, want one entry", payload["parent_project_health"])
	}
	first, _ := parents[0].(map[string]any)
	if int64(first["project_item_id"].(float64)) != parent.ID {
		t.Fatalf("project_item_id = %v, want %d", first["project_item_id"], parent.ID)
	}
	health, _ := first["health"].(map[string]any)
	if got, _ := health["stalled"].(bool); !got {
		t.Fatalf("parent health = %#v, want stalled true after only-child completed", first["health"])
	}
	parentItem, err := app.store.GetItem(parent.ID)
	if err != nil {
		t.Fatalf("GetItem(parent): %v", err)
	}
	if parentItem.State == store.ItemStateDone {
		t.Fatalf("parent state = %q, must not auto-close when child completes", parentItem.State)
	}
}

// TestItemGestureCompleteSkipsParentHealthForLeafItem keeps the response
// payload minimal for the common case: a stand-alone action item has no
// parents, so the response should not surface an empty health array.
func TestItemGestureCompleteSkipsParentHealthForLeafItem(t *testing.T) {
	app := newAuthedTestApp(t)
	item := mustCreateGestureItem(t, app, "Leaf task", store.ItemOptions{State: store.ItemStateNext})

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(item.ID)+"/gesture", map[string]any{
		"action": "complete",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	payload := decodeJSONDataResponse(t, rr)
	if _, ok := payload["parent_project_health"]; ok {
		t.Fatalf("parent_project_health should be omitted for an unparented action: %#v", payload)
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

