package web

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCountUnifiedDiffFiles(t *testing.T) {
	diff := strings.Join([]string{
		"diff --git a/a.txt b/a.txt",
		"--- a/a.txt",
		"+++ b/a.txt",
		"@@ -1 +1 @@",
		"-one",
		"+two",
		"diff --git a/b.txt b/b.txt",
		"--- a/b.txt",
		"+++ b/b.txt",
		"@@ -1 +1 @@",
		"-x",
		"+y",
	}, "\n")
	if got := countUnifiedDiffFiles(diff); got != 2 {
		t.Fatalf("countUnifiedDiffFiles = %d, want 2", got)
	}
}

func TestLoadGitHubPRReview(t *testing.T) {
	app := newAuthedTestApp(t)
	project, err := app.ensureDefaultProjectRecord()
	if err != nil {
		t.Fatalf("ensure default project: %v", err)
	}

	var calls [][]string
	app.ghCommandRunner = func(_ context.Context, _ string, args ...string) (string, error) {
		copied := append([]string(nil), args...)
		calls = append(calls, copied)
		if len(args) >= 2 && args[0] == "pr" && args[1] == "view" {
			return `{"number":17,"title":"Fix parser","url":"https://example.invalid/pr/17","headRefName":"feat/fix","baseRefName":"main"}`, nil
		}
		if len(args) >= 2 && args[0] == "pr" && args[1] == "diff" {
			return strings.Join([]string{
				"diff --git a/main.go b/main.go",
				"--- a/main.go",
				"+++ b/main.go",
				"@@ -1 +1 @@",
				"-old",
				"+new",
			}, "\n"), nil
		}
		return "", errors.New("unexpected command")
	}

	review, err := app.loadGitHubPRReview(project.ProjectKey, "17")
	if err != nil {
		t.Fatalf("loadGitHubPRReview: %v", err)
	}
	if review.View.Number != 17 {
		t.Fatalf("PR number = %d, want 17", review.View.Number)
	}
	if review.FileCount != 1 {
		t.Fatalf("file count = %d, want 1", review.FileCount)
	}
	if !strings.Contains(review.Diff, "diff --git a/main.go b/main.go") {
		t.Fatalf("unexpected diff content: %q", review.Diff)
	}
	if len(calls) != 2 {
		t.Fatalf("gh call count = %d, want 2", len(calls))
	}
	if got := strings.Join(calls[0], " "); !strings.Contains(got, "pr view 17") {
		t.Fatalf("view call mismatch: %q", got)
	}
	if got := strings.Join(calls[1], " "); !strings.Contains(got, "pr diff 17") || strings.Contains(got, "--patch") {
		t.Fatalf("diff call mismatch: %q", got)
	}
}
