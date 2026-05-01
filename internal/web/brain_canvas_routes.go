package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/sloppy-org/slopshell/internal/store"
)

type brainCanvasCardCreateRequest struct {
	Binding brainCanvasBinding `json:"binding"`
	X       float64            `json:"x"`
	Y       float64            `json:"y"`
	Width   float64            `json:"width"`
	Height  float64            `json:"height"`
	Color   string             `json:"color,omitempty"`
}

type brainCanvasCardPatchRequest struct {
	X      *float64 `json:"x,omitempty"`
	Y      *float64 `json:"y,omitempty"`
	Width  *float64 `json:"width,omitempty"`
	Height *float64 `json:"height,omitempty"`
	Color  *string  `json:"color,omitempty"`
	Title  *string  `json:"title,omitempty"`
	Body   *string  `json:"body,omitempty"`
}

type brainCanvasCardOpen struct {
	OK      bool               `json:"ok"`
	Kind    string             `json:"kind"`
	OpenURL string             `json:"open_url,omitempty"`
	Title   string             `json:"title,omitempty"`
	Body    string             `json:"body,omitempty"`
	Binding brainCanvasBinding `json:"binding"`
	Error   string             `json:"error,omitempty"`
}

const brainCanvasCardLimit = 1000

func (a *App) loadBrainCanvas(workspace store.Workspace, name string) (brainCanvasView, error) {
	resolver, err := newBrainCanvasResolver(a.store, workspace)
	if err != nil {
		return brainCanvasView{}, err
	}
	canvasPath, err := brainCanvasFilePath(resolver.brainRoot, name)
	if err != nil {
		return brainCanvasView{}, err
	}
	doc, err := loadBrainCanvasDocument(canvasPath)
	if err != nil {
		return brainCanvasView{}, err
	}
	view := brainCanvasView{
		OK:    true,
		Name:  normalizeBrainCanvasName(name),
		Path:  brainCanvasVaultRelative(resolver, canvasPath),
		Cards: make([]brainCanvasCardView, 0, len(doc.Nodes)),
		Edges: append([]brainCanvasEdge{}, doc.Edges...),
	}
	for _, node := range doc.Nodes {
		view.Cards = append(view.Cards, resolver.resolveCardView(node))
	}
	sortBrainCanvasCardViews(view.Cards)
	return view, nil
}

func brainCanvasVaultRelative(resolver brainCanvasResolver, canvasPath string) string {
	rel, err := filepath.Rel(resolver.vaultRoot, canvasPath)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(rel))
}

func (a *App) addBrainCanvasCard(workspace store.Workspace, name string, req brainCanvasCardCreateRequest) (brainCanvasCardView, error) {
	resolver, err := newBrainCanvasResolver(a.store, workspace)
	if err != nil {
		return brainCanvasCardView{}, err
	}
	binding, err := normalizeBrainCanvasBinding(req.Binding)
	if err != nil {
		return brainCanvasCardView{}, err
	}
	if err := assertBrainCanvasBindingExists(resolver, binding); err != nil {
		return brainCanvasCardView{}, err
	}
	canvasPath, err := brainCanvasFilePath(resolver.brainRoot, name)
	if err != nil {
		return brainCanvasCardView{}, err
	}
	doc, err := loadBrainCanvasDocument(canvasPath)
	if err != nil {
		return brainCanvasCardView{}, err
	}
	if len(doc.Nodes) >= brainCanvasCardLimit {
		return brainCanvasCardView{}, fmt.Errorf("canvas card limit reached (%d)", brainCanvasCardLimit)
	}
	for _, existing := range doc.Nodes {
		if bindingsEqual(existing.Binding, binding) {
			return brainCanvasCardView{}, errors.New("card for this binding already exists")
		}
	}
	node := brainCanvasNode{
		ID:      newBrainCanvasNodeID(),
		Type:    brainCanvasNodeTypeForKind(binding.Kind),
		X:       clampBrainCanvasCoordinate(req.X),
		Y:       clampBrainCanvasCoordinate(req.Y),
		Width:   clampBrainCanvasDimension(req.Width, brainCanvasDefaultWidth),
		Height:  clampBrainCanvasDimension(req.Height, brainCanvasDefaultHeight),
		Color:   strings.TrimSpace(req.Color),
		Binding: binding,
	}
	if binding.Kind == "note" {
		node.File = brainCanvasNodeFile(resolver, binding.Path)
	}
	if binding.Kind == "link" {
		node.URL = binding.URL
	}
	doc.Nodes = append(doc.Nodes, node)
	if err := writeBrainCanvasDocument(canvasPath, doc); err != nil {
		return brainCanvasCardView{}, err
	}
	return resolver.resolveCardView(node), nil
}

