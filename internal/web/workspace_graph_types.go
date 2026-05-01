package web

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/sloppy-org/slopshell/internal/store"
)

const (
	workspaceLocalGraphNodeCap = 80
	workspaceLocalGraphEdgeCap = 160
)

type workspaceLocalGraphNode struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Label   string `json:"label"`
	Path    string `json:"path,omitempty"`
	FileURL string `json:"file_url,omitempty"`
	Source  string `json:"source,omitempty"`
	Sphere  string `json:"sphere,omitempty"`
}

type workspaceLocalGraphEdge struct {
	ID       string `json:"id"`
	Source   string `json:"source"`
	Target   string `json:"target"`
	Relation string `json:"relation"`
	Label    string `json:"label,omitempty"`
	SourceID string `json:"source_id,omitempty"`
	Sphere   string `json:"sphere,omitempty"`
}

type workspaceLocalGraph struct {
	OK        bool                      `json:"ok"`
	Source    string                    `json:"source_path"`
	RootID    string                    `json:"root_id,omitempty"`
	Nodes     []workspaceLocalGraphNode `json:"nodes"`
	Edges     []workspaceLocalGraphEdge `json:"edges"`
	Truncated bool                      `json:"truncated,omitempty"`
	Error     string                    `json:"error,omitempty"`
}

type workspaceGraphFilter struct {
	Relations map[string]bool
	Source    string
	Label     string
	Sphere    string
}

type workspaceGraphBuilder struct {
	workspace store.Workspace
	filter    workspaceGraphFilter
	graph     workspaceLocalGraph
	nodes     map[string]workspaceLocalGraphNode
	edges     map[string]workspaceLocalGraphEdge
}

func graphRelationEnabled(filter workspaceGraphFilter, relation string) bool {
	return len(filter.Relations) == 0 || filter.Relations[relation]
}

func newWorkspaceGraphBuilder(workspace store.Workspace, filter workspaceGraphFilter, sourceRel, rootID string) *workspaceGraphBuilder {
	return &workspaceGraphBuilder{
		workspace: workspace,
		filter:    filter,
		graph: workspaceLocalGraph{
			OK:     true,
			Source: sourceRel,
			RootID: rootID,
			Nodes:  []workspaceLocalGraphNode{},
			Edges:  []workspaceLocalGraphEdge{},
		},
		nodes: map[string]workspaceLocalGraphNode{},
		edges: map[string]workspaceLocalGraphEdge{},
	}
}

func (b *workspaceGraphBuilder) addNode(node workspaceLocalGraphNode) {
	if node.ID == "" {
		return
	}
	if _, ok := b.nodes[node.ID]; ok {
		return
	}
	if len(b.graph.Nodes) >= workspaceLocalGraphNodeCap {
		b.graph.Truncated = true
		return
	}
	b.nodes[node.ID] = node
	b.graph.Nodes = append(b.graph.Nodes, node)
}

func (b *workspaceGraphBuilder) addEdge(edge workspaceLocalGraphEdge) {
	if edge.ID == "" || edge.Source == "" || edge.Target == "" {
		return
	}
	if _, ok := b.edges[edge.ID]; ok {
		return
	}
	if len(b.graph.Edges) >= workspaceLocalGraphEdgeCap {
		b.graph.Truncated = true
		return
	}
	b.edges[edge.ID] = edge
	b.graph.Edges = append(b.graph.Edges, edge)
}

func workspaceGraphNoteNodeID(path string) string {
	return "note:" + strings.TrimSpace(path)
}

func workspaceGraphNodeLabel(path string) string {
	clean := strings.TrimSpace(path)
	if clean == "" {
		return "Untitled"
	}
	return strings.TrimSuffix(filepath.Base(clean), filepath.Ext(clean))
}

func workspaceGraphNodeType(path, kind string) string {
	if strings.EqualFold(filepath.Ext(path), ".md") || strings.EqualFold(kind, "text") {
		return "note"
	}
	if strings.EqualFold(kind, "folder") {
		return "folder"
	}
	return "artifact"
}

func sortWorkspaceLocalGraph(graph *workspaceLocalGraph) {
	rootID := graph.RootID
	if rootID == "" {
		rootID = workspaceGraphNoteNodeID(graph.Source)
	}
	sort.SliceStable(graph.Nodes, func(i, j int) bool {
		if graph.Nodes[i].ID == rootID {
			return true
		}
		if graph.Nodes[j].ID == rootID {
			return false
		}
		if graph.Nodes[i].Type == graph.Nodes[j].Type {
			return graph.Nodes[i].Label < graph.Nodes[j].Label
		}
		return graph.Nodes[i].Type < graph.Nodes[j].Type
	})
	sort.SliceStable(graph.Edges, func(i, j int) bool {
		if graph.Edges[i].Relation == graph.Edges[j].Relation {
			return graph.Edges[i].ID < graph.Edges[j].ID
		}
		return graph.Edges[i].Relation < graph.Edges[j].Relation
	})
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
