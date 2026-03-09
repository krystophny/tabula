package web

import (
	"strings"
	"testing"
)

func TestFormatChatCursorPromptContext_IncludesStructuredLocationAndContext(t *testing.T) {
	ctx := &chatCursorContext{
		Title:        "internal/web/items.go",
		Page:         2,
		Line:         42,
		SelectedText: "if err != nil",
		Surrounding:  "41: func main() {\n42: if err != nil {\n43:   return err\n44: }",
	}
	prompt := formatChatCursorPromptContext(ctx)
	if !strings.Contains(prompt, `User is pointing at: page 2, line 42 of "internal/web/items.go"`) {
		t.Fatalf("prompt missing target, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, `Selected text: "if err != nil"`) {
		t.Fatalf("prompt missing selected text, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "42: if err != nil {") {
		t.Fatalf("prompt missing surrounding text, got:\n%s", prompt)
	}
}

func TestAppendChatCursorPrompt_PrependsContext(t *testing.T) {
	prompt := appendChatCursorPrompt("Conversation transcript:\nUSER:\nfix this", &chatCursorContext{
		Title: "test.txt",
		Line:  3,
	})
	if !strings.HasPrefix(prompt, "## Cursor Context") {
		t.Fatalf("prompt should start with cursor context, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, `line 3 of "test.txt"`) {
		t.Fatalf("prompt missing line reference, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Conversation transcript:\nUSER:\nfix this") {
		t.Fatalf("prompt missing original body, got:\n%s", prompt)
	}
}

func TestChatCursorContextTracker_ConsumesInOrder(t *testing.T) {
	tracker := newChatCursorContextTracker()
	tracker.enqueue("session-1", &chatCursorContext{Title: "first.txt", Line: 1})
	tracker.enqueue("session-1", nil)
	tracker.enqueue("session-1", &chatCursorContext{Title: "third.txt", Line: 3})

	first := tracker.consume("session-1")
	if first == nil || first.Title != "first.txt" || first.Line != 1 {
		t.Fatalf("first consume = %#v", first)
	}
	second := tracker.consume("session-1")
	if second != nil {
		t.Fatalf("second consume = %#v, want nil placeholder", second)
	}
	third := tracker.consume("session-1")
	if third == nil || third.Title != "third.txt" || third.Line != 3 {
		t.Fatalf("third consume = %#v", third)
	}
	if leftover := tracker.consume("session-1"); leftover != nil {
		t.Fatalf("leftover consume = %#v, want nil", leftover)
	}
}
