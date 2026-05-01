package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sloppy-org/slopshell/internal/store"
)

func TestItemDedupReviewAPIListsCandidateGroups(t *testing.T) {
	app := newAuthedTestApp(t)
	first, second := seedWebDedupItems(t, app)
	if _, err := app.store.CreateItemDedupCandidate(store.ItemDedupCandidateOptions{
		Kind:       store.ItemKindAction,
		Confidence: 0.88,
		Outcome:    "Finish review queue",
		Reasoning:  "local LLM matched source context",
		Detector:   "local-llm",
		Items: []store.ItemDedupCandidateItemInput{
			{ItemID: first.ID, Outcome: "Finish the review queue"},
			{ItemID: second.ID, Outcome: "Complete review queue"},
		},
	}); err != nil {
		t.Fatalf("CreateItemDedupCandidate() error: %v", err)
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/dedup?kind=action", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	payload := decodeJSONResponse(t, rr)
	groups, ok := payload["groups"].([]any)
	if !ok || len(groups) != 1 {
		t.Fatalf("groups = %#v, want one group", payload["groups"])
	}
	group := groups[0].(map[string]any)
	if group["reasoning"] != "local LLM matched source context" {
		t.Fatalf("reasoning = %v", group["reasoning"])
	}
	members := group["items"].([]any)
	for _, raw := range members {
		member := raw.(map[string]any)
		if len(member["source_bindings"].([]any)) != 1 {
			t.Fatalf("member bindings = %#v, want source binding", member)
		}
	}
}

func TestItemDedupReviewAPIImportsSloptoolsScanCandidates(t *testing.T) {
	app := newAuthedTestApp(t)
	first := seedWebDedupSourceItem(t, app, "First source item", "brain/commitments/a.md")
	second := seedWebDedupSourceItem(t, app, "Second source item", "brain/commitments/b.md")
	request := map[string]any{}
	mcp := newSloptoolsDedupScanServer(t, &request)
	defer mcp.Close()
	app.localMCPEndpoint = mcpEndpoint{httpURL: mcp.URL}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/dedup?sphere=work", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	assertSloptoolsScanRequest(t, request, store.SphereWork)
	group := onlyDedupGroup(t, decodeJSONResponse(t, rr))
	if group["reasoning"] != "same source outcome" || group["detector"] != "sloptools-test" {
		t.Fatalf("imported group metadata = %#v", group)
	}
	members := group["items"].([]any)
	if len(members) != 2 {
		t.Fatalf("members = %#v, want two imported candidate items", members)
	}
	assertDedupMemberIDs(t, members, first.ID, second.ID)

	assertDedupGroupCount(t, app, 1)
	keepDedupCandidateSeparate(t, app, int64(group["id"].(float64)))
	assertDedupGroupCount(t, app, 0)
}

func TestItemDedupReviewAPIActions(t *testing.T) {
	app := newAuthedTestApp(t)
	first, second := seedWebDedupItems(t, app)
	group, err := app.store.CreateItemDedupCandidate(store.ItemDedupCandidateOptions{
		Kind:  store.ItemKindAction,
		Items: []store.ItemDedupCandidateItemInput{{ItemID: first.ID}, {ItemID: second.ID}},
	})
	if err != nil {
		t.Fatalf("CreateItemDedupCandidate() error: %v", err)
	}

	later := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/dedup/"+itoa(group.ID)+"/review_later", map[string]any{})
	if later.Code != http.StatusOK {
		t.Fatalf("review_later status = %d, want 200: %s", later.Code, later.Body.String())
	}
	list := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/dedup", nil)
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200: %s", list.Code, list.Body.String())
	}
	groups := decodeJSONResponse(t, list)["groups"].([]any)
	if len(groups) != 1 || groups[0].(map[string]any)["state"] != store.ItemDedupStateReviewLater {
		t.Fatalf("groups after review_later = %#v", groups)
	}

	keep := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/dedup/"+itoa(group.ID)+"/keep_separate", map[string]any{})
	if keep.Code != http.StatusOK {
		t.Fatalf("keep_separate status = %d, want 200: %s", keep.Code, keep.Body.String())
	}
	empty := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/dedup", nil)
	if empty.Code != http.StatusOK {
		t.Fatalf("empty status = %d, want 200: %s", empty.Code, empty.Body.String())
	}
	emptyPayload := decodeJSONResponse(t, empty)
	groupsAfterKeep, ok := emptyPayload["groups"].([]any)
	if !ok {
		t.Fatalf("groups after keep_separate missing: %#v", emptyPayload)
	}
	if got := len(groupsAfterKeep); got != 0 {
		t.Fatalf("open groups after keep_separate = %d, want 0", got)
	}
}

