package web

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type inkSubmitPoint struct {
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Pressure float64 `json:"pressure"`
}

type inkSubmitStroke struct {
	PointerType string           `json:"pointer_type"`
	Width       float64          `json:"width"`
	Points      []inkSubmitPoint `json:"points"`
}

type inkSubmitRequest struct {
	WorkspaceID   string            `json:"workspace_id"`
	ArtifactKind  string            `json:"artifact_kind"`
	ArtifactTitle string            `json:"artifact_title"`
	ArtifactPath  string            `json:"artifact_path"`
	SVG           string            `json:"svg"`
	PNGBase64     string            `json:"png_base64"`
	Strokes       []inkSubmitStroke `json:"strokes"`
}

func (a *App) handleInkSubmit(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	var req inkSubmitRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	project, err := a.resolveRuntimeWorkspaceByIDOrActive(req.WorkspaceID)
	if err != nil {
		if isNoRows(err) {
			http.Error(w, "project not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	svgText := strings.TrimSpace(req.SVG)
	if svgText == "" || len(req.Strokes) == 0 {
		http.Error(w, "ink payload is required", http.StatusBadRequest)
		return
	}
	pngBytes, err := decodeInkPNG(req.PNGBase64)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	inkDir := filepath.Join(project.RootPath, ".tabura", "artifacts", "ink")
	if err := os.MkdirAll(inkDir, 0o755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	stamp := time.Now().UTC().Format("20060102-150405")
	baseName := sanitizeReviewFilename(req.ArtifactTitle)
	if baseName == "" || baseName == "review" {
		baseName = "canvas"
	}
	svgName := fmt.Sprintf("%s-%s-ink.svg", stamp, baseName)
	svgPath := filepath.Join(inkDir, svgName)
	if err := os.WriteFile(svgPath, []byte(svgText), 0o644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	pngPath := ""
	pngName := ""
	if len(pngBytes) > 0 {
		pngName = fmt.Sprintf("%s-%s-ink.png", stamp, baseName)
		pngPath = filepath.Join(inkDir, pngName)
		if err := os.WriteFile(pngPath, pngBytes, 0o644); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	summaryName := fmt.Sprintf("%s-%s-ink.md", stamp, baseName)
	summaryPath := filepath.Join(inkDir, summaryName)
	var b strings.Builder
	fmt.Fprintf(&b, "# Ink Capture: %s\n\n", strings.TrimSpace(req.ArtifactTitle))
	if path := strings.TrimSpace(req.ArtifactPath); path != "" {
		fmt.Fprintf(&b, "- Source path: `%s`\n", path)
	}
	if kind := strings.TrimSpace(req.ArtifactKind); kind != "" {
		fmt.Fprintf(&b, "- Artifact kind: `%s`\n", kind)
	}
	fmt.Fprintf(&b, "- Stroke count: `%d`\n", len(req.Strokes))
	fmt.Fprintf(&b, "- SVG artifact: `%s`\n\n", filepath.ToSlash(filepath.Join(".tabura", "artifacts", "ink", svgName)))
	if pngName != "" {
		fmt.Fprintf(&b, "- PNG artifact: `%s`\n\n", filepath.ToSlash(filepath.Join(".tabura", "artifacts", "ink", pngName)))
	}
	b.WriteString("## Stroke Summary\n\n")
	for i, stroke := range req.Strokes {
		points := len(stroke.Points)
		pointerType := strings.TrimSpace(stroke.PointerType)
		if pointerType == "" {
			pointerType = "unknown"
		}
		fmt.Fprintf(&b, "%d. `%s` stroke with `%d` points and width `%.1f`\n", i+1, pointerType, points, stroke.Width)
	}
	if err := os.WriteFile(summaryPath, []byte(b.String()), 0o644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	relSVGPath, err := filepath.Rel(project.RootPath, svgPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	relSummaryPath, err := filepath.Rel(project.RootPath, summaryPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	relPNGPath := ""
	if pngPath != "" {
		relPNGPath, err = filepath.Rel(project.RootPath, pngPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	revisionFiles := map[string]string{
		"ink_svg": filepath.ToSlash(relSVGPath),
		"summary": filepath.ToSlash(relSummaryPath),
	}
	if relPNGPath != "" {
		revisionFiles["ink_png"] = filepath.ToSlash(relPNGPath)
	}
	revisionManifestPath, revisionHistoryPath, err := appendLocalRevision(
		project.RootPath,
		req.ArtifactTitle,
		req.ArtifactPath,
		"ink",
		"handwritten ink capture",
		revisionFiles,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{
		"ok":                     true,
		"workspace_id":           project.ID,
		"ink_svg_path":           filepath.ToSlash(relSVGPath),
		"ink_png_path":           filepath.ToSlash(relPNGPath),
		"summary_path":           filepath.ToSlash(relSummaryPath),
		"revision_manifest_path": revisionManifestPath,
		"revision_history_path":  revisionHistoryPath,
	})
}

func decodeInkPNG(raw string) ([]byte, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	if idx := strings.Index(trimmed, ","); idx >= 0 && strings.HasPrefix(strings.ToLower(trimmed[:idx]), "data:image/png;base64") {
		trimmed = trimmed[idx+1:]
	}
	data, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		return nil, fmt.Errorf("invalid png payload")
	}
	return data, nil
}
