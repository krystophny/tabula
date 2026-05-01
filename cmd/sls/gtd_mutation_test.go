package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type capturedGtdMutation struct {
	method string
	path   string
	body   map[string]any
}

func newGtdMutationHarness(t *testing.T, handler http.HandlerFunc) (*httptest.Server, string, *[]capturedGtdMutation) {
	t.Helper()
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "cli-token")
	if err := os.WriteFile(tokenPath, []byte("test-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	captures := []capturedGtdMutation{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/cli/login", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if r.Body != nil && r.ContentLength != 0 {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
		}
		captures = append(captures, capturedGtdMutation{method: r.Method, path: r.URL.Path, body: body})
		handler(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, tokenPath, &captures
}

func runGtdMutationCommand(t *testing.T, srv *httptest.Server, token string, args ...string) (string, string, int) {
	t.Helper()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := handleGtdCommand(args, cliOptions{baseURL: srv.URL, tokenFile: token}, stdout, stderr)
	return stdout.String(), stderr.String(), code
}

func TestHandleGtdCloseRoutesThroughGTDStatusEndpoint(t *testing.T) {
	srv, token, captures := newGtdMutationHarness(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/items/42/gtd-status" {
			t.Fatalf("request = %s %s, want PUT /api/items/42/gtd-status", r.Method, r.URL.Path)
		}
		writeGtdMutationJSON(t, w, `{"item":{"id":42,"title":"Done","kind":"action","state":"done","sphere":"work"}}`)
	})

	out, stderr, code := runGtdMutationCommand(t, srv, token, "close", "#42")
	if code != 0 {
		t.Fatalf("exit code = %d stderr=%s", code, stderr)
	}
	if got := (*captures)[0].body["state"]; got != "done" {
		t.Fatalf("state body = %#v, want done", (*captures)[0].body)
	}
	if !strings.Contains(out, "closed #42 state=done") {
		t.Fatalf("output = %q", out)
	}
}

func TestHandleGtdDeferRoutesThroughItemUpdate(t *testing.T) {
	followUp := "2026-05-04T09:00:00Z"
	srv, token, captures := newGtdMutationHarness(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/items/43" {
			t.Fatalf("request = %s %s, want PUT /api/items/43", r.Method, r.URL.Path)
		}
		writeGtdMutationJSON(t, w, `{"item":{"id":43,"title":"Later","kind":"action","state":"deferred","sphere":"work","follow_up_at":"`+followUp+`"}}`)
	})

	out, stderr, code := runGtdMutationCommand(t, srv, token, "defer", "43", followUp)
	if code != 0 {
		t.Fatalf("exit code = %d stderr=%s", code, stderr)
	}
	if got := (*captures)[0].body["state"]; got != "deferred" {
		t.Fatalf("state body = %#v, want deferred", (*captures)[0].body)
	}
	if got := (*captures)[0].body["follow_up_at"]; got != followUp {
		t.Fatalf("follow_up_at body = %#v, want %s", (*captures)[0].body, followUp)
	}
	if !strings.Contains(out, "deferred #43 follow_up="+followUp) {
		t.Fatalf("output = %q", out)
	}
}

func TestHandleGtdDelegateAndRouteStaySeparate(t *testing.T) {
	srv, token, captures := newGtdMutationHarness(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/items/44/assign":
			writeGtdMutationJSON(t, w, `{"item":{"id":44,"title":"Wait","kind":"action","state":"waiting","sphere":"work","actor_id":7}}`)
		case "/api/items/44/workspace":
			writeGtdMutationJSON(t, w, `{"item":{"id":44,"title":"Wait","kind":"action","state":"waiting","sphere":"work","workspace_id":9}}`)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	})

	_, stderr, code := runGtdMutationCommand(t, srv, token, "delegate", "44", "7")
	if code != 0 {
		t.Fatalf("delegate exit code = %d stderr=%s", code, stderr)
	}
	out, stderr, code := runGtdMutationCommand(t, srv, token, "route", "44", "9")
	if code != 0 {
		t.Fatalf("route exit code = %d stderr=%s", code, stderr)
	}
	if (*captures)[0].path != "/api/items/44/assign" || (*captures)[0].body["actor_id"] != float64(7) {
		t.Fatalf("delegate capture = %#v", (*captures)[0])
	}
	if (*captures)[1].path != "/api/items/44/workspace" || (*captures)[1].body["workspace_id"] != float64(9) {
		t.Fatalf("route capture = %#v", (*captures)[1])
	}
	if !strings.Contains(out, "routed #44 workspace=9") {
		t.Fatalf("route output = %q", out)
	}
}

func TestHandleGtdDropRequiresExplicitIntentForSourceBackedItem(t *testing.T) {
	deleted := false
	srv, token, _ := newGtdMutationHarness(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/items/45":
			writeGtdMutationJSON(t, w, `{"item":{"id":45,"title":"Remote","kind":"action","state":"inbox","sphere":"work","source":"todoist"}}`)
		case r.Method == http.MethodDelete:
			deleted = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})

	_, stderr, code := runGtdMutationCommand(t, srv, token, "drop", "45")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if deleted {
		t.Fatal("source-backed drop without --yes reached DELETE")
	}
	if !strings.Contains(stderr, "requires --yes") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestHandleGtdLinkAndUnlinkProjectUseProjectItemOverlay(t *testing.T) {
	srv, token, captures := newGtdMutationHarness(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			writeGtdMutationJSON(t, w, `{"item":{"id":46,"title":"Action","kind":"action","state":"next","sphere":"work"},"links":[{"parent_item_id":90,"child_item_id":46,"role":"support"}]}`)
		case http.MethodDelete:
			writeGtdMutationJSON(t, w, `{"item":{"id":46,"title":"Action","kind":"action","state":"next","sphere":"work"},"links":[]}`)
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	})

	out, stderr, code := runGtdMutationCommand(t, srv, token, "link-project", "46", "90", "--role", "support")
	if code != 0 {
		t.Fatalf("link exit code = %d stderr=%s", code, stderr)
	}
	if (*captures)[0].path != "/api/items/46/project-item-link" || (*captures)[0].body["project_item_id"] != float64(90) || (*captures)[0].body["role"] != "support" {
		t.Fatalf("link capture = %#v", (*captures)[0])
	}
	if !strings.Contains(out, "linked #46 project_item=90 role=support") {
		t.Fatalf("link output = %q", out)
	}

	out, stderr, code = runGtdMutationCommand(t, srv, token, "unlink-project", "46", "90")
	if code != 0 {
		t.Fatalf("unlink exit code = %d stderr=%s", code, stderr)
	}
	if (*captures)[1].method != http.MethodDelete || (*captures)[1].path != "/api/items/46/project-item-link" || (*captures)[1].body["project_item_id"] != float64(90) {
		t.Fatalf("unlink capture = %#v", (*captures)[1])
	}
	if !strings.Contains(out, "unlinked #46 project_item=90") {
		t.Fatalf("unlink output = %q", out)
	}
}

func writeGtdMutationJSON(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatalf("write response: %v", err)
	}
}
