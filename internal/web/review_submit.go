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

type reviewSubmitAnchor struct {
	Title        string  `json:"title"`
	Line         int     `json:"line"`
	Page         int     `json:"page"`
	SelectedText string  `json:"selected_text"`
	X            float64 `json:"x"`
	Y            float64 `json:"y"`
}

type reviewSubmitComment struct {
	Text   string             `json:"text"`
	Anchor reviewSubmitAnchor `json:"anchor"`
}

type reviewSubmitRequest struct {
	ProjectID       string                `json:"project_id"`
	ArtifactKind    string                `json:"artifact_kind"`
	ArtifactTitle   string                `json:"artifact_title"`
	ArtifactPath    string                `json:"artifact_path"`
	Comments        []reviewSubmitComment `json:"comments"`
	PDFExportBase64 string                `json:"pdf_export_base64"`
}

func sanitizeReviewFilename(raw string) string {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "review"
	}
	name = filepath.Base(name)
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "\\", "-")
	return name
}

func (a *App) handleReviewSubmit(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	var req reviewSubmitRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	project, err := a.resolveProjectByIDOrActive(req.ProjectID)
	if err != nil {
		if isNoRows(err) {
			http.Error(w, "project not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(req.Comments) == 0 {
		http.Error(w, "at least one comment is required", http.StatusBadRequest)
		return
	}

	reviewsDir := filepath.Join(project.RootPath, ".tabura", "artifacts", "reviews")
	if err := os.MkdirAll(reviewsDir, 0o755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	stamp := time.Now().UTC().Format("20060102-150405")
	baseName := sanitizeReviewFilename(req.ArtifactTitle)
	if baseName == "" {
		baseName = "review"
	}
	markdownName := fmt.Sprintf("%s-%s-review.md", stamp, baseName)
	markdownPath := filepath.Join(reviewsDir, markdownName)

	var b strings.Builder
	fmt.Fprintf(&b, "# Review: %s\n\n", strings.TrimSpace(req.ArtifactTitle))
	if path := strings.TrimSpace(req.ArtifactPath); path != "" {
		fmt.Fprintf(&b, "- Source path: `%s`\n", path)
	}
	if kind := strings.TrimSpace(req.ArtifactKind); kind != "" {
		fmt.Fprintf(&b, "- Artifact kind: `%s`\n", kind)
	}
	b.WriteString("\n## Comments\n\n")
	for i, comment := range req.Comments {
		fmt.Fprintf(&b, "%d. %s\n", i+1, strings.TrimSpace(comment.Text))
		if title := strings.TrimSpace(comment.Anchor.Title); title != "" {
			fmt.Fprintf(&b, "   - Title: `%s`\n", title)
		}
		if comment.Anchor.Line > 0 {
			fmt.Fprintf(&b, "   - Line: `%d`\n", comment.Anchor.Line)
		}
		if comment.Anchor.Page > 0 {
			fmt.Fprintf(&b, "   - Page: `%d`\n", comment.Anchor.Page)
		}
		if selected := strings.TrimSpace(comment.Anchor.SelectedText); selected != "" {
			fmt.Fprintf(&b, "   - Selection: `%s`\n", selected)
		}
		if comment.Anchor.X != 0 || comment.Anchor.Y != 0 {
			fmt.Fprintf(&b, "   - Point: `%.1f, %.1f`\n", comment.Anchor.X, comment.Anchor.Y)
		}
	}
	if err := os.WriteFile(markdownPath, []byte(b.String()), 0o644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	pdfExportRel := ""
	if encoded := strings.TrimSpace(req.PDFExportBase64); encoded != "" {
		raw, err := base64.StdEncoding.DecodeString(encoded)
		if err == nil && len(raw) > 0 {
			pdfName := fmt.Sprintf("%s-%s-annotated.pdf", stamp, baseName)
			pdfPath := filepath.Join(reviewsDir, pdfName)
			if writeErr := os.WriteFile(pdfPath, raw, 0o644); writeErr == nil {
				rel, relErr := filepath.Rel(project.RootPath, pdfPath)
				if relErr == nil {
					pdfExportRel = filepath.ToSlash(rel)
				}
			}
		}
	}

	relMarkdownPath, err := filepath.Rel(project.RootPath, markdownPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	revisionFiles := map[string]string{
		"review_markdown": filepath.ToSlash(relMarkdownPath),
	}
	if pdfExportRel != "" {
		revisionFiles["pdf_export"] = pdfExportRel
	}
	revisionManifestPath, revisionHistoryPath, err := appendLocalRevision(
		project.RootPath,
		req.ArtifactTitle,
		req.ArtifactPath,
		"review",
		"submitted review batch",
		revisionFiles,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := a.markProjectReviewSubmitted(project); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{
		"ok":                     true,
		"project_id":             project.ID,
		"review_markdown_path":   filepath.ToSlash(relMarkdownPath),
		"pdf_export_path":        pdfExportRel,
		"revision_manifest_path": revisionManifestPath,
		"revision_history_path":  revisionHistoryPath,
	})
}
