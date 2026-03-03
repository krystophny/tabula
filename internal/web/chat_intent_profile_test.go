package web

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestParseIntentLLMProfileOptions(t *testing.T) {
	options := parseIntentLLMProfileOptions(" qwen3.5-9b, qwen3.5-4b, qwen3.5-9b ")
	if len(options) != 2 {
		t.Fatalf("options length = %d, want 2", len(options))
	}
	if options[0] != "qwen3.5-9b" {
		t.Fatalf("option[0] = %q, want qwen3.5-9b", options[0])
	}
	if options[1] != "qwen3.5-4b" {
		t.Fatalf("option[1] = %q, want qwen3.5-4b", options[1])
	}
}

func TestResolveIntentLLMProfileDefaults(t *testing.T) {
	if got := resolveIntentLLMProfile(""); got != DefaultIntentLLMProfile {
		t.Fatalf("default profile = %q, want %q", got, DefaultIntentLLMProfile)
	}
	if got := resolveIntentLLMProfile("QWEN3.5-4B"); got != "qwen3.5-4b" {
		t.Fatalf("normalized profile = %q, want qwen3.5-4b", got)
	}
}

func TestRuntimeIncludesIntentDelegatorProfiles(t *testing.T) {
	app := newAuthedTestApp(t)
	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/runtime", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("runtime status=%d body=%s", rr.Code, rr.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode runtime response: %v", err)
	}
	if got := strFromAny(payload["intent_llm_profile"]); got != "qwen3.5-9b" {
		t.Fatalf("intent_llm_profile = %q, want qwen3.5-9b", got)
	}
	profiles, _ := payload["available_intent_llm_profiles"].([]any)
	if len(profiles) < 2 {
		t.Fatalf("available_intent_llm_profiles len = %d, want >= 2", len(profiles))
	}
	if first := strFromAny(profiles[0]); first != "qwen3.5-9b" {
		t.Fatalf("available_intent_llm_profiles[0] = %q, want qwen3.5-9b", first)
	}
	if second := strFromAny(profiles[1]); second != "qwen3.5-4b" {
		t.Fatalf("available_intent_llm_profiles[1] = %q, want qwen3.5-4b", second)
	}
}

func TestClassifyIntentWithLLM_DelegationHintShortCircuitsNetwork(t *testing.T) {
	app := newAuthedTestApp(t)
	app.intentLLMURL = "http://127.0.0.1:1"

	action, err := app.classifyIntentWithLLM(context.Background(), "let codex audit this repo")
	if err != nil {
		t.Fatalf("classify intent with llm returned error: %v", err)
	}
	if action == nil {
		t.Fatal("expected action")
	}
	if action.Action != "delegate" {
		t.Fatalf("action = %q, want delegate", action.Action)
	}
	if got := strFromAny(action.Params["model"]); got != "codex" {
		t.Fatalf("delegate model = %q, want codex", got)
	}
	if got := strFromAny(action.Params["task"]); got == "" {
		t.Fatal("delegate task should not be empty")
	}
}
