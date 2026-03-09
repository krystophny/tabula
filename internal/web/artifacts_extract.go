package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/krystophny/tabura/internal/document"
	"github.com/krystophny/tabura/internal/store"
)

func (a *App) handleArtifactFigureExtract(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	artifactID, err := parseURLInt64Param(r, "artifact_id")
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	sourceArtifact, err := a.store.GetArtifact(artifactID)
	if err != nil {
		writeDomainStoreError(w, err)
		return
	}
	if sourceArtifact.Kind != store.ArtifactKindPDF {
		writeAPIError(w, http.StatusBadRequest, "artifact must be a pdf")
		return
	}
	pdfPath := strings.TrimSpace(stringFromPointer(sourceArtifact.RefPath))
	if pdfPath == "" {
		writeAPIError(w, http.StatusBadRequest, "pdf artifact ref_path is required")
		return
	}
	outputDir, err := a.figureExtractionOutputDir(pdfPath)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	figures, err := document.ExtractFiguresWithOptions(pdfPath, document.FigureExtractOptions{OutputDir: outputDir})
	if errors.Is(err, document.ErrPDFImagesBinaryMissing) {
		writeAPIError(w, http.StatusServiceUnavailable, "pdfimages binary not available")
		return
	}
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, err.Error())
		return
	}
	items, err := a.store.ListArtifactItems(sourceArtifact.ID)
	if err != nil {
		writeDomainStoreError(w, err)
		return
	}

	created := make([]store.Artifact, 0, len(figures))
	for _, figure := range figures {
		artifact, err := a.upsertExtractedFigureArtifact(sourceArtifact, figure)
		if err != nil {
			writeDomainStoreError(w, err)
			return
		}
		for _, item := range items {
			if err := a.store.LinkItemArtifact(item.ID, artifact.ID, "output"); err != nil {
				writeDomainStoreError(w, err)
				return
			}
		}
		created = append(created, artifact)
	}

	writeAPIData(w, http.StatusOK, map[string]any{
		"artifacts": created,
		"figures":   figures,
	})
}

func (a *App) figureExtractionOutputDir(pdfPath string) (string, error) {
	clean, err := filepath.Abs(strings.TrimSpace(pdfPath))
	if err != nil {
		return "", err
	}
	stem := sanitizeFigureArtifactStem(strings.TrimSuffix(filepath.Base(clean), filepath.Ext(clean)))
	workspaceID, err := a.store.FindWorkspaceContainingPath(clean)
	if err != nil {
		return "", err
	}
	if workspaceID != nil {
		workspace, err := a.store.GetWorkspace(*workspaceID)
		if err != nil {
			return "", err
		}
		return filepath.Join(workspace.DirPath, ".tabura", "artifacts", "figures", stem), nil
	}
	return filepath.Join(filepath.Dir(clean), ".tabura", "artifacts", "figures", stem), nil
}

func sanitizeFigureArtifactStem(raw string) string {
	clean := strings.TrimSpace(strings.ToLower(raw))
	if clean == "" {
		return "figures"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range clean {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	clean = strings.Trim(b.String(), "-")
	if clean == "" {
		return "figures"
	}
	return clean
}

func (a *App) upsertExtractedFigureArtifact(sourceArtifact store.Artifact, figure document.Figure) (store.Artifact, error) {
	metaJSON, err := figureArtifactMeta(sourceArtifact, figure)
	if err != nil {
		return store.Artifact{}, err
	}
	title := figure.Caption
	if existing, err := a.findArtifactByRefPath(figure.ImagePath); err != nil {
		return store.Artifact{}, err
	} else if existing != nil {
		if err := a.store.UpdateArtifact(existing.ID, store.ArtifactUpdate{
			Title:    &title,
			MetaJSON: &metaJSON,
		}); err != nil {
			return store.Artifact{}, err
		}
		return a.store.GetArtifact(existing.ID)
	}
	refPath := figure.ImagePath
	return a.store.CreateArtifact(store.ArtifactKindImage, &refPath, nil, &title, &metaJSON)
}

func (a *App) findArtifactByRefPath(path string) (*store.Artifact, error) {
	clean, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return nil, err
	}
	artifacts, err := a.store.ListArtifacts()
	if err != nil {
		return nil, err
	}
	for i := range artifacts {
		if filepath.Clean(strings.TrimSpace(stringFromPointer(artifacts[i].RefPath))) == clean {
			return &artifacts[i], nil
		}
	}
	return nil, nil
}

func figureArtifactMeta(sourceArtifact store.Artifact, figure document.Figure) (string, error) {
	sourcePath := filepath.Clean(strings.TrimSpace(stringFromPointer(sourceArtifact.RefPath)))
	sourceTitle := strings.TrimSpace(stringFromPointer(sourceArtifact.Title))
	if sourceTitle == "" {
		sourceTitle = filepath.Base(sourcePath)
	}
	meta := map[string]any{
		"caption":            figure.Caption,
		"figure_index":       figure.Index,
		"page":               figure.Page,
		"source_artifact_id": sourceArtifact.ID,
		"source_pdf_path":    sourcePath,
		"source_ref":         fmt.Sprintf("%s#page=%d", sourceTitle, figure.Page),
		"type":               "figure",
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
