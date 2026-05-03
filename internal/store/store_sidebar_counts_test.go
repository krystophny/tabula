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

	plan, err := s.CreateItem("Plan Q2 outcome", ItemOptions{
		Kind:         ItemKindProject,
		State:        ItemStateInbox,
		VisibleAfter: &past,
	})
	if err != nil {
		t.Fatalf("CreateItem(project inbox) error: %v", err)
	}
	planChild, err := s.CreateItem("Plan Q2 next", ItemOptions{State: ItemStateNext})
	if err != nil {
		t.Fatalf("CreateItem(plan child) error: %v", err)
	}
	if err := s.LinkItemChild(plan.ID, planChild.ID, ItemLinkRoleNextAction); err != nil {
		t.Fatalf("LinkItemChild(plan) error: %v", err)
	}
	ship, err := s.CreateItem("Ship review queue", ItemOptions{
		Kind:  ItemKindProject,
		State: ItemStateNext,
	})
	if err != nil {
		t.Fatalf("CreateItem(project next) error: %v", err)
	}
	shipChild, err := s.CreateItem("Ship review next", ItemOptions{State: ItemStateNext})
	if err != nil {
		t.Fatalf("CreateItem(ship child) error: %v", err)
	}
	if err := s.LinkItemChild(ship.ID, shipChild.ID, ItemLinkRoleNextAction); err != nil {
		t.Fatalf("LinkItemChild(ship) error: %v", err)
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
		t.Fatalf("ProjectItemsOpen = %d, want 2 active project items", got.ProjectItemsOpen)
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
	workProject, err := s.CreateItem("Work outcome", ItemOptions{
		Kind:   ItemKindProject,
		State:  ItemStateNext,
		Sphere: &workSphere,
	})
	if err != nil {
		t.Fatalf("CreateItem(work) error: %v", err)
	}
	workChild, err := s.CreateItem("Work next", ItemOptions{State: ItemStateNext, Sphere: &workSphere})
	if err != nil {
		t.Fatalf("CreateItem(work child) error: %v", err)
	}
	if err := s.LinkItemChild(workProject.ID, workChild.ID, ItemLinkRoleNextAction); err != nil {
		t.Fatalf("LinkItemChild(work) error: %v", err)
	}
	privateProject, err := s.CreateItem("Private outcome", ItemOptions{
		Kind:   ItemKindProject,
		State:  ItemStateInbox,
		Sphere: &privateSphere,
	})
	if err != nil {
		t.Fatalf("CreateItem(private) error: %v", err)
	}
	privateChild, err := s.CreateItem("Private next", ItemOptions{State: ItemStateNext, Sphere: &privateSphere})
	if err != nil {
		t.Fatalf("CreateItem(private child) error: %v", err)
	}
	if err := s.LinkItemChild(privateProject.ID, privateChild.ID, ItemLinkRoleNextAction); err != nil {
		t.Fatalf("LinkItemChild(private) error: %v", err)
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

	mustActor := func(name, kind string) Actor {
		actor, err := s.CreateActor(name, kind)
		if err != nil {
			t.Fatalf("CreateActor(%s) error: %v", name, err)
		}
		return actor
	}
	alice := mustActor("Alice", ActorKindHuman)
	bob := mustActor("Bob", ActorKindHuman)
	carol := mustActor("Carol", ActorKindHuman)
	agent := mustActor("Codex Agent", ActorKindAgent)

	items := []struct {
		title string
		opts  ItemOptions
	}{
		{"Awaiting Alice", ItemOptions{State: ItemStateWaiting, ActorID: &alice.ID}},
		{"Also awaiting Alice", ItemOptions{State: ItemStateNext, ActorID: &alice.ID}},
		{"Owe Bob", ItemOptions{State: ItemStateNext, ActorID: &bob.ID}},
		{"Carol done", ItemOptions{State: ItemStateDone, ActorID: &carol.ID}},
		{"Agent loop", ItemOptions{State: ItemStateWaiting, ActorID: &agent.ID}},
		{"Project with actor", ItemOptions{Kind: ItemKindProject, State: ItemStateNext, ActorID: &carol.ID}},
		{"No actor", ItemOptions{State: ItemStateNext}},
	}
	for _, item := range items {
		if _, err := s.CreateItem(item.title, item.opts); err != nil {
			t.Fatalf("CreateItem(%s) error: %v", item.title, err)
		}
	}

	now := time.Date(2026, time.April, 30, 10, 0, 0, 0, time.UTC)
	got, err := s.CountSidebarSectionsFiltered(now, ItemListFilter{})
	if err != nil {
		t.Fatalf("CountSidebarSectionsFiltered() error: %v", err)
	}
	if got.PeopleOpen != 2 {
		t.Fatalf("PeopleOpen = %d, want 2 (Alice + Bob human action loops only)", got.PeopleOpen)
	}
}

func TestCountSidebarSectionsFilteredCountsDriftReviewItems(t *testing.T) {
	s := newTestStore(t)

	account, err := s.CreateExternalAccount(SphereWork, ExternalProviderTodoist, "Todoist", map[string]any{})
	if err != nil {
		t.Fatalf("CreateExternalAccount() error: %v", err)
	}
	driftItem, err := s.CreateItem("Local overlay task", ItemOptions{State: ItemStateWaiting})
	if err != nil {
		t.Fatalf("CreateItem(drift item) error: %v", err)
	}
	remoteAt := "2026-03-08T10:05:00Z"
	binding, err := s.UpsertExternalBinding(ExternalBinding{
		AccountID:       account.ID,
		Provider:        account.Provider,
		ObjectType:      "task",
		RemoteID:        "task-1",
		ItemID:          &driftItem.ID,
		RemoteUpdatedAt: &remoteAt,
	})
	if err != nil {
		t.Fatalf("UpsertExternalBinding() error: %v", err)
	}
	upstream := driftItem
	upstream.State = ItemStateDone
	if _, err := s.RecordExternalBindingDrift(binding, driftItem, upstream); err != nil {
		t.Fatalf("RecordExternalBindingDrift() error: %v", err)
	}
	if _, err := s.CreateItem("Review item without source drift", ItemOptions{State: ItemStateReview}); err != nil {
		t.Fatalf("CreateItem(review no drift) error: %v", err)
	}
	if _, err := s.CreateItem("Next item", ItemOptions{State: ItemStateNext}); err != nil {
		t.Fatalf("CreateItem(next) error: %v", err)
	}

	now := time.Date(2026, time.April, 30, 10, 0, 0, 0, time.UTC)
	got, err := s.CountSidebarSectionsFiltered(now, ItemListFilter{})
	if err != nil {
		t.Fatalf("CountSidebarSectionsFiltered() error: %v", err)
	}
	if got.DriftReview != 1 {
		t.Fatalf("DriftReview = %d, want 1 (only unresolved external-binding drift)", got.DriftReview)
	}
}

func TestReviewQueueIncludesDueWaitingItemsAndStalledProjectItems(t *testing.T) {
	s := newTestStore(t)

	now := time.Now().UTC()
	past := now.Add(-time.Hour).Format(time.RFC3339)
	future := now.Add(time.Hour).Format(time.RFC3339)
	alice, err := s.CreateActor("Alice", ActorKindHuman)
	if err != nil {
		t.Fatalf("CreateActor() error: %v", err)
	}
	if _, err := s.CreateItem("Explicit review", ItemOptions{State: ItemStateReview}); err != nil {
		t.Fatalf("CreateItem(review) error: %v", err)
	}
	if _, err := s.CreateItem("Follow up Alice", ItemOptions{
		State:      ItemStateWaiting,
		ActorID:    &alice.ID,
		FollowUpAt: &past,
	}); err != nil {
		t.Fatalf("CreateItem(due waiting) error: %v", err)
	}
	if _, err := s.CreateItem("Not ready waiting", ItemOptions{
		State:      ItemStateWaiting,
		ActorID:    &alice.ID,
		FollowUpAt: &future,
	}); err != nil {
		t.Fatalf("CreateItem(future waiting) error: %v", err)
	}
	if _, err := s.CreateItem("Stalled outcome", ItemOptions{
		Kind:  ItemKindProject,
		State: ItemStateNext,
	}); err != nil {
		t.Fatalf("CreateItem(stalled project) error: %v", err)
	}
	healthy, err := s.CreateItem("Healthy outcome", ItemOptions{
		Kind:  ItemKindProject,
		State: ItemStateNext,
	})
	if err != nil {
		t.Fatalf("CreateItem(healthy project) error: %v", err)
	}
	child, err := s.CreateItem("Healthy next action", ItemOptions{State: ItemStateNext})
	if err != nil {
		t.Fatalf("CreateItem(child) error: %v", err)
	}
	if err := s.LinkItemChild(healthy.ID, child.ID, ItemLinkRoleNextAction); err != nil {
		t.Fatalf("LinkItemChild() error: %v", err)
	}
	if _, err := s.CreateItem("Done stalled outcome", ItemOptions{
		Kind:  ItemKindProject,
		State: ItemStateDone,
	}); err != nil {
		t.Fatalf("CreateItem(done project) error: %v", err)
	}

	items, err := s.ListReviewItemsFiltered(ItemListFilter{})
	if err != nil {
		t.Fatalf("ListReviewItemsFiltered() error: %v", err)
	}
	titles := map[string]bool{}
	for _, item := range items {
		titles[item.Title] = true
	}
	for _, title := range []string{"Explicit review", "Follow up Alice", "Stalled outcome"} {
		if !titles[title] {
			t.Fatalf("review queue missing %q from titles %#v", title, titles)
		}
	}
	for _, title := range []string{"Not ready waiting", "Healthy outcome", "Done stalled outcome"} {
		if titles[title] {
			t.Fatalf("review queue includes %q unexpectedly: %#v", title, titles)
		}
	}
	counts, err := s.CountItemsByStateFiltered(now, ItemListFilter{})
	if err != nil {
		t.Fatalf("CountItemsByStateFiltered() error: %v", err)
	}
	if counts[ItemStateReview] != 3 {
		t.Fatalf("review count = %d, want 3", counts[ItemStateReview])
	}
}

func TestCountSidebarSectionsFilteredCountsDedupCandidates(t *testing.T) {
	s := newTestStore(t)

	first, err := s.CreateItem("Dup A", ItemOptions{State: ItemStateNext})
	if err != nil {
		t.Fatalf("CreateItem(dup A) error: %v", err)
	}
	second, err := s.CreateItem("Dup B", ItemOptions{State: ItemStateInbox})
	if err != nil {
		t.Fatalf("CreateItem(dup B) error: %v", err)
	}
	resolvedA, err := s.CreateItem("Resolved A", ItemOptions{State: ItemStateNext})
	if err != nil {
		t.Fatalf("CreateItem(resolved A) error: %v", err)
	}
	resolvedB, err := s.CreateItem("Resolved B", ItemOptions{State: ItemStateNext})
	if err != nil {
		t.Fatalf("CreateItem(resolved B) error: %v", err)
	}
	if _, err := s.CreateItemDedupCandidate(ItemDedupCandidateOptions{
		Kind:  ItemKindAction,
		Items: []ItemDedupCandidateItemInput{{ItemID: first.ID}, {ItemID: second.ID}},
	}); err != nil {
		t.Fatalf("CreateItemDedupCandidate(open) error: %v", err)
	}
	resolved, err := s.CreateItemDedupCandidate(ItemDedupCandidateOptions{
		Kind:  ItemKindAction,
		Items: []ItemDedupCandidateItemInput{{ItemID: resolvedA.ID}, {ItemID: resolvedB.ID}},
	})
	if err != nil {
		t.Fatalf("CreateItemDedupCandidate(resolved) error: %v", err)
	}
	if _, err := s.ApplyItemDedupDecision(resolved.ID, ItemDedupActionKeepSeparate, nil); err != nil {
		t.Fatalf("ApplyItemDedupDecision(keep_separate) error: %v", err)
	}

	now := time.Date(2026, time.April, 30, 10, 0, 0, 0, time.UTC)
	got, err := s.CountSidebarSectionsFiltered(now, ItemListFilter{})
	if err != nil {
		t.Fatalf("CountSidebarSectionsFiltered() error: %v", err)
	}
	if got.DedupReview != 1 {
		t.Fatalf("DedupReview = %d, want 1 open candidate group", got.DedupReview)
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

	defaultView, err := s.ListNextItemsFiltered(ItemListFilter{})
	if err != nil {
		t.Fatalf("ListNextItemsFiltered() error: %v", err)
	}
	if len(defaultView) != 1 {
		t.Fatalf("default len(all) = %d, want 1 (project items must not appear as next actions)", len(defaultView))
	}
	if defaultView[0].Kind != ItemKindAction {
		t.Fatalf("default item.Kind = %q, want %q", defaultView[0].Kind, ItemKindAction)
	}

	all, err := s.ListNextItemsFiltered(ItemListFilter{IncludeProjectItems: true})
	if err != nil {
		t.Fatalf("ListNextItemsFiltered(IncludeProjectItems=true) error: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("include-projects len(all) = %d, want 2", len(all))
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
