package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/krystophny/tabura/internal/store"
)

const (
	localAssistantPromptMaxChars      = 120000
	localAssistantPromptKeepHeadChars = 16000
	localAssistantPromptKeepTailChars = 84000
	silentLiveEditDocMaxChars         = 96000
	silentLiveEditDocKeepHeadChars    = 12000
	silentLiveEditDocKeepFocusChars   = 24000
	silentLiveEditDocKeepTailChars    = 12000
)

type silentLiveEditResponse struct {
	Action   string `json:"action"`
	Document string `json:"document"`
	Reason   string `json:"reason"`
}

var silentLiveEditPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(?:edit|revise|rewrite|rephrase|clarify|tighten|improve|make\s+(?:(?:this|that)\s+(?:point|paragraph|section)|it|the\s+(?:point|paragraph|section))\s+more\s+(?:precise|clear))\b`),
	regexp.MustCompile(`(?i)\b(?:change|replace|swap|remove|delete|drop|add|insert|append|prepend)\b`),
	regexp.MustCompile(`(?i)\b(?:let(?:'s| us)|we should)\s+(?:change|replace|remove|delete|add|insert|rewrite|revise)\b`),
	regexp.MustCompile(`(?i)\b(?:at|in|on)\s+the\s+(?:top|bottom|end|start|beginning|first\s+(?:line|row|paragraph)|last\s+(?:line|row|paragraph)|next\s+(?:line|paragraph))\b`),
	regexp.MustCompile(`(?i)\b(?:aendern|umformulieren|praeziser|genauer|klarer|ersetzen|tauschen|loeschen|streichen|ergaenzen|hinzufuegen|einfuegen)\b`),
	regexp.MustCompile(`(?i)(?:fuegen\s+wir|tun\s+wir)\s+(?:hier\s+noch\s+)?(?:ein|dazu|hinzu)`),
	regexp.MustCompile(`(?i)\b(?:oben|unten|am\s+anfang|am\s+ende|in\s+der\s+ersten\s+zeile|in\s+der\s+ersten\s+reihe)\b`),
}

var silentLiveEditIntentNormalizer = strings.NewReplacer(
	"’", "'",
	"ä", "ae",
	"ö", "oe",
	"ü", "ue",
	"ß", "ss",
)

func looksLikeSilentLiveEditIntent(raw string) bool {
	text, _ := stripHotwordIntentPrefix(strings.TrimSpace(raw))
	text = strings.ToLower(strings.TrimSpace(text))
	text = silentLiveEditIntentNormalizer.Replace(text)
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if text == "" {
		return false
	}
	if strings.HasSuffix(text, "?") {
		return false
	}
	for _, pattern := range silentLiveEditPatterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

func compactPromptByChars(prompt string, maxChars int, keepHeadChars int, keepTailChars int) (string, bool) {
	clean := strings.TrimSpace(prompt)
	if clean == "" || maxChars <= 0 || len(clean) <= maxChars {
		return clean, false
	}
	headChars := keepHeadChars
	tailChars := keepTailChars
	if headChars < 0 {
		headChars = 0
	}
	if tailChars < 0 {
		tailChars = 0
	}
	if headChars+tailChars >= maxChars {
		if headChars > maxChars/2 {
			headChars = maxChars / 2
		}
		tailChars = maxChars - headChars
	}
	head := strings.TrimSpace(clean[:liveEditMinInt(len(clean), headChars)])
	tailStart := liveEditMaxInt(0, len(clean)-tailChars)
	tail := strings.TrimSpace(clean[tailStart:])
	omitted := len(clean) - len(head) - len(tail)
	if omitted < 0 {
		omitted = 0
	}
	var b strings.Builder
	if head != "" {
		b.WriteString(head)
	}
	if omitted > 0 {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "[[context_compact omitted_chars=%d]]", omitted)
	}
	if tail != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(tail)
	}
	return strings.TrimSpace(b.String()), true
}

func liveEditMinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func liveEditMaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func compactLocalAssistantPrompt(prompt string) (string, bool) {
	return compactPromptByChars(
		prompt,
		localAssistantPromptMaxChars,
		localAssistantPromptKeepHeadChars,
		localAssistantPromptKeepTailChars,
	)
}

