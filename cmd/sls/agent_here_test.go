package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsTopLevelAgentHere(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "plain", args: []string{"agent-here", "notes/topic.md"}, want: true},
		{name: "with flags", args: []string{"--base-url", "http://x", "agent-here", "notes/topic.md"}, want: true},
		{name: "other command", args: []string{"brain", "agent-here"}, want: false},
		{name: "empty", args: nil, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTopLevelAgentHere(tt.args); got != tt.want {
				t.Fatalf("isTopLevelAgentHere(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestResolveAgentHereSpecDirectFolder(t *testing.T) {
	cwd := t.TempDir()
	target := filepath.Join(cwd, "project", "path")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}

	res, err := resolveAgentHereSpec("project/path", cwd)
	if err != nil {
		t.Fatalf("resolveAgentHereSpec() error = %v", err)
	}
	if res.TargetPath != target {
		t.Fatalf("target path = %q, want %q", res.TargetPath, target)
	}
	if res.StartPath != target {
		t.Fatalf("start path = %q, want %q", res.StartPath, target)
	}
	if res.SourceCursor != nil {
		t.Fatalf("source cursor = %#v, want nil", res.SourceCursor)
	}
}

func TestResolveAgentHereSpecDirectFile(t *testing.T) {
	cwd := t.TempDir()
	targetDir := filepath.Join(cwd, "project", "path")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}
	target := filepath.Join(targetDir, "file.md")
	if err := os.WriteFile(target, []byte("file"), 0o644); err != nil {
		t.Fatalf("write target file: %v", err)
	}

	res, err := resolveAgentHereSpec("project/path/file.md", cwd)
	if err != nil {
		t.Fatalf("resolveAgentHereSpec() error = %v", err)
	}
	if res.TargetPath != target {
		t.Fatalf("target path = %q, want %q", res.TargetPath, target)
	}
	if res.StartPath != targetDir {
		t.Fatalf("start path = %q, want %q", res.StartPath, targetDir)
	}
}

func TestResolveAgentHereSpecSourceLinkPreservesCursor(t *testing.T) {
	vaultRoot := t.TempDir()
	brainRoot := filepath.Join(vaultRoot, "brain")
	personalRoot := filepath.Join(vaultRoot, "personal")
	targetDir := filepath.Join(vaultRoot, "project", "path")
	sourceDir := filepath.Join(brainRoot, "topics")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("mkdir source dir: %v", err)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}
	if err := os.MkdirAll(personalRoot, 0o755); err != nil {
		t.Fatalf("mkdir personal root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "source.md"), []byte("source"), 0o644); err != nil {
		t.Fatalf("write source note: %v", err)
	}
	t.Setenv("SLOPSHELL_BRAIN_WORK_ROOT", vaultRoot)

	res, err := resolveAgentHereSpec("topics/source.md::../../project/path", brainRoot)
	if err != nil {
		t.Fatalf("resolveAgentHereSpec() error = %v", err)
	}
	if res.TargetPath != targetDir {
		t.Fatalf("target path = %q, want %q", res.TargetPath, targetDir)
	}
	if res.StartPath != targetDir {
		t.Fatalf("start path = %q, want %q", res.StartPath, targetDir)
	}
	if res.SourceCursor == nil {
		t.Fatal("source cursor = nil, want cursor")
	}
	if res.SourceCursor.Path != "topics/source.md" {
		t.Fatalf("source cursor path = %q, want topics/source.md", res.SourceCursor.Path)
	}
	if res.SourceCursor.IsDir {
		t.Fatal("source cursor IsDir = true, want false")
	}
}

func TestResolveAgentHereSpecExplicitSourceIgnoresCwdFallback(t *testing.T) {
	vaultRoot := t.TempDir()
	brainRoot := filepath.Join(vaultRoot, "brain")
	privateRoot := t.TempDir()
	sourceDir := filepath.Join(brainRoot, "topics")
	sourceNote := filepath.Join(sourceDir, "source.md")
	cwd := t.TempDir()
	cwdTarget := filepath.Join(cwd, "relative.md")
	privateTarget := filepath.Join(privateRoot, "brain", "relative.md")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("mkdir source dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(privateTarget), 0o755); err != nil {
		t.Fatalf("mkdir private target dir: %v", err)
	}
	if err := os.WriteFile(sourceNote, []byte("source"), 0o644); err != nil {
		t.Fatalf("write source note: %v", err)
	}
	if err := os.WriteFile(cwdTarget, []byte("cwd"), 0o644); err != nil {
		t.Fatalf("write cwd target: %v", err)
	}
	if err := os.WriteFile(privateTarget, []byte("private"), 0o644); err != nil {
		t.Fatalf("write private target: %v", err)
	}
	t.Setenv("SLOPSHELL_BRAIN_WORK_ROOT", vaultRoot)
	t.Setenv("SLOPSHELL_BRAIN_PRIVATE_ROOT", privateRoot)

	_, err := resolveAgentHereSpec("topics/source.md::relative.md", cwd)
	if err == nil {
		t.Fatal("resolveAgentHereSpec() error = nil, want source-sphere guard")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "sphere") {
		t.Fatalf("resolveAgentHereSpec() error = %v, want sphere guard", err)
	}
	if strings.Contains(err.Error(), cwdTarget) {
		t.Fatalf("resolveAgentHereSpec() leaked cwd fallback: %v", err)
	}
	if strings.Contains(err.Error(), privateTarget) {
		t.Fatalf("resolveAgentHereSpec() leaked private fallback: %v", err)
	}
}

