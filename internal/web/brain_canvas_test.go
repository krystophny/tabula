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

func newBrainCanvasTestApp(t *testing.T) (*App, store.Workspace, string) {
	t.Helper()
	vaultRoot, _ := configureWorkPersonalGuardrail(t)
	brainRoot := filepath.Join(vaultRoot, "brain")
	app := newAuthedTestApp(t)
	workspace, err := app.store.CreateWorkspace("Work brain", brainRoot, store.SphereWork)
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	return app, workspace, brainRoot
}

func decodeBrainCanvasView(t *testing.T, body []byte) brainCanvasView {
	t.Helper()
	var view brainCanvasView
	if err := json.Unmarshal(body, &view); err != nil {
		t.Fatalf("decode brain canvas view: %v\nbody=%s", err, string(body))
	}
	return view
}

func decodeBrainCanvasCard(t *testing.T, body []byte) brainCanvasCardView {
	t.Helper()
	var card brainCanvasCardView
	if err := json.Unmarshal(body, &card); err != nil {
		t.Fatalf("decode brain canvas card: %v\nbody=%s", err, string(body))
	}
	return card
}

func brainCanvasURL(workspaceID int64, suffix string) string {
	return "/api/workspaces/" + itoa(workspaceID) + "/brain-canvas" + suffix
}

func brainCanvasTestSpherePointer() *string {
	sphere := store.SphereWork
	return &sphere
}

