package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsGtdSubcommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"bare gtd", []string{"gtd"}, true},
		{"gtd inbox", []string{"gtd", "inbox"}, true},
		{"gtd next with flags", []string{"gtd", "next", "--vault", "work"}, true},
		{"gtd projects", []string{"gtd", "projects"}, true},
		{"gtd later alias", []string{"gtd", "later"}, true},
		{"gtd deferred alias", []string{"gtd", "deferred"}, true},
		{"gtd defer alias", []string{"gtd", "defer"}, true},
		{"gtd someday alias", []string{"gtd", "maybe"}, true},
		{"gtd unknown queue", []string{"gtd", "bogus"}, false},
		{"top-level flags then gtd", []string{"--base-url", "http://x", "gtd", "review"}, true},
		{"chat", []string{"chat"}, false},
		{"empty", []string{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isGtdSubcommand(tt.args); got != tt.want {
				t.Errorf("isGtdSubcommand(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestFindGtdQueue(t *testing.T) {
	cases := map[string]string{
		"inbox":    "inbox",
		"INBOX":    "inbox",
		"next":     "next",
		"waiting":  "waiting",
		"later":    "later",
		"deferred": "later",
		"defer":    "later",
		"someday":  "someday",
		"maybe":    "someday",
		"review":   "review",
		"projects": "projects",
		" inbox ":  "inbox",
	}
	for input, wantName := range cases {
		t.Run(input, func(t *testing.T) {
			q := findGtdQueue(input)
			if q == nil {
				t.Fatalf("findGtdQueue(%q) = nil, want %q", input, wantName)
			}
			if q.Name != wantName {
				t.Errorf("findGtdQueue(%q).Name = %q, want %q", input, q.Name, wantName)
			}
		})
	}
	if findGtdQueue("nope") != nil {
		t.Error("findGtdQueue(nope) should be nil")
	}
	if findGtdQueue("") != nil {
		t.Error("findGtdQueue(empty) should be nil")
	}
}

func TestCommandArgsForGtd(t *testing.T) {
	args := []string{"--base-url", "http://x", "gtd", "next", "--vault", "work"}
	got := commandArgs(args)
	want := []string{"next", "--vault", "work"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("commandArgs(%v) = %v, want %v", args, got, want)
	}
}

func TestCommandArgsForBareGtd(t *testing.T) {
	args := []string{"gtd"}
	got := commandArgs(args)
	if len(got) != 1 || got[0] != "gtd" {
		t.Fatalf("commandArgs(gtd) = %v, want [gtd]", got)
	}
}

func TestParseGtdFiltersDefaults(t *testing.T) {
	f, err := parseGtdFilters(nil)
	if err != nil {
		t.Fatalf("parseGtdFilters(nil) err = %v", err)
	}
	if f != (gtdFilters{}) {
		t.Errorf("parseGtdFilters(nil) = %#v, want zero", f)
	}
}

func TestParseGtdFiltersAll(t *testing.T) {
	args := []string{
		"--vault", "work",
		"--source", "todoist",
		"--source-container", "Inbox",
		"--label", "deep-work",
		"--actor-id", "5",
		"--workspace", "12",
		"--project-item-id", "99",
		"--due-before", "2026-05-10T00:00:00Z",
		"--due-after", "2026-04-01T00:00:00Z",
		"--follow-up-before", "2026-05-15T00:00:00Z",
		"--follow-up-after", "2026-04-15T00:00:00Z",
		"--json",
	}
	f, err := parseGtdFilters(args)
	if err != nil {
		t.Fatalf("parseGtdFilters err = %v", err)
	}
	q := f.query()
	want := url.Values{
		"sphere":           {"work"},
		"source":           {"todoist"},
		"source_container": {"Inbox"},
		"label":            {"deep-work"},
		"actor_id":         {"5"},
		"workspace_id":     {"12"},
		"project_item_id":  {"99"},
		"due_before":       {"2026-05-10T00:00:00Z"},
		"due_after":        {"2026-04-01T00:00:00Z"},
		"follow_up_before": {"2026-05-15T00:00:00Z"},
		"follow_up_after":  {"2026-04-15T00:00:00Z"},
	}
	if got := q.Encode(); got != want.Encode() {
		t.Errorf("query() = %q, want %q", got, want.Encode())
	}
	if !f.jsonOut {
		t.Error("--json should set jsonOut")
	}
}

func TestParseGtdFiltersProjectAliases(t *testing.T) {
	for _, alias := range []string{"--project", "--project-item", "--project-item-id"} {
		t.Run(alias, func(t *testing.T) {
			f, err := parseGtdFilters([]string{alias, "42"})
			if err != nil {
				t.Fatalf("parseGtdFilters(%s) err = %v", alias, err)
			}
			if got := f.query().Get("project_item_id"); got != "42" {
				t.Errorf("project_item_id = %q, want 42", got)
			}
		})
	}
}

func TestParseGtdFiltersWorkspaceNull(t *testing.T) {
	f, err := parseGtdFilters([]string{"--workspace", "null"})
	if err != nil {
		t.Fatalf("parseGtdFilters err = %v", err)
	}
	if got := f.query().Get("workspace_id"); got != "null" {
		t.Errorf("workspace_id = %q, want null", got)
	}
}

func TestParseGtdFiltersRejectsExtraPositional(t *testing.T) {
	if _, err := parseGtdFilters([]string{"oops"}); err == nil {
		t.Fatal("parseGtdFilters with extra positional should error")
	}
}

func TestParseGtdFiltersRejectsUnknownFlag(t *testing.T) {
	if _, err := parseGtdFilters([]string{"--bogus"}); err == nil {
		t.Fatal("parseGtdFilters with unknown flag should error")
	}
}

func TestRenderGtdItemsEmpty(t *testing.T) {
	var buf bytes.Buffer
	body := []byte(`{"items":[]}`)
	if err := renderGtdResponse(&buf, gtdQueueList[0], gtdFilters{}, body); err != nil {
		t.Fatalf("renderGtdResponse err = %v", err)
	}
	if !strings.Contains(buf.String(), "(empty)") {
		t.Errorf("expected '(empty)' marker, got %q", buf.String())
	}
}

func TestRenderGtdItems(t *testing.T) {
	var buf bytes.Buffer
	body := []byte(`{
		"items":[
			{"id":42,"title":"Reply to Andrei","kind":"action","state":"next","sphere":"work",
			 "source":"todoist","actor_id":7,"actor_name":"Andrei","due_at":"2026-04-30T12:00:00Z"},
			{"id":43,"title":"Read paper","kind":"action","state":"next","sphere":"private","follow_up_at":"2026-05-02T00:00:00Z"}
		],
		"overdue":[42]
	}`)
	queue := *findGtdQueue("next")
	if err := renderGtdResponse(&buf, queue, gtdFilters{}, body); err != nil {
		t.Fatalf("renderGtdResponse err = %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Next actions (2 items, 1 overdue)") {
		t.Errorf("missing header, got %q", out)
	}
	if !strings.Contains(out, "#42") || !strings.Contains(out, "[overdue]") || !strings.Contains(out, "[todoist]") {
		t.Errorf("missing overdue/todoist markers: %q", out)
	}
	if !strings.Contains(out, "actor=Andrei") {
		t.Errorf("missing actor name: %q", out)
	}
	if !strings.Contains(out, "#43") || !strings.Contains(out, "follow_up=2026-05-02T00:00:00Z") {
		t.Errorf("missing follow-up details: %q", out)
	}
	if !strings.Contains(out, "[local]") {
		t.Errorf("expected local marker for sourceless item: %q", out)
	}
}

func TestRenderGtdProjects(t *testing.T) {
	var buf bytes.Buffer
	body := []byte(`{
		"project_items":[
			{
				"item":{"id":100,"title":"Onboarding revamp","kind":"project","state":"next","sphere":"work"},
				"health":{"has_next_action":false,"stalled":true},
				"children":{"next":0,"waiting":1,"deferred":0,"someday":0,"review":0,"done":2}
			},
			{
				"item":{"id":101,"title":"Conference talk","kind":"project","state":"next","sphere":"work"},
				"health":{"has_next_action":true,"stalled":false},
				"children":{"next":2,"waiting":0,"deferred":0,"someday":0,"review":0,"done":0}
			}
		],
		"total":2,
		"stalled":1
	}`)
	queue := *findGtdQueue("projects")
	if err := renderGtdResponse(&buf, queue, gtdFilters{}, body); err != nil {
		t.Fatalf("renderGtdResponse err = %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Project items (2 total, 1 stalled)") {
		t.Errorf("missing header, got %q", out)
	}
	if !strings.Contains(out, "#100") || !strings.Contains(out, "[stall]") {
		t.Errorf("missing stalled marker: %q", out)
	}
	if !strings.Contains(out, "#101") || strings.Contains(stallLineFor(out, "#101"), "[stall]") {
		t.Errorf("non-stalled project should not be marked: %q", out)
	}
	if !strings.Contains(out, "next=0 waiting=1") {
		t.Errorf("missing child counts: %q", out)
	}
}

func stallLineFor(out, prefix string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, prefix) {
			return line
		}
	}
	return ""
}

func TestRenderGtdJSONPassthrough(t *testing.T) {
	var buf bytes.Buffer
	body := []byte(`{"items":[{"id":1,"title":"x","kind":"action","state":"inbox","sphere":"work"}]}`)
	if err := renderGtdResponse(&buf, *findGtdQueue("inbox"), gtdFilters{jsonOut: true}, body); err != nil {
		t.Fatalf("renderGtdResponse err = %v", err)
	}
	out := strings.TrimSpace(buf.String())
	if out != strings.TrimSpace(string(body)) {
		t.Errorf("json passthrough = %q, want %q", out, string(body))
	}
}

// gtdServerCapture records the request URL of the most recent JSON list call.
type gtdServerCapture struct {
	last *http.Request
}

func newGtdTestServer(t *testing.T, capture *gtdServerCapture, responses map[string]string) (*httptest.Server, string) {
	t.Helper()
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "cli-token")
	if err := os.WriteFile(tokenPath, []byte("test-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/cli/login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	for path, body := range responses {
		body := body
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			capture.last = r.Clone(r.Context())
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(body))
		})
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, tokenPath
}

func runGtdHarness(t *testing.T, queue, body string, args []string) (*gtdServerCapture, string, int) {
	t.Helper()
	cap := &gtdServerCapture{}
	q := findGtdQueue(queue)
	if q == nil {
		t.Fatalf("unknown queue %q", queue)
	}
	srv, token := newGtdTestServer(t, cap, map[string]string{q.Path: body})
	opts := cliOptions{baseURL: srv.URL, tokenFile: token}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := handleGtdCommand(append([]string{queue}, args...), opts, stdout, stderr)
	if code != 0 {
		t.Logf("stderr: %s", stderr.String())
	}
	return cap, stdout.String(), code
}

func TestHandleGtdInboxForwardsSourceFilter(t *testing.T) {
	cap, _, code := runGtdHarness(t, "inbox",
		`{"items":[{"id":1,"title":"capture","kind":"action","state":"inbox","sphere":"work","source":"imap"}]}`,
		[]string{"--vault", "work", "--source", "imap"})
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if cap.last == nil {
		t.Fatal("server received no request")
	}
	if got := cap.last.URL.Query().Get("source"); got != "imap" {
		t.Errorf("source query = %q, want imap", got)
	}
	if got := cap.last.URL.Query().Get("sphere"); got != "work" {
		t.Errorf("sphere query = %q, want work", got)
	}
}

func TestHandleGtdNextForwardsWorkspaceFilter(t *testing.T) {
	cap, _, code := runGtdHarness(t, "next",
		`{"items":[],"overdue":[]}`,
		[]string{"--workspace", "12"})
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got := cap.last.URL.Query().Get("workspace_id"); got != "12" {
		t.Errorf("workspace_id = %q, want 12", got)
	}
}

func TestHandleGtdWaitingForwardsSourceContainerFilter(t *testing.T) {
	cap, _, code := runGtdHarness(t, "waiting",
		`{"items":[]}`,
		[]string{"--source-container", "Triage"})
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got := cap.last.URL.Query().Get("source_container"); got != "Triage" {
		t.Errorf("source_container = %q, want Triage", got)
	}
}

func TestHandleGtdLaterForwardsProjectItemFilter(t *testing.T) {
	cap, out, code := runGtdHarness(t, "later",
		`{"items":[{"id":7,"title":"deferred-task","kind":"action","state":"later","sphere":"work"}]}`,
		[]string{"--project-item-id", "42"})
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got := cap.last.URL.Path; got != "/api/items/deferred" {
		t.Errorf("later alias should hit /api/items/deferred, got %q", got)
	}
	if got := cap.last.URL.Query().Get("project_item_id"); got != "42" {
		t.Errorf("project_item_id = %q, want 42", got)
	}
	if !strings.Contains(out, "Later") || !strings.Contains(out, "#7") {
		t.Errorf("expected later listing, got %q", out)
	}
}

func TestHandleGtdProjectsForwardsListFilters(t *testing.T) {
	cap, out, code := runGtdHarness(t, "projects",
		`{"project_items":[{"item":{"id":11,"title":"Outcome","kind":"project","state":"next","sphere":"work"},"health":{"stalled":false},"children":{"next":1,"waiting":0,"deferred":0,"someday":0,"review":0,"done":0}}],"total":1,"stalled":0}`,
		[]string{"--vault", "work", "--label", "deep"})
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got := cap.last.URL.Query().Get("sphere"); got != "work" {
		t.Errorf("sphere = %q, want work", got)
	}
	if got := cap.last.URL.Query().Get("label"); got != "deep" {
		t.Errorf("label = %q, want deep", got)
	}
	if !strings.Contains(out, "Project items (1 total, 0 stalled)") {
		t.Errorf("missing project header, got %q", out)
	}
	if !strings.Contains(out, "#11") {
		t.Errorf("missing project row, got %q", out)
	}
}

func TestHandleGtdReviewQueue(t *testing.T) {
	cap, out, code := runGtdHarness(t, "review",
		`{"items":[{"id":3,"title":"stalled","kind":"project","state":"review","sphere":"work"}]}`,
		nil)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if cap.last.URL.Path != "/api/items/review" {
		t.Errorf("expected /api/items/review path, got %q", cap.last.URL.Path)
	}
	if !strings.Contains(out, "Review") || !strings.Contains(out, "#3") {
		t.Errorf("missing review output, got %q", out)
	}
}

func TestHandleGtdSomedayJSONOutput(t *testing.T) {
	body := `{"items":[{"id":2,"title":"someday-thing","kind":"action","state":"someday","sphere":"private"}]}`
	_, out, code := runGtdHarness(t, "someday", body, []string{"--json"})
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if strings.TrimSpace(out) != strings.TrimSpace(body) {
		t.Errorf("--json passthrough mismatch: got %q, want %q", out, body)
	}
}

func TestHandleGtdRejectsUnknownQueue(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := handleGtdCommand([]string{"bogus"}, cliOptions{baseURL: "http://127.0.0.1:1"}, stdout, stderr)
	if code != 2 {
		t.Errorf("unknown queue exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unknown gtd subcommand") {
		t.Errorf("missing usage error, got %q", stderr.String())
	}
}

func TestHandleGtdBareUsage(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := handleGtdCommand([]string{"gtd"}, cliOptions{baseURL: "http://127.0.0.1:1"}, stdout, stderr)
	if code != 2 {
		t.Errorf("bare gtd exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "queues:") {
		t.Errorf("expected usage text, got %q", stderr.String())
	}
}

func TestHandleGtdRejectsBadFilter(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := handleGtdCommand([]string{"inbox", "--unknown-flag"}, cliOptions{baseURL: "http://127.0.0.1:1"}, stdout, stderr)
	if code != 2 {
		t.Errorf("bad flag exit code = %d, want 2", code)
	}
}
