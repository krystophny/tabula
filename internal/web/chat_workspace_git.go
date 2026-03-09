package web

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/krystophny/tabura/internal/protocol"
	"github.com/krystophny/tabura/internal/store"
)

const workspaceCloneTimeout = 45 * time.Second

func parseCreateWorkspaceFromGitIntent(text string) *SystemAction {
	repoURL, targetPath, ok := parseWorkspaceCreateFromGitRequest(text)
	if !ok {
		return nil
	}
	params := map[string]interface{}{"repo_url": repoURL}
	if targetPath != "" {
		params["target_path"] = targetPath
	}
	return &SystemAction{Action: "create_workspace_from_git", Params: params}
}

func parseWorkspaceCreateFromGitRequest(raw string) (string, string, bool) {
	trimmed := strings.TrimSpace(raw)
	lower := strings.ToLower(trimmed)
	prefix := ""
	switch {
	case strings.HasPrefix(lower, "create workspace from "):
		prefix = "create workspace from "
	case strings.HasPrefix(lower, "add workspace from "):
		prefix = "add workspace from "
	default:
		return "", "", false
	}

	rest := strings.TrimSpace(trimmed[len(prefix):])
	rest = strings.TrimRight(rest, " \t\r\n!?,:;.")
	if strings.HasPrefix(strings.ToLower(rest), "git ") {
		rest = strings.TrimSpace(rest[len("git "):])
	}
	if rest == "" {
		return "", "", false
	}

	targetPath := ""
	for _, marker := range []string{" clone to ", " to "} {
		idx := strings.LastIndex(strings.ToLower(rest), marker)
		if idx <= 0 {
			continue
		}
		repoURL := strings.TrimSpace(rest[:idx])
		target := strings.TrimSpace(rest[idx+len(marker):])
		if looksLikeGitRepoURL(repoURL) && target != "" {
			return repoURL, target, true
		}
	}
	if !looksLikeGitRepoURL(rest) {
		return "", "", false
	}
	return rest, targetPath, true
}

func looksLikeGitRepoURL(raw string) bool {
	value := strings.TrimSpace(raw)
	if value == "" {
		return false
	}
	for _, prefix := range []string{"git@", "ssh://", "https://", "http://", "file://", "/", "./", "../", "~/"} {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return strings.Contains(value, ".git")
}

func systemActionGitRepoURL(params map[string]interface{}) string {
	for _, key := range []string{"repo_url", "url", "repo", "remote"} {
		value := strings.TrimSpace(fmt.Sprint(params[key]))
		if value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func systemActionGitTargetPath(params map[string]interface{}) string {
	for _, key := range []string{"target_path", "path", "clone_to", "target"} {
		value := strings.TrimSpace(fmt.Sprint(params[key]))
		if value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func workspaceRepoNameFromURL(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Scheme != "" {
		value = parsed.Path
	} else if strings.HasPrefix(value, "git@") {
		if idx := strings.Index(value, ":"); idx >= 0 && idx+1 < len(value) {
			value = value[idx+1:]
		}
	}
	value = strings.TrimRight(value, "/")
	base := path.Base(strings.ReplaceAll(value, "\\", "/"))
	base = strings.TrimSuffix(base, ".git")
	return strings.TrimSpace(base)
}

func workspaceCloneRootDir() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("TABURA_WORKSPACE_CLONE_ROOT")); configured != "" {
		return filepath.Abs(filepath.Clean(expandWorkspacePathReference(configured)))
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "code"), nil
}

func resolveWorkspaceCloneTarget(repoURL, targetPath string) (string, string, error) {
	if cleanTarget := strings.TrimSpace(targetPath); cleanTarget != "" {
		expanded := expandWorkspacePathReference(cleanTarget)
		absTarget, err := filepath.Abs(filepath.Clean(expanded))
		if err != nil {
			return "", "", err
		}
		return absTarget, filepath.Base(absTarget), nil
	}
	repoName := workspaceRepoNameFromURL(repoURL)
	if repoName == "" {
		return "", "", fmt.Errorf("unable to determine repository name from %q", repoURL)
	}
	rootDir, err := workspaceCloneRootDir()
	if err != nil {
		return "", "", err
	}
	absRoot, err := filepath.Abs(filepath.Clean(rootDir))
	if err != nil {
		return "", "", err
	}
	target := filepath.Join(absRoot, repoName)
	return target, repoName, nil
}

func cloneWorkspaceRepo(ctx context.Context, repoURL, targetDir string) error {
	cmd := exec.CommandContext(ctx, "git", "clone", "--", repoURL, targetDir)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	message := strings.TrimSpace(string(output))
	if message == "" {
		return err
	}
	return fmt.Errorf("git clone failed: %s", message)
}

func (a *App) createWorkspaceFromGit(repoURL, targetPath string) (store.Workspace, string, error) {
	cleanRepoURL := strings.TrimSpace(repoURL)
	if cleanRepoURL == "" {
		return store.Workspace{}, "", fmt.Errorf("git repository URL is required")
	}
	targetDir, workspaceName, err := resolveWorkspaceCloneTarget(cleanRepoURL, targetPath)
	if err != nil {
		return store.Workspace{}, "", err
	}
	if err := os.MkdirAll(filepath.Dir(targetDir), 0o755); err != nil {
		return store.Workspace{}, "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), workspaceCloneTimeout)
	defer cancel()
	if err := cloneWorkspaceRepo(ctx, cleanRepoURL, targetDir); err != nil {
		return store.Workspace{}, "", err
	}
	if _, err := protocol.BootstrapProject(targetDir); err != nil {
		return store.Workspace{}, "", err
	}

	workspace, err := a.store.CreateWorkspace(workspaceName, targetDir)
	if err != nil {
		return store.Workspace{}, "", err
	}
	if err := a.setActiveWorkspaceTracked(workspace.ID, "workspace_switch"); err != nil {
		return store.Workspace{}, "", err
	}
	workspace, err = a.store.GetWorkspace(workspace.ID)
	if err != nil {
		return store.Workspace{}, "", err
	}
	return workspace, cleanRepoURL, nil
}
