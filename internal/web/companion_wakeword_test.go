package web

import "testing"

func TestContainsWakeWordPhraseMatchesWholeWordsInsideTranscript(t *testing.T) {
	matched, ok := containsWakeWordPhrase("Alice said: computer, what did we decide?", []string{"Computer"})
	if !ok {
		t.Fatal("containsWakeWordPhrase() ok = false, want true")
	}
	if matched != "computer" {
		t.Fatalf("matched phrase = %q, want computer", matched)
	}

	if matched, ok := containsWakeWordPhrase("We discussed microcomputer procurement.", []string{"Computer"}); ok {
		t.Fatalf("containsWakeWordPhrase() matched %q inside another word", matched)
	}
}

func TestContainsWakeWordPhraseSupportsMultiWordAliases(t *testing.T) {
	matched, ok := containsWakeWordPhrase("Before we stop, hey helper, summarize that.", []string{"Hey Helper"})
	if !ok {
		t.Fatal("containsWakeWordPhrase() ok = false, want true")
	}
	if matched != "hey helper" {
		t.Fatalf("matched phrase = %q, want hey helper", matched)
	}
}

func TestDetectMeetingWakeWordFallsBackFromPrefixToTranscriptSearch(t *testing.T) {
	app := newAuthedTestApp(t)

	if got := app.detectMeetingWakeWord("Computer, open the transcript."); got != "computer" {
		t.Fatalf("prefix wake word = %q, want computer", got)
	}
	if got := app.detectMeetingWakeWord("Alice said computer, open the transcript."); got != "computer" {
		t.Fatalf("transcript wake word = %q, want computer", got)
	}
}
