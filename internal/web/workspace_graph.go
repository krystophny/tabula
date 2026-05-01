package web

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/sloppy-org/slopshell/internal/store"
)

func parseWorkspaceGraphFilter(r *http.Request, workspace store.Workspace) workspaceGraphFilter {
	filter := workspaceGraphFilter{
		Relations: map[string]bool{},
		Source:    strings.TrimSpace(r.URL.Query().Get("source_filter")),
		Label:     strings.TrimSpace(r.URL.Query().Get("label")),
		Sphere:    strings.TrimSpace(r.URL.Query().Get("sphere")),
	}
	for _, relation := range r.URL.Query()["relation"] {
		clean := strings.TrimSpace(relation)
		if clean != "" {
			filter.Relations[clean] = true
		}
	}
	if filter.Sphere == "" {
		filter.Sphere = workspace.Sphere
	}
	return filter
}

func buildWorkspaceLocalGraphForRequest(a *App, workspace store.Workspace, req workspaceGraphRequest, filter workspaceGraphFilter) workspaceLocalGraph {
	root, err := resolveWorkspaceGraphRoot(a, workspace, req)
	if err != nil {
		return workspaceLocalGraph{Error: err.Error()}
	}
	builder := newWorkspaceGraphBuilder(workspace, filter, root.Source, root.Node.ID)
	builder.addNode(root.Node)
	if filter.Sphere != "" && filter.Sphere != workspace.Sphere {
		return builder.graph
	}
	appendWorkspaceGraphRoot(a, builder, root)
	sortWorkspaceLocalGraph(&builder.graph)
	return builder.graph
}

func appendWorkspaceGraphMarkdown(builder *workspaceGraphBuilder, sourceRel, rootID string) {
	if graphRelationEnabled(builder.filter, "markdown_link") {
		panel := buildWorkspaceMarkdownLinkPanel(builder.workspace, sourceRel)
		for _, ref := range panel.Outgoing {
			if !ref.OK || ref.VaultRelativePath == "" {
				continue
			}
			targetID := workspaceGraphNoteNodeID(ref.VaultRelativePath)
			builder.addNode(workspaceLocalGraphNode{
				ID:      targetID,
				Type:    workspaceGraphNodeType(ref.VaultRelativePath, ref.Kind),
				Label:   workspaceGraphNodeLabel(ref.VaultRelativePath),
				Path:    ref.VaultRelativePath,
				FileURL: ref.FileURL,
				Sphere:  builder.workspace.Sphere,
			})
			builder.addEdge(workspaceLocalGraphEdge{
				ID:       rootID + "->" + targetID + ":markdown_link",
				Source:   rootID,
				Target:   targetID,
				Relation: "markdown_link",
				Label:    ref.Type,
				Sphere:   builder.workspace.Sphere,
			})
		}
	}
	if !graphRelationEnabled(builder.filter, "backlink") {
		return
	}
	backlinks, truncated, scanLimit, err := collectMarkdownBacklinks(builder.workspace, sourceRel)
	if err != nil {
		builder.graph.Error = err.Error()
		builder.graph.OK = false
		return
	}
	builder.graph.Truncated = builder.graph.Truncated || truncated || scanLimit
	for _, backlink := range backlinks {
		sourceID := workspaceGraphNoteNodeID(backlink.SourcePath)
		builder.addNode(workspaceLocalGraphNode{
			ID:      sourceID,
			Type:    "note",
			Label:   workspaceGraphNodeLabel(backlink.SourcePath),
			Path:    backlink.SourcePath,
			FileURL: backlink.FileURL,
			Sphere:  builder.workspace.Sphere,
		})
		builder.addEdge(workspaceLocalGraphEdge{
			ID:       sourceID + "->" + rootID + ":backlink",
			Source:   sourceID,
			Target:   rootID,
			Relation: "backlink",
			Label:    backlink.LinkType,
			Sphere:   builder.workspace.Sphere,
		})
	}
}

func appendWorkspaceGraphStoreMetadata(a *App, builder *workspaceGraphBuilder, sourceRel, rootID string) {
	artifacts := workspaceGraphMatchingArtifacts(a, builder.workspace, sourceRel)
	itemIDs := map[int64]store.Item{}
	items := workspaceGraphItems(a, builder.workspace.Sphere)
	for _, artifact := range artifacts {
		artifactID := "artifact:" + workspaceIDStr(artifact.ID)
		builder.addNode(workspaceGraphArtifactNode(artifact))
		if graphRelationEnabled(builder.filter, "artifact") {
			builder.addEdge(workspaceLocalGraphEdge{
				ID:       rootID + "->" + artifactID + ":artifact",
				Source:   rootID,
				Target:   artifactID,
				Relation: "artifact",
				Label:    string(artifact.Kind),
				Sphere:   builder.workspace.Sphere,
			})
		}
		workspaceGraphAddArtifactBindings(a, builder, artifact, artifactID)
		for _, item := range items {
			if item.ArtifactID != nil && *item.ArtifactID == artifact.ID {
				itemIDs[item.ID] = item
			}
		}
	}
	for _, item := range itemIDs {
		workspaceGraphAddItem(a, builder, item)
	}
}

