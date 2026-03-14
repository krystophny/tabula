package web

import (
	"strings"
	"testing"

	"github.com/krystophny/tabura/internal/store"
)

func TestBuildPromptFromHistoryOmitsLegacyAsyncToolSection(t *testing.T) {
	messages := []store.ChatMessage{{Role: "user", ContentPlain: "hello"}}
	prompt := buildPromptFromHistory("chat", messages, nil)
	if strings.Contains(prompt, "cancel-delegates") {
		t.Error("prompt should not include removed cancel-delegates endpoint")
	}
}

func TestBuildPromptFromHistoryKeepsOriginalUserText(t *testing.T) {
	messages := []store.ChatMessage{{Role: "user", ContentPlain: "let codex review the code"}}
	prompt := buildPromptFromHistory("chat", messages, nil)
	if !strings.Contains(prompt, "let codex review the code") {
		t.Error("prompt should include original user text")
	}
}

func TestBuildTurnPromptKeepsOriginalUserText(t *testing.T) {
	messages := []store.ChatMessage{{Role: "user", ContentPlain: "ask gpt about this"}}
	prompt := buildTurnPrompt(messages, nil)
	if !strings.Contains(prompt, "ask gpt about this") {
		t.Error("turn prompt should include original user text")
	}
}

func TestBuildTurnPromptChatOnlyContract(t *testing.T) {
	messages := []store.ChatMessage{{Role: "user", ContentPlain: "explain this function"}}
	prompt := buildTurnPrompt(messages, nil)
	if !strings.Contains(prompt, "Voice mode is chat-first") {
		t.Error("turn prompt should define chat-first voice mode")
	}
	if !strings.Contains(prompt, "Do not emit :::file blocks unless the user explicitly asks to show/open/render content on canvas.") {
		t.Error("turn prompt should explicitly limit file blocks to explicit canvas requests")
	}
	if !strings.Contains(prompt, "show/open an existing file") {
		t.Error("turn prompt should define existing-file canvas behavior")
	}
	if !strings.Contains(prompt, "explain this function") {
		t.Error("original message should be present")
	}
}
