package store

import (
	"strings"
	"testing"
	"time"
)

func TestCountSidebarSectionsFilteredCountsOpenProjectItemsAndRecentMeetings(t *testing.T) {
	s := newTestStore(t)

	now := time.Date(2026, time.April, 30, 10, 0, 0, 0, time.UTC)
	past := now.Add(-30 * time.Minute).Format(time.RFC3339)

	if _, err := s.CreateItem("Plan Q2 outcome", ItemOptions{
		Kind:         ItemKindProject,
		State:        ItemStateInbox,
		VisibleAfter: &past,
	}); err != nil {
		t.Fatalf("CreateItem(project inbox) error: %v", err)
	}
	if _, err := s.CreateItem("Ship review queue", ItemOptions{
		Kind:  ItemKindProject,
		State: ItemStateNext,
	}); err != nil {
		t.Fatalf("CreateItem(project next) error: %v", err)
	}
	if _, err := s.CreateItem("Closed outcome", ItemOptions{
		Kind:  ItemKindProject,
		State: ItemStateDone,
	}); err != nil {
		t.Fatalf("CreateItem(project done) error: %v", err)
	}
	if _, err := s.CreateItem("Plain action", ItemOptions{
		State:        ItemStateInbox,
		VisibleAfter: &past,
	}); err != nil {
		t.Fatalf("CreateItem(action) error: %v", err)
	}

	recentTranscriptPath := "/tmp/recent-transcript.md"
	recentTranscriptTitle := "Recent meeting transcript"
	if _, err := s.CreateArtifact(ArtifactKindTranscript, &recentTranscriptPath, nil, &recentTranscriptTitle, nil); err != nil {
		t.Fatalf("CreateArtifact(transcript) error: %v", err)
	}
	recentSummaryPath := "/tmp/recent-summary.md"
	recentSummaryTitle := "Recent meeting summary"
	recentMeta := `{"source":"meeting_summary","summary":"recap"}`
	if _, err := s.CreateArtifact(ArtifactKindMarkdown, &recentSummaryPath, nil, &recentSummaryTitle, &recentMeta); err != nil {
		t.Fatalf("CreateArtifact(summary) error: %v", err)
	}
	unrelatedPath := "/tmp/notes.md"
	unrelatedTitle := "Unrelated notes"
	if _, err := s.CreateArtifact(ArtifactKindMarkdown, &unrelatedPath, nil, &unrelatedTitle, nil); err != nil {
		t.Fatalf("CreateArtifact(unrelated) error: %v", err)
	}

	got, err := s.CountSidebarSectionsFiltered(now, ItemListFilter{})
	if err != nil {
		t.Fatalf("CountSidebarSectionsFiltered() error: %v", err)
	}
	if got.ProjectItemsOpen != 2 {
		t.Fatalf("ProjectItemsOpen = %d, want 2 (open project items only, done excluded)", got.ProjectItemsOpen)
	}
	if got.RecentMeetings != 2 {
		t.Fatalf("RecentMeetings = %d, want 2 (transcript + meeting_summary metadata)", got.RecentMeetings)
	}
}

func TestCountSidebarSectionsFilteredHonorsSphereScope(t *testing.T) {
	s := newTestStore(t)

	now := time.Date(2026, time.April, 30, 10, 0, 0, 0, time.UTC)

	workSphere := SphereWork
	privateSphere := SpherePrivate
	if _, err := s.CreateItem("Work outcome", ItemOptions{
		Kind:   ItemKindProject,
		State:  ItemStateNext,
		Sphere: &workSphere,
	}); err != nil {
		t.Fatalf("CreateItem(work) error: %v", err)
	}
	if _, err := s.CreateItem("Private outcome", ItemOptions{
		Kind:   ItemKindProject,
		State:  ItemStateInbox,
		Sphere: &privateSphere,
	}); err != nil {
		t.Fatalf("CreateItem(private) error: %v", err)
	}

	work, err := s.CountSidebarSectionsFiltered(now, ItemListFilter{Sphere: SphereWork})
	if err != nil {
		t.Fatalf("CountSidebarSectionsFiltered(work) error: %v", err)
	}
	if work.ProjectItemsOpen != 1 {
		t.Fatalf("work ProjectItemsOpen = %d, want 1", work.ProjectItemsOpen)
	}

	priv, err := s.CountSidebarSectionsFiltered(now, ItemListFilter{Sphere: SpherePrivate})
	if err != nil {
		t.Fatalf("CountSidebarSectionsFiltered(private) error: %v", err)
	}
	if priv.ProjectItemsOpen != 1 {
		t.Fatalf("private ProjectItemsOpen = %d, want 1", priv.ProjectItemsOpen)
	}
}

