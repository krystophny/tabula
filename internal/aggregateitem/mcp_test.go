package aggregateitem

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestBindUsesSloptoolsSourceBindingsOverMCP(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp" {
			t.Fatalf("path = %q, want /mcp", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		writeMCPResult(t, w, map[string]any{
			"sphere":        "work",
			"winner_path":   "brain/commitments/winner.md",
			"binding_count": float64(3),
		})
	}))
	defer server.Close()

	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	result, err := client.Bind(context.Background(), BindRequest{
		Sphere:     "work",
		WinnerPath: "brain/commitments/winner.md",
		Paths:      []string{"brain/commitments/winner.md", "brain/commitments/mail.md"},
		Outcome:    "Reply to reviewer",
		SourceBindings: []SourceBinding{
			{
				Provider:         "github",
				Ref:              "sloppy-org/slopshell#725",
				Location:         BindingLocation{Path: "internal/aggregateitem/mcp.go", Anchor: "L1"},
				URL:              "https://github.com/sloppy-org/slopshell/issues/725",
				Writeable:        true,
				AuthoritativeFor: []string{"title", "status"},
				Summary:          "review feedback",
			},
			binding("todoist", "task-1", false),
			binding("mail", "AAMk-msg", true),
		},
	})
	if err != nil {
		t.Fatalf("Bind() error: %v", err)
	}
	if result["binding_count"] != float64(3) {
		t.Fatalf("binding_count = %#v, want 3", result["binding_count"])
	}

	params := objectAt(t, got, "params")
	if params["name"] != toolGTDBind {
		t.Fatalf("tool name = %q, want %q", params["name"], toolGTDBind)
	}
	args := objectAt(t, params, "arguments")
	if _, ok := args["bindings"]; ok {
		t.Fatalf("arguments used Slopshell bindings key: %#v", args)
	}
	bindings := arrayAt(t, args, "source_bindings")
	first := bindings[0].(map[string]any)
	for _, key := range []string{"location", "writeable", "authoritative_for"} {
		if _, ok := first[key]; !ok {
			t.Fatalf("first binding missing %s: %#v", key, first)
		}
	}
	if _, ok := first["authority"]; ok {
		t.Fatalf("binding carried local authority field: %#v", first)
	}
	if got := stringSliceAt(t, first, "authoritative_for"); !reflect.DeepEqual(got, []string{"title", "status"}) {
		t.Fatalf("authoritative_for = %#v", got)
	}
}

func TestSetStatusUsesSloptoolsLocalOverlayTool(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		writeMCPResult(t, w, map[string]any{
			"sphere": "work",
			"path":   "brain/commitments/winner.md",
			"status": "closed",
		})
	}))
	defer server.Close()

	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	_, err = client.SetStatus(context.Background(), SetStatusRequest{
		Sphere:    "work",
		Path:      "brain/commitments/winner.md",
		Status:    "closed",
		ClosedAt:  "2026-04-29T18:00:00Z",
		ClosedVia: "slopshell",
	})
	if err != nil {
		t.Fatalf("SetStatus() error: %v", err)
	}

	params := objectAt(t, got, "params")
	if params["name"] != toolGTDSetStatus {
		t.Fatalf("tool name = %q, want %q", params["name"], toolGTDSetStatus)
	}
	args := objectAt(t, params, "arguments")
	if args["status"] != "closed" || args["closed_via"] != "slopshell" {
		t.Fatalf("status arguments = %#v", args)
	}
	if _, ok := args["local_overlay"]; ok {
		t.Fatalf("arguments should use sloptools overlay tool, got local_overlay: %#v", args)
	}
}

func TestBindRequiresProviderAndRef(t *testing.T) {
	client, err := NewClient("http://127.0.0.1:1", nil)
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	_, err = client.Bind(context.Background(), BindRequest{
		Sphere:         "work",
		WinnerPath:     "brain/commitments/winner.md",
		SourceBindings: []SourceBinding{{Provider: "github"}},
	})
	if err == nil {
		t.Fatal("Bind() error = nil, want missing ref error")
	}
}

