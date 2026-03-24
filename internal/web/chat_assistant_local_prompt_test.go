package web

import (
	"strings"
	"testing"

	"github.com/krystophny/tabura/internal/store"
)

func TestBuildLeanLocalAssistantPromptIsCompact(t *testing.T) {
	workspace := &store.Workspace{Name: "Tabura", DirPath: "/tmp/tabura"}
	messages := []store.ChatMessage{
		{Role: "user", ContentPlain: "first question"},
		{Role: "assistant", ContentPlain: "first answer"},
		{Role: "user", ContentPlain: "latest question"},
	}
	prompt := buildLeanLocalAssistantPrompt(
		workspace,
		messages,
		&canvasContext{HasArtifact: true, ArtifactTitle: "notes.md", ArtifactKind: "markdown"},
		&companionPromptContext{SummaryText: "Planning next steps."},
		turnOutputModeVoice,
	)
	for _, snippet := range []string{
		"Workspace: Tabura (/tmp/tabura)",
		"Canvas: notes.md [markdown]",
		"## Companion Context",
		"- Summary: Planning next steps.",
		"Reply briefly for speech.",
		"Recent messages:",
		"USER: latest question",
	} {
		if !strings.Contains(prompt, snippet) {
			t.Fatalf("prompt missing %q:\n%s", snippet, prompt)
		}
	}
	for _, forbidden := range []string{
		"## Response Format",
		"Conversation transcript:",
		"## Workspace Context",
		"Voice mode is chat-first",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("prompt unexpectedly contains %q:\n%s", forbidden, prompt)
		}
	}
}

func TestBuildLeanLocalAssistantPrompt_DefaultsToPlainShortChat(t *testing.T) {
	workspace := &store.Workspace{Name: "Tabura", DirPath: "/tmp/tabura"}
	prompt := buildLeanLocalAssistantPrompt(
		workspace,
		[]store.ChatMessage{{Role: "user", ContentPlain: "explain fusion"}},
		nil,
		nil,
		turnOutputModeSilent,
	)
	if !strings.Contains(prompt, "Default to one short paragraph in plain text unless the user explicitly asks for a list, code, or markdown.") {
		t.Fatalf("prompt missing plain short chat guidance:\n%s", prompt)
	}
}

func TestCollectLeanLocalAssistantHistoryKeepsRecentMessages(t *testing.T) {
	messages := []store.ChatMessage{
		{Role: "user", ContentPlain: strings.Repeat("a", 2600)},
		{Role: "assistant", ContentPlain: strings.Repeat("b", 2600)},
		{Role: "user", ContentPlain: "latest"},
	}
	selected := collectLeanLocalAssistantHistory(messages)
	if len(selected) != 2 {
		t.Fatalf("selected len = %d, want 2", len(selected))
	}
	if got := strings.TrimSpace(selected[0].ContentPlain); got != strings.Repeat("b", 2600) {
		t.Fatalf("selected[0] = %q", got)
	}
	if got := strings.TrimSpace(selected[1].ContentPlain); got != "latest" {
		t.Fatalf("selected[1] = %q, want latest", got)
	}
}

func TestStripLocalAssistantThinkingPreamble(t *testing.T) {
	raw := "</think>\n\nready"
	if got := stripLocalAssistantThinkingPreamble(raw); got != "ready" {
		t.Fatalf("stripLocalAssistantThinkingPreamble() = %q, want ready", got)
	}
}

func TestAnnotateLocalAssistantSafetyStop(t *testing.T) {
	if got := annotateLocalAssistantSafetyStop("Hello world from Tabura"); got != "Hello world from Tabura\n\n[stopped at local safety limit]" {
		t.Fatalf("annotateLocalAssistantSafetyStop() = %q", got)
	}
}

func TestLocalAssistantVisibleStreamDeltaPreservesSpaces(t *testing.T) {
	chunks := []localIntentLLMStreamDelta{
		{Reasoning: "Yes,"},
		{Reasoning: " everything"},
		{Reasoning: " is"},
		{Reasoning: " fine!"},
	}
	var got string
	for _, chunk := range chunks {
		got += localAssistantVisibleStreamDelta(chunk, false)
	}
	if got != "Yes, everything is fine!" {
		t.Fatalf("streamed text = %q", got)
	}
}
