package web

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type canvasAction struct {
	Title   string
	Kind    string
	Content string
}

var canvasShowRe = regexp.MustCompile(`(?s):::canvas_show\{([^}]*)\}\n?(.*?):::`)

func parseCanvasActions(text string) ([]canvasAction, string) {
	matches := canvasShowRe.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return nil, text
	}
	var actions []canvasAction
	cleaned := text
	for i := len(matches) - 1; i >= 0; i-- {
		m := matches[i]
		fullStart, fullEnd := m[0], m[1]
		attrsStart, attrsEnd := m[2], m[3]
		contentStart, contentEnd := m[4], m[5]

		attrs := text[attrsStart:attrsEnd]
		content := strings.TrimSpace(text[contentStart:contentEnd])

		title := extractAttr(attrs, "title")
		kind := extractAttr(attrs, "kind")
		if kind == "" {
			kind = "text"
		}

		actions = append([]canvasAction{{
			Title:   title,
			Kind:    kind,
			Content: content,
		}}, actions...)

		ref := fmt.Sprintf("[canvas: %s]", title)
		cleaned = cleaned[:fullStart] + ref + cleaned[fullEnd:]
	}
	return actions, strings.TrimSpace(cleaned)
}

var attrRe = regexp.MustCompile(`(\w+)="([^"]*)"`)

func extractAttr(attrs, name string) string {
	for _, m := range attrRe.FindAllStringSubmatch(attrs, -1) {
		if m[1] == name {
			return m[2]
		}
	}
	return ""
}

func (a *App) executeCanvasActions(canvasSessionID string, actions []canvasAction) {
	a.mu.Lock()
	port, ok := a.tunnelPorts[canvasSessionID]
	a.mu.Unlock()
	if !ok {
		return
	}
	for _, action := range actions {
		_, _ = a.mcpToolsCall(port, "canvas_artifact_show", map[string]interface{}{
			"session_id":       canvasSessionID,
			"kind":             "text",
			"title":            action.Title,
			"markdown_or_text": action.Content,
		})
	}
}

// resolveArtifactFilePath maps an artifact title to an absolute file path.
// Returns "" if title has no file-like indicator (dot or separator) or the
// resolved file does not exist on disk.
func resolveArtifactFilePath(cwd, title string) string {
	t := strings.TrimSpace(title)
	if t == "" {
		return ""
	}
	if !strings.Contains(t, ".") && !strings.Contains(t, "/") {
		return ""
	}
	var abs string
	if filepath.IsAbs(t) {
		abs = t
	} else {
		abs = filepath.Join(cwd, t)
	}
	abs = filepath.Clean(abs)
	info, err := os.Stat(abs)
	if err != nil || info.IsDir() {
		return ""
	}
	return abs
}

// canvasFileTarget holds the resolved file-to-canvas binding for refresh.
type canvasFileTarget struct {
	sessionID string
	port      int
	title     string
	filePath  string
}

// resolveCanvasFileTarget resolves the active canvas artifact to a disk file.
// Returns nil if the artifact is not a text file or the title doesn't map to
// an existing file on disk.
func (a *App) resolveCanvasFileTarget(projectKey string) *canvasFileTarget {
	key := strings.TrimSpace(projectKey)
	if key == "" {
		return nil
	}
	project, err := a.store.GetProjectByProjectKey(key)
	if err != nil {
		return nil
	}
	sid := a.canvasSessionIDForProject(project)
	a.mu.Lock()
	port, ok := a.tunnelPorts[sid]
	a.mu.Unlock()
	if !ok {
		return nil
	}
	status, err := a.mcpToolsCall(port, "canvas_status", map[string]interface{}{"session_id": sid})
	if err != nil {
		return nil
	}
	active, _ := status["active_artifact"].(map[string]interface{})
	if active == nil {
		return nil
	}
	kind := strings.TrimSpace(fmt.Sprint(active["kind"]))
	if kind != "text_artifact" && kind != "text" {
		return nil
	}
	title := strings.TrimSpace(fmt.Sprint(active["title"]))
	if title == "" || title == "<nil>" {
		return nil
	}
	cwd := a.cwdForProjectKey(key)
	filePath := resolveArtifactFilePath(cwd, title)
	if filePath == "" {
		return nil
	}
	return &canvasFileTarget{sessionID: sid, port: port, title: title, filePath: filePath}
}

// refreshCanvasFromDisk does a single check: reads the file, compares with
// the canvas text, and pushes if different. Returns true if an update was pushed.
func (a *App) refreshCanvasFromDisk(projectKey string) bool {
	t := a.resolveCanvasFileTarget(projectKey)
	if t == nil {
		return false
	}
	return a.pushCanvasFileIfChanged(t)
}

func (a *App) pushCanvasFileIfChanged(t *canvasFileTarget) bool {
	diskBytes, err := os.ReadFile(t.filePath)
	if err != nil {
		return false
	}
	diskContent := string(diskBytes)
	status, err := a.mcpToolsCall(t.port, "canvas_status", map[string]interface{}{"session_id": t.sessionID})
	if err != nil {
		return false
	}
	active, _ := status["active_artifact"].(map[string]interface{})
	if active == nil {
		return false
	}
	currentText, _ := active["text"].(string)
	if strings.TrimSpace(diskContent) == strings.TrimSpace(currentText) {
		return false
	}
	_, _ = a.mcpToolsCall(t.port, "canvas_artifact_show", map[string]interface{}{
		"session_id":       t.sessionID,
		"kind":             "text",
		"title":            t.title,
		"markdown_or_text": diskContent,
	})
	return true
}

// pollCanvasFileRefresh polls the disk file backing the active canvas artifact
// every 500ms until ctx is cancelled. Intended to run in a goroutine for the
// duration of an assistant turn.
func (a *App) pollCanvasFileRefresh(ctx context.Context, projectKey string) {
	t := a.resolveCanvasFileTarget(projectKey)
	if t == nil {
		return
	}
	lastContent := ""
	if b, err := os.ReadFile(t.filePath); err == nil {
		lastContent = string(b)
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b, err := os.ReadFile(t.filePath)
			if err != nil {
				continue
			}
			content := string(b)
			if content == lastContent {
				continue
			}
			lastContent = content
			_, _ = a.mcpToolsCall(t.port, "canvas_artifact_show", map[string]interface{}{
				"session_id":       t.sessionID,
				"kind":             "text",
				"title":            t.title,
				"markdown_or_text": content,
			})
		}
	}
}
