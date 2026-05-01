package web

import (
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sloppy-org/slopshell/internal/store"
)

type workspaceGraphRequest struct {
	SourcePath   string
	ArtifactID   int64
	ArtifactPath string
	RootID       string
}

type workspaceGraphRoot struct {
	Kind     string
	Source   string
	Node     workspaceLocalGraphNode
	Artifact *store.Artifact
	Item     *store.Item
	Actor    *store.Actor
	Label    *store.Label
}

type errGraph string

func (e errGraph) Error() string {
	return string(e)
}

func parseWorkspaceGraphRequest(r *http.Request) workspaceGraphRequest {
	artifactID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("artifact_id")), 10, 64)
	return workspaceGraphRequest{
		SourcePath:   strings.TrimSpace(r.URL.Query().Get("source")),
		ArtifactID:   artifactID,
		ArtifactPath: strings.TrimSpace(r.URL.Query().Get("artifact_path")),
		RootID:       strings.TrimSpace(r.URL.Query().Get("root")),
	}
}

func resolveWorkspaceGraphRoot(a *App, workspace store.Workspace, req workspaceGraphRequest) (workspaceGraphRoot, error) {
	if req.ArtifactID > 0 {
		return resolveWorkspaceGraphArtifactID(a, workspace, req.ArtifactID)
	}
	if req.RootID != "" {
		return resolveWorkspaceGraphEntityRoot(a, workspace, req.RootID)
	}
	if req.ArtifactPath != "" {
		return resolveWorkspaceGraphArtifactPath(a, workspace, req.ArtifactPath)
	}
	return resolveWorkspaceGraphNoteRoot(workspace, req.SourcePath)
}

func resolveWorkspaceGraphNoteRoot(workspace store.Workspace, sourceRaw string) (workspaceGraphRoot, error) {
	sourceRel, err := normalizeMarkdownSourcePath(sourceRaw)
	if err != nil {
		return workspaceGraphRoot{}, err
	}
	sourceRel = stripBrainPrefixForWorkspace(workspace, sourceRel)
	brainRoot, _, err := brainWorkspaceRoots(workspace)
	if err != nil {
		return workspaceGraphRoot{}, err
	}
	sourceAbs := filepath.Clean(filepath.Join(brainRoot, filepath.FromSlash(sourceRel)))
	if !pathInsideOrEqual(sourceAbs, brainRoot) {
		return workspaceGraphRoot{}, errGraph("source note is outside the brain workspace")
	}
	if err := enforceWorkPersonalPath(sourceAbs); err != nil {
		return workspaceGraphRoot{}, errGraph(workPersonalGuardrailMessage)
	}
	node := workspaceLocalGraphNode{
		ID:      workspaceGraphNoteNodeID(sourceRel),
		Type:    "note",
		Label:   workspaceGraphNodeLabel(sourceRel),
		Path:    sourceRel,
		FileURL: workspaceMarkdownLinkFileURL(workspace, filepath.ToSlash(filepath.Join("brain", sourceRel))),
		Sphere:  workspace.Sphere,
	}
	return workspaceGraphRoot{Kind: "note", Source: sourceRel, Node: node}, nil
}

func resolveWorkspaceGraphArtifactID(a *App, workspace store.Workspace, artifactID int64) (workspaceGraphRoot, error) {
	artifact, err := a.store.GetArtifact(artifactID)
	if err != nil {
		return workspaceGraphRoot{}, err
	}
	if !workspaceGraphArtifactInWorkspace(a, workspace.ID, artifactID) {
		return workspaceGraphRoot{}, errGraph("artifact is not in this workspace")
	}
	node := workspaceGraphArtifactNode(artifact)
	return workspaceGraphRoot{Kind: "artifact", Source: node.ID, Node: node, Artifact: &artifact}, nil
}