func seedWebDedupSourceItem(t *testing.T, app *App, title, path string) store.Item {
	t.Helper()
	source := "brain.gtd"
	sphere := store.SphereWork
	item, err := app.store.CreateItem(title, store.ItemOptions{
		State:     store.ItemStateNext,
		Sphere:    &sphere,
		Source:    &source,
		SourceRef: &path,
	})
	if err != nil {
		t.Fatalf("CreateItem(%q) error: %v", title, err)
	}
	return item
}

func seedWebDedupItems(t *testing.T, app *App) (store.Item, store.Item) {
	t.Helper()
	first := seedWebDedupItem(t, app, "Review candidate group", store.ExternalProviderTodoist, "task-1", "Inbox")
	second := seedWebDedupItem(t, app, "Review candidate duplicate", store.ExternalProviderGmail, "msg-2", "Follow-up")
	return first, second
}

func seedWebDedupItem(t *testing.T, app *App, title, provider, remoteID, container string) store.Item {
	t.Helper()
	item, err := app.store.CreateItem(title, store.ItemOptions{State: store.ItemStateNext})
	if err != nil {
		t.Fatalf("CreateItem(%q) error: %v", title, err)
	}
	account, err := app.store.CreateExternalAccount(store.SphereWork, provider, provider+" "+remoteID, map[string]any{})
	if err != nil {
		t.Fatalf("CreateExternalAccount(%s) error: %v", provider, err)
	}
	if _, err := app.store.UpsertExternalBinding(store.ExternalBinding{
		AccountID:    account.ID,
		Provider:     account.Provider,
		ObjectType:   "task",
		RemoteID:     remoteID,
		ItemID:       &item.ID,
		ContainerRef: &container,
	}); err != nil {
		t.Fatalf("UpsertExternalBinding(%s) error: %v", remoteID, err)
	}
	return item
}

func newSloptoolsDedupScanServer(t *testing.T, request *map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(request); err != nil {
			t.Fatalf("decode MCP request: %v", err)
		}
		writeWebMCPResult(t, w, map[string]any{"dedup": map[string]any{
			"candidates": []map[string]any{{
				"id":           "candidate-1",
				"paths":        []string{"brain/commitments/a.md", "brain/commitments/b.md"},
				"score":        0.91,
				"confidence":   0.86,
				"reasoning":    "same source outcome",
				"detector":     "sloptools-test",
				"review_state": "open",
			}},
		}})
	}))
}

func writeWebMCPResult(t *testing.T, w http.ResponseWriter, structured map[string]any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"result":  map[string]any{"structuredContent": structured},
	})
	if err != nil {
		t.Fatalf("encode MCP response: %v", err)
	}
}

func assertSloptoolsScanRequest(t *testing.T, request map[string]any, sphere string) {
	t.Helper()
	params := request["params"].(map[string]any)
	if params["name"] != "brain.gtd.dedup_scan" {
		t.Fatalf("MCP tool name = %q, want brain.gtd.dedup_scan", params["name"])
	}
	args := params["arguments"].(map[string]any)
	if args["sphere"] != sphere {
		t.Fatalf("scan sphere = %q, want %q", args["sphere"], sphere)
	}
}

func onlyDedupGroup(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()
	groups := payload["groups"].([]any)
	if len(groups) != 1 {
		t.Fatalf("groups = %#v, want one group", groups)
	}
	return groups[0].(map[string]any)
}

func assertDedupGroupCount(t *testing.T, app *App, want int) {
	t.Helper()
	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/dedup?sphere=work", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("dedup list status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	if got := len(decodeJSONResponse(t, rr)["groups"].([]any)); got != want {
		t.Fatalf("dedup groups = %d, want %d", got, want)
	}
}

func keepDedupCandidateSeparate(t *testing.T, app *App, candidateID int64) {
	t.Helper()
	keep := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/dedup/"+itoa(candidateID)+"/keep_separate", map[string]any{})
	if keep.Code != http.StatusOK {
		t.Fatalf("keep_separate status = %d, want 200: %s", keep.Code, keep.Body.String())
	}
}

func assertDedupMemberIDs(t *testing.T, members []any, want ...int64) {
	t.Helper()
	got := map[int64]bool{}
	for _, raw := range members {
		member := raw.(map[string]any)
		item := member["item"].(map[string]any)
		got[int64(item["id"].(float64))] = true
	}
	for _, id := range want {
		if !got[id] {
			t.Fatalf("member ids = %#v, missing %d", got, id)
		}
	}
}
