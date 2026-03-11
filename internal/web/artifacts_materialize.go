package web

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/krystophny/tabura/internal/store"
)

type artifactMaterializeRequest struct {
	WorkspaceID  *int64 `json:"workspace_id,omitempty"`
	RelativePath string `json:"relative_path,omitempty"`
}

type artifactMaterializeError struct {
	message string
}

func (e artifactMaterializeError) Error() string {
	return e.message
}

var artifactMaterializeStemPattern = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func newArtifactMaterializeError(message string) error {
	return artifactMaterializeError{message: strings.TrimSpace(message)}
}

func artifactIsAlreadyFileBacked(kind store.ArtifactKind) bool {
	switch kind {
	case store.ArtifactKindDocument, store.ArtifactKindMarkdown, store.ArtifactKindPDF, store.ArtifactKindImage:
		return true
	default:
		return false
	}
}

func artifactMaterializeExtension(kind store.ArtifactKind) string {
	switch kind {
	case store.ArtifactKindEmail:
		return ".eml"
	default:
		return ".md"
	}
}

func artifactMaterializeStem(artifact store.Artifact) string {
	if title := strings.TrimSpace(optionalStringValue(artifact.Title)); title != "" {
		base := strings.TrimSuffix(filepath.Base(title), filepath.Ext(title))
		if clean := sanitizeArtifactMaterializeStem(base); clean != "" {
			return clean
		}
	}
	if rawURL := strings.TrimSpace(optionalStringValue(artifact.RefURL)); rawURL != "" {
		if parsed, err := url.Parse(rawURL); err == nil {
			path := strings.Trim(strings.TrimSpace(parsed.Path), "/")
			if path != "" {
				parts := strings.Split(path, "/")
				candidate := parts[len(parts)-1]
				if len(parts) >= 2 && (parts[len(parts)-2] == "issues" || parts[len(parts)-2] == "pull") {
					candidate = parts[len(parts)-2] + "-" + candidate
				}
				if clean := sanitizeArtifactMaterializeStem(candidate); clean != "" {
					return clean
				}
			}
		}
	}
	if clean := sanitizeArtifactMaterializeStem(string(artifact.Kind)); clean != "" {
		return clean
	}
	return "artifact"
}

func sanitizeArtifactMaterializeStem(raw string) string {
	clean := strings.ToLower(strings.TrimSpace(raw))
	if clean == "" {
		return ""
	}
	clean = artifactMaterializeStemPattern.ReplaceAllString(clean, "-")
	clean = strings.Trim(clean, "-.")
	if len(clean) > 64 {
		clean = strings.Trim(clean[:64], "-.")
	}
	return clean
}

func resolveArtifactMaterializePath(workspace store.Workspace, artifact store.Artifact, requested string) (string, string, error) {
	root := filepath.Clean(strings.TrimSpace(workspace.DirPath))
	if root == "" {
		return "", "", newArtifactMaterializeError("workspace path is required")
	}
	extension := artifactMaterializeExtension(artifact.Kind)
	var absolutePath string
	switch {
	case strings.TrimSpace(requested) != "":
		target := strings.TrimSpace(requested)
		if filepath.IsAbs(target) {
			absolutePath = filepath.Clean(target)
		} else {
			absolutePath = filepath.Clean(filepath.Join(root, target))
		}
		if filepath.Ext(absolutePath) == "" {
			absolutePath += extension
		}
	case strings.TrimSpace(optionalStringValue(artifact.RefPath)) != "":
		absolutePath = filepath.Clean(strings.TrimSpace(*artifact.RefPath))
	default:
		name := fmt.Sprintf("%s-%d%s", artifactMaterializeStem(artifact), artifact.ID, extension)
		absolutePath = filepath.Join(root, ".tabura", "artifacts", "materialized", name)
	}
	relPath, err := filepath.Rel(root, absolutePath)
	if err != nil {
		return "", "", err
	}
	relPath = filepath.Clean(relPath)
	if relPath == "." || relPath == ".." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) {
		return "", "", newArtifactMaterializeError("materialized artifact path must stay inside the workspace")
	}
	return absolutePath, filepath.ToSlash(relPath), nil
}