func TestBrainCanvasNormalizeNameSafe(t *testing.T) {
	cases := map[string]string{
		"":           brainCanvasDefaultName,
		"   ":        brainCanvasDefaultName,
		"My Canvas":  "my-canvas",
		"../escape":  "escape",
		"WeIrD/Path": "weird-path",
	}
	for raw, want := range cases {
		got := normalizeBrainCanvasName(raw)
		if got != want {
			t.Errorf("normalizeBrainCanvasName(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestBrainCanvasLayoutPersistsAcrossReloads(t *testing.T) {
	app, workspace, brainRoot := newBrainCanvasTestApp(t)
	notePath := filepath.Join(brainRoot, "topics", "active.md")
	if err := os.MkdirAll(filepath.Dir(notePath), 0o755); err != nil {
		t.Fatalf("mkdir topics: %v", err)
	}
	if err := os.WriteFile(notePath, []byte("# Active\n"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}

	createPayload := brainCanvasCardCreateRequest{
		Binding: brainCanvasBinding{Kind: "note", Path: "topics/active.md"},
		X:       10,
		Y:       20,
		Width:   320,
		Height:  240,
	}
	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, brainCanvasURL(workspace.ID, "/cards"), createPayload)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create card status = %d: %s", rr.Code, rr.Body.String())
	}
	created := decodeBrainCanvasCard(t, rr.Body.Bytes())
	if created.ID == "" || created.Binding.Kind != "note" || created.Binding.Path != "topics/active.md" {
		t.Fatalf("created card unexpected: %+v", created)
	}
	if created.X != 10 || created.Y != 20 || created.Width != 320 || created.Height != 240 {
		t.Fatalf("layout not preserved: %+v", created)
	}
	if created.Title != "active" {
		t.Fatalf("note title = %q, want %q", created.Title, "active")
	}
	if created.OpenURL == "" || !strings.Contains(created.OpenURL, "brain%2Ftopics%2Factive.md") {
		t.Fatalf("note open_url = %q", created.OpenURL)
	}

	moveX := 256.0
	moveY := 384.0
	resizeW := 480.0
	resizeH := 320.0
	patch := brainCanvasCardPatchRequest{X: &moveX, Y: &moveY, Width: &resizeW, Height: &resizeH}
	patchURL := brainCanvasURL(workspace.ID, "/cards/"+created.ID)
	rr = doAuthedJSONRequest(t, app.Router(), http.MethodPatch, patchURL, patch)
	if rr.Code != http.StatusOK {
		t.Fatalf("patch status = %d: %s", rr.Code, rr.Body.String())
	}
	patched := decodeBrainCanvasCard(t, rr.Body.Bytes())
	if patched.X != moveX || patched.Y != moveY || patched.Width != resizeW || patched.Height != resizeH {
		t.Fatalf("patch did not apply layout: %+v", patched)
	}

	noteAfter, err := os.ReadFile(notePath)
	if err != nil || string(noteAfter) != "# Active\n" {
		t.Fatalf("source note must not change for layout patch; got=%q err=%v", string(noteAfter), err)
	}

	canvasFile := filepath.Join(brainRoot, "canvas", "default.canvas")
	canvasBytes, err := os.ReadFile(canvasFile)
	if err != nil {
		t.Fatalf("canvas file missing: %v", err)
	}
	var doc brainCanvasDocument
	if err := json.Unmarshal(canvasBytes, &doc); err != nil {
		t.Fatalf("canvas file is not valid JSON Canvas: %v", err)
	}
	if len(doc.Nodes) != 1 || doc.Nodes[0].X != moveX || doc.Nodes[0].Y != moveY {
		t.Fatalf("durable canvas layout not persisted: %+v", doc.Nodes)
	}
	if doc.Nodes[0].Type != "file" || doc.Nodes[0].File != "brain/topics/active.md" {
		t.Fatalf("canvas node should be JSON Canvas file-typed: %+v", doc.Nodes[0])
	}

	rr = doAuthedJSONRequest(t, app.Router(), http.MethodGet, brainCanvasURL(workspace.ID, ""), nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("reload status = %d: %s", rr.Code, rr.Body.String())
	}
	view := decodeBrainCanvasView(t, rr.Body.Bytes())
	if !view.OK || len(view.Cards) != 1 {
		t.Fatalf("reloaded canvas = %+v", view)
	}
	if view.Cards[0].X != moveX || view.Cards[0].Y != moveY {
		t.Fatalf("reload layout drift: %+v", view.Cards[0])
	}
}

func TestBrainCanvasArtifactBindingWritesThroughTitle(t *testing.T) {
	app, workspace, _ := newBrainCanvasTestApp(t)
	artifactKind := store.ArtifactKind("file_text")
	artifactTitle := "Old artifact title"
	artifact, err := app.store.CreateArtifact(artifactKind, nil, nil, &artifactTitle, nil)
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}

	createPayload := brainCanvasCardCreateRequest{
		Binding: brainCanvasBinding{Kind: "artifact", ID: artifact.ID},
	}
	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, brainCanvasURL(workspace.ID, "/cards"), createPayload)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create artifact card = %d: %s", rr.Code, rr.Body.String())
	}
	card := decodeBrainCanvasCard(t, rr.Body.Bytes())
	if card.Title != artifactTitle {
		t.Fatalf("card title from artifact = %q, want %q", card.Title, artifactTitle)
	}

	newTitle := "Renamed via canvas"
	patch := brainCanvasCardPatchRequest{Title: &newTitle}
	rr = doAuthedJSONRequest(t, app.Router(), http.MethodPatch, brainCanvasURL(workspace.ID, "/cards/"+card.ID), patch)
	if rr.Code != http.StatusOK {
		t.Fatalf("patch artifact card = %d: %s", rr.Code, rr.Body.String())
	}
	patched := decodeBrainCanvasCard(t, rr.Body.Bytes())
	if patched.Title != newTitle {
		t.Fatalf("artifact card title not updated: %+v", patched)
	}

	reloaded, err := app.store.GetArtifact(artifact.ID)
	if err != nil {
		t.Fatalf("GetArtifact: %v", err)
	}
	if got := stringPointerValue(reloaded.Title); got != newTitle {
		t.Fatalf("artifact title in store = %q, want %q", got, newTitle)
	}
}