func resolveEditableCanvasTextArtifact(a *App, workspacePath string) (string, string, string, error) {
	if a == nil {
		return "", "", "", errors.New("app unavailable")
	}
	target := a.resolveCanvasRefreshTarget(workspacePath)
	if target == nil || target.kind != canvasRefreshKindText {
		return "", "", "", errors.New("editable text artifact is not active")
	}
	content, err := os.ReadFile(target.sourcePath)
	if err != nil {
		return "", "", "", err
	}
	return target.sourcePath, strings.TrimSpace(target.title), string(content), nil
}

func compactSilentLiveEditDocument(text string, cursor *chatCursorContext) (string, bool) {
	clean := strings.ReplaceAll(text, "\r\n", "\n")
	if len(clean) <= silentLiveEditDocMaxChars {
		return clean, false
	}
	focus := ""
	if cursor != nil {
		if selected := strings.TrimSpace(cursor.SelectedText); selected != "" {
			if idx := strings.Index(clean, selected); idx >= 0 {
				start := liveEditMaxInt(0, idx-(silentLiveEditDocKeepFocusChars/2))
				end := liveEditMinInt(len(clean), idx+len(selected)+(silentLiveEditDocKeepFocusChars/2))
				focus = strings.TrimSpace(clean[start:end])
			}
		}
		if focus == "" && cursor.Line > 0 {
			lines := strings.Split(clean, "\n")
			if len(lines) > 0 {
				startLine := liveEditMaxInt(0, cursor.Line-1-80)
				endLine := liveEditMinInt(len(lines), cursor.Line+80)
				focus = strings.TrimSpace(strings.Join(lines[startLine:endLine], "\n"))
			}
		}
	}
	if focus == "" {
		centerStart := liveEditMaxInt(0, (len(clean)/2)-(silentLiveEditDocKeepFocusChars/2))
		centerEnd := liveEditMinInt(len(clean), centerStart+silentLiveEditDocKeepFocusChars)
		focus = strings.TrimSpace(clean[centerStart:centerEnd])
	}
	head := strings.TrimSpace(clean[:liveEditMinInt(len(clean), silentLiveEditDocKeepHeadChars)])
	tail := strings.TrimSpace(clean[liveEditMaxInt(0, len(clean)-silentLiveEditDocKeepTailChars):])
	omitted := len(clean) - len(head) - len(focus) - len(tail)
	if omitted < 0 {
		omitted = 0
	}
	var b strings.Builder
	b.WriteString(head)
	b.WriteString("\n\n[[document_compact head_preserved=true]]\n\n")
	if focus != "" {
		b.WriteString("## Focus Window\n")
		b.WriteString(focus)
		b.WriteString("\n\n")
	}
	if omitted > 0 {
		fmt.Fprintf(&b, "[[document_compact omitted_chars=%d]]\n\n", omitted)
	}
	if tail != "" {
		b.WriteString("## Tail Window\n")
		b.WriteString(tail)
	}
	return strings.TrimSpace(b.String()), true
}

func buildSilentLiveEditPrompt(path string, title string, document string, instruction string, cursor *chatCursorContext, positionCtx []*chatCanvasPositionEvent) (string, bool) {
	compactDocument, compacted := compactSilentLiveEditDocument(document, cursor)
	var lines []string
	lines = append(lines, "Apply the spoken live edit silently to the current file-backed document.")
	if path != "" || title != "" {
		lines = append(lines, fmt.Sprintf("Document: path=%q title=%q", path, title))
	}
	if cursorBlock := formatChatCursorPromptContext(cursor); cursorBlock != "" {
		lines = append(lines, cursorBlock)
	}
	if positionBlock := formatCanvasPositionPromptContext(positionCtx); positionBlock != "" {
		lines = append(lines, positionBlock)
	}
	lines = append(lines,
		"User edit instruction:",
		strings.TrimSpace(instruction),
		"",
		"Current document follows.",
		"Return JSON only:",
		`{"action":"replace_document|no_change","document":"full updated document when replacing","reason":"short reason"}`,
		"- Preserve formatting, structure, and language unless the instruction asks to change them.",
		"- Use cursor and canvas position context to resolve references like top, bottom, first row, this paragraph, or next bullet.",
		"- The instruction may be English or German.",
		"",
		"## Current Document",
		compactDocument,
	)
	return strings.Join(lines, "\n"), compacted
}