func parseArtifactMetaMap(raw *string) map[string]any {
	text := strings.TrimSpace(optionalStringValue(raw))
	if text == "" {
		return map[string]any{}
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return map[string]any{}
	}
	return parsed
}

func artifactMetaString(meta map[string]any, keys ...string) string {
	for _, key := range keys {
		value := strings.TrimSpace(fmt.Sprint(meta[key]))
		if value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func artifactMetaStringList(meta map[string]any, key string) []string {
	value, ok := meta[key]
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		out := make([]string, 0, len(typed))
		for _, entry := range typed {
			if clean := strings.TrimSpace(entry); clean != "" {
				out = append(out, clean)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, entry := range typed {
			if clean := strings.TrimSpace(fmt.Sprint(entry)); clean != "" && clean != "<nil>" {
				out = append(out, clean)
			}
		}
		return out
	default:
		if clean := strings.TrimSpace(fmt.Sprint(typed)); clean != "" && clean != "<nil>" {
			return []string{clean}
		}
	}
	return nil
}

func renderMaterializedEmailArtifact(artifact store.Artifact) string {
	meta := parseArtifactMetaMap(artifact.MetaJSON)
	subject := firstPrintableValue(artifactMetaString(meta, "subject"), optionalStringValue(artifact.Title), "Email")
	from := artifactMetaString(meta, "sender")
	date := artifactMetaString(meta, "date")
	body := strings.TrimSpace(printableArtifactContent(artifact, artifactMetaString(meta, "body", "text", "content", "summary", "snippet")))
	recipients := strings.Join(artifactMetaStringList(meta, "recipients"), ", ")
	labels := strings.Join(artifactMetaStringList(meta, "labels"), ", ")

	lines := []string{
		"Subject: " + subject,
	}
	if from != "" {
		lines = append(lines, "From: "+from)
	}
	if recipients != "" {
		lines = append(lines, "To: "+recipients)
	}
	if date != "" {
		lines = append(lines, "Date: "+date)
	}
	if labels != "" {
		lines = append(lines, "X-Tabura-Contexts: "+labels)
	}
	lines = append(lines, "MIME-Version: 1.0", "Content-Type: text/plain; charset=utf-8", "", body)
	return strings.Join(lines, "\r\n")
}

func renderMaterializedMarkdownArtifact(artifact store.Artifact) string {
	metaText, metaPretty := printableArtifactMeta(artifact.MetaJSON)
	title := firstPrintableValue(optionalStringValue(artifact.Title), optionalStringValue(artifact.RefURL), string(artifact.Kind))
	lines := []string{
		"# " + title,
		"",
		fmt.Sprintf("- Kind: `%s`", artifact.Kind),
	}
	if refURL := strings.TrimSpace(optionalStringValue(artifact.RefURL)); refURL != "" {
		lines = append(lines, fmt.Sprintf("- Source: %s", refURL))
	}
	if metaText != "" {
		lines = append(lines, "", "## Content", "", metaText)
	}
	if metaPretty != "" {
		lines = append(lines, "", "## Metadata", "", "```json", metaPretty, "```")
	}
	return strings.TrimSpace(strings.Join(lines, "\n")) + "\n"
}

func renderMaterializedArtifact(artifact store.Artifact) (string, error) {
	if artifactIsAlreadyFileBacked(artifact.Kind) {
		return "", newArtifactMaterializeError("artifact is already file-backed")
	}
	switch artifact.Kind {
	case store.ArtifactKindEmail:
		return renderMaterializedEmailArtifact(artifact), nil
	default:
		return renderMaterializedMarkdownArtifact(artifact), nil
	}
}

func (a *App) resolveArtifactMaterializeWorkspace(artifact store.Artifact, requestedWorkspaceID *int64) (store.Workspace, error) {
	if requestedWorkspaceID != nil {
		if *requestedWorkspaceID <= 0 {
			return store.Workspace{}, newArtifactMaterializeError("workspace_id must be a positive integer")
		}
		return a.store.GetWorkspace(*requestedWorkspaceID)
	}
	if artifact.RefPath != nil {
		workspaceID, err := a.store.FindWorkspaceContainingPath(*artifact.RefPath)
		if err != nil {
			return store.Workspace{}, err
		}
		if workspaceID != nil {
			return a.store.GetWorkspace(*workspaceID)
		}
	}
	workspaceID, err := a.store.InferWorkspaceForArtifact(artifact)
	if err != nil {
		return store.Workspace{}, err
	}
	if workspaceID != nil {
		return a.store.GetWorkspace(*workspaceID)
	}
	items, err := a.store.ListArtifactItems(artifact.ID)
	if err != nil {
		return store.Workspace{}, err
	}
	itemWorkspaceID := int64(0)
	for _, item := range items {
		if item.WorkspaceID == nil {
			continue
		}
		if itemWorkspaceID == 0 {
			itemWorkspaceID = *item.WorkspaceID
			continue
		}
		if itemWorkspaceID != *item.WorkspaceID {
			return store.Workspace{}, newArtifactMaterializeError("artifact is linked to multiple workspaces; provide workspace_id")
		}
	}
	if itemWorkspaceID > 0 {
		return a.store.GetWorkspace(itemWorkspaceID)
	}
	workspaces, err := a.store.ListArtifactLinkWorkspaces(artifact.ID)
	if err != nil {
		return store.Workspace{}, err
	}
	if len(workspaces) == 1 {
		return workspaces[0], nil
	}
	if len(workspaces) > 1 {
		return store.Workspace{}, newArtifactMaterializeError("artifact is linked to multiple workspaces; provide workspace_id")
	}
	return store.Workspace{}, newArtifactMaterializeError("workspace_id is required to materialize this artifact")
}

func (a *App) ensureMaterializedArtifactLink(workspace store.Workspace, artifact store.Artifact) error {
	if err := a.store.LinkArtifactToWorkspace(workspace.ID, artifact.ID); err != nil && err.Error() != "artifact already belongs to workspace" {
		return err
	}
	return nil
}

func (a *App) materializeArtifact(artifact store.Artifact, req artifactMaterializeRequest) (store.Workspace, store.Artifact, string, error) {
	workspace, err := a.resolveArtifactMaterializeWorkspace(artifact, req.WorkspaceID)
	if err != nil {
		return store.Workspace{}, store.Artifact{}, "", err
	}
	content, err := renderMaterializedArtifact(artifact)
	if err != nil {
		return store.Workspace{}, store.Artifact{}, "", err
	}
	absolutePath, relativePath, err := resolveArtifactMaterializePath(workspace, artifact, req.RelativePath)
	if err != nil {
		return store.Workspace{}, store.Artifact{}, "", err
	}
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
		return store.Workspace{}, store.Artifact{}, "", err
	}
	if err := os.WriteFile(absolutePath, []byte(content), 0o644); err != nil {
		return store.Workspace{}, store.Artifact{}, "", err
	}
	refPath := absolutePath
	if err := a.store.UpdateArtifact(artifact.ID, store.ArtifactUpdate{RefPath: &refPath}); err != nil {
		return store.Workspace{}, store.Artifact{}, "", err
	}
	updated, err := a.store.GetArtifact(artifact.ID)
	if err != nil {
		return store.Workspace{}, store.Artifact{}, "", err
	}
	if err := a.ensureMaterializedArtifactLink(workspace, updated); err != nil {
		return store.Workspace{}, store.Artifact{}, "", err
	}
	return workspace, updated, relativePath, nil
}

func (a *App) handleArtifactMaterialize(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	artifactID, err := parseURLInt64Param(r, "artifact_id")
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req artifactMaterializeRequest
	if err := decodeJSON(r, &req); err != nil && !errors.Is(err, io.EOF) {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	artifact, err := a.store.GetArtifact(artifactID)
	if err != nil {
		writeDomainStoreError(w, err)
		return
	}
	workspace, updated, relativePath, err := a.materializeArtifact(artifact, req)
	if err != nil {
		var requestErr artifactMaterializeError
		switch {
		case errors.As(err, &requestErr):
			writeAPIError(w, http.StatusBadRequest, requestErr.Error())
		case errors.Is(err, sql.ErrNoRows):
			writeDomainStoreError(w, err)
		default:
			writeAPIError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeAPIData(w, http.StatusOK, map[string]any{
		"artifact":      updated,
		"workspace":     workspace,
		"relative_path": relativePath,
	})
}