func TestBrainCanvasItemBindingWritesThroughTitle(t *testing.T) {
	app, workspace, _ := newBrainCanvasTestApp(t)
	item, err := app.store.CreateItem("Original item", store.ItemOptions{
		Sphere:      brainCanvasTestSpherePointer(),
		WorkspaceID: int64Pointer(workspace.ID),
	})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}

	createPayload := brainCanvasCardCreateRequest{
		Binding: brainCanvasBinding{Kind: "item", ID: item.ID},
	}
	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, brainCanvasURL(workspace.ID, "/cards"), createPayload)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create item card = %d: %s", rr.Code, rr.Body.String())
	}
	card := decodeBrainCanvasCard(t, rr.Body.Bytes())
	if card.Title != "Original item" {
		t.Fatalf("item card title = %q", card.Title)
	}

	newTitle := "Item retitled from canvas"
	patch := brainCanvasCardPatchRequest{Title: &newTitle}
	rr = doAuthedJSONRequest(t, app.Router(), http.MethodPatch, brainCanvasURL(workspace.ID, "/cards/"+card.ID), patch)
	if rr.Code != http.StatusOK {
		t.Fatalf("patch item card = %d: %s", rr.Code, rr.Body.String())
	}
	updated, err := app.store.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if updated.Title != newTitle {
		t.Fatalf("item title in store = %q, want %q", updated.Title, newTitle)
	}
}

func TestBrainCanvasNoteBindingWritesThroughBody(t *testing.T) {
	app, workspace, brainRoot := newBrainCanvasTestApp(t)
	notePath := filepath.Join(brainRoot, "people", "alice.md")
	if err := os.MkdirAll(filepath.Dir(notePath), 0o755); err != nil {
		t.Fatalf("mkdir people: %v", err)
	}
	if err := os.WriteFile(notePath, []byte("# Alice"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, brainCanvasURL(workspace.ID, "/cards"),
		brainCanvasCardCreateRequest{Binding: brainCanvasBinding{Kind: "note", Path: "people/alice.md"}})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create note card = %d: %s", rr.Code, rr.Body.String())
	}
	card := decodeBrainCanvasCard(t, rr.Body.Bytes())

	newBody := "# Alice\n\nUpdated from canvas\n"
	patch := brainCanvasCardPatchRequest{Body: &newBody}
	rr = doAuthedJSONRequest(t, app.Router(), http.MethodPatch, brainCanvasURL(workspace.ID, "/cards/"+card.ID), patch)
	if rr.Code != http.StatusOK {
		t.Fatalf("patch note card = %d: %s", rr.Code, rr.Body.String())
	}
	got, err := os.ReadFile(notePath)
	if err != nil || string(got) != newBody {
		t.Fatalf("note file body = %q err=%v, want %q", string(got), err, newBody)
	}
}

func TestBrainCanvasRejectsUnsupportedSemanticPatch(t *testing.T) {
	app, workspace, _ := newBrainCanvasTestApp(t)
	item, err := app.store.CreateItem("Original item", store.ItemOptions{Sphere: brainCanvasTestSpherePointer()})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, brainCanvasURL(workspace.ID, "/cards"),
		brainCanvasCardCreateRequest{Binding: brainCanvasBinding{Kind: "item", ID: item.ID}})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create item card = %d: %s", rr.Code, rr.Body.String())
	}
	card := decodeBrainCanvasCard(t, rr.Body.Bytes())

	body := "body edits are not item semantics"
	rr = doAuthedJSONRequest(t, app.Router(), http.MethodPatch, brainCanvasURL(workspace.ID, "/cards/"+card.ID),
		brainCanvasCardPatchRequest{Body: &body})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unsupported item body patch = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
	updated, err := app.store.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if updated.Title != "Original item" {
		t.Fatalf("unsupported patch changed item title = %q", updated.Title)
	}
}

