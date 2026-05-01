package web

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sloppy-org/slopshell/internal/store"
)

const (
	gtdSetStatusTool = "brain.gtd.set_status"
	gtdParseTool     = "brain.note.parse"
)

type itemGTDStatusRequest struct {
	State     string `json:"state"`
	Status    string `json:"status"`
	ClosedAt  string `json:"closed_at"`
	ClosedVia string `json:"closed_via"`
}

type gtdStatusTarget struct {
	Sphere       string
	Path         string
	CommitmentID string
}

type gtdStatusRoute struct {
	Target           string `json:"target"`
	WriteableBinding bool   `json:"writeable_binding"`
}

func (a *App) handleItemGTDStatusUpdate(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	itemID, err := parseItemIDParam(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req itemGTDStatusRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	item, err := a.store.GetItem(itemID)
	if err != nil {
		writeItemStoreError(w, err)
		return
	}
	state, status, err := normalizeGTDStatus(req)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	target, ok, err := a.gtdStatusTarget(item)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		a.updateLocalGTDStatus(w, r, item, state)
		return
	}
	route, result, err := a.setBrainGTDStatus(target, req, status)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, err.Error())
		return
	}
	if err := a.store.UpdateItemState(itemID, state); err != nil {
		writeItemStoreError(w, err)
		return
	}
	item, err = a.store.GetItem(itemID)
	if err != nil {
		writeItemStoreError(w, err)
		return
	}
	writeAPIData(w, http.StatusOK, map[string]any{
		"item":      item,
		"gtd_route": route,
		"gtd":       result,
	})
}

func (a *App) updateLocalGTDStatus(w http.ResponseWriter, r *http.Request, item store.Item, state string) {
	if err := a.updateItemState(r.Context(), item, state); err != nil {
		writeItemStateUpdateError(w, err)
		return
	}
	item, err := a.store.GetItem(item.ID)
	if err != nil {
		writeItemStoreError(w, err)
		return
	}
	writeAPIData(w, http.StatusOK, map[string]any{"item": item})
}

func (a *App) setBrainGTDStatus(target gtdStatusTarget, req itemGTDStatusRequest, status string) (gtdStatusRoute, map[string]any, error) {
	route, err := a.readGTDStatusRoute(target)
	if err != nil {
		return gtdStatusRoute{}, nil, err
	}
	args := map[string]any{
		"sphere":     target.Sphere,
		"status":     status,
		"closed_at":  closedAtForGTDStatus(req.ClosedAt, status),
		"closed_via": closedViaForGTDStatus(req.ClosedVia, status),
	}
	if target.Path != "" {
		args["path"] = target.Path
	} else {
		args["commitment_id"] = target.CommitmentID
	}
	result, err := a.mcpToolsCall(a.localMCPEndpoint, gtdSetStatusTool, args)
	if err != nil {
		return gtdStatusRoute{}, nil, err
	}
	return route, result, nil
}

func (a *App) readGTDStatusRoute(target gtdStatusTarget) (gtdStatusRoute, error) {
	if target.Path == "" {
		return gtdStatusRoute{Target: "local_overlay"}, nil
	}
	result, err := a.mcpToolsCall(a.localMCPEndpoint, gtdParseTool, map[string]any{
		"sphere": target.Sphere,
		"path":   target.Path,
	})
	if err != nil {
		return gtdStatusRoute{}, err
	}
	commitment, _ := result["commitment"].(map[string]any)
	if hasWriteableGTDBinding(commitment) {
		return gtdStatusRoute{Target: "source_binding", WriteableBinding: true}, nil
	}
	return gtdStatusRoute{Target: "local_overlay"}, nil
}

func hasWriteableGTDBinding(commitment map[string]any) bool {
	if commitment == nil {
		return false
	}
	bindings, _ := commitment["source_bindings"].([]any)
	for _, raw := range bindings {
		binding, _ := raw.(map[string]any)
		if writeable, _ := binding["writeable"].(bool); writeable {
			return true
		}
	}
	return false
}

func (a *App) gtdStatusTarget(item store.Item) (gtdStatusTarget, bool, error) {
	target := gtdStatusTarget{Sphere: item.Sphere}
	if target.Sphere == "" {
		target.Sphere = store.SphereWork
	}
	source := strings.ToLower(strings.TrimSpace(optionalStoreString(item.Source)))
	ref := strings.TrimSpace(optionalStoreString(item.SourceRef))
	if isBrainGTDSource(source) && ref != "" {
		target.Path, target.CommitmentID = splitGTDSourceRef(ref)
		return target, true, nil
	}
	path, err := a.gtdArtifactPath(item)
	if err != nil || path == "" {
		return target, false, err
	}
	target.Path = path
	return target, true, nil
}

func (a *App) gtdArtifactPath(item store.Item) (string, error) {
	if item.ArtifactID == nil {
		return "", nil
	}
	artifact, err := a.store.GetArtifact(*item.ArtifactID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	path := strings.TrimSpace(optionalStoreString(artifact.RefPath))
	if isGTDMarkdownPath(path) {
		return path, nil
	}
	return "", nil
}

func normalizeGTDStatus(req itemGTDStatusRequest) (string, string, error) {
	raw := strings.TrimSpace(firstNonEmptyString(req.State, req.Status))
	if raw == "" {
		return "", "", errors.New("state or status is required")
	}
	status := strings.ToLower(raw)
	switch status {
	case "done", "closed":
		return store.ItemStateDone, "closed", nil
	case store.ItemStateInbox, store.ItemStateNext, store.ItemStateWaiting, store.ItemStateDeferred, store.ItemStateSomeday, store.ItemStateReview:
		return status, status, nil
	default:
		return "", "", fmt.Errorf("unsupported GTD status %q", raw)
	}
}

func closedAtForGTDStatus(value, status string) string {
	clean := strings.TrimSpace(value)
	if clean != "" || status != "closed" {
		return clean
	}
	return time.Now().UTC().Format(time.RFC3339)
}

func closedViaForGTDStatus(value, status string) string {
	clean := strings.TrimSpace(value)
	if clean != "" || status != "closed" {
		return clean
	}
	return "slopshell"
}

func isBrainGTDSource(source string) bool {
	switch source {
	case "markdown", "meetings", "brain", "brain.gtd":
		return true
	default:
		return false
	}
}

func splitGTDSourceRef(ref string) (string, string) {
	ref = strings.TrimSpace(ref)
	if isGTDMarkdownPath(ref) {
		return ref, ""
	}
	return "", ref
}

func isGTDMarkdownPath(path string) bool {
	clean := strings.TrimSpace(path)
	return strings.HasSuffix(strings.ToLower(clean), ".md") && strings.Contains(clean, "/")
}

func optionalStoreString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if clean := strings.TrimSpace(value); clean != "" {
			return clean
		}
	}
	return ""
}
