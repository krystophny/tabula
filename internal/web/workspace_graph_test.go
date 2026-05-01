package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sloppy-org/slopshell/internal/store"
)

func TestWorkspaceLocalGraphMarkdownNeighborhoodExcludesPersonal(t *testing.T) {
	vaultRoot, personalRoot := configureWorkPersonalGuardrail(t)
	brainRoot := filepath.Join(vaultRoot, "brain")
	mustWriteGraphFile(t, filepath.Join(brainRoot, "topics", "active.md"), "See [Related](related.md).")
	mustWriteGraphFile(t, filepath.Join(brainRoot, "topics", "related.md"), "# Related")
	mustWriteGraphFile(t, filepath.Join(brainRoot, "people", "alice.md"), "Alice links [back](../topics/active.md).")
	mustWriteGraphFile(t, filepath.Join(personalRoot, "diary.md"), "Private [back](../brain/topics/active.md).")

	app := newAuthedTestApp(t)
	workspace, err := app.store.CreateWorkspace("Work brain", brainRoot, store.SphereWork)
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	graph := requestWorkspaceLocalGraph(t, app, workspace.ID, "topics/active.md", nil)
	if !graph.OK {
		t.Fatalf("graph not ok: %+v", graph)
	}
	assertGraphNode(t, graph, "note:brain/topics/related.md", "note")
	assertGraphNode(t, graph, "note:brain/people/alice.md", "note")
	assertGraphEdge(t, graph, "note:topics/active.md", "note:brain/topics/related.md", "markdown_link")
	assertGraphEdge(t, graph, "note:brain/people/alice.md", "note:topics/active.md", "backlink")
	for _, node := range graph.Nodes {
		if strings.Contains(node.Path, "personal/") || strings.Contains(node.Path, personalRoot) {
			t.Fatalf("graph leaked personal node: %+v", node)
		}
	}
}

func TestWorkspaceLocalGraphAddsConnectedStoreMetadata(t *testing.T) {
	vaultRoot, _ := configureWorkPersonalGuardrail(t)
	brainRoot := filepath.Join(vaultRoot, "brain")
	sourcePath := filepath.Join(brainRoot, "topics", "active.md")
	mustWriteGraphFile(t, sourcePath, "# Active")

	app := newAuthedTestApp(t)
	workspace, err := app.store.CreateWorkspace("Work brain", brainRoot, store.SphereWork)
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	title := "Active note artifact"
	artifact, err := app.store.CreateArtifact(store.ArtifactKindMarkdown, &sourcePath, nil, &title, nil)
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}
	actor, err := app.store.CreateActor("Ada", store.ActorKindHuman)
	if err != nil {
		t.Fatalf("CreateActor: %v", err)
	}
	source := store.ExternalProviderTodoist
	sourceRef := "task-123"
	item, err := app.store.CreateItem("Follow up active note", store.ItemOptions{
		WorkspaceID: &workspace.ID,
		Sphere:      graphStringPtr(store.SphereWork),
		ArtifactID:  &artifact.ID,
		ActorID:     &actor.ID,
		Source:      &source,
		SourceRef:   &sourceRef,
	})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	label, err := app.store.CreateLabel("deep-work", nil)
	if err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	if err := app.store.LinkLabelToItem(label.ID, item.ID); err != nil {
		t.Fatalf("LinkLabelToItem: %v", err)
	}

	graph := requestWorkspaceLocalGraph(t, app, workspace.ID, "topics/active.md", nil)
	assertGraphNode(t, graph, "artifact:"+itoa(artifact.ID), "artifact")
	assertGraphNode(t, graph, "item:"+itoa(item.ID), "item")
	assertGraphNode(t, graph, "actor:"+itoa(actor.ID), "actor")
	assertGraphNode(t, graph, "label:"+itoa(label.ID), "label")
	assertGraphNode(t, graph, "source:todoist:task-123", "source")
	assertGraphEdge(t, graph, "note:topics/active.md", "artifact:"+itoa(artifact.ID), "artifact")
	assertGraphEdge(t, graph, "item:"+itoa(item.ID), "actor:"+itoa(actor.ID), "item_actor")
	assertGraphEdge(t, graph, "item:"+itoa(item.ID), "label:"+itoa(label.ID), "item_label")
	assertGraphEdge(t, graph, "item:"+itoa(item.ID), "source:todoist:task-123", "source_binding")
}