func TestBrainCanvasOpenReturnsBackingFileURL(t *testing.T) {
	app, workspace, brainRoot := newBrainCanvasTestApp(t)
	notePath := filepath.Join(brainRoot, "topics", "active.md")
	if err := os.MkdirAll(filepath.Dir(notePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(notePath, []byte("# Active"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, brainCanvasURL(workspace.ID, "/cards"),
		brainCanvasCardCreateRequest{Binding: brainCanvasBinding{Kind: "note", Path: "topics/active.md"}})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create note card = %d: %s", rr.Code, rr.Body.String())
	}
	card := decodeBrainCanvasCard(t, rr.Body.Bytes())

	rr = doAuthedJSONRequest(t, app.Router(), http.MethodGet, brainCanvasURL(workspace.ID, "/cards/"+card.ID+"/open"), nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("open status = %d: %s", rr.Code, rr.Body.String())
	}
	var open brainCanvasCardOpen
	if err := json.Unmarshal(rr.Body.Bytes(), &open); err != nil {
		t.Fatalf("decode open: %v", err)
	}
	if !open.OK || open.Kind != "note" || open.OpenURL == "" {
		t.Fatalf("open response = %+v", open)
	}
	parsed, err := url.Parse(open.OpenURL)
	if err != nil {
		t.Fatalf("parse open url: %v", err)
	}
	if got := parsed.Query().Get("path"); got != "brain/topics/active.md" {
		t.Fatalf("open url path query = %q, want brain/topics/active.md", got)
	}
}

func TestBrainCanvasRejectsDuplicateBinding(t *testing.T) {
	app, workspace, _ := newBrainCanvasTestApp(t)
	item, err := app.store.CreateItem("Reuse me", store.ItemOptions{Sphere: brainCanvasTestSpherePointer()})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	payload := brainCanvasCardCreateRequest{Binding: brainCanvasBinding{Kind: "item", ID: item.ID}}
	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, brainCanvasURL(workspace.ID, "/cards"), payload)
	if rr.Code != http.StatusCreated {
		t.Fatalf("first create = %d: %s", rr.Code, rr.Body.String())
	}
	rr = doAuthedJSONRequest(t, app.Router(), http.MethodPost, brainCanvasURL(workspace.ID, "/cards"), payload)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("duplicate create status = %d, want 400", rr.Code)
	}
}

func TestBrainCanvasRejectsMissingBindingTarget(t *testing.T) {
	app, workspace, _ := newBrainCanvasTestApp(t)
	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, brainCanvasURL(workspace.ID, "/cards"),
		brainCanvasCardCreateRequest{Binding: brainCanvasBinding{Kind: "artifact", ID: 9999}})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("missing artifact create = %d", rr.Code)
	}
}

func TestBrainCanvasRejectsMissingNoteBindingTarget(t *testing.T) {
	app, workspace, _ := newBrainCanvasTestApp(t)
	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, brainCanvasURL(workspace.ID, "/cards"),
		brainCanvasCardCreateRequest{Binding: brainCanvasBinding{Kind: "note", Path: "topics/missing.md"}})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("missing note create = %d", rr.Code)
	}
}

func TestBrainCanvasNoteEditRequiresExistingBackingFile(t *testing.T) {
	app, workspace, brainRoot := newBrainCanvasTestApp(t)
	notePath := filepath.Join(brainRoot, "topics", "active.md")
	if err := os.MkdirAll(filepath.Dir(notePath), 0o755); err != nil {
		t.Fatalf("mkdir topics: %v", err)
	}
	if err := os.WriteFile(notePath, []byte("# Active\n"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, brainCanvasURL(workspace.ID, "/cards"),
		brainCanvasCardCreateRequest{Binding: brainCanvasBinding{Kind: "note", Path: "topics/active.md"}})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create note card = %d: %s", rr.Code, rr.Body.String())
	}
	card := decodeBrainCanvasCard(t, rr.Body.Bytes())
	if err := os.Remove(notePath); err != nil {
		t.Fatalf("remove backing note: %v", err)
	}

	newBody := "# Recreated\n"
	rr = doAuthedJSONRequest(t, app.Router(), http.MethodPatch, brainCanvasURL(workspace.ID, "/cards/"+card.ID),
		brainCanvasCardPatchRequest{Body: &newBody})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("patch deleted note = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(notePath); !os.IsNotExist(err) {
		t.Fatalf("semantic patch must not recreate deleted note; stat err=%v", err)
	}
}

