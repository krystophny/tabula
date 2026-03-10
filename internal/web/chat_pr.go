package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const githubPRCommandTimeout = 60 * time.Second

type ghCommandRunner func(ctx context.Context, cwd string, args ...string) (string, error)

type ghPRView struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	HeadRefName string `json:"headRefName"`
	BaseRefName string `json:"baseRefName"`
}

type ghPRReview struct {
	View      ghPRView
	Diff      string
	FileCount int
}

func runGitHubCLI(ctx context.Context, cwd string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	if commandDir := resolveGitHubCommandDir(cwd); commandDir != "" {
		cmd.Dir = commandDir
	}
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("github command timed out after %s", githubPRCommandTimeout)
		}
		if text == "" {
			text = err.Error()
		}
		return "", fmt.Errorf("gh %s failed: %s", strings.Join(args, " "), text)
	}
	return string(out), nil
}

func resolveGitHubCommandDir(cwd string) string {
	clean := strings.TrimSpace(cwd)
	candidates := []string{}
	if clean != "" {
		candidates = append(candidates, clean)
		dir := filepath.Clean(clean)
		for {
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			candidates = append(candidates, parent)
			dir = parent
		}
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, wd, filepath.Clean(filepath.Dir(wd)))
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates, exeDir, filepath.Clean(filepath.Dir(exeDir)))
	}
	for _, candidate := range candidates {
		if looksLikeGitRepo(candidate) {
			return candidate
		}
	}
	return clean
}

func looksLikeGitRepo(dir string) bool {
	command := exec.Command("git", "-C", dir, "rev-parse", "--is-inside-work-tree")
	out, err := command.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

func countUnifiedDiffFiles(diff string) int {
	count := 0
	for _, line := range strings.Split(strings.ReplaceAll(diff, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			count++
		}
	}
	return count
}

func (a *App) loadGitHubPRReview(projectKey, selector string) (ghPRReview, error) {
	runner := a.ghCommandRunner
	if runner == nil {
		runner = runGitHubCLI
	}
	cwd := strings.TrimSpace(a.cwdForProjectKey(projectKey))
	if cwd == "" {
		cwd = "."
	}
	ref := strings.TrimSpace(selector)
	if strings.EqualFold(ref, "refresh") {
		ref = ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), githubPRCommandTimeout)
	defer cancel()

	viewArgs := []string{"pr", "view"}
	if ref != "" {
		viewArgs = append(viewArgs, ref)
	}
	viewArgs = append(viewArgs, "--json", "number,title,url,headRefName,baseRefName")
	viewRaw, err := runner(ctx, cwd, viewArgs...)
	if err != nil {
		return ghPRReview{}, err
	}
	var view ghPRView
	if err := json.Unmarshal([]byte(viewRaw), &view); err != nil {
		return ghPRReview{}, fmt.Errorf("invalid github PR response: %w", err)
	}
	if view.Number <= 0 {
		return ghPRReview{}, errors.New("github PR number is missing")
	}

	// Use cumulative PR diff (not per-commit mailbox patches) so each file
	// appears as the current net change for review mode.
	diffArgs := []string{"pr", "diff", strconv.Itoa(view.Number)}
	diffRaw, err := runner(ctx, cwd, diffArgs...)
	if err != nil {
		return ghPRReview{}, err
	}
	diff := strings.TrimSpace(diffRaw)
	if diff == "" {
		return ghPRReview{}, fmt.Errorf("github PR #%d has no diff output", view.Number)
	}
	fileCount := countUnifiedDiffFiles(diff)
	if fileCount == 0 {
		fileCount = 1
	}
	return ghPRReview{
		View:      view,
		Diff:      diff,
		FileCount: fileCount,
	}, nil
}