func TestBindRequiresExtendedSourceBindingSchema(t *testing.T) {
	client, err := NewClient("http://127.0.0.1:1", nil)
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	_, err = client.Bind(context.Background(), BindRequest{
		Sphere:     "work",
		WinnerPath: "brain/commitments/winner.md",
		SourceBindings: []SourceBinding{{
			Provider:  "github",
			Ref:       "sloppy-org/slopshell#725",
			Location:  BindingLocation{Path: "brain/commitments/winner.md"},
			Writeable: false,
		}},
	})
	if err == nil {
		t.Fatal("Bind() error = nil, want missing authoritative_for error")
	}
}

func TestScanDecodesSloptoolsAggregateResult(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		writeMCPResult(t, w, map[string]any{"dedup": map[string]any{
			"aggregates": []map[string]any{{
				"id":           "gtd-aggregate-1",
				"paths":        []string{"brain/commitments/a.md"},
				"title":        "Send alpha budget",
				"outcome":      "Budget sent",
				"review_state": "open",
				"bindings": []map[string]any{{
					"provider":          "gitlab",
					"ref":               "plasma/slopshell#11",
					"location":          map[string]any{"path": "brain/commitments/a.md"},
					"writeable":         true,
					"authoritative_for": []string{"title", "status"},
				}},
				"binding_ids": []string{"gitlab:plasma/slopshell#11"},
			}},
			"changed": false,
		}})
	}))
	defer server.Close()

	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	result, err := client.Scan(context.Background(), ScanRequest{Sphere: "work"})
	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	params := objectAt(t, got, "params")
	if params["name"] != toolGTDDedupScan {
		t.Fatalf("tool name = %q, want %q", params["name"], toolGTDDedupScan)
	}
	if result.Aggregates[0].SourceBindings[0].Provider != "gitlab" {
		t.Fatalf("aggregate bindings = %#v", result.Aggregates[0].SourceBindings)
	}
	projection := result.Aggregates[0].Projection()
	if !reflect.DeepEqual(projection.SourceKinds, []string{SourceKindGitLab}) || !projection.Writeable {
		t.Fatalf("projection = %#v", projection)
	}
}

func TestParseCommitmentDecodesLocalOverlayForProjection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeMCPResult(t, w, map[string]any{"commitment": map[string]any{
			"title":  "Reply to reviewer",
			"status": "open",
			"source_bindings": []map[string]any{{
				"provider":          "markdown",
				"ref":               "brain/commitments/reviewer.md",
				"location":          map[string]any{"path": "brain/commitments/reviewer.md"},
				"writeable":         false,
				"authoritative_for": []string{"title", "status"},
			}},
			"local_overlay": map[string]any{
				"status":     "closed",
				"closed_via": "slopshell",
			},
		}})
	}))
	defer server.Close()

	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	commitment, err := client.ParseCommitment(context.Background(), ParseCommitmentRequest{
		Sphere: "work",
		Path:   "brain/commitments/reviewer.md",
	})
	if err != nil {
		t.Fatalf("ParseCommitment() error: %v", err)
	}

	projection := commitment.Projection("brain/commitments/reviewer.md")
	if projection.Status != "closed" || projection.ClosedVia != "slopshell" {
		t.Fatalf("projection overlay = %#v", projection)
	}
	if !reflect.DeepEqual(projection.SourceKinds, []string{SourceKindMarkdown}) {
		t.Fatalf("projection SourceKinds = %#v", projection.SourceKinds)
	}
}

func writeMCPResult(t *testing.T, w http.ResponseWriter, structured map[string]any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"result": map[string]any{
			"structuredContent": structured,
		},
	})
	if err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

func objectAt(t *testing.T, values map[string]any, key string) map[string]any {
	t.Helper()
	got, ok := values[key].(map[string]any)
	if !ok {
		t.Fatalf("%s = %#v, want object", key, values[key])
	}
	return got
}

func arrayAt(t *testing.T, values map[string]any, key string) []any {
	t.Helper()
	got, ok := values[key].([]any)
	if !ok {
		t.Fatalf("%s = %#v, want array", key, values[key])
	}
	return got
}

func stringSliceAt(t *testing.T, values map[string]any, key string) []string {
	t.Helper()
	raw := arrayAt(t, values, key)
	out := make([]string, 0, len(raw))
	for _, value := range raw {
		out = append(out, value.(string))
	}
	return out
}
