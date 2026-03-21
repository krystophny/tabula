package web

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	modelRaw, ok := payload["model"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected model details, got %#v", payload["model"])
	}
	if modelRaw["file"] != hotwordModelFileName {
		t.Fatalf("model file = %#v, want %q", modelRaw["file"], hotwordModelFileName)
	}
	if exists, _ := modelRaw["exists"].(bool); exists {
		t.Fatalf("expected model exists=false, got %#v", modelRaw["exists"])
	}
	if training, _ := payload["training_in_progress"].(bool); training {
		t.Fatalf("expected training_in_progress=false, got %#v", payload["training_in_progress"])
	}
}

func TestHotwordStatusReportsReadyWhenAllAssetsPresent(t *testing.T) {
	app := newAuthedTestApp(t)
	root := t.TempDir()
	app.localProjectDir = root

	vendorDir := filepath.Join(root, "internal", "web", "static", "vendor", "openwakeword")
	if err := os.MkdirAll(vendorDir, 0o755); err != nil {
		t.Fatalf("mkdir vendor dir: %v", err)
	}
	for _, file := range hotwordRuntimeAssetFiles {
		if err := os.WriteFile(filepath.Join(vendorDir, file), []byte("x"), 0o644); err != nil {
			t.Fatalf("write vendor file %s: %v", file, err)
		}
	}
	modelPath := filepath.Join(vendorDir, hotwordModelFileName)
	modifiedAt := time.Date(2026, time.March, 21, 12, 34, 56, 0, time.UTC)
	if err := os.Chtimes(modelPath, modifiedAt, modifiedAt); err != nil {
		t.Fatalf("chtimes %s: %v", hotwordModelFileName, err)
	}

	rr := doAuthedJSONRequest(t, app.Router(), "GET", "/api/hotword/status", nil)
	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	payload := decodeJSONBody(t, rr.Body.String())
	if ready, _ := payload["ready"].(bool); !ready {
		t.Fatalf("expected ready=true when all assets present, got payload=%#v", payload)
	}
	missingRaw, _ := payload["missing"].([]interface{})
	if len(missingRaw) != 0 {
		t.Fatalf("expected empty missing list, got %v", missingRaw)
	}
	modelRaw, ok := payload["model"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected model details, got %#v", payload["model"])
	}
	if exists, _ := modelRaw["exists"].(bool); !exists {
		t.Fatalf("expected model exists=true, got %#v", modelRaw["exists"])
	}
	if size, _ := modelRaw["size_bytes"].(float64); size != 1 {
		t.Fatalf("model size = %#v, want 1", modelRaw["size_bytes"])
	}
	if modified, _ := modelRaw["modified_at"].(string); modified != modifiedAt.Format(time.RFC3339) {
		t.Fatalf("model modified_at = %#v, want %q", modelRaw["modified_at"], modifiedAt.Format(time.RFC3339))
	}
}
