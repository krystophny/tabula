package web

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sloppy-org/slopshell/internal/store"
)

// brainCanvasNode is a single card in a brain canvas. The schema mirrors the
// JSON Canvas v1.0 spec (jsoncanvas.org) so the file is round-trippable with
// Obsidian Canvas, but adds an explicit `binding` field that tracks the
// Slopshell object the card represents. Layout fields (X/Y/Width/Height) are
// the only durable canvas-only state; semantic fields live in the binding's
// backing source and are loaded on read.
type brainCanvasNode struct {
	ID      string             `json:"id"`
	Type    string             `json:"type"`
	X       float64            `json:"x"`
	Y       float64            `json:"y"`
	Width   float64            `json:"width"`
	Height  float64            `json:"height"`
	Color   string             `json:"color,omitempty"`
	Label   string             `json:"label,omitempty"`
	Text    string             `json:"text,omitempty"`
	File    string             `json:"file,omitempty"`
	URL     string             `json:"url,omitempty"`
	Binding brainCanvasBinding `json:"binding"`
}

// brainCanvasBinding describes the canonical Slopshell object the card backs.
// Kind values: artifact, item, actor, label, note, source, link.
type brainCanvasBinding struct {
	Kind     string `json:"kind"`
	ID       int64  `json:"id,omitempty"`
	Path     string `json:"path,omitempty"`
	URL      string `json:"url,omitempty"`
	Provider string `json:"provider,omitempty"`
	Ref      string `json:"ref,omitempty"`
}

// brainCanvasEdge is reserved for future typed-relation promotion (#738) and
// kept here so the on-disk format already conforms to JSON Canvas.
type brainCanvasEdge struct {
	ID       string `json:"id"`
	From     string `json:"fromNode"`
	To       string `json:"toNode"`
	Label    string `json:"label,omitempty"`
	Relation string `json:"relation,omitempty"`
}

type brainCanvasDocument struct {
	Nodes []brainCanvasNode `json:"nodes"`
	Edges []brainCanvasEdge `json:"edges"`
}

// brainCanvasView is the rendered canvas exposed via the API. Layout fields
// reflect the durable on-disk state; Title/Body/OpenURL are derived from the
// backing object on every read so the canvas stays in sync with the source.
type brainCanvasView struct {
	OK    bool                  `json:"ok"`
	Name  string                `json:"name"`
	Path  string                `json:"path,omitempty"`
	Cards []brainCanvasCardView `json:"cards"`
	Edges []brainCanvasEdge     `json:"edges"`
	Error string                `json:"error,omitempty"`
}

type brainCanvasCardView struct {
	ID      string             `json:"id"`
	Type    string             `json:"type"`
	X       float64            `json:"x"`
	Y       float64            `json:"y"`
	Width   float64            `json:"width"`
	Height  float64            `json:"height"`
	Color   string             `json:"color,omitempty"`
	Title   string             `json:"title"`
	Body    string             `json:"body,omitempty"`
	OpenURL string             `json:"open_url,omitempty"`
	Stale   bool               `json:"stale,omitempty"`
	Reason  string             `json:"reason,omitempty"`
	Binding brainCanvasBinding `json:"binding"`
}

const (
	brainCanvasDirName        = "canvas"
	brainCanvasFileExt        = ".canvas"
	brainCanvasDefaultName    = "default"
	brainCanvasDefaultWidth   = 280.0
	brainCanvasDefaultHeight  = 160.0
	brainCanvasMinDimension   = 40.0
	brainCanvasMaxDimension   = 4000.0
	brainCanvasMaxCoordinate  = 100000.0
	brainCanvasNodeIDByteSize = 8
)

func brainCanvasFilePath(brainRoot, name string) (string, error) {
	clean := normalizeBrainCanvasName(name)
	root := absoluteCleanPath(brainRoot)
	if root == "" {
		return "", errors.New("brain root is required")
	}
	return filepath.Join(root, brainCanvasDirName, clean+brainCanvasFileExt), nil
}

func normalizeBrainCanvasName(raw string) string {
	clean := strings.TrimSpace(raw)
	if clean == "" {
		return brainCanvasDefaultName
	}
	clean = strings.ToLower(clean)
	out := make([]byte, 0, len(clean))
	for i := 0; i < len(clean); i++ {
		c := clean[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '_':
			out = append(out, c)
		default:
			out = append(out, '-')
		}
	}
	trimmed := strings.Trim(string(out), "-_")
	if trimmed == "" {
		return brainCanvasDefaultName
	}
	return trimmed
}

func loadBrainCanvasDocument(path string) (brainCanvasDocument, error) {
	doc := brainCanvasDocument{Nodes: []brainCanvasNode{}, Edges: []brainCanvasEdge{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return doc, nil
		}
		return doc, err
	}
	if len(data) == 0 {
		return doc, nil
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return brainCanvasDocument{}, err
	}
	if doc.Nodes == nil {
		doc.Nodes = []brainCanvasNode{}
	}
	if doc.Edges == nil {
		doc.Edges = []brainCanvasEdge{}
	}
	return doc, nil
}