func resolveWorkspaceGraphArtifactPath(a *App, workspace store.Workspace, raw string) (workspaceGraphRoot, error) {
	_, vaultRoot, _ := brainWorkspaceRoots(workspace)
	clean := cleanWorkspaceGraphArtifactPath(raw, vaultRoot)
	for _, artifact := range workspaceGraphMatchingArtifacts(a, workspace, clean) {
		return resolveWorkspaceGraphArtifactID(a, workspace, artifact.ID)
	}
	node := workspaceLocalGraphNode{
		ID:     "artifact_path:" + clean,
		Type:   "artifact",
		Label:  workspaceGraphNodeLabel(clean),
		Path:   clean,
		Sphere: workspace.Sphere,
	}
	return workspaceGraphRoot{Kind: "artifact_path", Source: clean, Node: node}, nil
}

func resolveWorkspaceGraphEntityRoot(a *App, workspace store.Workspace, rootID string) (workspaceGraphRoot, error) {
	kind, idText, ok := strings.Cut(strings.TrimSpace(rootID), ":")
	if !ok {
		return workspaceGraphRoot{}, errGraph("graph root must use kind:id")
	}
	id, _ := strconv.ParseInt(idText, 10, 64)
	switch kind {
	case "artifact":
		return resolveWorkspaceGraphArtifactID(a, workspace, id)
	case "item":
		return resolveWorkspaceGraphItemRoot(a, workspace, id)
	case "actor":
		return resolveWorkspaceGraphActorRoot(a, id)
	case "label":
		return resolveWorkspaceGraphLabelRoot(a, id)
	case "source":
		return workspaceGraphSourceRoot(rootID)
	default:
		return workspaceGraphRoot{}, errGraph("unsupported graph root kind")
	}
}

func resolveWorkspaceGraphItemRoot(a *App, workspace store.Workspace, itemID int64) (workspaceGraphRoot, error) {
	item, err := a.store.GetItem(itemID)
	if err != nil {
		return workspaceGraphRoot{}, err
	}
	if !workspaceGraphItemVisibleInWorkspace(workspace, item) {
		return workspaceGraphRoot{}, errGraph("item is not in this workspace")
	}
	node := workspaceGraphItemNode(item)
	return workspaceGraphRoot{Kind: "item", Source: node.ID, Node: node, Item: &item}, nil
}

func workspaceGraphItemVisibleInWorkspace(workspace store.Workspace, item store.Item) bool {
	if item.WorkspaceID != nil {
		return *item.WorkspaceID == workspace.ID && strings.EqualFold(item.Sphere, workspace.Sphere)
	}
	return strings.EqualFold(item.Sphere, workspace.Sphere)
}

func resolveWorkspaceGraphActorRoot(a *App, actorID int64) (workspaceGraphRoot, error) {
	actor, err := a.store.GetActor(actorID)
	if err != nil {
		return workspaceGraphRoot{}, err
	}
	node := workspaceLocalGraphNode{ID: "actor:" + workspaceIDStr(actor.ID), Type: "actor", Label: actor.Name}
	return workspaceGraphRoot{Kind: "actor", Source: node.ID, Node: node, Actor: &actor}, nil
}

func resolveWorkspaceGraphLabelRoot(a *App, labelID int64) (workspaceGraphRoot, error) {
	label, err := a.store.GetLabel(labelID)
	if err != nil {
		return workspaceGraphRoot{}, err
	}
	node := workspaceLocalGraphNode{ID: "label:" + workspaceIDStr(label.ID), Type: "label", Label: label.Name}
	return workspaceGraphRoot{Kind: "label", Source: node.ID, Node: node, Label: &label}, nil
}

func workspaceGraphSourceRoot(rootID string) (workspaceGraphRoot, error) {
	parts := strings.Split(strings.TrimSpace(rootID), ":")
	if len(parts) < 3 {
		return workspaceGraphRoot{}, errGraph("source graph root must include provider and remote id")
	}
	node := workspaceLocalGraphNode{
		ID:     rootID,
		Type:   "source",
		Label:  parts[len(parts)-1],
		Source: parts[1],
	}
	return workspaceGraphRoot{Kind: "source", Source: rootID, Node: node}, nil
}

