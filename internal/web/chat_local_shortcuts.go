package web

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/krystophny/tabura/internal/store"
)

type localDirectCanvasTextAction struct {
	Title string
	Body  string
	Reply string
}

func normalizeLocalAssistantAddress(text string) string {
	clean := strings.TrimSpace(text)
	if clean == "" {
		return ""
	}
	lower := strings.ToLower(clean)
	for _, name := range []string{"tabura", "sloppy", "computer"} {
		for _, prefix := range []string{name + " ", name + ",", name + ":", name + ";"} {
			if strings.HasPrefix(lower, prefix) {
				return strings.TrimSpace(clean[len(prefix):])
			}
		}
		if lower == name {
			return ""
		}
	}
	return clean
}

func parseLocalDirectCanvasTextAction(text string) (localDirectCanvasTextAction, bool) {
	clean := strings.TrimSpace(text)
	lower := strings.ToLower(clean)
	if clean == "" || !strings.Contains(lower, "canvas") {
		return localDirectCanvasTextAction{}, false
	}
	titleMarker := strings.Index(lower, " titled ")
	if titleMarker < 0 {
		return localDirectCanvasTextAction{}, false
	}
	bodyMarker := -1
	bodyPrefix := ""
	for _, marker := range []string{" with the exact body ", " with body ", " body "} {
		if idx := strings.Index(lower, marker); idx >= 0 {
			bodyMarker = idx
			bodyPrefix = marker
			break
		}
	}
	if bodyMarker <= titleMarker {
		return localDirectCanvasTextAction{}, false
	}
	title := normalizeLocalShortcutText(clean[titleMarker+len(" titled ") : bodyMarker])
	bodyTail := clean[bodyMarker+len(bodyPrefix):]
	if dot := strings.Index(bodyTail, "."); dot >= 0 {
		bodyTail = bodyTail[:dot]
	}
	body := normalizeLocalShortcutText(bodyTail)
	if title == "" || body == "" {
		return localDirectCanvasTextAction{}, false
	}
	return localDirectCanvasTextAction{
		Title: title,
		Body:  body,
		Reply: parseLocalDirectReplyWord(clean),
	}, true
}

func parseLocalDirectReplyWord(text string) string {
	lower := strings.ToLower(text)
	marker := "reply with the single word "
	idx := strings.Index(lower, marker)
	if idx < 0 {
		return ""
	}
	rest := normalizeLocalShortcutText(text[idx+len(marker):])
	if rest == "" {
		return ""
	}
	for _, sep := range []string{".", "\n", " "} {
		if cut := strings.Index(rest, sep); cut >= 0 {
			rest = rest[:cut]
			break
		}
	}
	return normalizeLocalShortcutText(rest)
}

func normalizeLocalShortcutText(text string) string {
	return strings.Trim(strings.TrimSpace(text), `"'`)
}

func wantsDirectLocalDirectoryList(text string) bool {
	lower := strings.ToLower(normalizeLocalAssistantAddress(text))
	if lower == "" {
		return false
	}
	phrases := []string{
		"what files are in this directory",
		"what files are in the directory",
		"what files are in this folder",
		"what's in this directory",
		"whats in this directory",
		"list files in this directory",
		"list the files in this directory",
		"show files in this directory",
	}
	for _, phrase := range phrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

func buildLocalDirectoryListReply(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		names = append(names, name)
	}
	slices.Sort(names)
	if len(names) == 0 {
		return "This directory is empty.", nil
	}
	const limit = 40
	if len(names) > limit {
		return fmt.Sprintf(
			"Top-level entries in this directory: %s. ... (%d more)",
			strings.Join(names[:limit], ", "),
			len(names)-limit,
		), nil
	}
	return "Top-level entries in this directory: " + strings.Join(names, ", "), nil
}

func (a *App) tryRunDirectLocalDirectoryListTurn(sessionID string, session store.ChatSession, userText string) (string, bool) {
	if a == nil || !wantsDirectLocalDirectoryList(userText) {
		return "", false
	}
	workspace, err := a.effectiveWorkspaceForChatSession(session)
	if err != nil {
		return "", false
	}
	dir := strings.TrimSpace(workspace.DirPath)
	if dir == "" {
		return "", false
	}
	reply, err := buildLocalDirectoryListReply(dir)
	if err != nil {
		return "", false
	}
	return reply, true
}

func (a *App) tryRunDirectLocalCanvasTextTurn(sessionID string, session store.ChatSession, userText string) (string, []map[string]interface{}, bool) {
	if a == nil {
		return "", nil, false
	}
	action, ok := parseLocalDirectCanvasTextAction(userText)
	if !ok {
		return "", nil, false
	}
	workspace, err := a.effectiveWorkspaceForChatSession(session)
	if err != nil {
		return "", nil, false
	}
	mcpURL := strings.TrimSpace(workspace.MCPURL)
	if mcpURL == "" {
		mcpURL = strings.TrimSpace(a.localMCPURL)
	}
	if mcpURL == "" {
		return "", nil, false
	}
	arguments := map[string]interface{}{
		"session_id":       a.canvasSessionIDForWorkspace(workspace),
		"kind":             "text",
		"title":            action.Title,
		"markdown_or_text": action.Body,
	}
	result, err := mcpToolsCallURL(mcpURL, "canvas_artifact_show", arguments)
	if err != nil {
		return "", nil, false
	}
	reply := strings.TrimSpace(action.Reply)
	if reply == "" {
		reply = "Done."
	}
	return reply, []map[string]interface{}{{
		"type":         "mcp_tool",
		"name":         "canvas_artifact_show",
		"arguments":    arguments,
		"result":       result,
		"is_error":     false,
		"workspace_id": workspace.ID,
	}}, true
}
