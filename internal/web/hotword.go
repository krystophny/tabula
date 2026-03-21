package web

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	hotwordModelFileName = "sloppy.onnx"
)

var hotwordRuntimeAssetFiles = []string{
	"melspectrogram.onnx",
	"embedding_model.onnx",
	hotwordModelFileName,
}

func (a *App) hotwordProjectRoot() string {
	root := strings.TrimSpace(a.localProjectDir)
	if root == "" {
		root = "."
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return root
	}
	return abs
}

func hotwordVendorDir(root string) string {
	return filepath.Join(root, "internal", "web", "static", "vendor", "openwakeword")
}

func hotwordVendorModelPath(root string) string {
	return filepath.Join(hotwordVendorDir(root), hotwordModelFileName)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func checkHotwordStatus(root string) map[string]interface{} {
	vendorDir := hotwordVendorDir(root)
	missing := make([]string, 0, len(hotwordRuntimeAssetFiles))
	for _, file := range hotwordRuntimeAssetFiles {
		if !fileExists(filepath.Join(vendorDir, file)) {
			missing = append(missing, file)
		}
	}
	ready := len(missing) == 0
	modelPath := hotwordVendorModelPath(root)
	model := map[string]interface{}{
		"exists": false,
		"file":   hotwordModelFileName,
	}
	if info, err := os.Stat(modelPath); err == nil && !info.IsDir() {
		model["exists"] = true
		model["modified_at"] = info.ModTime().UTC().Format(time.RFC3339)
		model["size_bytes"] = info.Size()
	}
	return map[string]interface{}{
		"ok":                   true,
		"model":                model,
		"ready":                ready,
		"missing":              missing,
		"training_in_progress": false,
	}
}

func (a *App) handleHotwordStatus(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	root := a.hotwordProjectRoot()
	writeJSON(w, checkHotwordStatus(root))
}
