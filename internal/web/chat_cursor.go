package web

import (
	"fmt"
	"strings"
	"sync"
)

type chatCursorContext struct {
	Title        string  `json:"title,omitempty"`
	Page         int     `json:"page,omitempty"`
	Line         int     `json:"line,omitempty"`
	RelativeX    float64 `json:"relative_x,omitempty"`
	RelativeY    float64 `json:"relative_y,omitempty"`
	SelectedText string  `json:"selected_text,omitempty"`
	Surrounding  string  `json:"surrounding_text,omitempty"`
}

type chatCursorContextTracker struct {
	mu     sync.Mutex
	queues map[string][]*chatCursorContext
}

func newChatCursorContextTracker() *chatCursorContextTracker {
	return &chatCursorContextTracker{
		queues: map[string][]*chatCursorContext{},
	}
}

func normalizeChatCursorContext(raw *chatCursorContext) *chatCursorContext {
	if raw == nil {
		return nil
	}
	ctx := &chatCursorContext{
		Title:        strings.TrimSpace(raw.Title),
		Page:         raw.Page,
		Line:         raw.Line,
		RelativeX:    raw.RelativeX,
		RelativeY:    raw.RelativeY,
		SelectedText: strings.TrimSpace(raw.SelectedText),
		Surrounding:  strings.TrimSpace(raw.Surrounding),
	}
	if ctx.Page < 0 {
		ctx.Page = 0
	}
	if ctx.Line < 0 {
		ctx.Line = 0
	}
	if ctx.RelativeX < 0 || ctx.RelativeX > 1 {
		ctx.RelativeX = 0
	}
	if ctx.RelativeY < 0 || ctx.RelativeY > 1 {
		ctx.RelativeY = 0
	}
	if ctx.Title == "" && ctx.Page == 0 && ctx.Line == 0 && ctx.RelativeX == 0 && ctx.RelativeY == 0 && ctx.SelectedText == "" && ctx.Surrounding == "" {
		return nil
	}
	return ctx
}

func (t *chatCursorContextTracker) enqueue(sessionID string, raw *chatCursorContext) {
	if t == nil {
		return
	}
	cleanSessionID := strings.TrimSpace(sessionID)
	if cleanSessionID == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.queues[cleanSessionID] = append(t.queues[cleanSessionID], normalizeChatCursorContext(raw))
}

func (t *chatCursorContextTracker) consume(sessionID string) *chatCursorContext {
	if t == nil {
		return nil
	}
	cleanSessionID := strings.TrimSpace(sessionID)
	if cleanSessionID == "" {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	queue := t.queues[cleanSessionID]
	if len(queue) == 0 {
		return nil
	}
	next := queue[0]
	if len(queue) == 1 {
		delete(t.queues, cleanSessionID)
	} else {
		t.queues[cleanSessionID] = queue[1:]
	}
	return next
}

func appendChatCursorPrompt(prompt string, cursor *chatCursorContext) string {
	contextBlock := formatChatCursorPromptContext(cursor)
	prompt = strings.TrimSpace(prompt)
	if contextBlock == "" {
		return prompt
	}
	if prompt == "" {
		return contextBlock
	}
	return contextBlock + "\n\n" + prompt
}

func formatChatCursorPromptContext(cursor *chatCursorContext) string {
	cursor = normalizeChatCursorContext(cursor)
	if cursor == nil {
		return ""
	}

	targetParts := make([]string, 0, 4)
	if cursor.Page > 0 {
		targetParts = append(targetParts, fmt.Sprintf("page %d", cursor.Page))
	}
	if cursor.Line > 0 {
		targetParts = append(targetParts, fmt.Sprintf("line %d", cursor.Line))
	}
	if cursor.RelativeX > 0 || cursor.RelativeY > 0 {
		targetParts = append(targetParts, fmt.Sprintf("point %.0f%%, %.0f%%", cursor.RelativeX*100, cursor.RelativeY*100))
	}
	target := strings.Join(targetParts, ", ")
	if title := strings.TrimSpace(cursor.Title); title != "" {
		if target != "" {
			target += " of "
		}
		target += fmt.Sprintf("%q", title)
	}
	if target == "" {
		target = "active artifact"
	}

	lines := []string{
		"## Cursor Context",
		"User is pointing at: " + target,
	}
	if text := strings.TrimSpace(cursor.SelectedText); text != "" {
		lines = append(lines, "Selected text: "+quotePromptText(text, 220))
	}
	if text := strings.TrimSpace(cursor.Surrounding); text != "" {
		lines = append(lines, "Surrounding text:")
		lines = append(lines, limitPromptLines(text, 6, 420))
	}
	return strings.Join(lines, "\n")
}

func quotePromptText(raw string, maxChars int) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return `""`
	}
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if maxChars > 0 && len(runes) > maxChars {
		text = string(runes[:maxChars]) + "..."
	}
	return fmt.Sprintf("%q", text)
}

func limitPromptLines(raw string, maxLines, maxChars int) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	text = strings.TrimSpace(strings.Join(lines, "\n"))
	runes := []rune(text)
	if maxChars > 0 && len(runes) > maxChars {
		text = string(runes[:maxChars]) + "..."
	}
	return text
}
