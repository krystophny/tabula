package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInkSubmitWritesArtifacts(t *testing.T) {
	app := newAuthedTestApp(t)

	rrProjects := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/runtime/workspaces", nil)
	if rrProjects.Code != http.StatusOK {
		t.Fatalf("projects status=%d body=%s", rrProjects.Code, rrProjects.Body.String())
	}
	var listPayload workspacesListResponse
	if err := json.Unmarshal(rrProjects.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode projects response: %v", err)
	}
	if len(listPayload.Projects) == 0 {
		t.Fatalf("expected at least one project")
	}
	workspaceID := listPayload.Projects[0].ID

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/ink/submit", map[string]any{
		"workspace_id":   workspaceID,
		"artifact_kind":  "text",
		"artifact_title": "README.md",
		"artifact_path":  "README.md",
		"svg":            `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 80"><path d="M 10 10 L 40 40" /></svg>`,
		"png_base64":     "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a3xwAAAAASUVORK5CYII=",
		"strokes": []map[string]any{
			{
				"pointer_type": "pen",
				"width":        3.2,
				"points": []map[string]any{
					{"x": 10, "y": 10, "pressure": 0.7},
					{"x": 40, "y": 40, "pressure": 0.8},
				},
			},
		},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("ink submit status=%d body=%s", rr.Code, rr.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode ink submit response: %v", err)
	}
	inkPath := strFromAny(payload["ink_svg_path"])
	pngPath := strFromAny(payload["ink_png_path"])
	summaryPath := strFromAny(payload["summary_path"])
	manifestPath := strFromAny(payload["revision_manifest_path"])
	historyPath := strFromAny(payload["revision_history_path"])
	if inkPath == "" || pngPath == "" || summaryPath == "" || manifestPath == "" || historyPath == "" {
		t.Fatalf("expected ink paths in response: %v", payload)
	}

	project, err := app.store.GetEnrichedWorkspace(workspaceID)
	if err != nil {
		t.Fatalf("get project: %v", err)
	}
	inkContent, err := os.ReadFile(filepath.Join(project.RootPath, filepath.FromSlash(inkPath)))
	if err != nil {
		t.Fatalf("read ink svg: %v", err)
	}
	if !strings.Contains(string(inkContent), "<svg") {
		t.Fatalf("ink svg missing svg root: %s", string(inkContent))
	}
	pngContent, err := os.ReadFile(filepath.Join(project.RootPath, filepath.FromSlash(pngPath)))
	if err != nil {
		t.Fatalf("read ink png: %v", err)
	}
	if len(pngContent) == 0 {
		t.Fatal("ink png was empty")
	}

	summaryContent, err := os.ReadFile(filepath.Join(project.RootPath, filepath.FromSlash(summaryPath)))
	if err != nil {
		t.Fatalf("read ink summary: %v", err)
	}
	summaryText := string(summaryContent)
	if !strings.Contains(summaryText, "Stroke count: `1`") {
		t.Fatalf("ink summary missing stroke count: %s", summaryText)
	}
	if !strings.Contains(summaryText, "PNG artifact:") {
		t.Fatalf("ink summary missing png reference: %s", summaryText)
	}
	if !strings.Contains(summaryText, "README.md") {
		t.Fatalf("ink summary missing artifact title/path: %s", summaryText)
	}

	manifestContent, err := os.ReadFile(filepath.Join(project.RootPath, filepath.FromSlash(manifestPath)))
	if err != nil {
		t.Fatalf("read revision manifest: %v", err)
	}
	if !strings.Contains(string(manifestContent), "\"kind\": \"ink\"") {
		t.Fatalf("revision manifest missing ink entry: %s", string(manifestContent))
	}

	historyContent, err := os.ReadFile(filepath.Join(project.RootPath, filepath.FromSlash(historyPath)))
	if err != nil {
		t.Fatalf("read revision history: %v", err)
	}
	if !strings.Contains(string(historyContent), "Local Revision History: README.md") {
		t.Fatalf("revision history missing heading: %s", string(historyContent))
	}
}
