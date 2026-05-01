package web

import (
	"fmt"
	"strings"
	"unicode"
)

const companionWakeWordDefault = "Computer"

func normalizeWakeWordText(raw string) string {
	var b strings.Builder
	lastSpace := true
	for _, r := range strings.ToLower(strings.TrimSpace(raw)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace {
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

func normalizeWakeWordAlias(raw string) string {
	return normalizeWakeWordText(raw)
}

func stripWakeWordIntentPrefix(raw string, phrases []string) (string, string, bool) {
	text := normalizeWakeWordText(raw)
	if text == "" {
		return "", "", false
	}
	for _, phrase := range phrases {
		alias := normalizeWakeWordAlias(phrase)
		if alias == "" {
			continue
		}
		candidates := []string{
			alias,
			"hey " + alias,
			"ok " + alias,
			"okay " + alias,
		}
		for _, candidate := range candidates {
			switch {
			case text == candidate:
				return "", alias, true
			case strings.HasPrefix(text, candidate+" "):
				return strings.TrimSpace(text[len(candidate):]), alias, true
			}
		}
	}
	return text, "", false
}

func containsWakeWordPhrase(raw string, phrases []string) (string, bool) {
	fields := strings.Fields(normalizeWakeWordText(raw))
	if len(fields) == 0 {
		return "", false
	}
	for _, phrase := range phrases {
		alias := normalizeWakeWordAlias(phrase)
		phraseFields := strings.Fields(alias)
		if len(phraseFields) == 0 || len(phraseFields) > len(fields) {
			continue
		}
		if containsFieldSequence(fields, phraseFields) {
			return alias, true
		}
	}
	return "", false
}

func containsFieldSequence(fields, sequence []string) bool {
	lastStart := len(fields) - len(sequence)
	for start := 0; start <= lastStart; start++ {
		matched := true
		for i, value := range sequence {
			if fields[start+i] != value {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func (a *App) configuredWakeWordPhrases() []string {
	phrases := []string{companionWakeWordDefault, "Sloppy", "Slopshell"}
	if a == nil {
		return phrases
	}
	status := a.hotwordStatusPayload()
	model, _ := status["model"].(map[string]interface{})
	if model != nil {
		if phrase := strings.TrimSpace(fmt.Sprint(model["phrase"])); phrase != "" && phrase != "<nil>" {
			phrases = append([]string{phrase}, phrases...)
		}
	}
	return firstNonEmptyStrings(phrases, len(phrases))
}

func (a *App) detectMeetingWakeWord(raw string) string {
	_, matched, ok := stripWakeWordIntentPrefix(raw, a.configuredWakeWordPhrases())
	if !ok {
		var found bool
		matched, found = containsWakeWordPhrase(raw, a.configuredWakeWordPhrases())
		if !found {
			return ""
		}
	}
	return matched
}
