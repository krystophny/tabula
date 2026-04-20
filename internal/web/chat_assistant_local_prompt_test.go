package web

import (
	"strings"
	"testing"

	"github.com/sloppy-org/slopshell/internal/store"
)

func TestBuildLeanLocalAssistantPromptIsCompact(t *testing.T) {
	workspace := &store.Workspace{Name: "Slopshell", DirPath: "/tmp/slopshell"}
	messages := []store.ChatMessage{
		{Role: "user", ContentPlain: "first question"},
		{Role: "assistant", ContentPlain: "first answer"},
		{Role: "user", ContentPlain: "latest question"},
	}
	prompt := buildLeanLocalAssistantPrompt(
		workspace,
		messages,
		&canvasContext{HasArtifact: true, ArtifactTitle: "notes.md", ArtifactKind: "markdown", ArtifactText: "line one\nline two"},
		&companionPromptContext{SummaryText: "Planning next steps."},
		turnOutputModeVoice,
	)
	for _, snippet := range []string{
		"Workspace: Slopshell (/tmp/slopshell)",
		"Canvas: notes.md [markdown]",
		"Canvas content:\nline one\nline two",
		"## Companion Context",
		"- Summary: Planning next steps.",
		"Reply clearly for speech. For substantive questions, give a satisfying spoken answer in 3-6 sentences; for simple questions, answer briefly. Do not use markdown unless the user explicitly asks for it.",
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
	workspace := &store.Workspace{Name: "Slopshell", DirPath: "/tmp/slopshell"}
	prompt := buildLeanLocalAssistantPrompt(
		workspace,
		[]store.ChatMessage{{Role: "user", ContentPlain: "explain fusion"}},
		nil,
		nil,
		turnOutputModeSilent,
	)
	if !strings.Contains(prompt, "Default to plain text. For substantive questions, answer with a compact but complete explanation, usually one short paragraph or 3-6 sentences. For simple questions, answer briefly. Use lists or markdown only when the user explicitly asks for them.") {
		t.Fatalf("prompt missing plain short chat guidance:\n%s", prompt)
	}
}

func TestBuildLeanLocalAssistantPrompt_VoiceKeepsPlainShortSpeech(t *testing.T) {
	prompt := buildLeanLocalAssistantPrompt(
		nil,
		[]store.ChatMessage{{Role: "user", ContentPlain: "hello"}},
		nil,
		nil,
		turnOutputModeVoice,
	)
	if !strings.Contains(prompt, "Reply clearly for speech. For substantive questions, give a satisfying spoken answer in 3-6 sentences; for simple questions, answer briefly. Do not use markdown unless the user explicitly asks for it.") {
		t.Fatalf("prompt missing short speech guidance:\n%s", prompt)
	}
}

func TestBuildLocalAssistantFastPromptAddsShortPlainGuidance(t *testing.T) {
	prompt := buildLocalAssistantFastPrompt("Reply with the single word ORBIT.")
	for _, snippet := range []string{
		"You are Slopshell, the assistant in this workspace.",
		"Answer in plain text only. Be concise, but do not under-answer: default to 2-4 short sentences for normal questions.",
		"If a single word or short phrase answers the request, reply with exactly that.",
		"Do not use markdown, headings, bullets, or numbered lists unless the user explicitly asks for them.",
		"User request:\nReply with the single word ORBIT.",
	} {
		if !strings.Contains(prompt, snippet) {
			t.Fatalf("fast prompt missing %q:\n%s", snippet, prompt)
		}
	}
}

func TestBuildLocalAssistantFastPromptStripsLeadingAssistantName(t *testing.T) {
	prompt := buildLocalAssistantFastPrompt("Slopshell, what's up")
	if strings.Contains(strings.ToLower(prompt), "user request:\nslopshell, what's up") {
		t.Fatalf("fast prompt should strip leading assistant name:\n%s", prompt)
	}
	if !strings.Contains(prompt, "User request:\nwhat's up") {
		t.Fatalf("fast prompt missing normalized request:\n%s", prompt)
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

// TestStripLocalAssistantThinkingPreamblePreservesWhitespaceDelta guards
// against a regression where whitespace-only streaming chunks (commonly a
// lone "\n" emitted between bullets or paragraphs) were silently dropped,
// collapsing the assistant's formatting downstream.
func TestStripLocalAssistantThinkingPreamblePreservesWhitespaceDelta(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
	}{
		{"newline only", "\n"},
		{"double newline", "\n\n"},
		{"crlf", "\r\n"},
		{"spaces", "   "},
	} {
		if got := stripLocalAssistantThinkingPreamble(tc.in); got != tc.in {
			t.Fatalf("%s: stripLocalAssistantThinkingPreamble(%q) = %q, want %q", tc.name, tc.in, got, tc.in)
		}
	}
}

func TestAnnotateLocalAssistantSafetyStop(t *testing.T) {
	if got := annotateLocalAssistantSafetyStop("Hello world from Slopshell"); got != "Hello world from Slopshell\n\n[stopped at local safety limit]" {
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

func TestBuildLocalAssistantCanvasGenerationPromptKeepsStructuredFollowUp(t *testing.T) {
	prompt := buildLocalAssistantCanvasGenerationPrompt(
		"Mach es schöner und füge eine Turbine hinzu.",
		"[Fusion Reactor]\n  |\n[Plasma]\n  |\n[Turbine]",
		"",
	)
	for _, snippet := range []string{
		"The current canvas already contains a structured diagram. Keep the revised result as a structured multi-line diagram.",
		"When revising, improve clarity and detail rather than shrinking the diagram.",
	} {
		if !strings.Contains(prompt, snippet) {
			t.Fatalf("canvas prompt missing %q:\n%s", snippet, prompt)
		}
	}
}

func TestBuildLocalAssistantCanvasGenerationPromptRequestsRicherDiagram(t *testing.T) {
	prompt := buildLocalAssistantCanvasGenerationPrompt(
		"zeichne mir einen tokamak auf der canvas als flowchart ascii-diagramm",
		"",
		"",
	)
	for _, snippet := range []string{
		"This request needs a readable, information-rich ASCII diagram.",
		"Prefer 8-14 non-empty lines unless a denser boxed flowchart is clearly better.",
		"Put each stage or component on its own line.",
		"Do not collapse the diagram into a tiny glossary or two-column word list.",
	} {
		if !strings.Contains(prompt, snippet) {
			t.Fatalf("canvas prompt missing %q:\n%s", snippet, prompt)
		}
	}
}

func TestNormalizeLocalAssistantCanvasDiagramTextSplitsCollapsedStages(t *testing.T) {
	raw := "Fusionsreaktor[Plasma Einschluss] | v\n[Heizsystem] | v[Turbine]"
	got := normalizeLocalAssistantCanvasDiagramText(raw)
	for _, snippet := range []string{
		"Fusionsreaktor\n[Plasma Einschluss]",
		"[Plasma Einschluss]\n |\n v\n[Heizsystem]",
		"[Heizsystem]\n |\n v\n[Turbine]",
	} {
		if !strings.Contains(got, snippet) {
			t.Fatalf("normalized diagram missing %q:\n%s", snippet, got)
		}
	}
}

func TestRecoverLocalAssistantCanvasTextFromMalformedToolOutputRejectsNonCanvasTools(t *testing.T) {
	raw := `{"tool_calls":[{"name":"workspace_read","arguments":{"content":"not canvas"}}]}`
	if got, ok := recoverLocalAssistantCanvasTextFromMalformedToolOutput(raw); ok || got != "" {
		t.Fatalf("recoverLocalAssistantCanvasTextFromMalformedToolOutput() = %q, %v; want empty false", got, ok)
	}
}

func TestSynthesizeLocalAssistantCanvasDiagramForFusionReactor(t *testing.T) {
	got := synthesizeLocalAssistantCanvasDiagram(
		"Please draw a flowchart on the canvas showing how a fusion reactor works.",
		"",
	)
	for _, snippet := range []string{"fusion reactor", "plasma", "fusion", "turbine"} {
		if !strings.Contains(strings.ToLower(got), snippet) {
			t.Fatalf("fallback diagram missing %q:\n%s", snippet, got)
		}
	}
	if !localAssistantCanvasHasStructuredDiagramText(got) {
		t.Fatalf("fallback diagram is not structured:\n%s", got)
	}
}

func TestSynthesizeLocalAssistantCanvasDiagramRefinesGermanPrompt(t *testing.T) {
	got := synthesizeLocalAssistantCanvasDiagram(
		"Mach es schöner, behalte das Flussdiagramm auf der Canvas und füge Magnetspulen und Turbine hinzu.",
		"Current canvas content:\nFusionsreaktor\n[Plasma]\n |\n v\n[Waerme]",
	)
	for _, snippet := range []string{"Fusionsreaktor", "Plasma", "Magnetspulen", "Turbine"} {
		if !strings.Contains(got, snippet) {
			t.Fatalf("fallback diagram missing %q:\n%s", snippet, got)
		}
	}
	if !localAssistantCanvasHasStructuredDiagramText(got) {
		t.Fatalf("fallback diagram is not structured:\n%s", got)
	}
}

func TestLocalAssistantCanvasShouldBypassLLMForNoisyFusionTranscript(t *testing.T) {
	if !localAssistantCanvasShouldBypassLLM(
		"computer bitte zeichne fusie aktor flowchart auf der canvas",
		"",
	) {
		t.Fatal("expected noisy fusion-reactor transcript to bypass the LLM")
	}
}

func TestLocalAssistantCanvasDoesNotBypassLLMForCleanFusionPrompt(t *testing.T) {
	if localAssistantCanvasShouldBypassLLM(
		"Bitte zeichne mir wie ein Fusionsreaktor funktioniert als Flowchart auf der Canvas.",
		"",
	) {
		t.Fatal("expected clean fusion-reactor prompt to keep the LLM repair flow")
	}
}
