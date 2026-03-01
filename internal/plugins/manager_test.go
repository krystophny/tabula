package plugins

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeManifest(t *testing.T, dir, name string, payload map[string]any) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), raw, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

func TestManagerApplyRewriteAndBlockSequence(t *testing.T) {
	var sawAuth string
	rewriteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = strings.TrimSpace(r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"text":"rewritten message"}`))
	}))
	defer rewriteServer.Close()

	blockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"blocked":true,"reason":"policy block"}`))
	}))
	defer blockServer.Close()

	t.Setenv("TABURA_PLUGIN_TEST_SECRET", "abc123")
	dir := t.TempDir()
	writeManifest(t, dir, "01-rewrite.json", map[string]any{
		"id":         "rewrite",
		"kind":       "webhook",
		"endpoint":   rewriteServer.URL,
		"hooks":      []string{HookChatPreUserMessage},
		"enabled":    true,
		"secret_env": "TABURA_PLUGIN_TEST_SECRET",
	})
	writeManifest(t, dir, "02-block.json", map[string]any{
		"id":       "blocker",
		"kind":     "webhook",
		"endpoint": blockServer.URL,
		"hooks":    []string{HookChatPreUserMessage},
		"enabled":  true,
	})

	mgr, err := New(Options{Dir: dir})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	got := mgr.Apply(context.Background(), HookRequest{
		Hook: HookChatPreUserMessage,
		Text: "hello",
	})
	if !got.Blocked {
		t.Fatalf("expected blocked=true")
	}
	if got.Reason != "policy block" {
		t.Fatalf("blocked reason = %q, want %q", got.Reason, "policy block")
	}
	if got.Text != "rewritten message" {
		t.Fatalf("text = %q, want %q", got.Text, "rewritten message")
	}
	if sawAuth != "Bearer abc123" {
		t.Fatalf("authorization header = %q, want %q", sawAuth, "Bearer abc123")
	}
}

func TestManagerApplyContinuesAfterPluginHTTPError(t *testing.T) {
	errServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "fail", http.StatusBadGateway)
	}))
	defer errServer.Close()

	rewriteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"text":"second plugin output"}`))
	}))
	defer rewriteServer.Close()

	dir := t.TempDir()
	writeManifest(t, dir, "01-error.json", map[string]any{
		"id":       "erroring",
		"kind":     "webhook",
		"endpoint": errServer.URL,
		"hooks":    []string{HookChatPreAssistantPrompt},
		"enabled":  true,
	})
	writeManifest(t, dir, "02-rewrite.json", map[string]any{
		"id":       "rewrite",
		"kind":     "webhook",
		"endpoint": rewriteServer.URL,
		"hooks":    []string{HookChatPreAssistantPrompt},
		"enabled":  true,
	})

	mgr, err := New(Options{Dir: dir})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	got := mgr.Apply(context.Background(), HookRequest{
		Hook: HookChatPreAssistantPrompt,
		Text: "original",
	})
	if got.Blocked {
		t.Fatalf("expected blocked=false")
	}
	if got.Text != "second plugin output" {
		t.Fatalf("text = %q, want %q", got.Text, "second plugin output")
	}
}