func TestBrainCanvasDeleteCard(t *testing.T) {
	app, workspace, _ := newBrainCanvasTestApp(t)
	item, err := app.store.CreateItem("Drop me", store.ItemOptions{Sphere: brainCanvasTestSpherePointer()})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, brainCanvasURL(workspace.ID, "/cards"),
		brainCanvasCardCreateRequest{Binding: brainCanvasBinding{Kind: "item", ID: item.ID}})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rr.Code, rr.Body.String())
	}
	card := decodeBrainCanvasCard(t, rr.Body.Bytes())

	rr = doAuthedJSONRequest(t, app.Router(), http.MethodDelete, brainCanvasURL(workspace.ID, "/cards/"+card.ID), nil)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("delete = %d: %s", rr.Code, rr.Body.String())
	}

	rr = doAuthedJSONRequest(t, app.Router(), http.MethodGet, brainCanvasURL(workspace.ID, ""), nil)
	view := decodeBrainCanvasView(t, rr.Body.Bytes())
	if len(view.Cards) != 0 {
		t.Fatalf("expected canvas empty after delete, got %+v", view.Cards)
	}
}

func TestBrainCanvasRejectsPersonalSubtreeNote(t *testing.T) {
	app, workspace, brainRoot := newBrainCanvasTestApp(t)
	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, brainCanvasURL(workspace.ID, "/cards"),
		brainCanvasCardCreateRequest{Binding: brainCanvasBinding{Kind: "note", Path: "../personal/diary.md"}})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("personal note create = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}

	personalNote := filepath.Join(filepath.Dir(brainRoot), "personal", "diary.md")
	if err := os.WriteFile(personalNote, []byte("private"), 0o600); err != nil {
		t.Fatalf("write personal note: %v", err)
	}
	linkPath := filepath.Join(brainRoot, "linked-diary.md")
	if err := os.Symlink(personalNote, linkPath); err != nil {
		t.Fatalf("symlink personal note: %v", err)
	}
	rr = doAuthedJSONRequest(t, app.Router(), http.MethodPost, brainCanvasURL(workspace.ID, "/cards"),
		brainCanvasCardCreateRequest{Binding: brainCanvasBinding{Kind: "note", Path: "linked-diary.md"}})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("personal symlink note create = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
}

func TestBrainCanvasNamedFilesAreIsolated(t *testing.T) {
	app, workspace, _ := newBrainCanvasTestApp(t)
	item, err := app.store.CreateItem("Named", store.ItemOptions{Sphere: brainCanvasTestSpherePointer()})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, brainCanvasURL(workspace.ID, "/cards?name=secondary"),
		brainCanvasCardCreateRequest{Binding: brainCanvasBinding{Kind: "item", ID: item.ID}})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create on named = %d: %s", rr.Code, rr.Body.String())
	}
	rr = doAuthedJSONRequest(t, app.Router(), http.MethodGet, brainCanvasURL(workspace.ID, ""), nil)
	view := decodeBrainCanvasView(t, rr.Body.Bytes())
	if len(view.Cards) != 0 {
		t.Fatalf("default canvas should be empty: %+v", view.Cards)
	}
	rr = doAuthedJSONRequest(t, app.Router(), http.MethodGet, brainCanvasURL(workspace.ID, "?name=secondary"), nil)
	view = decodeBrainCanvasView(t, rr.Body.Bytes())
	if len(view.Cards) != 1 {
		t.Fatalf("secondary canvas should have one card: %+v", view.Cards)
	}
}
