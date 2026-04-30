package web

import (
	"net/http"
	"testing"

	"github.com/sloppy-org/slopshell/internal/store"
)

// The compact sidebar (issue #746) renders Workspace pin, item queues, and a
// secondary expandable section that surfaces project-item, people,
// drift-review, dedup-review, and recent-meeting counts as filters. The counts
// API must include both the per-state map and a `sections` payload so the
// frontend can render those filters without confusing project items with
// Workspaces.
func TestItemCountsExposesSidebarSectionCountsAlongsidePerStateCounts(t *testing.T) {
	app := newAuthedTestApp(t)

	if _, err := app.store.CreateItem("Plan GTD outcome", store.ItemOptions{
		Kind:  store.ItemKindProject,
		State: store.ItemStateNext,
	}); err != nil {
		t.Fatalf("CreateItem(project next) error: %v", err)
	}
	if _, err := app.store.CreateItem("Closed outcome", store.ItemOptions{
		Kind:  store.ItemKindProject,
		State: store.ItemStateDone,
	}); err != nil {
		t.Fatalf("CreateItem(project done) error: %v", err)
	}
	if _, err := app.store.CreateItem("Routine action", store.ItemOptions{
		State: store.ItemStateNext,
	}); err != nil {
		t.Fatalf("CreateItem(action) error: %v", err)
	}

	alice, err := app.store.CreateActor("Alice", store.ActorKindHuman)
	if err != nil {
		t.Fatalf("CreateActor(Alice) error: %v", err)
	}
	bob, err := app.store.CreateActor("Bob", store.ActorKindHuman)
	if err != nil {
		t.Fatalf("CreateActor(Bob) error: %v", err)
	}
	if _, err := app.store.CreateItem("Awaiting Alice", store.ItemOptions{
		State:   store.ItemStateWaiting,
		ActorID: &alice.ID,
	}); err != nil {
		t.Fatalf("CreateItem(awaiting Alice) error: %v", err)
	}
	if _, err := app.store.CreateItem("Owe Bob", store.ItemOptions{
		State:   store.ItemStateNext,
		ActorID: &bob.ID,
	}); err != nil {
		t.Fatalf("CreateItem(owe Bob) error: %v", err)
	}

	driftItem, err := app.store.CreateItem("Drifted PR", store.ItemOptions{State: store.ItemStateReview})
	if err != nil {
		t.Fatalf("CreateItem(drift) error: %v", err)
	}
	target := store.ItemReviewTargetGitHub
	reviewer := "krystophny"
	if err := app.store.UpdateItemReviewDispatch(driftItem.ID, &target, &reviewer); err != nil {
		t.Fatalf("UpdateItemReviewDispatch() error: %v", err)
	}

	source := "github"
	dupRef := "krystophny/repo#42"
	if _, err := app.store.CreateItem("Dup A", store.ItemOptions{
		State:     store.ItemStateNext,
		Source:    &source,
		SourceRef: &dupRef,
	}); err != nil {
		t.Fatalf("CreateItem(dup A) error: %v", err)
	}
	if _, err := app.store.CreateItem("Dup B", store.ItemOptions{
		State:     store.ItemStateInbox,
		Source:    &source,
		SourceRef: &dupRef,
	}); err != nil {
		t.Fatalf("CreateItem(dup B) error: %v", err)
	}

	transcriptPath := "/tmp/transcript.md"
	transcriptTitle := "Recent transcript"
	if _, err := app.store.CreateArtifact(store.ArtifactKindTranscript, &transcriptPath, nil, &transcriptTitle, nil); err != nil {
		t.Fatalf("CreateArtifact(transcript) error: %v", err)
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/counts", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("counts status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	payload := decodeJSONResponse(t, rr)
	counts, ok := payload["counts"].(map[string]any)
	if !ok {
		t.Fatalf("counts payload = %#v", payload)
	}
	if got := int(counts[store.ItemStateDone].(float64)); got != 1 {
		t.Fatalf("counts[done] = %d, want 1", got)
	}

	sections, ok := payload["sections"].(map[string]any)
	if !ok {
		t.Fatalf("sections payload missing in %#v", payload)
	}
	if got := int(sections["project_items_open"].(float64)); got != 1 {
		t.Fatalf("sections[project_items_open] = %d, want 1 (only the open project item; done excluded)", got)
	}
	if got := int(sections["people_open"].(float64)); got != 2 {
		t.Fatalf("sections[people_open] = %d, want 2 (Alice + Bob)", got)
	}
	if got := int(sections["drift_review"].(float64)); got != 1 {
		t.Fatalf("sections[drift_review] = %d, want 1 (review item with review_target set)", got)
	}
	if got := int(sections["dedup_review"].(float64)); got != 2 {
		t.Fatalf("sections[dedup_review] = %d, want 2 (the colliding source/source_ref pair)", got)
	}
	if got := int(sections["recent_meetings"].(float64)); got != 1 {
		t.Fatalf("sections[recent_meetings] = %d, want 1", got)
	}
}

// Section drill-down filter on the list endpoint scopes results to the
// targeted subset (project items / people / drift / dedup), so the sidebar
// rows behave as drill-down filters rather than placeholder buttons.
func TestItemListFilterSectionDrillsDownToProjectItems(t *testing.T) {
	app := newAuthedTestApp(t)

	if _, err := app.store.CreateItem("Plan outcome", store.ItemOptions{
		Kind:  store.ItemKindProject,
		State: store.ItemStateNext,
	}); err != nil {
		t.Fatalf("CreateItem(project) error: %v", err)
	}
	if _, err := app.store.CreateItem("Routine action", store.ItemOptions{
		State: store.ItemStateNext,
	}); err != nil {
		t.Fatalf("CreateItem(action) error: %v", err)
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/next?section=project_items", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	payload := decodeJSONResponse(t, rr)
	items, ok := payload["items"].([]any)
	if !ok {
		t.Fatalf("items payload = %#v", payload)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	first, _ := items[0].(map[string]any)
	if first["kind"] != store.ItemKindProject {
		t.Fatalf("items[0].kind = %v, want %q", first["kind"], store.ItemKindProject)
	}
}

func TestItemListFilterSectionRejectsUnknownValue(t *testing.T) {
	app := newAuthedTestApp(t)
	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/next?section=bogus", nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for invalid section", rr.Code)
	}
}

// Section drill-down for `recent_meetings` returns only review items linked
// to a meeting transcript or summary artifact created in the last seven days.
func TestItemListFilterSectionDrillsDownToRecentMeetings(t *testing.T) {
	app := newAuthedTestApp(t)

	transcriptPath := "/tmp/recent-transcript.md"
	transcriptTitle := "Recent transcript"
	transcript, err := app.store.CreateArtifact(store.ArtifactKindTranscript, &transcriptPath, nil, &transcriptTitle, nil)
	if err != nil {
		t.Fatalf("CreateArtifact(transcript) error: %v", err)
	}
	plainPath := "/tmp/plain.md"
	plainTitle := "Plain note"
	plain, err := app.store.CreateArtifact(store.ArtifactKindMarkdown, &plainPath, nil, &plainTitle, nil)
	if err != nil {
		t.Fatalf("CreateArtifact(plain) error: %v", err)
	}

	if _, err := app.store.CreateItem("Triage transcript", store.ItemOptions{
		State:      store.ItemStateReview,
		ArtifactID: &transcript.ID,
	}); err != nil {
		t.Fatalf("CreateItem(transcript) error: %v", err)
	}
	if _, err := app.store.CreateItem("Triage plain note", store.ItemOptions{
		State:      store.ItemStateReview,
		ArtifactID: &plain.ID,
	}); err != nil {
		t.Fatalf("CreateItem(plain) error: %v", err)
	}
	if _, err := app.store.CreateItem("Triage no artifact", store.ItemOptions{
		State: store.ItemStateReview,
	}); err != nil {
		t.Fatalf("CreateItem(no artifact) error: %v", err)
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/review?section=recent_meetings", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	payload := decodeJSONResponse(t, rr)
	items, ok := payload["items"].([]any)
	if !ok {
		t.Fatalf("items payload = %#v", payload)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1 (only the transcript-linked review item)", len(items))
	}
	first, _ := items[0].(map[string]any)
	if first["title"] != "Triage transcript" {
		t.Fatalf("items[0].title = %v, want 'Triage transcript'", first["title"])
	}
}
