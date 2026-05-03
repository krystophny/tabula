package protocol

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBootstrapProjectCreatesExpectedFilesWithoutAgentsMutation(t *testing.T) {
	projectDir := t.TempDir()

	result, err := BootstrapProject(projectDir)
	if err != nil {
		t.Fatalf("BootstrapProject() error = %v", err)
	}
	if result.GitInitialized {
		t.Fatalf("GitInitialized = true, want false")
	}
	if result.Paths.WorkspaceDir == "" {
		t.Fatalf("WorkspaceDir should not be empty")
	}
	if _, err := os.Stat(filepath.Join(projectDir, "AGENTS.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("AGENTS.md should not be created, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, ".slopshell", "AGENTS.slopshell.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("AGENTS.slopshell.md should not be created, stat err = %v", err)
	}

	mcpBody, err := os.ReadFile(result.Paths.MCPConfigPath)
	if err != nil {
		t.Fatalf("read mcp config: %v", err)
	}
	body := string(mcpBody)
	if strings.Contains(body, "[mcp_servers.slopshell]") {
		t.Fatalf("bootstrap mcp config should not register slopshell: %q", body)
	}
	if !strings.Contains(body, "[mcp_servers.sloppy]") {
		t.Fatalf("bootstrap mcp config missing sloppy server block")
	}
	if !strings.Contains(body, "mcp-server") {
		t.Fatalf("mcp config missing mcp-server invocation")
	}
	if !strings.Contains(body, "--project-dir") {
		t.Fatalf("mcp config missing --project-dir flag")
	}

	gitignoreBody, err := os.ReadFile(filepath.Join(projectDir, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if !strings.Contains(string(gitignoreBody), ".slopshell/artifacts/") {
		t.Fatalf(".gitignore missing .slopshell/artifacts/ entry")
	}
}

func TestBootstrapProjectPreservesExistingAgentsAndDetectsGit(t *testing.T) {
	projectDir := t.TempDir()
	agentsPath := filepath.Join(projectDir, "AGENTS.md")
	initial := "custom agents content\n"
	if err := os.WriteFile(agentsPath, []byte(initial), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
	if err := os.Mkdir(filepath.Join(projectDir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	result, err := BootstrapProject(projectDir)
	if err != nil {
		t.Fatalf("BootstrapProject() error = %v", err)
	}
	if !result.GitInitialized {
		t.Fatalf("GitInitialized = false, want true")
	}

	agentsBody, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if string(agentsBody) != initial {
		t.Fatalf("AGENTS.md was unexpectedly modified")
	}
	if _, err := os.Stat(filepath.Join(projectDir, ".slopshell", "AGENTS.slopshell.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("AGENTS.slopshell.md should not be created, stat err = %v", err)
	}
}

func TestEnsureGitignoreAppendsEntryOnlyOnce(t *testing.T) {
	projectDir := t.TempDir()
	gitignorePath := filepath.Join(projectDir, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte("node_modules/\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	if err := ensureGitignore(projectDir); err != nil {
		t.Fatalf("ensureGitignore() first call: %v", err)
	}
	if err := ensureGitignore(projectDir); err != nil {
		t.Fatalf("ensureGitignore() second call: %v", err)
	}

	body, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	content := string(body)
	if strings.Count(content, ".slopshell/artifacts/") != 1 {
		t.Fatalf("expected .slopshell/artifacts/ exactly once, got content:\n%s", content)
	}
}
