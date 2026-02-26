package web

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func decodeJSONBody(t *testing.T, body string) map[string]interface{} {
	t.Helper()
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	return payload
}

func TestHotwordStatusReportsMissingAssets(t *testing.T) {
	app := newAuthedTestApp(t)
	root := t.TempDir()
	app.localProjectDir = root

	rr := doAuthedJSONRequest(t, app.Router(), "GET", "/api/hotword/status", nil)
	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	payload := decodeJSONBody(t, rr.Body.String())
	if ready, _ := payload["ready"].(bool); ready {
		t.Fatalf("expected ready=false, got true")
	}
	missingRaw, ok := payload["missing"].([]interface{})
	if !ok || len(missingRaw) == 0 {
		t.Fatalf("expected non-empty missing assets list, got %#v", payload["missing"])
	}
}

func TestHotwordTrainStartAndStatus(t *testing.T) {
	app := newAuthedTestApp(t)
	root := t.TempDir()
	app.localProjectDir = root

	vendorDir := filepath.Join(root, "internal", "web", "static", "vendor", "openwakeword")
	if err := os.MkdirAll(vendorDir, 0o755); err != nil {
		t.Fatalf("mkdir vendor dir: %v", err)
	}
	for _, file := range []string{"ort.min.js", "melspectrogram.onnx", "embedding_model.onnx"} {
		if err := os.WriteFile(filepath.Join(vendorDir, file), []byte("x"), 0o644); err != nil {
			t.Fatalf("write vendor file %s: %v", file, err)
		}
	}

	app.hotwordTrainRunner = func(_ context.Context, cwd string, onProgress func(string)) error {
		onProgress("mock training started")
		modelDir := filepath.Join(cwd, "models", "hotword")
		if err := os.MkdirAll(modelDir, 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(modelDir, "tabura.onnx"), []byte("model"), 0o644)
	}

	rrStart := doAuthedJSONRequest(t, app.Router(), "POST", "/api/hotword/train", map[string]interface{}{})
	if rrStart.Code != 200 {
		t.Fatalf("expected 200, got %d", rrStart.Code)
	}
	startPayload := decodeJSONBody(t, rrStart.Body.String())
	jobID := strings.TrimSpace(startPayload["job_id"].(string))
	if jobID == "" {
		t.Fatal("expected non-empty job_id")
	}

	var statusPayload map[string]interface{}
	deadline := time.Now().Add(3 * time.Second)
	for {
		rrStatus := doAuthedJSONRequest(t, app.Router(), "GET", "/api/hotword/train/"+jobID, nil)
		if rrStatus.Code != 200 {
			t.Fatalf("expected 200 from status endpoint, got %d", rrStatus.Code)
		}
		statusPayload = decodeJSONBody(t, rrStatus.Body.String())
		status := strings.TrimSpace(statusPayload["status"].(string))
		if status == "succeeded" || status == "failed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for training job completion: %#v", statusPayload)
		}
		time.Sleep(25 * time.Millisecond)
	}

	if status := strings.TrimSpace(statusPayload["status"].(string)); status != "succeeded" {
		t.Fatalf("expected succeeded status, got %q payload=%#v", status, statusPayload)
	}

	modelPath := filepath.Join(vendorDir, "tabura.onnx")
	if _, err := os.Stat(modelPath); err != nil {
		t.Fatalf("expected installed vendor model at %s: %v", modelPath, err)
	}

	rrReady := doAuthedJSONRequest(t, app.Router(), "GET", "/api/hotword/status", nil)
	if rrReady.Code != 200 {
		t.Fatalf("expected 200, got %d", rrReady.Code)
	}
	readyPayload := decodeJSONBody(t, rrReady.Body.String())
	if ready, _ := readyPayload["ready"].(bool); !ready {
		t.Fatalf("expected ready=true after training, got payload=%#v", readyPayload)
	}
}