func workspaceGraphMatchingArtifacts(a *App, workspace store.Workspace, sourceRel string) []store.Artifact {
	artifacts, err := a.store.ListArtifactsForWorkspace(workspace.ID)
	if err != nil {
		return nil
	}
	_, vaultRoot, _ := brainWorkspaceRoots(workspace)
	keys := map[string]bool{
		sourceRel: true,
		filepath.ToSlash(filepath.Join("brain", sourceRel)): true,
	}
	out := []store.Artifact{}
	for _, artifact := range artifacts {
		for _, value := range []string{stringPointerValue(artifact.RefPath), stringPointerValue(artifact.Title)} {
			clean := cleanWorkspaceGraphArtifactPath(value, vaultRoot)
			if keys[clean] {
				out = append(out, artifact)
				break
			}
		}
	}
	return out
}

func cleanWorkspaceGraphArtifactPath(value, vaultRoot string) string {
	clean := strings.TrimSpace(value)
	if clean == "" {
		return ""
	}
	if filepath.IsAbs(clean) && pathInsideOrEqual(clean, vaultRoot) {
		if rel, err := filepath.Rel(vaultRoot, clean); err == nil {
			return filepath.ToSlash(filepath.Clean(rel))
		}
	}
	return strings.TrimPrefix(strings.TrimSpace(strings.ReplaceAll(clean, "\\", "/")), "/")
}

func workspaceGraphArtifactNode(artifact store.Artifact) workspaceLocalGraphNode {
	label := stringPointerValue(artifact.Title)
	if label == "" {
		label = stringPointerValue(artifact.RefPath)
	}
	if label == "" {
		label = "Artifact " + workspaceIDStr(artifact.ID)
	}
	return workspaceLocalGraphNode{
		ID:     "artifact:" + workspaceIDStr(artifact.ID),
		Type:   "artifact",
		Label:  label,
		Path:   stringPointerValue(artifact.RefPath),
		Source: string(artifact.Kind),
	}
}

func workspaceGraphItemNode(item store.Item) workspaceLocalGraphNode {
	return workspaceLocalGraphNode{
		ID:     "item:" + workspaceIDStr(item.ID),
		Type:   "item",
		Label:  item.Title,
		Source: stringPointerValue(item.Source),
		Sphere: item.Sphere,
	}
}

func workspaceGraphAddItem(a *App, builder *workspaceGraphBuilder, item store.Item) {
	if builder.filter.Source != "" && !workspaceGraphItemMatchesSource(item, builder.filter.Source) {
		return
	}
	if builder.filter.Label != "" && !workspaceGraphItemHasLabel(a, item.ID, builder.filter.Label, item.Sphere) {
		return
	}
	itemID := "item:" + workspaceIDStr(item.ID)
	builder.addNode(workspaceGraphItemNode(item))
	if item.ArtifactID != nil && graphRelationEnabled(builder.filter, "artifact") {
		artifactID := "artifact:" + workspaceIDStr(*item.ArtifactID)
		workspaceGraphAddItemArtifact(a, builder, *item.ArtifactID)
		builder.addEdge(workspaceLocalGraphEdge{
			ID:       itemID + "->" + artifactID + ":artifact",
			Source:   itemID,
			Target:   artifactID,
			Relation: "artifact",
			Label:    item.Kind,
			Sphere:   item.Sphere,
		})
	}
	workspaceGraphAddItemActor(a, builder, item, itemID)
	workspaceGraphAddItemLabels(a, builder, item, itemID)
	workspaceGraphAddItemSourceBindings(a, builder, item, itemID)
}

func workspaceGraphAddItemArtifact(a *App, builder *workspaceGraphBuilder, artifactID int64) {
	artifact, err := a.store.GetArtifact(artifactID)
	if err != nil {
		return
	}
	builder.addNode(workspaceGraphArtifactNode(artifact))
}

func workspaceGraphAddItemActor(a *App, builder *workspaceGraphBuilder, item store.Item, itemID string) {
	if item.ActorID == nil || !graphRelationEnabled(builder.filter, "item_actor") {
		return
	}
	actor, err := a.store.GetActor(*item.ActorID)
	if err != nil {
		return
	}
	actorID := "actor:" + workspaceIDStr(actor.ID)
	builder.addNode(workspaceLocalGraphNode{
		ID:    actorID,
		Type:  "actor",
		Label: actor.Name,
	})
	builder.addEdge(workspaceLocalGraphEdge{
		ID:       itemID + "->" + actorID + ":item_actor",
		Source:   itemID,
		Target:   actorID,
		Relation: "item_actor",
		Label:    actor.Kind,
		Sphere:   item.Sphere,
	})
}