func buildSilentLiveEditSystemPrompt() string {
	return strings.TrimSpace(`
You are Tabura's local live-edit engine.
Apply a spoken edit instruction to the provided document and return JSON only.

Return exactly one object:
{"action":"replace_document","document":"<full updated document>","reason":"short_reason"}
or
{"action":"no_change","reason":"short_reason"}

Rules:
- Never answer conversationally.
- Never use code fences.
- When replacing, return the full updated document, not a patch.
- Be conservative. If the instruction is ambiguous enough that you would likely damage the document, return no_change.
- Handle English and German edit instructions.
`)
}

func (a *App) requestSilentLiveEdit(ctx context.Context, prompt string, visual *chatVisualAttachment) (silentLiveEditResponse, error) {
	baseURL := a.assistantLLMBaseURL()
	if baseURL == "" {
		return silentLiveEditResponse{}, errors.New("local assistant is not configured")
	}
	requestBody, _ := json.Marshal(map[string]any{
		"model":       a.localAssistantLLMModel(),
		"temperature": 0,
		"max_tokens":  assistantLLMToolMaxTokens,
		"response_format": map[string]any{
			"type": "json_object",
		},
		"chat_template_kwargs": map[string]any{
			"enable_thinking": false,
		},
		"messages": []map[string]any{
			{"role": "system", "content": buildSilentLiveEditSystemPrompt()},
			{"role": "user", "content": buildLocalAssistantUserContent(prompt, visual)},
		},
	})
	requestCtx, cancel := context.WithTimeout(ctx, assistantLLMRequestTimeout())
	defer cancel()
	req, err := http.NewRequestWithContext(
		requestCtx,
		http.MethodPost,
		baseURL+"/v1/chat/completions",
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return silentLiveEditResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return silentLiveEditResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, assistantLLMResponseLimit))
		return silentLiveEditResponse{}, fmt.Errorf("assistant llm HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload localIntentLLMChatCompletionResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, assistantLLMResponseLimit)).Decode(&payload); err != nil {
		return silentLiveEditResponse{}, err
	}
	if len(payload.Choices) == 0 {
		return silentLiveEditResponse{}, errors.New("assistant llm returned no choices")
	}
	content := strings.TrimSpace(stripCodeFence(payload.Choices[0].Message.Content))
	if content == "" {
		return silentLiveEditResponse{}, errors.New("assistant llm returned empty content")
	}
	var result silentLiveEditResponse
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return silentLiveEditResponse{}, err
	}
	result.Action = strings.ToLower(strings.TrimSpace(result.Action))
	result.Document = strings.ReplaceAll(result.Document, "\r\n", "\n")
	result.Reason = strings.TrimSpace(result.Reason)
	return result, nil
}

func (a *App) broadcastSilentEditStatus(sessionID string, phase string, message string) {
	if a == nil {
		return
	}
	a.broadcastChatEvent(sessionID, map[string]any{
		"type":    "silent_edit_status",
		"phase":   strings.TrimSpace(phase),
		"message": strings.TrimSpace(message),
	})
}

func (a *App) applySilentLiveDocumentEdit(sessionID string, session store.ChatSession, instruction string, cursor *chatCursorContext, positionCtx []*chatCanvasPositionEvent) (bool, error) {
	path, title, document, err := resolveEditableCanvasTextArtifact(a, session.WorkspacePath)
	if err != nil {
		return false, err
	}
	prompt, compacted := buildSilentLiveEditPrompt(path, title, document, instruction, cursor, positionCtx)
	visual := latestCanvasPositionVisualAttachment(positionCtx)
	if compacted {
		a.broadcastChatEvent(sessionID, map[string]any{
			"type": "context_compact",
		})
	}
	a.broadcastSilentEditStatus(sessionID, "started", "editing current document")
	result, err := a.requestSilentLiveEdit(context.Background(), prompt, visual)
	if err != nil {
		a.broadcastSilentEditStatus(sessionID, "failed", "silent edit failed")
		return false, err
	}
	switch result.Action {
	case "no_change":
		a.broadcastSilentEditStatus(sessionID, "skipped", firstNonEmptyCursorText(result.Reason, "no edit applied"))
		return true, nil
	case "replace_document":
		next := strings.TrimSpace(result.Document)
		if next == "" {
			a.broadcastSilentEditStatus(sessionID, "failed", "silent edit returned empty document")
			return false, errors.New("silent edit returned empty document")
		}
		if err := os.WriteFile(path, []byte(result.Document), 0644); err != nil {
			a.broadcastSilentEditStatus(sessionID, "failed", "failed to write edited document")
			return false, err
		}
		a.refreshCanvasFromDisk(session.WorkspacePath)
		a.markWorkspaceOutput(session.WorkspacePath)
		a.broadcastSilentEditStatus(sessionID, "applied", "document updated")
		return true, nil
	default:
		a.broadcastSilentEditStatus(sessionID, "failed", "silent edit returned invalid action")
		return false, fmt.Errorf("silent edit returned invalid action %q", result.Action)
	}
}

