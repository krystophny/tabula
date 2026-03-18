package web

import (
	"strings"
	"sync"
)

type workspaceAttentionTracker struct {
	mu                 sync.Mutex
	lastSeenAt         map[string]int64
	lastCanvasChangeAt map[string]int64
	lastReviewSubmitAt map[string]int64
}

func newProjectAttentionTracker() *workspaceAttentionTracker {
	return &workspaceAttentionTracker{
		lastSeenAt:         map[string]int64{},
		lastCanvasChangeAt: map[string]int64{},
		lastReviewSubmitAt: map[string]int64{},
	}
}

func (t *workspaceAttentionTracker) markSeen(workspacePath string, at int64) {
	key := strings.TrimSpace(workspacePath)
	if key == "" || at <= 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if at > t.lastSeenAt[key] {
		t.lastSeenAt[key] = at
	}
}

func (t *workspaceAttentionTracker) markCanvasChange(workspacePath string, at int64) {
	key := strings.TrimSpace(workspacePath)
	if key == "" || at <= 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if at > t.lastCanvasChangeAt[key] {
		t.lastCanvasChangeAt[key] = at
	}
}

func (t *workspaceAttentionTracker) markReviewSubmitted(workspacePath string, at int64) {
	key := strings.TrimSpace(workspacePath)
	if key == "" || at <= 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if at > t.lastReviewSubmitAt[key] {
		t.lastReviewSubmitAt[key] = at
	}
}

func (t *workspaceAttentionTracker) snapshot(workspacePath string) (lastSeenAt, lastCanvasChangeAt, lastReviewSubmitAt int64) {
	key := strings.TrimSpace(workspacePath)
	if key == "" {
		return 0, 0, 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastSeenAt[key], t.lastCanvasChangeAt[key], t.lastReviewSubmitAt[key]
}
