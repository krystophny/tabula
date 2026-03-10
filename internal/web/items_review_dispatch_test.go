package web

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/krystophny/tabura/internal/store"
)

func TestResolveReviewDispatchTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		target    string
		policy    string
		reviewer  string
		email     string
		want      string
		wantError bool
	}{
		{name: "explicit github", target: "github", want: store.ItemReviewTargetGitHub},
		{name: "always agent policy", target: "auto", policy: reviewPolicyAlwaysAgent, want: store.ItemReviewTargetAgent},
		{name: "agent then human policy", target: "", policy: reviewPolicyAgentThenHuman, want: store.ItemReviewTargetAgent},
		{name: "always human with reviewer", target: "auto", policy: reviewPolicyAlwaysHuman, reviewer: "alice", want: store.ItemReviewTargetGitHub},
		{name: "always human with email", target: "auto", policy: reviewPolicyAlwaysHuman, email: "alice@example.com", want: store.ItemReviewTargetEmail},
		{name: "missing target", wantError: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveReviewDispatchTarget(tc.target, tc.policy, tc.reviewer, tc.email)
			if tc.wantError {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveReviewDispatchTarget() error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("target = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestItemReviewDispatchGitHubAPI(t *testing.T) {
	app := newAuthedTestApp(t)

	workspace, item := reviewDispatchTestPRItem(t, app)
	var calls [][]string
	app.ghCommandRunner = func(_ context.Context, cwd string, args ...string) (string, error) {
		if cwd != workspace.DirPath {
			t.Fatalf("gh cwd = %q, want %q", cwd, workspace.DirPath)
		}
		calls = append(calls, append([]string(nil), args...))
		return "", nil
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(item.ID)+"/dispatch-review", map[string]any{
		"target":   "github",
		"reviewer": "octocat",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("dispatch github status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	got, err := app.store.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem() error: %v", err)
	}
	if got.State != store.ItemStateWaiting {
		t.Fatalf("item state = %q, want waiting", got.State)
	}
	if got.ReviewTarget == nil || *got.ReviewTarget != store.ItemReviewTargetGitHub {
		t.Fatalf("review_target = %v, want github", got.ReviewTarget)
	}
	if got.Reviewer == nil || *got.Reviewer != "octocat" {
		t.Fatalf("reviewer = %v, want octocat", got.Reviewer)
	}
	if len(calls) != 1 {
		t.Fatalf("gh call count = %d, want 1", len(calls))
	}
	command := strings.Join(calls[0], " ")
	if !strings.Contains(command, "pr edit 21 --add-reviewer octocat") {
		t.Fatalf("gh args = %q, want pr edit add-reviewer", command)
	}
}

func TestItemReviewDispatchEmailAPI(t *testing.T) {
	app := newAuthedTestApp(t)

	_, item := reviewDispatchTestPRItem(t, app)
	var sent reviewDispatchEmailMessage
	app.reviewEmailSender = func(_ context.Context, message reviewDispatchEmailMessage) error {
		sent = message
		return nil
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(item.ID)+"/dispatch-review", map[string]any{
		"target": "email",
		"email":  "reviewer@example.com",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("dispatch email status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	got, err := app.store.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem() error: %v", err)
	}
	if got.ReviewTarget == nil || *got.ReviewTarget != store.ItemReviewTargetEmail {
		t.Fatalf("review_target = %v, want email", got.ReviewTarget)
	}
	if got.Reviewer == nil || *got.Reviewer != "reviewer@example.com" {
		t.Fatalf("reviewer = %v, want reviewer@example.com", got.Reviewer)
	}
	if sent.To != "reviewer@example.com" {
		t.Fatalf("email to = %q, want reviewer@example.com", sent.To)
	}
	if !strings.Contains(sent.Body, "https://github.com/owner/tabula/pull/21") {
		t.Fatalf("email body = %q, want PR URL", sent.Body)
	}
}

func TestItemReviewDispatchAgentRerouteCancelsInFlightWork(t *testing.T) {
	app := newAuthedTestApp(t)

	workspace, item := reviewDispatchTestPRItem(t, app)
	started := make(chan struct{}, 1)
	canceled := make(chan struct{}, 1)
	app.workspaceWatchProcessor = func(ctx context.Context, _ store.Workspace, _ store.ItemSummary) error {
		started <- struct{}{}
		<-ctx.Done()
		canceled <- struct{}{}
		return ctx.Err()
	}
	app.ghCommandRunner = func(_ context.Context, cwd string, args ...string) (string, error) {
		if cwd != workspace.DirPath {
			t.Fatalf("gh cwd = %q, want %q", cwd, workspace.DirPath)
		}
		return "", nil
	}

	rrAgent := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(item.ID)+"/dispatch-review", map[string]any{
		"target": "agent",
	})
	if rrAgent.Code != http.StatusOK {
		t.Fatalf("dispatch agent status = %d, want 200: %s", rrAgent.Code, rrAgent.Body.String())
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("agent dispatch did not start")
	}

	rrGitHub := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/"+itoa(item.ID)+"/dispatch-review", map[string]any{
		"target":   "github",
		"reviewer": "alice",
	})
	if rrGitHub.Code != http.StatusOK {
		t.Fatalf("reroute github status = %d, want 200: %s", rrGitHub.Code, rrGitHub.Body.String())
	}
	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("agent dispatch was not canceled on reroute")
	}

	time.Sleep(50 * time.Millisecond)
	got, err := app.store.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem() error: %v", err)
	}
	if got.State != store.ItemStateWaiting {
		t.Fatalf("item state = %q, want waiting", got.State)
	}
	if got.ReviewTarget == nil || *got.ReviewTarget != store.ItemReviewTargetGitHub {
		t.Fatalf("review_target = %v, want github", got.ReviewTarget)
	}
	if got.Reviewer == nil || *got.Reviewer != "alice" {
		t.Fatalf("reviewer = %v, want alice", got.Reviewer)
	}
}

