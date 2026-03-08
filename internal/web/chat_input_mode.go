package web

import (
	"strings"
	"sync"
)

const (
	chatInputModeText  = "text"
	chatInputModeVoice = "voice"
)

type chatInputModeTracker struct {
	mu    sync.Mutex
	modes map[string]string
}

func newChatInputModeTracker() *chatInputModeTracker {
	return &chatInputModeTracker{
		modes: map[string]string{},
	}
}

func normalizeChatInputMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case chatInputModeVoice:
		return chatInputModeVoice
	default:
		return chatInputModeText
	}
}

func (t *chatInputModeTracker) set(sessionID, mode string) {
	if t == nil {
		return
	}
	cleanSessionID := sessionID
	cleanMode := normalizeChatInputMode(mode)
	t.mu.Lock()
	defer t.mu.Unlock()
	if cleanSessionID == "" {
		return
	}
	t.modes[cleanSessionID] = cleanMode
}

func (t *chatInputModeTracker) consume(sessionID string) string {
	if t == nil {
		return chatInputModeText
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if sessionID == "" {
		return chatInputModeText
	}
	mode := normalizeChatInputMode(t.modes[sessionID])
	delete(t.modes, sessionID)
	return mode
}
