package main

import "testing"

func TestIsLocalOnlyOneShotPromptRecognizesSyncCommands(t *testing.T) {
	cases := []struct {
		prompt string
		want   bool
	}{
		{"sync now", true},
		{"please sync all sources", true},
		{"bitte sync everything", true},
		{"show next actions", false},
	}
	for _, tc := range cases {
		if got := isLocalOnlyOneShotPrompt(tc.prompt); got != tc.want {
			t.Fatalf("isLocalOnlyOneShotPrompt(%q) = %v, want %v", tc.prompt, got, tc.want)
		}
	}
}