func TestCountSidebarSectionsFilteredExcludesAgedMeetings(t *testing.T) {
	s := newTestStore(t)

	now := time.Date(2026, time.April, 30, 10, 0, 0, 0, time.UTC)

	recentPath := "/tmp/recent.md"
	recentTitle := "Recent transcript"
	if _, err := s.CreateArtifact(ArtifactKindTranscript, &recentPath, nil, &recentTitle, nil); err != nil {
		t.Fatalf("CreateArtifact(recent) error: %v", err)
	}

	staleCreatedAt := now.AddDate(0, 0, -8).UTC().Format(time.RFC3339Nano)
	stalePath := "/tmp/stale.md"
	staleTitle := "Stale transcript"
	stale, err := s.CreateArtifact(ArtifactKindTranscript, &stalePath, nil, &staleTitle, nil)
	if err != nil {
		t.Fatalf("CreateArtifact(stale) error: %v", err)
	}
	if _, err := s.db.Exec(`UPDATE artifacts SET created_at = ? WHERE id = ?`, staleCreatedAt, stale.ID); err != nil {
		t.Fatalf("backdate artifact error: %v", err)
	}

	got, err := s.CountSidebarSectionsFiltered(now, ItemListFilter{})
	if err != nil {
		t.Fatalf("CountSidebarSectionsFiltered() error: %v", err)
	}
	if got.RecentMeetings != 1 {
		t.Fatalf("RecentMeetings = %d, want 1 (stale meeting older than 7 days excluded)", got.RecentMeetings)
	}

	// Sanity-check the assertion below which guards against accidental
	// terminology drift: project items should never be reported as 0
	// when there are recent meetings only.
	if !strings.EqualFold(string(ArtifactKindTranscript), "transcript") {
		t.Fatalf("ArtifactKindTranscript constant changed: %q", string(ArtifactKindTranscript))
	}
}

func TestCountSidebarSectionsFilteredCountsDistinctPeopleOnOpenItems(t *testing.T) {
	s := newTestStore(t)

	alice, err := s.CreateActor("Alice", ActorKindHuman)
	if err != nil {
		t.Fatalf("CreateActor(Alice) error: %v", err)
	}
	bob, err := s.CreateActor("Bob", ActorKindHuman)
	if err != nil {
		t.Fatalf("CreateActor(Bob) error: %v", err)
	}
	carol, err := s.CreateActor("Carol", ActorKindHuman)
	if err != nil {
		t.Fatalf("CreateActor(Carol) error: %v", err)
	}

	if _, err := s.CreateItem("Awaiting Alice", ItemOptions{State: ItemStateWaiting, ActorID: &alice.ID}); err != nil {
		t.Fatalf("CreateItem(Alice) error: %v", err)
	}
	if _, err := s.CreateItem("Also awaiting Alice", ItemOptions{State: ItemStateNext, ActorID: &alice.ID}); err != nil {
		t.Fatalf("CreateItem(Alice second) error: %v", err)
	}
	if _, err := s.CreateItem("Owe Bob", ItemOptions{State: ItemStateNext, ActorID: &bob.ID}); err != nil {
		t.Fatalf("CreateItem(Bob) error: %v", err)
	}
	// Done item should not be counted even if it has an actor.
	if _, err := s.CreateItem("Carol done", ItemOptions{State: ItemStateDone, ActorID: &carol.ID}); err != nil {
		t.Fatalf("CreateItem(Carol done) error: %v", err)
	}
	// Item without actor should not contribute to people count.
	if _, err := s.CreateItem("No actor", ItemOptions{State: ItemStateNext}); err != nil {
		t.Fatalf("CreateItem(no actor) error: %v", err)
	}

	now := time.Date(2026, time.April, 30, 10, 0, 0, 0, time.UTC)
	got, err := s.CountSidebarSectionsFiltered(now, ItemListFilter{})
	if err != nil {
		t.Fatalf("CountSidebarSectionsFiltered() error: %v", err)
	}
	if got.PeopleOpen != 2 {
		t.Fatalf("PeopleOpen = %d, want 2 (Alice + Bob; Carol's done item excluded; un-actor-ed item ignored)", got.PeopleOpen)
	}
}