func brainCanvasNodeFile(resolver brainCanvasResolver, rel string) string {
	abs := resolver.resolveBrainPath(rel)
	if abs == "" {
		return rel
	}
	if vaultRel := resolver.vaultRelative(abs); vaultRel != "" {
		return vaultRel
	}
	return rel
}

func assertBrainCanvasBindingExists(resolver brainCanvasResolver, binding brainCanvasBinding) error {
	switch binding.Kind {
	case "artifact":
		_, err := resolver.store.GetArtifact(binding.ID)
		if err != nil {
			return errors.New("artifact not found")
		}
	case "item":
		_, err := resolver.store.GetItem(binding.ID)
		if err != nil {
			return errors.New("item not found")
		}
	case "actor":
		_, err := resolver.store.GetActor(binding.ID)
		if err != nil {
			return errors.New("actor not found")
		}
	case "label":
		_, err := resolver.store.GetLabel(binding.ID)
		if err != nil {
			return errors.New("label not found")
		}
	case "note":
		abs := resolver.resolveBrainPath(binding.Path)
		if abs == "" {
			return errors.New("note path is outside the vault")
		}
		if err := enforceWorkPersonalPath(abs); err != nil {
			return errors.New(workPersonalGuardrailMessage)
		}
		if err := requireBrainCanvasBackingFile(resolver, abs); err != nil {
			return err
		}
	}
	return nil
}

func requireBrainCanvasBackingFile(resolver brainCanvasResolver, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return errors.New("note not found")
		}
		return err
	}
	if info.IsDir() {
		return errors.New("note path is a directory")
	}
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	if !pathInsideOrEqual(realPath, resolver.brainRoot) {
		return errors.New("note path is outside the vault")
	}
	if err := enforceWorkPersonalPath(realPath); err != nil {
		return errors.New(workPersonalGuardrailMessage)
	}
	return nil
}

func (a *App) patchBrainCanvasCard(workspace store.Workspace, name, nodeID string, req brainCanvasCardPatchRequest) (brainCanvasCardView, error) {
	resolver, err := newBrainCanvasResolver(a.store, workspace)
	if err != nil {
		return brainCanvasCardView{}, err
	}
	canvasPath, err := brainCanvasFilePath(resolver.brainRoot, name)
	if err != nil {
		return brainCanvasCardView{}, err
	}
	doc, err := loadBrainCanvasDocument(canvasPath)
	if err != nil {
		return brainCanvasCardView{}, err
	}
	idx := indexOfBrainCanvasNode(doc.Nodes, nodeID)
	if idx < 0 {
		return brainCanvasCardView{}, errors.New("card not found")
	}
	node := doc.Nodes[idx]
	applyBrainCanvasLayoutPatch(&node, req)
	if err := applyBrainCanvasSemanticPatch(resolver, &node, req); err != nil {
		return brainCanvasCardView{}, err
	}
	doc.Nodes[idx] = node
	if err := writeBrainCanvasDocument(canvasPath, doc); err != nil {
		return brainCanvasCardView{}, err
	}
	return resolver.resolveCardView(node), nil
}

