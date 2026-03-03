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
	if got := boolFromAny(payload["disclaimer_ack_required"]); !got {
		t.Fatalf("disclaimer_ack_required = %v, want true", got)
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
