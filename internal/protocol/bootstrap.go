package protocol

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Paths struct {
	WorkspaceDir  string
	MCPConfigPath string
}

type Result struct {
	Paths          Paths
	GitInitialized bool
}

func BootstrapProject(projectDir string) (Result, error) {
	abs, err := filepath.Abs(projectDir)
	if err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return Result{}, err
	}
	slopshellDir := filepath.Join(abs, ".slopshell")
	if err := os.MkdirAll(slopshellDir, 0o755); err != nil {
		return Result{}, err
	}
	paths := Paths{
		WorkspaceDir:  abs,
		MCPConfigPath: filepath.Join(slopshellDir, "codex-mcp.toml"),
	}
	config := fmt.Sprintf(
		"[mcp_servers.sloppy]\ncommand = \"sloptools\"\nargs = [\"mcp-server\", \"--project-dir\", \"%s\", \"--data-dir\", \"$HOME/.local/share/sloppy\"]\n",
		strings.ReplaceAll(abs, "\\", "\\\\"),
	)
	_ = os.WriteFile(paths.MCPConfigPath, []byte(config), 0o644)
	_ = ensureGitignore(abs)
	gitInit := false
	if _, err := os.Stat(filepath.Join(abs, ".git")); err == nil {
		gitInit = true
	}
	return Result{Paths: paths, GitInitialized: gitInit}, nil
}

func ensureGitignore(projectDir string) error {
	gitignore := filepath.Join(projectDir, ".gitignore")
	data := ""
	if b, err := os.ReadFile(gitignore); err == nil {
		data = string(b)
	}
	want := ".slopshell/artifacts/\n"
	if strings.Contains(data, ".slopshell/artifacts/") {
		return nil
	}
	if data != "" && !strings.HasSuffix(data, "\n") {
		data += "\n"
	}
	data += want
	return os.WriteFile(gitignore, []byte(data), 0o644)
}