func indexOfBrainCanvasNode(nodes []brainCanvasNode, nodeID string) int {
	for i, node := range nodes {
		if node.ID == nodeID {
			return i
		}
	}
	return -1
}

func applyBrainCanvasLayoutPatch(node *brainCanvasNode, req brainCanvasCardPatchRequest) {
	if req.X != nil {
		node.X = clampBrainCanvasCoordinate(*req.X)
	}
	if req.Y != nil {
		node.Y = clampBrainCanvasCoordinate(*req.Y)
	}
	if req.Width != nil {
		node.Width = clampBrainCanvasDimension(*req.Width, brainCanvasDefaultWidth)
	}
	if req.Height != nil {
		node.Height = clampBrainCanvasDimension(*req.Height, brainCanvasDefaultHeight)
	}
	if req.Color != nil {
		node.Color = strings.TrimSpace(*req.Color)
	}
}

func applyBrainCanvasSemanticPatch(resolver brainCanvasResolver, node *brainCanvasNode, req brainCanvasCardPatchRequest) error {
	if req.Title == nil && req.Body == nil {
		return nil
	}
	switch node.Binding.Kind {
	case "artifact":
		if req.Body != nil {
			return errors.New("body edits are not supported for artifact cards")
		}
		return updateArtifactTitleFromCanvas(resolver, node.Binding.ID, req.Title)
	case "item":
		if req.Body != nil {
			return errors.New("body edits are not supported for item cards")
		}
		return updateItemTitleFromCanvas(resolver, node.Binding.ID, req.Title)
	case "note":
		if req.Title != nil {
			return errors.New("title edits are not supported for note cards")
		}
		return updateNoteFromCanvas(resolver, node.Binding.Path, req.Body)
	case "link":
		if req.Body != nil {
			return errors.New("body edits are not supported for link cards")
		}
		if req.Title != nil {
			node.Label = strings.TrimSpace(*req.Title)
		}
		return nil
	}
	if req.Title != nil || req.Body != nil {
		return fmt.Errorf("semantic edits are not supported for %s cards", node.Binding.Kind)
	}
	return nil
}

func updateArtifactTitleFromCanvas(resolver brainCanvasResolver, id int64, title *string) error {
	if title == nil {
		return nil
	}
	clean := strings.TrimSpace(*title)
	if clean == "" {
		return errors.New("artifact title must not be empty")
	}
	return resolver.store.UpdateArtifact(id, store.ArtifactUpdate{Title: &clean})
}

func updateItemTitleFromCanvas(resolver brainCanvasResolver, id int64, title *string) error {
	if title == nil {
		return nil
	}
	clean := strings.TrimSpace(*title)
	if clean == "" {
		return errors.New("item title must not be empty")
	}
	return resolver.store.UpdateItem(id, store.ItemUpdate{Title: &clean})
}

func updateNoteFromCanvas(resolver brainCanvasResolver, rel string, body *string) error {
	if body == nil {
		return nil
	}
	abs := resolver.resolveBrainPath(rel)
	if abs == "" {
		return errors.New("note path is outside the vault")
	}
	if err := enforceWorkPersonalPath(abs); err != nil {
		return errors.New(workPersonalGuardrailMessage)
	}
	if err := requireBrainCanvasBackingFile(resolver, abs); err != nil {
		return err
	}
	return os.WriteFile(abs, []byte(*body), 0o644)
}

func (a *App) deleteBrainCanvasCard(workspace store.Workspace, name, nodeID string) error {
	resolver, err := newBrainCanvasResolver(a.store, workspace)
	if err != nil {
		return err
	}
	canvasPath, err := brainCanvasFilePath(resolver.brainRoot, name)
	if err != nil {
		return err
	}
	doc, err := loadBrainCanvasDocument(canvasPath)
	if err != nil {
		return err
	}
	idx := indexOfBrainCanvasNode(doc.Nodes, nodeID)
	if idx < 0 {
		return errors.New("card not found")
	}
	doc.Nodes = append(doc.Nodes[:idx], doc.Nodes[idx+1:]...)
	return writeBrainCanvasDocument(canvasPath, doc)
}

