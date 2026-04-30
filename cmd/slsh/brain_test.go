package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveSphere(t *testing.T) {
	tests := []struct {
		sphere  string
		want    string
		wantErr bool
	}{
		{"work", "work", false},
		{"WORK", "work", false},
		{"private", "private", false},
		{"PRIVATE", "private", false},
		{"invalid", "", true},
		{"", "", true},
		{"work ", "work", false},
	}
	for _, tt := range tests {
		t.Run(tt.sphere, func(t *testing.T) {
			got, err := resolveSphere(tt.sphere)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveSphere(%q) error = %v, wantErr %v", tt.sphere, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("resolveSphere(%q) = %q, want %q", tt.sphere, got, tt.want)
			}
		})
	}
}

func TestBrainVaultAvailable(t *testing.T) {
	tmp := t.TempDir()
	if brainVaultAvailable(tmp) {
		t.Errorf("brainVaultAvailable(%q) = true, want false (no brain/ dir)", tmp)
	}
	brainDir := filepath.Join(tmp, "brain")
	if err := os.MkdirAll(brainDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if !brainVaultAvailable(tmp) {
		t.Errorf("brainVaultAvailable(%q) = false, want true", tmp)
	}
}

func TestBrainVaultRoot(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("SLOPSHELL_BRAIN_WORK_ROOT", tmp)
	if got := brainVaultRoot("work"); got != tmp {
		t.Errorf("brainVaultRoot(work) = %q, want %q", got, tmp)
	}
	t.Setenv("SLOPSHELL_BRAIN_PRIVATE_ROOT", tmp)
	if got := brainVaultRoot("private"); got != tmp {
		t.Errorf("brainVaultRoot(private) = %q, want %q", got, tmp)
	}
	t.Setenv("SLOPSHELL_BRAIN_WORK_ROOT", "")
	if got := brainVaultRoot("work"); got != "" {
		t.Errorf("brainVaultRoot(work) with unset env = %q, want empty", got)
	}
}

func TestFindBrainRoots(t *testing.T) {
	tmp := t.TempDir()
	brainDir := filepath.Join(tmp, "brain")
	if err := os.MkdirAll(brainDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SLOPSHELL_BRAIN_WORK_ROOT", tmp)
	roots := findBrainRoots()
	if len(roots) != 1 {
		t.Fatalf("findBrainRoots() = %d roots, want 1", len(roots))
	}
	if got, ok := roots["work"]; !ok || got != tmp {
		t.Errorf("findBrainRoots() work = %v, want %q", roots, tmp)
	}
}

func TestResolveSphereWithGuard(t *testing.T) {
	tmp := t.TempDir()
	brainDir := filepath.Join(tmp, "brain")
	if err := os.MkdirAll(brainDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SLOPSHELL_BRAIN_WORK_ROOT", tmp)
	s, root, err := resolveSphereWithGuard("work")
	if err != nil {
		t.Fatalf("resolveSphereWithGuard(work) error = %v", err)
	}
	if s != "work" {
		t.Errorf("sphere = %q, want %q", s, "work")
	}
	if root != tmp {
		t.Errorf("root = %q, want %q", root, tmp)
	}
	t.Setenv("SLOPSHELL_BRAIN_PRIVATE_ROOT", "")
	_, _, err = resolveSphereWithGuard("private")
	if err == nil {
		t.Error("resolveSphereWithGuard(private) with unset env: want error, got nil")
	}
}

func TestIsBrainSubcommand(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{[]string{"brain", "open", "work"}, true},
		{[]string{"brain", "search", "foo"}, true},
		{[]string{"brain", "links", "foo", "work"}, true},
		{[]string{"brain", "backlinks", "foo", "work"}, true},
		{[]string{"brain", "link", "follow", "foo", "bar", "work"}, true},
		{[]string{"--base-url", "http://x", "brain", "open", "work"}, true},
		{[]string{"brain"}, true},
		{[]string{"chat"}, false},
		{[]string{}, false},
		{[]string{"brain", "invalid"}, false},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			got := isBrainSubcommand(tt.args)
			if got != tt.want {
				t.Errorf("isBrainSubcommand(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestSplitBrainArgs(t *testing.T) {
	args := []string{"--base-url", "http://x", "brain", "open", "work"}
	brainArgs, rest := splitBrainArgs(args)
	if brainArgs == nil {
		t.Fatalf("splitBrainArgs(%v) returned nil", args)
	}
	if len(brainArgs) < 1 {
		t.Fatalf("splitBrainArgs(%v) brainArgs = %v, want at least [open]", args, brainArgs)
	}
	if brainArgs[0] != "open" {
		t.Errorf("brainArgs[0] = %q, want %q", brainArgs[0], "open")
	}
	if len(rest) < 1 || rest[0] != "work" {
		t.Errorf("rest = %v, want [work]", rest)
	}
}

func captureStdout(fn func() error) (string, error) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := fn()
	os.Stdout = oldStdout
	w.Close()
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String(), err
}

func TestBrainLinks(t *testing.T) {
	tmp := t.TempDir()
	brainDir := filepath.Join(tmp, "brain")
	if err := os.MkdirAll(brainDir, 0o755); err != nil {
		t.Fatal(err)
	}
	notePath := filepath.Join(brainDir, "test-note.md")
	noteContent := "# Test Note\n\nSee [[Some Topic]] and [[Another Topic]].\nAlso [[Some Topic]] again.\n"
	if err := os.WriteFile(notePath, []byte(noteContent), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SLOPSHELL_BRAIN_WORK_ROOT", tmp)
	output, err := captureStdout(func() error {
		return brainLinks("test-note.md", "work", "", "")
	})
	if err != nil {
		t.Fatalf("brainLinks() error = %v", err)
	}
	if !strings.Contains(output, "Some Topic") {
		t.Errorf("output missing link 'Some Topic': %s", output)
	}
	if !strings.Contains(output, "Another Topic") {
		t.Errorf("output missing link 'Another Topic': %s", output)
	}
}

func TestBrainBacklinks(t *testing.T) {
	tmp := t.TempDir()
	brainDir := filepath.Join(tmp, "brain")
	if err := os.MkdirAll(brainDir, 0o755); err != nil {
		t.Fatal(err)
	}
	note1 := filepath.Join(brainDir, "note-a.md")
	if err := os.WriteFile(note1, []byte("# Note A\n\nSee [[target]]"), 0o644); err != nil {
		t.Fatal(err)
	}
	note2 := filepath.Join(brainDir, "note-b.md")
	if err := os.WriteFile(note2, []byte("# Note B\n\nReference [[target]]"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SLOPSHELL_BRAIN_WORK_ROOT", tmp)
	output, err := captureStdout(func() error {
		return brainBacklinks("target", "work", "", "")
	})
	if err != nil {
		t.Fatalf("brainBacklinks() error = %v", err)
	}
	if !strings.Contains(output, "note-a") {
		t.Errorf("output missing backlink to note-a: %s", output)
	}
	if !strings.Contains(output, "note-b") {
		t.Errorf("output missing backlink to note-b: %s", output)
	}
}

func TestBrainSearch(t *testing.T) {
	tmp := t.TempDir()
	brainDir := filepath.Join(tmp, "brain")
	if err := os.MkdirAll(brainDir, 0o755); err != nil {
		t.Fatal(err)
	}
	note := filepath.Join(brainDir, "searchable.md")
	if err := os.WriteFile(note, []byte("# Searchable Topic\n\nThis note contains the word foo"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SLOPSHELL_BRAIN_WORK_ROOT", tmp)
	output, err := captureStdout(func() error {
		return brainSearch("foo", 0)
	})
	if err != nil {
		t.Fatalf("brainSearch() error = %v", err)
	}
	if !strings.Contains(output, "searchable.md") {
		t.Errorf("output missing searchable note: %s", output)
	}
}

func TestBrainSearchNoMatches(t *testing.T) {
	tmp := t.TempDir()
	brainDir := filepath.Join(tmp, "brain")
	if err := os.MkdirAll(brainDir, 0o755); err != nil {
		t.Fatal(err)
	}
	note := filepath.Join(brainDir, "other.md")
	if err := os.WriteFile(note, []byte("# Other Topic"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SLOPSHELL_BRAIN_WORK_ROOT", tmp)
	output, err := captureStdout(func() error {
		return brainSearch("zzznonexistent", 0)
	})
	if err != nil {
		t.Fatalf("brainSearch() error = %v", err)
	}
	if !strings.Contains(output, "(no matches)") {
		t.Errorf("output should say (no matches): %s", output)
	}
}

func TestLinkFollow(t *testing.T) {
	tmp := t.TempDir()
	brainDir := filepath.Join(tmp, "brain")
	if err := os.MkdirAll(brainDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(brainDir, "target.md")
	if err := os.WriteFile(target, []byte("# Target"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SLOPSHELL_BRAIN_WORK_ROOT", tmp)
	output, err := captureStdout(func() error {
		return linkFollow("", "target.md", "work", "", "")
	})
	if err != nil {
		t.Fatalf("linkFollow() error = %v", err)
	}
	if strings.TrimSpace(output) == "" {
		t.Errorf("linkFollow() produced empty output")
	}
}

func TestRunRgNotFound(t *testing.T) {
	_, err := runRg([]string{"--version"})
	if err != nil && !strings.Contains(err.Error(), "rg") {
		t.Errorf("runRg error should mention rg: %v", err)
	}
}

func TestBrainOpenMissingSphere(t *testing.T) {
	err := brainOpen("", "http://127.0.0.1:8420", "")
	if err == nil {
		t.Error("brainOpen with empty sphere: want error, got nil")
	}
}

func TestBrainSearchNoVaults(t *testing.T) {
	t.Setenv("SLOPSHELL_BRAIN_WORK_ROOT", "")
	t.Setenv("SLOPSHELL_BRAIN_PRIVATE_ROOT", "")
	err := brainSearch("test", 0)
	if err == nil {
		t.Error("brainSearch with no vaults: want error, got nil")
	}
}

func TestIsBrainSubcommandWithFlagValues(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{[]string{"-base-url", "http://x", "brain", "search", "foo"}, true},
		{[]string{"--base-url", "http://x", "--token-file", "/tmp/t", "brain", "open", "work"}, true},
		{[]string{"--base-url=http://x", "brain", "search", "foo"}, true},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			got := isBrainSubcommand(tt.args)
			if got != tt.want {
				t.Errorf("isBrainSubcommand(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestBrainSubcommandRequiresRg(t *testing.T) {
	_, err := exec.LookPath("rg")
	if err != nil {
		t.Skipf("rg not available: %v", err)
	}
}
