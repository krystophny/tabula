package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/sloppy-org/slopshell/internal/store"
)

type capturedMCPCall struct {
	Name string
	Args map[string]any
}

func TestItemGTDStatusUsesSloptoolsForMarkdownBackedItems(t *testing.T) {
	app := newAuthedTestApp(t)
	source := "markdown"
	ref := "brain/commitments/reviewer.md"
	sphere := store.SphereWork
	item, err := app.store.CreateItem("Review feedback", store.ItemOptions{
		State:     store.ItemStateNext,
		Sphere:    &sphere,
		Source:    &source,
		SourceRef: &ref,
	})
	if err != nil {
		t.Fatalf("CreateItem() error: %v", err)
	}
	calls := []capturedMCPCall{}
	mcp := newGTDStatusMCPServer(t, &calls, false)
	app.localMCPEndpoint = mcpEndpoint{httpURL: mcp.URL}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPut, "/api/items/"+itoa(item.ID)+"/gtd-status", map[string]any{
		"state":     "done",
		"closed_at": "2026-05-01T08:00:00Z",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}

	if got := callNames(calls); !reflect.DeepEqual(got, []string{gtdParseTool, gtdSetStatusTool}) {
		t.Fatalf("MCP calls = %#v", got)
	}
	if calls[0].Args["path"] != ref || calls[0].Args["sphere"] != store.SphereWork {
		t.Fatalf("parse args = %#v", calls[0].Args)
	}
	if calls[1].Args["path"] != ref || calls[1].Args["status"] != "closed" || calls[1].Args["closed_via"] != "slopshell" {
		t.Fatalf("set_status args = %#v", calls[1].Args)
	}
	if calls[1].Args["closed_at"] != "2026-05-01T08:00:00Z" {
		t.Fatalf("closed_at = %#v", calls[1].Args)
	}
	updated, err := app.store.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem() error: %v", err)
	}
	if updated.State != store.ItemStateDone {
		t.Fatalf("item state = %q, want %q", updated.State, store.ItemStateDone)
	}
	payload := decodeJSONDataResponse(t, rr)
	route, _ := payload["gtd_route"].(map[string]any)
	if route["target"] != "local_overlay" || route["writeable_binding"] != false {
		t.Fatalf("gtd_route = %#v", route)
	}
}

func TestItemGTDStatusFallsBackForNonMarkdownItems(t *testing.T) {
	app := newAuthedTestApp(t)
	item, err := app.store.CreateItem("Plain item", store.ItemOptions{State: store.ItemStateNext})
	if err != nil {
		t.Fatalf("CreateItem() error: %v", err)
	}
	mcp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("non-markdown GTD status should not call MCP")
	}))
	defer mcp.Close()
	app.localMCPEndpoint = mcpEndpoint{httpURL: mcp.URL}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPut, "/api/items/"+itoa(item.ID)+"/gtd-status", map[string]any{"state": "done"})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	updated, err := app.store.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem() error: %v", err)
	}
	if updated.State != store.ItemStateDone {
		t.Fatalf("item state = %q, want %q", updated.State, store.ItemStateDone)
	}
}

func TestItemGTDStatusReportsWriteableBindingRoute(t *testing.T) {
	app := newAuthedTestApp(t)
	source := "markdown"
	ref := "brain/commitments/source.md"
	item, err := app.store.CreateItem("Source writable", store.ItemOptions{
		State:     store.ItemStateNext,
		Source:    &source,
		SourceRef: &ref,
	})
	if err != nil {
		t.Fatalf("CreateItem() error: %v", err)
	}
	calls := []capturedMCPCall{}
	mcp := newGTDStatusMCPServer(t, &calls, true)
	app.localMCPEndpoint = mcpEndpoint{httpURL: mcp.URL}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPut, "/api/items/"+itoa(item.ID)+"/gtd-status", map[string]any{"state": "done"})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	payload := decodeJSONDataResponse(t, rr)
	route, _ := payload["gtd_route"].(map[string]any)
	if route["target"] != "source_binding" || route["writeable_binding"] != true {
		t.Fatalf("gtd_route = %#v", route)
	}
}

func newGTDStatusMCPServer(t *testing.T, calls *[]capturedMCPCall, writeable bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode MCP payload: %v", err)
		}
		params, _ := payload["params"].(map[string]any)
		args, _ := params["arguments"].(map[string]any)
		name, _ := params["name"].(string)
		*calls = append(*calls, capturedMCPCall{Name: name, Args: args})
		w.Header().Set("Content-Type", "application/json")
		switch name {
		case gtdParseTool:
			writeMCPStructuredResult(t, w, map[string]any{
				"commitment": map[string]any{
					"source_bindings": []map[string]any{{
						"provider":          "markdown",
						"ref":               "brain/commitments/reviewer.md",
						"location":          map[string]any{"path": "brain/commitments/reviewer.md"},
						"writeable":         writeable,
						"authoritative_for": []string{"status"},
					}},
				},
			})
		case gtdSetStatusTool:
			writeMCPStructuredResult(t, w, map[string]any{
				"status":        args["status"],
				"local_overlay": map[string]any{"status": args["status"]},
			})
		default:
			t.Fatalf("unexpected MCP tool %q", name)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func writeMCPStructuredResult(t *testing.T, w http.ResponseWriter, structured map[string]any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(map[string]any{
		"result": map[string]any{"structuredContent": structured},
	}); err != nil {
		t.Fatalf("write MCP result: %v", err)
	}
}

func callNames(calls []capturedMCPCall) []string {
	out := make([]string, 0, len(calls))
	for _, call := range calls {
		out = append(out, call.Name)
	}
	return out
}