func appendWorkspaceGraphRoot(a *App, builder *workspaceGraphBuilder, root workspaceGraphRoot) {
	switch root.Kind {
	case "note":
		appendWorkspaceGraphMarkdown(builder, root.Node.Path, root.Node.ID)
		appendWorkspaceGraphStoreMetadata(a, builder, root.Node.Path, root.Node.ID)
	case "artifact":
		appendWorkspaceGraphArtifactRoot(a, builder, *root.Artifact, root.Node.ID)
	case "item":
		workspaceGraphAddItem(a, builder, *root.Item)
	case "actor":
		appendWorkspaceGraphActorRoot(a, builder, *root.Actor)
	case "label":
		appendWorkspaceGraphLabelRoot(a, builder, *root.Label)
	case "source":
		appendWorkspaceGraphSourceRoot(a, builder, root.Node)
	}
}

func appendWorkspaceGraphArtifactRoot(a *App, builder *workspaceGraphBuilder, artifact store.Artifact, artifactID string) {
	workspaceGraphAddArtifactNoteLink(builder, artifact, artifactID)
	workspaceGraphAddArtifactBindings(a, builder, artifact, artifactID)
	for _, item := range workspaceGraphItems(a, builder.workspace.Sphere) {
		if item.ArtifactID == nil || *item.ArtifactID != artifact.ID {
			continue
		}
		workspaceGraphAddItem(a, builder, item)
	}
}

func workspaceGraphAddArtifactNoteLink(builder *workspaceGraphBuilder, artifact store.Artifact, artifactID string) {
	path := workspaceGraphArtifactNotePath(builder.workspace, artifact)
	if path == "" {
		return
	}
	noteID := workspaceGraphNoteNodeID(path)
	builder.addNode(workspaceLocalGraphNode{
		ID:      noteID,
		Type:    workspaceGraphNodeType(path, string(artifact.Kind)),
		Label:   workspaceGraphNodeLabel(path),
		Path:    path,
		FileURL: workspaceMarkdownLinkFileURL(builder.workspace, filepath.ToSlash(filepath.Join("brain", path))),
		Sphere:  builder.workspace.Sphere,
	})
	if graphRelationEnabled(builder.filter, "artifact") {
		builder.addEdge(workspaceLocalGraphEdge{
			ID:       artifactID + "->" + noteID + ":artifact",
			Source:   artifactID,
			Target:   noteID,
			Relation: "artifact",
			Label:    string(artifact.Kind),
			Sphere:   builder.workspace.Sphere,
		})
	}
}

func workspaceGraphArtifactNotePath(workspace store.Workspace, artifact store.Artifact) string {
	_, vaultRoot, _ := brainWorkspaceRoots(workspace)
	clean := cleanWorkspaceGraphArtifactPath(stringPointerValue(artifact.RefPath), vaultRoot)
	if clean == "" {
		return ""
	}
	return stripBrainPrefixForWorkspace(workspace, clean)
}

func appendWorkspaceGraphActorRoot(a *App, builder *workspaceGraphBuilder, actor store.Actor) {
	for _, item := range workspaceGraphItems(a, builder.workspace.Sphere) {
		if item.ActorID == nil || *item.ActorID != actor.ID {
			continue
		}
		workspaceGraphAddItem(a, builder, item)
	}
}

func appendWorkspaceGraphLabelRoot(a *App, builder *workspaceGraphBuilder, label store.Label) {
	labelID := label.ID
	items, err := a.store.ListItemsFiltered(store.ItemListFilter{Sphere: builder.workspace.Sphere, LabelID: &labelID})
	if err != nil {
		return
	}
	for _, item := range items {
		workspaceGraphAddItem(a, builder, item)
	}
}

func appendWorkspaceGraphSourceRoot(a *App, builder *workspaceGraphBuilder, source workspaceLocalGraphNode) {
	for _, item := range workspaceGraphItems(a, builder.workspace.Sphere) {
		if !workspaceGraphItemMatchesSourceNode(item, source) {
			continue
		}
		workspaceGraphAddItem(a, builder, item)
	}
}

func workspaceGraphArtifactInWorkspace(a *App, workspaceID, artifactID int64) bool {
	artifacts, err := a.store.ListArtifactsForWorkspace(workspaceID)
	if err != nil {
		return false
	}
	for _, artifact := range artifacts {
		if artifact.ID == artifactID {
			return true
		}
	}
	return false
}