func TestItemReviewDispatchRejectsInvalidInputs(t *testing.T) {
	app := newAuthedTestApp(t)

	_, item := reviewDispatchTestPRItem(t, app)
	nonPR, err := app.store.CreateItem("Non PR", store.ItemOptions{})
	if err != nil {
		t.Fatalf("CreateItem(nonPR) error: %v", err)
	}

	tests := []struct {
		name   string
		path   string
		body   map[string]any
		status int
	}{
		{
			name:   "invalid email",
			path:   "/api/items/" + itoa(item.ID) + "/dispatch-review",
			body:   map[string]any{"target": "email", "email": "bad address"},
			status: http.StatusBadRequest,
		},
		{
			name:   "missing reviewer",
			path:   "/api/items/" + itoa(item.ID) + "/dispatch-review",
			body:   map[string]any{"target": "github"},
			status: http.StatusBadRequest,
		},
		{
			name:   "not pr item",
			path:   "/api/items/" + itoa(nonPR.ID) + "/dispatch-review",
			body:   map[string]any{"target": "agent"},
			status: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, tc.path, tc.body)
			if rr.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.status, rr.Body.String())
			}
		})
	}
}

func reviewDispatchTestPRItem(t *testing.T, app *App) (store.Workspace, store.Item) {
	t.Helper()

	repoDir := filepath.Join(t.TempDir(), "workspace")
	initGitHubWorkspaceRepo(t, repoDir, "https://github.com/owner/tabula.git")
	workspace, err := app.store.CreateWorkspace("Repo", repoDir)
	if err != nil {
		t.Fatalf("CreateWorkspace() error: %v", err)
	}
	refURL := "https://github.com/owner/tabula/pull/21"
	title := "PR #21"
	artifact, err := app.store.CreateArtifact(store.ArtifactKindGitHubPR, nil, &refURL, &title, nil)
	if err != nil {
		t.Fatalf("CreateArtifact() error: %v", err)
	}
	source := "github"
	sourceRef := "owner/tabula#PR-21"
	item, err := app.store.CreateItem("Review parser cleanup", store.ItemOptions{
		WorkspaceID: &workspace.ID,
		ArtifactID:  &artifact.ID,
		Source:      &source,
		SourceRef:   &sourceRef,
	})
	if err != nil {
		t.Fatalf("CreateItem() error: %v", err)
	}
	return workspace, item
}
