package web

import (
	"encoding/json"
	"net/http"
	"testing"
)

func boolFromAny(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return parseBoolString(t, false)
	default:
		return false
	}
}

func TestRuntimeIncludesSafetyPreferences(t *testing.T) {
	app := newAuthedTestApp(t)
	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/runtime", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("runtime status=%d body=%s", rr.Code, rr.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode runtime response: %v", err)
	}
	if got := boolFromAny(payload["safety_yolo_mode"]); got {
		t.Fatalf("safety_yolo_mode = %v, want false", got)
	}
	if got := strFromAny(payload["execution_policy"]); got != executionPolicyDefault {
		t.Fatalf("execution_policy = %q, want %q", got, executionPolicyDefault)
	}
	if got := boolFromAny(payload["disclaimer_ack_required"]); !got {
		t.Fatalf("disclaimer_ack_required = %v, want true", got)
	}
	if got := boolFromAny(payload["silent_mode"]); got {
		t.Fatalf("silent_mode = %v, want false", got)
	}
	if got := strFromAny(payload["tool"]); got != "pointer" {
		t.Fatalf("tool = %q, want %q", got, "pointer")
	}
	if got := strFromAny(payload["startup_behavior"]); got != "hub_first" {
		t.Fatalf("startup_behavior = %q, want %q", got, "hub_first")
	}
	if got := strFromAny(payload["active_sphere"]); got != "private" {
		t.Fatalf("active_sphere = %q, want %q", got, "private")
	}
	if got := strFromAny(payload["disclaimer_version"]); got != disclaimerVersionCurrent {
		t.Fatalf("disclaimer_version = %q, want %q", got, disclaimerVersionCurrent)
	}
}

func TestRuntimeYoloModeUpdatePersists(t *testing.T) {
	app := newAuthedTestApp(t)
	setRR := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/runtime/yolo", map[string]any{"enabled": true})
	if setRR.Code != http.StatusOK {
		t.Fatalf("set yolo status=%d body=%s", setRR.Code, setRR.Body.String())
	}
	var setPayload map[string]any
	if err := json.Unmarshal(setRR.Body.Bytes(), &setPayload); err != nil {
		t.Fatalf("decode yolo response: %v", err)
	}
	if got := strFromAny(setPayload["execution_policy"]); got != executionPolicyAutonomous {
		t.Fatalf("execution_policy = %q, want %q", got, executionPolicyAutonomous)
	}
	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/runtime", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("runtime status=%d body=%s", rr.Code, rr.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode runtime response: %v", err)
	}
	if got := boolFromAny(payload["safety_yolo_mode"]); !got {
		t.Fatalf("safety_yolo_mode = %v, want true", got)
	}
	if got := strFromAny(payload["execution_policy"]); got != executionPolicyAutonomous {
		t.Fatalf("execution_policy = %q, want %q", got, executionPolicyAutonomous)
	}
}

func TestRuntimeDisclaimerAckClearsRequiredFlag(t *testing.T) {
	app := newAuthedTestApp(t)
	ackRR := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/runtime/disclaimer-ack", map[string]any{"version": disclaimerVersionCurrent})
	if ackRR.Code != http.StatusOK {
		t.Fatalf("ack status=%d body=%s", ackRR.Code, ackRR.Body.String())
	}
	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/runtime", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("runtime status=%d body=%s", rr.Code, rr.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode runtime response: %v", err)
	}
	if got := boolFromAny(payload["disclaimer_ack_required"]); got {
		t.Fatalf("disclaimer_ack_required = %v, want false", got)
	}
	if got := strFromAny(payload["disclaimer_ack_version"]); got != disclaimerVersionCurrent {
		t.Fatalf("disclaimer_ack_version = %q, want %q", got, disclaimerVersionCurrent)
	}
}

func TestRuntimePreferenceUpdatePersists(t *testing.T) {
	app := newAuthedTestApp(t)
	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPatch, "/api/runtime/preferences", map[string]any{
		"silent_mode":      true,
		"tool":             "text_note",
		"startup_behavior": "hub_first",
		"active_sphere":    "work",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("preference update status=%d body=%s", rr.Code, rr.Body.String())
	}

	runtimeRR := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/runtime", nil)
	if runtimeRR.Code != http.StatusOK {
		t.Fatalf("runtime status=%d body=%s", runtimeRR.Code, runtimeRR.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(runtimeRR.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode runtime response: %v", err)
	}
	if got := boolFromAny(payload["silent_mode"]); !got {
		t.Fatalf("silent_mode = %v, want true", got)
	}
	if got := strFromAny(payload["tool"]); got != "text_note" {
		t.Fatalf("tool = %q, want %q", got, "text_note")
	}
	if got := strFromAny(payload["startup_behavior"]); got != "hub_first" {
		t.Fatalf("startup_behavior = %q, want %q", got, "hub_first")
	}
	if got := strFromAny(payload["active_sphere"]); got != "work" {
		t.Fatalf("active_sphere = %q, want %q", got, "work")
	}
}

func TestRuntimePreferenceUpdateRejectsInvalidSphere(t *testing.T) {
	app := newAuthedTestApp(t)
	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPatch, "/api/runtime/preferences", map[string]any{
		"active_sphere": "office",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid sphere status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestRuntimeDoesNotReadDeletedLegacyInteractionMode(t *testing.T) {
	app := newAuthedTestApp(t)

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/runtime", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("runtime status=%d body=%s", rr.Code, rr.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode runtime response: %v", err)
	}
	if got := strFromAny(payload["tool"]); got != "pointer" {
		t.Fatalf("tool = %q, want %q", got, "pointer")
	}
}