func TestWorkspaceLocalGraphFiltersMetadata(t *testing.T) {
	vaultRoot, _ := configureWorkPersonalGuardrail(t)
	brainRoot := filepath.Join(vaultRoot, "brain")
	sourcePath := filepath.Join(brainRoot, "topics", "active.md")
	mustWriteGraphFile(t, sourcePath, "# Active")

	app := newAuthedTestApp(t)
	workspace, err := app.store.CreateWorkspace("Work brain", brainRoot, store.SphereWork)
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	artifact, err := app.store.CreateArtifact(store.ArtifactKindMarkdown, &sourcePath, nil, graphStringPtr("Active"), nil)
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}
	todoist := store.ExternalProviderTodoist
	github := "github"
	deep, _ := app.store.CreateLabel("deep-work", nil)
	shallow, _ := app.store.CreateLabel("shallow", nil)
	wanted, _ := app.store.CreateItem("Wanted", store.ItemOptions{WorkspaceID: &workspace.ID, Sphere: graphStringPtr(store.SphereWork), ArtifactID: &artifact.ID, Source: &todoist, SourceRef: graphStringPtr("a")})
	other, _ := app.store.CreateItem("Other", store.ItemOptions{WorkspaceID: &workspace.ID, Sphere: graphStringPtr(store.SphereWork), ArtifactID: &artifact.ID, Source: &github, SourceRef: graphStringPtr("b")})
	if err := app.store.LinkLabelToItem(deep.ID, wanted.ID); err != nil {
		t.Fatalf("LinkLabelToItem(deep): %v", err)
	}
	if err := app.store.LinkLabelToItem(shallow.ID, other.ID); err != nil {
		t.Fatalf("LinkLabelToItem(shallow): %v", err)
	}

	graph := requestWorkspaceLocalGraph(t, app, workspace.ID, "topics/active.md", map[string]string{
		"source_filter": "todoist",
		"label":         "deep-work",
		"sphere":        store.SphereWork,
	})
	assertGraphNode(t, graph, "item:"+itoa(wanted.ID), "item")
	assertGraphMissingNode(t, graph, "item:"+itoa(other.ID))

	privateGraph := requestWorkspaceLocalGraph(t, app, workspace.ID, "topics/active.md", map[string]string{
		"sphere": store.SpherePrivate,
	})
	if len(privateGraph.Nodes) != 1 || privateGraph.Nodes[0].ID != "note:topics/active.md" {
		t.Fatalf("private sphere graph = %+v, want only root note", privateGraph)
	}
}

func TestWorkspaceLocalGraphArtifactRootAddsConnectedMetadata(t *testing.T) {
	vaultRoot, _ := configureWorkPersonalGuardrail(t)
	brainRoot := filepath.Join(vaultRoot, "brain")
	sourcePath := filepath.Join(brainRoot, "topics", "active.pdf")
	mustWriteGraphFile(t, sourcePath, "%PDF-1.7")

	app := newAuthedTestApp(t)
	workspace, err := app.store.CreateWorkspace("Work brain", brainRoot, store.SphereWork)
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	artifact, err := app.store.CreateArtifact(store.ArtifactKindPDF, &sourcePath, nil, graphStringPtr("Active PDF"), nil)
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}
	item, err := app.store.CreateItem("Read active PDF", store.ItemOptions{
		WorkspaceID: &workspace.ID,
		Sphere:      graphStringPtr(store.SphereWork),
		ArtifactID:  &artifact.ID,
		Source:      graphStringPtr(store.ExternalProviderTodoist),
		SourceRef:   graphStringPtr("task-pdf"),
	})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}

	graph := requestWorkspaceLocalGraph(t, app, workspace.ID, "", map[string]string{
		"artifact_id": itoa(artifact.ID),
	})
	assertGraphNode(t, graph, "artifact:"+itoa(artifact.ID), "artifact")
	assertGraphNode(t, graph, "item:"+itoa(item.ID), "item")
	assertGraphNode(t, graph, "source:todoist:task-pdf", "source")
	assertGraphEdge(t, graph, "item:"+itoa(item.ID), "artifact:"+itoa(artifact.ID), "artifact")
	assertGraphEdge(t, graph, "item:"+itoa(item.ID), "source:todoist:task-pdf", "source_binding")
	if graph.RootID != "artifact:"+itoa(artifact.ID) {
		t.Fatalf("root_id = %q, want artifact root", graph.RootID)
	}
}