func shouldAttemptSilentLiveEdit(policy LivePolicy, captureMode string, raw string) bool {
	if normalizeChatCaptureMode(captureMode) != chatCaptureModeVoice {
		return false
	}
	if !looksLikeSilentLiveEditIntent(raw) {
		return false
	}
	if !policy.RequiresExplicitAddress() {
		return true
	}
	text, hotwordAddressed := stripHotwordIntentPrefix(strings.TrimSpace(raw))
	if hotwordAddressed || isCompanionDirectAddress(text) {
		return false
	}
	return true
}

func (a *App) maybeRunSilentLiveEditTurn(sessionID string, session store.ChatSession, userText string, cursor *chatCursorContext, positionCtx []*chatCanvasPositionEvent, captureMode string) bool {
	if a == nil || strings.TrimSpace(userText) == "" {
		return false
	}
	if !shouldAttemptSilentLiveEdit(a.LivePolicy(), captureMode, userText) {
		return false
	}
	if _, _, _, err := resolveEditableCanvasTextArtifact(a, session.WorkspacePath); err != nil {
		return false
	}
	if _, err := a.applySilentLiveDocumentEdit(sessionID, session, userText, cursor, positionCtx); err != nil {
		errText := normalizeAssistantError(err)
		_, _ = a.store.AddChatMessage(sessionID, "system", errText, errText, "text")
		a.broadcastChatEvent(sessionID, map[string]any{
			"type":  "error",
			"error": errText,
		})
	}
	a.finishCompanionPendingTurn(sessionID, "assistant_turn_suppressed")
	return true
}

func (a *App) triggerSilentLiveEditForParticipantSegment(participantSessionID string, participantSession store.ParticipantSession, seg store.ParticipantSegment) bool {
	if a == nil || a.store == nil || strings.TrimSpace(seg.Text) == "" {
		return false
	}
	if !shouldAttemptSilentLiveEdit(LivePolicyMeeting, chatCaptureModeVoice, seg.Text) {
		return false
	}
	if _, _, _, err := resolveEditableCanvasTextArtifact(a, participantSession.WorkspacePath); err != nil {
		return false
	}
	chatSession, err := a.store.GetOrCreateChatSessionForWorkspace(participantSession.WorkspaceID)
	if err != nil {
		return false
	}
	storedUser, err := a.store.AddChatMessage(chatSession.ID, "user", strings.TrimSpace(seg.Text), strings.TrimSpace(seg.Text), "text")
	if err != nil {
		return false
	}
	queuedTurns := a.enqueueAssistantTurn(chatSession.ID, turnOutputModeSilent, chatTurnOptions{
		messageID:   storedUser.ID,
		captureMode: chatCaptureModeVoice,
	})
	a.noteCompanionPendingTurn(chatSession.ID, participantSessionID, seg.ID)
	_ = a.store.AddParticipantEvent(
		participantSessionID,
		seg.ID,
		"assistant_triggered",
		fmt.Sprintf(`{"chat_session_id":%q,"chat_message_id":%d,"queued_turns":%d,"policy_decision":"silent_edit"}`, chatSession.ID, storedUser.ID, queuedTurns),
	)
	a.broadcastChatEvent(chatSession.ID, map[string]interface{}{
		"type":                   "message_accepted",
		"role":                   "user",
		"content":                strings.TrimSpace(seg.Text),
		"id":                     storedUser.ID,
		"source":                 "participant_transcript",
		"participant_session_id": participantSessionID,
		"participant_segment_id": seg.ID,
	})
	a.broadcastCompanionRuntimeState(participantSession.WorkspacePath, companionRuntimeSnapshot{
		State:                companionRuntimeStateThinking,
		Reason:               "silent_edit_triggered",
		WorkspacePath:        participantSession.WorkspacePath,
		ParticipantSessionID: participantSessionID,
		ParticipantSegmentID: seg.ID,
		OutputMode:           turnOutputModeSilent,
	})
	return true
}