func workspaceGraphAddItemLabels(a *App, builder *workspaceGraphBuilder, item store.Item, itemID string) {
	if !graphRelationEnabled(builder.filter, "item_label") {
		return
	}
	for _, label := range workspaceGraphItemLabels(a, item.ID, item.Sphere) {
		if builder.filter.Label != "" && !strings.EqualFold(label.Name, builder.filter.Label) {
			continue
		}
		labelID := "label:" + workspaceIDStr(label.ID)
		builder.addNode(workspaceLocalGraphNode{
			ID:    labelID,
			Type:  "label",
			Label: label.Name,
		})
		builder.addEdge(workspaceLocalGraphEdge{
			ID:       itemID + "->" + labelID + ":item_label",
			Source:   itemID,
			Target:   labelID,
			Relation: "item_label",
			Sphere:   item.Sphere,
		})
	}
}

func workspaceGraphAddItemSourceBindings(a *App, builder *workspaceGraphBuilder, item store.Item, itemID string) {
	if graphRelationEnabled(builder.filter, "source_binding") && item.Source != nil && item.SourceRef != nil {
		sourceID := "source:" + *item.Source + ":" + *item.SourceRef
		builder.addNode(workspaceLocalGraphNode{
			ID:     sourceID,
			Type:   "source",
			Label:  *item.SourceRef,
			Source: *item.Source,
			Sphere: item.Sphere,
		})
		builder.addEdge(workspaceLocalGraphEdge{
			ID:       itemID + "->" + sourceID + ":source_binding",
			Source:   itemID,
			Target:   sourceID,
			Relation: "source_binding",
			Label:    *item.Source,
			SourceID: *item.Source,
			Sphere:   item.Sphere,
		})
	}
	bindings, err := a.store.GetBindingsByItem(item.ID)
	if err != nil {
		return
	}
	workspaceGraphAddExternalBindings(builder, bindings, itemID, item.Sphere)
}

func workspaceGraphItems(a *App, sphere string) []store.Item {
	items, err := a.store.ListItemsFiltered(store.ItemListFilter{Sphere: sphere})
	if err != nil {
		return nil
	}
	return items
}

func workspaceGraphAddArtifactBindings(a *App, builder *workspaceGraphBuilder, artifact store.Artifact, artifactID string) {
	bindings, err := a.store.GetBindingsByArtifact(artifact.ID)
	if err != nil {
		return
	}
	workspaceGraphAddExternalBindings(builder, bindings, artifactID, builder.workspace.Sphere)
}

func workspaceGraphAddExternalBindings(builder *workspaceGraphBuilder, bindings []store.ExternalBinding, ownerID, sphere string) {
	if !graphRelationEnabled(builder.filter, "source_binding") {
		return
	}
	for _, binding := range bindings {
		if builder.filter.Source != "" && !strings.EqualFold(binding.Provider, builder.filter.Source) {
			continue
		}
		sourceID := "source:" + binding.Provider + ":" + binding.ObjectType + ":" + binding.RemoteID
		builder.addNode(workspaceLocalGraphNode{
			ID:     sourceID,
			Type:   "source",
			Label:  binding.RemoteID,
			Source: binding.Provider,
			Sphere: sphere,
		})
		builder.addEdge(workspaceLocalGraphEdge{
			ID:       ownerID + "->" + sourceID + ":source_binding",
			Source:   ownerID,
			Target:   sourceID,
			Relation: "source_binding",
			Label:    binding.ObjectType,
			SourceID: binding.Provider,
			Sphere:   sphere,
		})
	}
}

func workspaceGraphItemMatchesSource(item store.Item, source string) bool {
	return item.Source != nil && strings.EqualFold(*item.Source, source)
}

func workspaceGraphItemMatchesSourceNode(item store.Item, source workspaceLocalGraphNode) bool {
	if item.Source == nil || item.SourceRef == nil {
		return false
	}
	sourceID := "source:" + *item.Source + ":" + *item.SourceRef
	return source.ID == sourceID
}

func workspaceGraphItemHasLabel(a *App, itemID int64, labelName, sphere string) bool {
	for _, label := range workspaceGraphItemLabels(a, itemID, sphere) {
		if strings.EqualFold(label.Name, labelName) {
			return true
		}
	}
	return false
}

func workspaceGraphItemLabels(a *App, itemID int64, sphere string) []store.Label {
	labels, err := a.store.ListLabels()
	if err != nil {
		return nil
	}
	out := []store.Label{}
	for _, label := range labels {
		labelID := label.ID
		items, err := a.store.ListItemsFiltered(store.ItemListFilter{Sphere: sphere, LabelID: &labelID})
		if err != nil {
			continue
		}
		for _, item := range items {
			if item.ID == itemID {
				out = append(out, label)
				break
			}
		}
	}
	return out
}

func (a *App) handleWorkspaceLocalGraph(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	workspace, err := a.resolveRuntimeWorkspaceByIDOrActive(chi.URLParam(r, "workspace_id"))
	if err != nil {
		if isNoRows(err) {
			http.Error(w, "workspace not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := enforceWorkPersonalWorkspace(workspace); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	graph := buildWorkspaceLocalGraphForRequest(a, workspace, parseWorkspaceGraphRequest(r), parseWorkspaceGraphFilter(r, workspace))
	writeJSON(w, graph)
}
