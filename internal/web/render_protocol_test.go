package web

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestRenderMarkdownHTMLRendersStructuredHTML(t *testing.T) {
	rendered := renderMarkdownHTML("# Heading\n\n```go\nfmt.Println(\"hi\")\n```\n")
	if !strings.Contains(rendered, "<h1") || !strings.Contains(rendered, "Heading") {
		t.Fatalf("rendered heading missing: %q", rendered)
	}
	if !strings.Contains(rendered, "<pre") || !strings.Contains(rendered, "<code") {
		t.Fatalf("rendered code block missing: %q", rendered)
	}
	if !strings.Contains(rendered, "Println") || !strings.Contains(rendered, "&#34;hi&#34;") {
		t.Fatalf("rendered code content missing: %q", rendered)
	}
}

func TestFinalizeAssistantResponseBroadcastsRenderChatCommand(t *testing.T) {
	app := newAuthedTestApp(t)

	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensureDefaultWorkspace: %v", err)
	}
	session, err := app.store.GetOrCreateChatSession(project.WorkspacePath)
	if err != nil {
		t.Fatalf("GetOrCreateChatSession: %v", err)
	}

	conn, clientConn, cleanup := newParticipantTestWSConn(t)
	defer cleanup()
	app.hub.registerChat(session.ID, conn)
	defer app.hub.unregisterChat(session.ID, conn)

	persistedAssistantID := int64(0)
	persistedAssistantText := ""
	app.finalizeAssistantResponseWithMetadata(
		session.ID,
		project.WorkspacePath,
		"## Rendered\n\n```go\nfmt.Println(\"hi\")\n```",
		&persistedAssistantID,
		&persistedAssistantText,
		"turn-render-1",
		"",
		"thread-render-1",
		turnOutputModeVoice,
		newAssistantResponseMetadata(assistantProviderOpenAI, "gpt-5.4", 42*time.Millisecond),
	)

	_ = waitForWSJSONMessageType(t, clientConn, 2*time.Second, "assistant_output")
	payload := waitForWSJSONMessageType(t, clientConn, 2*time.Second, "render_chat")
	if got := strFromAny(payload["turn_id"]); got != "turn-render-1" {
		t.Fatalf("turn_id = %q, want turn-render-1", got)
	}
	html := strFromAny(payload["html"])
	if !strings.Contains(html, "<h2") || !strings.Contains(html, "<pre") {
		t.Fatalf("render_chat html = %q", html)
	}
	if got := strFromAny(payload["provider_model"]); got != "gpt-5.4" {
		t.Fatalf("provider_model = %q, want gpt-5.4", got)
	}
}

func TestHandleChatWSTextMessageTapAliasQueuesRequestedTurn(t *testing.T) {
	app := newAuthedTestApp(t)
	sessionID := testSessionForCanvasPosition(t, app)
	holdAssistantTurnWorker(t, app, sessionID)

	handleChatWSTextMessage(app, newChatWSConn(nil), sessionID, []byte(`{
		"type": "tap",
		"output_mode": "voice",
		"request_response": true,
		"cursor": {
			"title": "test.txt",
			"line": 7,
			"surrounding_text": "6: beta\n7: gamma\n8: delta"
		}
	}`))

	if got := app.turns.queuedCount(sessionID); got != 1 {
		t.Fatalf("queuedCount = %d, want 1", got)
	}
	events := app.chatCanvasPositions.consume(sessionID)
	if len(events) != 1 {
		t.Fatalf("consume len = %d, want 1", len(events))
	}
	if got := strings.TrimSpace(events[0].Gesture); got != "tap" {
		t.Fatalf("gesture = %q, want tap", got)
	}
}

func TestHandleChatWSTextMessageAudioAliasesStartAndStopSTT(t *testing.T) {
	app := newAuthedTestApp(t)
	conn, clientConn, cleanup := newParticipantTestWSConn(t)
	defer cleanup()

	audioData := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 512)))
	handleChatWSTextMessage(app, conn, "session-audio", []byte(`{
		"type": "audio_pcm",
		"mime_type": "audio/webm",
		"data": "`+audioData+`"
	}`))

	_ = waitForWSJSONMessageType(t, clientConn, 2*time.Second, "stt_started")
	conn.sttMu.Lock()
	active := conn.sttActive
	size := len(conn.sttBuf)
	conn.sttMu.Unlock()
	if !active {
		t.Fatal("expected STT session to be active after audio_pcm")
	}
	if size != 512 {
		t.Fatalf("stt buffer = %d, want 512", size)
	}

	handleChatWSTextMessage(app, conn, "session-audio", []byte(`{"type":"audio_stop"}`))
	payload := waitForWSJSONMessageType(t, clientConn, 2*time.Second, "stt_empty")
	if got := strFromAny(payload["reason"]); got != "recording_too_short" {
		t.Fatalf("reason = %q, want recording_too_short", got)
	}
}