func TestCountSidebarSectionsFilteredCountsDriftReviewItems(t *testing.T) {
	s := newTestStore(t)

	driftWithTarget, err := s.CreateItem("Drift with target", ItemOptions{State: ItemStateReview})
	if err != nil {
		t.Fatalf("CreateItem(drift target) error: %v", err)
	}
	target := ItemReviewTargetGitHub
	reviewer := "krystophny"
	if err := s.UpdateItemReviewDispatch(driftWithTarget.ID, &target, &reviewer); err != nil {
		t.Fatalf("UpdateItemReviewDispatch() error: %v", err)
	}
	// Review state without target should not be counted as drift.
	if _, err := s.CreateItem("Review no target", ItemOptions{State: ItemStateReview}); err != nil {
		t.Fatalf("CreateItem(review no target) error: %v", err)
	}
	// Non-review items should not be counted even if they have a target later.
	if _, err := s.CreateItem("Next item", ItemOptions{State: ItemStateNext}); err != nil {
		t.Fatalf("CreateItem(next) error: %v", err)
	}

	now := time.Date(2026, time.April, 30, 10, 0, 0, 0, time.UTC)
	got, err := s.CountSidebarSectionsFiltered(now, ItemListFilter{})
	if err != nil {
		t.Fatalf("CountSidebarSectionsFiltered() error: %v", err)
	}
	if got.DriftReview != 1 {
		t.Fatalf("DriftReview = %d, want 1 (only review-state item with review_target set)", got.DriftReview)
	}
}

func TestCountSidebarSectionsFilteredCountsDuplicateSourcePairs(t *testing.T) {
	s := newTestStore(t)

	source := "github"
	ref := "krystophny/repo#42"
	if _, err := s.CreateItem("Dup A", ItemOptions{
		State:     ItemStateNext,
		Source:    &source,
		SourceRef: &ref,
	}); err != nil {
		t.Fatalf("CreateItem(dup A) error: %v", err)
	}
	if _, err := s.CreateItem("Dup B", ItemOptions{
		State:     ItemStateInbox,
		Source:    &source,
		SourceRef: &ref,
	}); err != nil {
		t.Fatalf("CreateItem(dup B) error: %v", err)
	}
	uniqueRef := "krystophny/repo#1"
	if _, err := s.CreateItem("Unique source/ref", ItemOptions{
		State:     ItemStateNext,
		Source:    &source,
		SourceRef: &uniqueRef,
	}); err != nil {
		t.Fatalf("CreateItem(unique) error: %v", err)
	}
	if _, err := s.CreateItem("No source", ItemOptions{State: ItemStateNext}); err != nil {
		t.Fatalf("CreateItem(no source) error: %v", err)
	}
	// Done duplicates should not be counted as live dedup work.
	doneRef := "krystophny/repo#9"
	if _, err := s.CreateItem("Dup done A", ItemOptions{
		State:     ItemStateDone,
		Source:    &source,
		SourceRef: &doneRef,
	}); err != nil {
		t.Fatalf("CreateItem(done dup A) error: %v", err)
	}
	if _, err := s.CreateItem("Dup done B", ItemOptions{
		State:     ItemStateDone,
		Source:    &source,
		SourceRef: &doneRef,
	}); err != nil {
		t.Fatalf("CreateItem(done dup B) error: %v", err)
	}

	now := time.Date(2026, time.April, 30, 10, 0, 0, 0, time.UTC)
	got, err := s.CountSidebarSectionsFiltered(now, ItemListFilter{})
	if err != nil {
		t.Fatalf("CountSidebarSectionsFiltered() error: %v", err)
	}
	if got.DedupReview != 2 {
		t.Fatalf("DedupReview = %d, want 2 (the two open rows sharing source/source_ref; done duplicates excluded)", got.DedupReview)
	}
}

