package web

import (
	"strings"
	"sync"
)

const (
	chatCaptureModeText  = "text"
	chatCaptureModeVoice = "voice"
)

type chatCaptureModeTracker struct {
	mu    sync.Mutex
	modes map[string]string
}

func newChatCaptureModeTracker() *chatCaptureModeTracker {
	return &chatCaptureModeTracker{
		modes: map[string]string{},
	}
}

func normalizeChatCaptureMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case chatCaptureModeVoice:
		return chatCaptureModeVoice
	default:
		return chatCaptureModeText
	}
}

func (t *chatCaptureModeTracker) set(sessionID, mode string) {
	if t == nil {
		return
	}
	cleanSessionID := sessionID
	cleanMode := normalizeChatCaptureMode(mode)
	t.mu.Lock()
	defer t.mu.Unlock()
	if cleanSessionID == "" {
		return
	}
	t.modes[cleanSessionID] = cleanMode
}

func (t *chatCaptureModeTracker) consume(sessionID string) string {
	if t == nil {
		return chatCaptureModeText
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if sessionID == "" {
		return chatCaptureModeText
	}
	mode := normalizeChatCaptureMode(t.modes[sessionID])
	delete(t.modes, sessionID)
	return mode
}