func TestWorkspaceLocalGraphEntityRootAddsConnectedMetadata(t *testing.T) {
	vaultRoot, _ := configureWorkPersonalGuardrail(t)
	brainRoot := filepath.Join(vaultRoot, "brain")
	sourcePath := filepath.Join(brainRoot, "topics", "active.md")
	mustWriteGraphFile(t, sourcePath, "# Active")

	app := newAuthedTestApp(t)
	workspace, err := app.store.CreateWorkspace("Work brain", brainRoot, store.SphereWork)
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	artifact, err := app.store.CreateArtifact(store.ArtifactKindMarkdown, &sourcePath, nil, graphStringPtr("Active note"), nil)
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}
	item, err := app.store.CreateItem("Follow active note", store.ItemOptions{
		WorkspaceID: &workspace.ID,
		Sphere:      graphStringPtr(store.SphereWork),
		ArtifactID:  &artifact.ID,
		Source:      graphStringPtr(store.ExternalProviderTodoist),
		SourceRef:   graphStringPtr("task-note"),
	})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}

	graph := requestWorkspaceLocalGraph(t, app, workspace.ID, "", map[string]string{
		"root": "item:" + itoa(item.ID),
	})
	assertGraphNode(t, graph, "item:"+itoa(item.ID), "item")
	assertGraphNode(t, graph, "artifact:"+itoa(artifact.ID), "artifact")
	assertGraphNode(t, graph, "source:todoist:task-note", "source")
	assertGraphEdge(t, graph, "item:"+itoa(item.ID), "artifact:"+itoa(artifact.ID), "artifact")
	assertGraphEdge(t, graph, "item:"+itoa(item.ID), "source:todoist:task-note", "source_binding")
}

func TestWorkspaceLocalGraphRejectsPrivateItemRootFromWorkWorkspace(t *testing.T) {
	vaultRoot, _ := configureWorkPersonalGuardrail(t)
	workBrainRoot := filepath.Join(vaultRoot, "brain")
	privateBrainRoot := filepath.Join(t.TempDir(), "private", "brain")

	app := newAuthedTestApp(t)
	workWorkspace, err := app.store.CreateWorkspace("Work brain", workBrainRoot, store.SphereWork)
	if err != nil {
		t.Fatalf("CreateWorkspace(work): %v", err)
	}
	privateWorkspace, err := app.store.CreateWorkspace("Private brain", privateBrainRoot, store.SpherePrivate)
	if err != nil {
		t.Fatalf("CreateWorkspace(private): %v", err)
	}
	privateItem, err := app.store.CreateItem("Private reminder", store.ItemOptions{
		WorkspaceID: &privateWorkspace.ID,
	})
	if err != nil {
		t.Fatalf("CreateItem(private): %v", err)
	}

	graph := requestWorkspaceLocalGraph(t, app, workWorkspace.ID, "", map[string]string{
		"root": "item:" + itoa(privateItem.ID),
	})
	if graph.OK {
		t.Fatalf("graph OK = true, want rejected graph")
	}
	if graph.Error != "item is not in this workspace" {
		t.Fatalf("graph error = %q, want item scope error", graph.Error)
	}
	assertGraphMissingNode(t, graph, "item:"+itoa(privateItem.ID))
}

func requestWorkspaceLocalGraph(t *testing.T, app *App, workspaceID int64, sourcePath string, query map[string]string) workspaceLocalGraph {
	t.Helper()
	values := url.Values{}
	if sourcePath != "" {
		values.Set("source", sourcePath)
	}
	for key, value := range query {
		values.Set(key, value)
	}
	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/workspaces/"+itoa(workspaceID)+"/graph?"+values.Encode(), nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("graph status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var graph workspaceLocalGraph
	if err := json.Unmarshal(rr.Body.Bytes(), &graph); err != nil {
		t.Fatalf("decode graph: %v", err)
	}
	return graph
}

func assertGraphNode(t *testing.T, graph workspaceLocalGraph, id, kind string) {
	t.Helper()
	for _, node := range graph.Nodes {
		if node.ID == id && node.Type == kind {
			return
		}
	}
	t.Fatalf("missing graph node %s/%s in %+v", id, kind, graph.Nodes)
}

func assertGraphMissingNode(t *testing.T, graph workspaceLocalGraph, id string) {
	t.Helper()
	for _, node := range graph.Nodes {
		if node.ID == id {
			t.Fatalf("unexpected graph node %s in %+v", id, graph.Nodes)
		}
	}
}

func assertGraphEdge(t *testing.T, graph workspaceLocalGraph, source, target, relation string) {
	t.Helper()
	for _, edge := range graph.Edges {
		if edge.Source == source && edge.Target == target && edge.Relation == relation {
			return
		}
	}
	t.Fatalf("missing graph edge %s -> %s (%s) in %+v", source, target, relation, graph.Edges)
}

func mustWriteGraphFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func graphStringPtr(value string) *string {
	return &value
}