func (a *App) openBrainCanvasCard(workspace store.Workspace, name, nodeID string) (brainCanvasCardOpen, error) {
	resolver, err := newBrainCanvasResolver(a.store, workspace)
	if err != nil {
		return brainCanvasCardOpen{}, err
	}
	canvasPath, err := brainCanvasFilePath(resolver.brainRoot, name)
	if err != nil {
		return brainCanvasCardOpen{}, err
	}
	doc, err := loadBrainCanvasDocument(canvasPath)
	if err != nil {
		return brainCanvasCardOpen{}, err
	}
	idx := indexOfBrainCanvasNode(doc.Nodes, nodeID)
	if idx < 0 {
		return brainCanvasCardOpen{}, errors.New("card not found")
	}
	view := resolver.resolveCardView(doc.Nodes[idx])
	open := brainCanvasCardOpen{
		OK:      !view.Stale,
		Kind:    view.Binding.Kind,
		OpenURL: view.OpenURL,
		Title:   view.Title,
		Body:    view.Body,
		Binding: view.Binding,
	}
	if view.Stale {
		open.Error = view.Reason
	}
	return open, nil
}

func (a *App) brainCanvasWorkspace(w http.ResponseWriter, r *http.Request) (store.Workspace, bool) {
	workspace, err := a.resolveRuntimeWorkspaceByIDOrActive(chi.URLParam(r, "workspace_id"))
	if err != nil {
		if isNoRows(err) {
			http.Error(w, "workspace not found", http.StatusNotFound)
			return store.Workspace{}, false
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return store.Workspace{}, false
	}
	if err := enforceWorkPersonalWorkspace(workspace); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return store.Workspace{}, false
	}
	return workspace, true
}

func brainCanvasNameFromQuery(r *http.Request) string {
	return r.URL.Query().Get("name")
}

func (a *App) handleBrainCanvasGet(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	workspace, ok := a.brainCanvasWorkspace(w, r)
	if !ok {
		return
	}
	view, err := a.loadBrainCanvas(workspace, brainCanvasNameFromQuery(r))
	if err != nil {
		writeJSON(w, brainCanvasView{Error: err.Error()})
		return
	}
	writeJSON(w, view)
}

func (a *App) handleBrainCanvasCardCreate(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	workspace, ok := a.brainCanvasWorkspace(w, r)
	if !ok {
		return
	}
	var req brainCanvasCardCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid card payload", http.StatusBadRequest)
		return
	}
	view, err := a.addBrainCanvasCard(workspace, brainCanvasNameFromQuery(r), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSONStatus(w, http.StatusCreated, view)
}

func (a *App) handleBrainCanvasCardPatch(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	workspace, ok := a.brainCanvasWorkspace(w, r)
	if !ok {
		return
	}
	var req brainCanvasCardPatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid card patch", http.StatusBadRequest)
		return
	}
	view, err := a.patchBrainCanvasCard(workspace, brainCanvasNameFromQuery(r), chi.URLParam(r, "node_id"), req)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, view)
}

func (a *App) handleBrainCanvasCardDelete(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	workspace, ok := a.brainCanvasWorkspace(w, r)
	if !ok {
		return
	}
	if err := a.deleteBrainCanvasCard(workspace, brainCanvasNameFromQuery(r), chi.URLParam(r, "node_id")); err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleBrainCanvasCardOpen(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	workspace, ok := a.brainCanvasWorkspace(w, r)
	if !ok {
		return
	}
	open, err := a.openBrainCanvasCard(workspace, brainCanvasNameFromQuery(r), chi.URLParam(r, "node_id"))
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, open)
}