func TestResolveAgentHereSpecCurrentNotePreservesCursor(t *testing.T) {
	vaultRoot := t.TempDir()
	brainRoot := filepath.Join(vaultRoot, "brain")
	currentNote := filepath.Join(brainRoot, "topics", "current.md")
	targetDir := filepath.Join(brainRoot, "projects", "path")
	if err := os.MkdirAll(filepath.Dir(currentNote), 0o755); err != nil {
		t.Fatalf("mkdir current note dir: %v", err)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}
	if err := os.WriteFile(currentNote, []byte("current"), 0o644); err != nil {
		t.Fatalf("write current note: %v", err)
	}
	t.Setenv("SLOPSHELL_BRAIN_WORK_ROOT", vaultRoot)

	res, err := resolveAgentHereSpec("../projects/path", currentNote)
	if err != nil {
		t.Fatalf("resolveAgentHereSpec() error = %v", err)
	}
	if res.TargetPath != targetDir {
		t.Fatalf("target path = %q, want %q", res.TargetPath, targetDir)
	}
	if res.StartPath != targetDir {
		t.Fatalf("start path = %q, want %q", res.StartPath, targetDir)
	}
	if res.SourceCursor == nil {
		t.Fatal("source cursor = nil, want cursor")
	}
	if res.SourceCursor.Path != currentNote {
		t.Fatalf("source cursor path = %q, want %q", res.SourceCursor.Path, currentNote)
	}
	if res.SourceCursor.IsDir {
		t.Fatal("source cursor IsDir = true, want false")
	}
}

func TestResolveAgentHereSpecKeepsWorkSphere(t *testing.T) {
	workRoot := t.TempDir()
	privateRoot := t.TempDir()
	workCurrentNote := filepath.Join(workRoot, "brain", "topics", "current.md")
	privateTarget := filepath.Join(privateRoot, "brain", "private-only.md")
	if err := os.MkdirAll(filepath.Dir(workCurrentNote), 0o755); err != nil {
		t.Fatalf("mkdir work note dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(privateTarget), 0o755); err != nil {
		t.Fatalf("mkdir private target dir: %v", err)
	}
	if err := os.WriteFile(workCurrentNote, []byte("current"), 0o644); err != nil {
		t.Fatalf("write work note: %v", err)
	}
	if err := os.WriteFile(privateTarget, []byte("private"), 0o644); err != nil {
		t.Fatalf("write private target: %v", err)
	}
	t.Setenv("SLOPSHELL_BRAIN_WORK_ROOT", workRoot)
	t.Setenv("SLOPSHELL_BRAIN_PRIVATE_ROOT", privateRoot)

	_, err := resolveAgentHereSpec("private-only.md", workCurrentNote)
	if err == nil {
		t.Fatal("resolveAgentHereSpec() error = nil, want originating sphere guard")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "sphere") {
		t.Fatalf("resolveAgentHereSpec() error = %v, want sphere guard", err)
	}
	if strings.Contains(err.Error(), privateRoot) {
		t.Fatalf("resolveAgentHereSpec() leaked private root: %v", err)
	}
}

func TestResolveAgentHereSpecMissingTarget(t *testing.T) {
	cwd := t.TempDir()
	if _, err := resolveAgentHereSpec("missing.md", cwd); err == nil {
		t.Fatal("resolveAgentHereSpec() error = nil, want missing target error")
	} else if !strings.Contains(strings.ToLower(err.Error()), "not found") {
		t.Fatalf("resolveAgentHereSpec() error = %v, want not found", err)
	}
}

func TestResolveAgentHereSpecBlocksWorkPersonal(t *testing.T) {
	vaultRoot := t.TempDir()
	brainRoot := filepath.Join(vaultRoot, "brain")
	personalRoot := filepath.Join(vaultRoot, "personal")
	if err := os.MkdirAll(brainRoot, 0o755); err != nil {
		t.Fatalf("mkdir brain root: %v", err)
	}
	if err := os.MkdirAll(personalRoot, 0o755); err != nil {
		t.Fatalf("mkdir personal root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(personalRoot, "secret.md"), []byte("secret"), 0o644); err != nil {
		t.Fatalf("write personal note: %v", err)
	}
	t.Setenv("SLOPSHELL_BRAIN_WORK_ROOT", vaultRoot)

	_, err := resolveAgentHereSpec("personal/secret.md", vaultRoot)
	if err == nil {
		t.Fatal("resolveAgentHereSpec() error = nil, want personal guard")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "personal") {
		t.Fatalf("resolveAgentHereSpec() error = %v, want personal guard", err)
	}
	if strings.Contains(err.Error(), vaultRoot) {
		t.Fatalf("resolveAgentHereSpec() leaked absolute path: %v", err)
	}
}