func writeBrainCanvasDocument(path string, doc brainCanvasDocument) error {
	if doc.Nodes == nil {
		doc.Nodes = []brainCanvasNode{}
	}
	if doc.Edges == nil {
		doc.Edges = []brainCanvasEdge{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func newBrainCanvasNodeID() string {
	buf := make([]byte, brainCanvasNodeIDByteSize)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return "card-" + hex.EncodeToString(buf)
}

func clampBrainCanvasDimension(value, fallback float64) float64 {
	if value <= 0 {
		return fallback
	}
	if value < brainCanvasMinDimension {
		return brainCanvasMinDimension
	}
	if value > brainCanvasMaxDimension {
		return brainCanvasMaxDimension
	}
	return value
}

func clampBrainCanvasCoordinate(value float64) float64 {
	if value < -brainCanvasMaxCoordinate {
		return -brainCanvasMaxCoordinate
	}
	if value > brainCanvasMaxCoordinate {
		return brainCanvasMaxCoordinate
	}
	return value
}

func bindingsEqual(a, b brainCanvasBinding) bool {
	return a.Kind == b.Kind && a.ID == b.ID && a.Path == b.Path && a.URL == b.URL && a.Provider == b.Provider && a.Ref == b.Ref
}

func normalizeBrainCanvasBinding(raw brainCanvasBinding) (brainCanvasBinding, error) {
	out := brainCanvasBinding{
		Kind:     strings.ToLower(strings.TrimSpace(raw.Kind)),
		ID:       raw.ID,
		Path:     strings.TrimSpace(strings.ReplaceAll(raw.Path, "\\", "/")),
		URL:      strings.TrimSpace(raw.URL),
		Provider: strings.ToLower(strings.TrimSpace(raw.Provider)),
		Ref:      strings.TrimSpace(raw.Ref),
	}
	switch out.Kind {
	case "artifact", "item", "actor", "label":
		if out.ID <= 0 {
			return out, fmt.Errorf("%s binding requires a positive id", out.Kind)
		}
	case "note":
		if out.Path == "" {
			return out, errors.New("note binding requires path")
		}
		out.Path = strings.TrimPrefix(out.Path, "/")
	case "link":
		if out.URL == "" {
			return out, errors.New("link binding requires url")
		}
		if parsed, err := url.Parse(out.URL); err != nil || parsed.Scheme == "" {
			return out, errors.New("link binding requires absolute url")
		}
	case "source":
		if out.Provider == "" || out.Ref == "" {
			return out, errors.New("source binding requires provider and ref")
		}
	default:
		return out, fmt.Errorf("unsupported binding kind %q", raw.Kind)
	}
	return out, nil
}

func brainCanvasNodeTypeForKind(kind string) string {
	switch kind {
	case "note":
		return "file"
	case "link":
		return "link"
	case "artifact", "item", "actor", "label", "source":
		return "text"
	}
	return "text"
}

// brainCanvasResolver loads backing-object data for cards. It is decoupled
// from the HTTP layer so unit tests can drive it directly.
type brainCanvasResolver struct {
	store     *store.Store
	workspace store.Workspace
	brainRoot string
	vaultRoot string
}

func newBrainCanvasResolver(s *store.Store, workspace store.Workspace) (brainCanvasResolver, error) {
	brainRoot, vaultRoot, err := brainWorkspaceRoots(workspace)
	if err != nil {
		return brainCanvasResolver{}, err
	}
	return brainCanvasResolver{store: s, workspace: workspace, brainRoot: brainRoot, vaultRoot: vaultRoot}, nil
}

func (r brainCanvasResolver) resolveCardView(node brainCanvasNode) brainCanvasCardView {
	view := brainCanvasCardView{
		ID:      node.ID,
		Type:    node.Type,
		X:       node.X,
		Y:       node.Y,
		Width:   node.Width,
		Height:  node.Height,
		Color:   node.Color,
		Binding: node.Binding,
		Title:   strings.TrimSpace(node.Label),
		Body:    node.Text,
	}
	switch node.Binding.Kind {
	case "artifact":
		r.fillArtifactView(&view)
	case "item":
		r.fillItemView(&view)
	case "actor":
		r.fillActorView(&view)
	case "label":
		r.fillLabelView(&view)
	case "note":
		r.fillNoteView(&view, node)
	case "link":
		r.fillLinkView(&view, node)
	case "source":
		r.fillSourceView(&view)
	default:
		view.Stale = true
		view.Reason = "unknown binding"
	}
	if strings.TrimSpace(view.Title) == "" {
		view.Title = brainCanvasFallbackTitle(node)
	}
	return view
}

func (r brainCanvasResolver) fillArtifactView(view *brainCanvasCardView) {
	artifact, err := r.store.GetArtifact(view.Binding.ID)
	if err != nil {
		view.Stale = true
		view.Reason = "artifact not found"
		return
	}
	if title := stringPointerValue(artifact.Title); title != "" {
		view.Title = title
	} else if path := stringPointerValue(artifact.RefPath); path != "" {
		view.Title = path
	}
	if path := stringPointerValue(artifact.RefPath); path != "" {
		if abs := r.resolveBrainPath(path); abs != "" {
			rel := r.vaultRelative(abs)
			if rel != "" {
				view.OpenURL = workspaceMarkdownLinkFileURL(r.workspace, rel)
			}
		}
	}
	if view.Body == "" {
		view.Body = string(artifact.Kind)
	}
}

func (r brainCanvasResolver) fillItemView(view *brainCanvasCardView) {
	item, err := r.store.GetItem(view.Binding.ID)
	if err != nil {
		view.Stale = true
		view.Reason = "item not found"
		return
	}
	view.Title = item.Title
	if view.Body == "" {
		view.Body = strings.TrimSpace(strings.Join([]string{item.Kind, item.State}, " · "))
	}
}

func (r brainCanvasResolver) fillActorView(view *brainCanvasCardView) {
	actor, err := r.store.GetActor(view.Binding.ID)
	if err != nil {
		view.Stale = true
		view.Reason = "actor not found"
		return
	}
	view.Title = actor.Name
	if view.Body == "" {
		view.Body = actor.Kind
	}
}

func (r brainCanvasResolver) fillLabelView(view *brainCanvasCardView) {
	label, err := r.store.GetLabel(view.Binding.ID)
	if err != nil {
		view.Stale = true
		view.Reason = "label not found"
		return
	}
	view.Title = label.Name
}

func (r brainCanvasResolver) fillNoteView(view *brainCanvasCardView, node brainCanvasNode) {
	rel := strings.TrimSpace(node.Binding.Path)
	if rel == "" {
		rel = strings.TrimSpace(node.File)
	}
	if rel == "" {
		view.Stale = true
		view.Reason = "note path missing"
		return
	}
	abs := r.resolveBrainPath(rel)
	if abs == "" {
		view.Stale = true
		view.Reason = "note outside vault"
		return
	}
	if err := enforceWorkPersonalPath(abs); err != nil {
		view.Stale = true
		view.Reason = workPersonalGuardrailMessage
		return
	}
	view.Title = brainCanvasNoteTitle(rel)
	if data, err := os.ReadFile(abs); err == nil {
		view.Body = string(data)
	} else if os.IsNotExist(err) {
		view.Stale = true
		view.Reason = "note not found"
	}
	vaultRel := r.vaultRelative(abs)
	if vaultRel != "" {
		view.OpenURL = workspaceMarkdownLinkFileURL(r.workspace, vaultRel)
	}
}

func (r brainCanvasResolver) fillLinkView(view *brainCanvasCardView, node brainCanvasNode) {
	urlStr := strings.TrimSpace(node.Binding.URL)
	if urlStr == "" {
		urlStr = strings.TrimSpace(node.URL)
	}
	if urlStr == "" {
		view.Stale = true
		view.Reason = "link url missing"
		return
	}
	if view.Title == "" {
		view.Title = urlStr
	}
	view.OpenURL = urlStr
}

func (r brainCanvasResolver) fillSourceView(view *brainCanvasCardView) {
	if view.Title == "" {
		view.Title = view.Binding.Provider + ":" + view.Binding.Ref
	}
}

func (r brainCanvasResolver) resolveBrainPath(rel string) string {
	clean := strings.TrimPrefix(strings.ReplaceAll(strings.TrimSpace(rel), "\\", "/"), "/")
	if clean == "" {
		return ""
	}
	root := r.brainRoot
	if strings.HasPrefix(clean, "brain/") {
		root = r.vaultRoot
	}
	abs := filepath.Clean(filepath.Join(root, filepath.FromSlash(clean)))
	if !pathInsideOrEqual(abs, r.brainRoot) {
		return ""
	}
	return abs
}

func (r brainCanvasResolver) vaultRelative(abs string) string {
	if !pathInsideOrEqual(abs, r.vaultRoot) {
		return ""
	}
	rel, err := filepath.Rel(r.vaultRoot, abs)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(rel))
}

func brainCanvasNoteTitle(rel string) string {
	base := filepath.Base(strings.ReplaceAll(rel, "\\", "/"))
	if ext := filepath.Ext(base); ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	return base
}

func brainCanvasFallbackTitle(node brainCanvasNode) string {
	if title := strings.TrimSpace(node.Label); title != "" {
		return title
	}
	switch node.Binding.Kind {
	case "artifact", "item", "actor", "label":
		return fmt.Sprintf("%s #%d", node.Binding.Kind, node.Binding.ID)
	case "note":
		return brainCanvasNoteTitle(node.Binding.Path)
	case "link":
		if node.Binding.URL != "" {
			return node.Binding.URL
		}
		return node.URL
	case "source":
		return node.Binding.Provider + ":" + node.Binding.Ref
	}
	return node.ID
}

func sortBrainCanvasCardViews(cards []brainCanvasCardView) {
	sort.SliceStable(cards, func(i, j int) bool {
		if cards[i].Y != cards[j].Y {
			return cards[i].Y < cards[j].Y
		}
		if cards[i].X != cards[j].X {
			return cards[i].X < cards[j].X
		}
		return cards[i].ID < cards[j].ID
	})
}
