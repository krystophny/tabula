package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type documentCanvasMCPMock struct {
	mu            sync.Mutex
	artifactTitle string
	artifactKind  string
	artifactPath  string
	showCalls     int32
	lastShown     map[string]interface{}
}

func (m *documentCanvasMCPMock) setupServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || strings.TrimSpace(r.URL.Path) != "/mcp" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		params, _ := payload["params"].(map[string]interface{})
		name := strings.TrimSpace(fmt.Sprint(params["name"]))
		args, _ := params["arguments"].(map[string]interface{})

		var structured map[string]interface{}
		switch name {
		case "canvas_status":
			m.mu.Lock()
			active := map[string]interface{}{
				"title": m.artifactTitle,
				"kind":  m.artifactKind,
				"path":  m.artifactPath,
			}
			m.mu.Unlock()
			structured = map[string]interface{}{"active_artifact": active}
		case "canvas_artifact_show":
			atomic.AddInt32(&m.showCalls, 1)
			copied := map[string]interface{}{}
			for key, value := range args {
				copied[key] = value
			}
			m.mu.Lock()
			m.artifactTitle = strings.TrimSpace(fmt.Sprint(args["title"]))
			m.artifactKind = strings.TrimSpace(fmt.Sprint(args["kind"]))
			m.artifactPath = strings.TrimSpace(fmt.Sprint(args["path"]))
			m.lastShown = copied
			m.mu.Unlock()
			structured = map[string]interface{}{"ok": true}
		default:
			http.Error(w, "unknown tool", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      payload["id"],
			"result": map[string]interface{}{
				"structuredContent": structured,
				"isError":           false,
			},
		})
	}))
}

func writePandocEchoStub(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	stub := `#!/bin/sh
out=""
prev=""
for arg in "$@"; do
  if [ "$prev" = "-o" ]; then
    out="$arg"
  fi
  prev="$arg"
done
cat "$1" > "$out"
`
	if err := os.WriteFile(filepath.Join(binDir, "pandoc"), []byte(stub), 0o755); err != nil {
		t.Fatalf("write pandoc stub: %v", err)
	}
	return binDir
}

func TestRefreshCanvasFromDisk_RebuildsRenderedDocumentPDF(t *testing.T) {
	app := newAuthedTestApp(t)
	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensure default project: %v", err)
	}
	docPath := filepath.Join(project.RootPath, "docs", "brief.md")
	if err := os.MkdirAll(filepath.Dir(docPath), 0o755); err != nil {
		t.Fatalf("mkdir docs dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(project.RootPath, ".tabura"), 0o755); err != nil {
		t.Fatalf("mkdir .tabura: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project.RootPath, ".tabura", "document.json"), []byte(`{"builder":"pandoc","main_file":"docs/brief.md"}`), 0o644); err != nil {
		t.Fatalf("write document config: %v", err)
	}
	if err := os.WriteFile(docPath, []byte("version one\n"), 0o644); err != nil {
		t.Fatalf("write initial doc: %v", err)
	}
	binDir := writePandocEchoStub(t)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	renderedPath, err := app.renderDocumentArtifact(project.RootPath, docPath)
	if err != nil {
		t.Fatalf("renderDocumentArtifact() error = %v", err)
	}
	renderedAbs := filepath.Join(project.RootPath, filepath.FromSlash(renderedPath))

	mock := &documentCanvasMCPMock{
		artifactTitle: "docs/brief.md",
		artifactKind:  "pdf",
		artifactPath:  renderedPath,
	}
	server := mock.setupServer(t)
	defer server.Close()
	port, err := extractPort(server.URL)
	if err != nil {
		t.Fatalf("extract port: %v", err)
	}
	app.tunnels.setPort(app.canvasSessionIDForWorkspace(project), port)

	time.Sleep(20 * time.Millisecond)
	if err := os.WriteFile(docPath, []byte("version two\n"), 0o644); err != nil {
		t.Fatalf("write updated doc: %v", err)
	}

	if !app.refreshCanvasFromDisk(project.WorkspacePath) {
		t.Fatal("expected rendered document PDF refresh")
	}
	if got := atomic.LoadInt32(&mock.showCalls); got != 1 {
		t.Fatalf("canvas_artifact_show calls = %d, want 1", got)
	}

	mock.mu.Lock()
	lastShown := mock.lastShown
	mock.mu.Unlock()
	if strings.TrimSpace(fmt.Sprint(lastShown["kind"])) != "pdf" {
		t.Fatalf("shown kind = %v, want pdf", lastShown["kind"])
	}
	if strings.TrimSpace(fmt.Sprint(lastShown["title"])) != "docs/brief.md" {
		t.Fatalf("shown title = %v, want docs/brief.md", lastShown["title"])
	}
	if strings.TrimSpace(fmt.Sprint(lastShown["path"])) != renderedPath {
		t.Fatalf("shown path = %v, want %s", lastShown["path"], renderedPath)
	}
	renderedBytes, err := os.ReadFile(renderedAbs)
	if err != nil {
		t.Fatalf("read rendered artifact: %v", err)
	}
	if string(renderedBytes) != "version two\n" {
		t.Fatalf("rendered artifact = %q", string(renderedBytes))
	}
}

func TestWatchCanvasFile_RebuildsRenderedDocumentPDFOnSourceWrite(t *testing.T) {
	app := newAuthedTestApp(t)
	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensure default project: %v", err)
	}
	docPath := filepath.Join(project.RootPath, "docs", "brief.md")
	if err := os.MkdirAll(filepath.Dir(docPath), 0o755); err != nil {
		t.Fatalf("mkdir docs dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(project.RootPath, ".tabura"), 0o755); err != nil {
		t.Fatalf("mkdir .tabura: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project.RootPath, ".tabura", "document.json"), []byte(`{"builder":"pandoc","main_file":"docs/brief.md"}`), 0o644); err != nil {
		t.Fatalf("write document config: %v", err)
	}
	if err := os.WriteFile(docPath, []byte("draft one\n"), 0o644); err != nil {
		t.Fatalf("write initial doc: %v", err)
	}
	binDir := writePandocEchoStub(t)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	renderedPath, err := app.renderDocumentArtifact(project.RootPath, docPath)
	if err != nil {
		t.Fatalf("renderDocumentArtifact() error = %v", err)
	}
	renderedAbs := filepath.Join(project.RootPath, filepath.FromSlash(renderedPath))

	mock := &documentCanvasMCPMock{
		artifactTitle: "docs/brief.md",
		artifactKind:  "pdf",
		artifactPath:  renderedPath,
	}
	server := mock.setupServer(t)
	defer server.Close()
	port, err := extractPort(server.URL)
	if err != nil {
		t.Fatalf("extract port: %v", err)
	}
	app.tunnels.setPort(app.canvasSessionIDForWorkspace(project), port)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go app.watchCanvasFile(ctx, project.WorkspacePath)

	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(docPath, []byte("draft two\n"), 0o644); err != nil {
		t.Fatalf("write updated doc: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&mock.showCalls) == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if got := atomic.LoadInt32(&mock.showCalls); got == 0 {
		t.Fatal("expected canvas_artifact_show after source write")
	}
	renderedBytes, err := os.ReadFile(renderedAbs)
	if err != nil {
		t.Fatalf("read rendered artifact: %v", err)
	}
	if string(renderedBytes) != "draft two\n" {
		t.Fatalf("rendered artifact = %q", string(renderedBytes))
	}
}