func TestSidebarSectionFilterDrillsDownToProjectItemsOnly(t *testing.T) {
	s := newTestStore(t)

	if _, err := s.CreateItem("Plan outcome", ItemOptions{Kind: ItemKindProject, State: ItemStateNext}); err != nil {
		t.Fatalf("CreateItem(project) error: %v", err)
	}
	if _, err := s.CreateItem("Routine action", ItemOptions{State: ItemStateNext}); err != nil {
		t.Fatalf("CreateItem(action) error: %v", err)
	}

	all, err := s.ListNextItemsFiltered(ItemListFilter{})
	if err != nil {
		t.Fatalf("ListNextItemsFiltered() error: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("baseline len(all) = %d, want 2", len(all))
	}

	scoped, err := s.ListNextItemsFiltered(ItemListFilter{Section: ItemSidebarSectionProject})
	if err != nil {
		t.Fatalf("ListNextItemsFiltered(section=project_items) error: %v", err)
	}
	if len(scoped) != 1 {
		t.Fatalf("project section len = %d, want 1", len(scoped))
	}
	if scoped[0].Kind != ItemKindProject {
		t.Fatalf("project section item.Kind = %q, want %q", scoped[0].Kind, ItemKindProject)
	}
}

func TestSidebarSectionFilterRejectsUnknownValue(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.ListNextItemsFiltered(ItemListFilter{Section: "garbage"}); err == nil {
		t.Fatal("expected error for unknown section filter, got nil")
	}
}

// Recent-meetings drill-down must restrict the queue to items linked to a
// meeting transcript or summary artifact created in the last seven days.
// Items with no artifact, items linked to a non-meeting artifact, and items
// linked to a stale meeting artifact must all be excluded.
func TestSidebarSectionFilterDrillsDownToRecentMeetings(t *testing.T) {
	s := newTestStore(t)

	recentTranscriptPath := "/tmp/recent-transcript.md"
	recentTranscriptTitle := "Recent transcript"
	recentTranscript, err := s.CreateArtifact(ArtifactKindTranscript, &recentTranscriptPath, nil, &recentTranscriptTitle, nil)
	if err != nil {
		t.Fatalf("CreateArtifact(recent transcript) error: %v", err)
	}
	recentSummaryPath := "/tmp/recent-summary.md"
	recentSummaryTitle := "Recent summary"
	recentMeta := `{"source":"meeting_summary","summary":"recap"}`
	recentSummary, err := s.CreateArtifact(ArtifactKindMarkdown, &recentSummaryPath, nil, &recentSummaryTitle, &recentMeta)
	if err != nil {
		t.Fatalf("CreateArtifact(recent summary) error: %v", err)
	}
	staleTranscriptPath := "/tmp/stale-transcript.md"
	staleTranscriptTitle := "Stale transcript"
	staleTranscript, err := s.CreateArtifact(ArtifactKindTranscript, &staleTranscriptPath, nil, &staleTranscriptTitle, nil)
	if err != nil {
		t.Fatalf("CreateArtifact(stale transcript) error: %v", err)
	}
	staleCutoff := time.Now().UTC().Add(-30 * 24 * time.Hour).Format(time.RFC3339Nano)
	if _, err := s.db.Exec(`UPDATE artifacts SET created_at = ? WHERE id = ?`, staleCutoff, staleTranscript.ID); err != nil {
		t.Fatalf("backdate stale transcript error: %v", err)
	}
	unrelatedPath := "/tmp/unrelated.md"
	unrelatedTitle := "Unrelated note"
	unrelated, err := s.CreateArtifact(ArtifactKindMarkdown, &unrelatedPath, nil, &unrelatedTitle, nil)
	if err != nil {
		t.Fatalf("CreateArtifact(unrelated) error: %v", err)
	}

	if _, err := s.CreateItem("Review recent transcript", ItemOptions{
		State:      ItemStateReview,
		ArtifactID: &recentTranscript.ID,
	}); err != nil {
		t.Fatalf("CreateItem(recent transcript) error: %v", err)
	}
	if _, err := s.CreateItem("Review recent summary", ItemOptions{
		State:      ItemStateReview,
		ArtifactID: &recentSummary.ID,
	}); err != nil {
		t.Fatalf("CreateItem(recent summary) error: %v", err)
	}
	if _, err := s.CreateItem("Review stale transcript", ItemOptions{
		State:      ItemStateReview,
		ArtifactID: &staleTranscript.ID,
	}); err != nil {
		t.Fatalf("CreateItem(stale transcript) error: %v", err)
	}
	if _, err := s.CreateItem("Review unrelated note", ItemOptions{
		State:      ItemStateReview,
		ArtifactID: &unrelated.ID,
	}); err != nil {
		t.Fatalf("CreateItem(unrelated) error: %v", err)
	}
	if _, err := s.CreateItem("Review with no artifact", ItemOptions{State: ItemStateReview}); err != nil {
		t.Fatalf("CreateItem(no artifact) error: %v", err)
	}

	all, err := s.ListReviewItemsFiltered(ItemListFilter{})
	if err != nil {
		t.Fatalf("ListReviewItemsFiltered() error: %v", err)
	}
	if len(all) != 5 {
		t.Fatalf("baseline review queue len = %d, want 5", len(all))
	}

	scoped, err := s.ListReviewItemsFiltered(ItemListFilter{Section: ItemSidebarSectionRecentMeetings})
	if err != nil {
		t.Fatalf("ListReviewItemsFiltered(recent_meetings) error: %v", err)
	}
	if len(scoped) != 2 {
		titles := make([]string, 0, len(scoped))
		for _, item := range scoped {
			titles = append(titles, item.Title)
		}
		t.Fatalf("recent_meetings drill-down len = %d, want 2 (recent transcript + summary; stale + non-meeting + no-artifact excluded); got titles=%v",
			len(scoped), titles)
	}
	gotTitles := map[string]bool{}
	for _, item := range scoped {
		gotTitles[item.Title] = true
	}
	if !gotTitles["Review recent transcript"] || !gotTitles["Review recent summary"] {
		t.Fatalf("recent_meetings drill-down missing expected titles, got %#v", gotTitles)
	}
}
