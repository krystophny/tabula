package web

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/sloppy-org/slopshell/internal/email"
	"github.com/sloppy-org/slopshell/internal/providerdata"
	"github.com/sloppy-org/slopshell/internal/store"
)

func TestItemCountsExposesSidebarSectionCountsAlongsidePerStateCounts(t *testing.T) {
	app := newAuthedTestApp(t)

	seedSidebarCountsFixture(t, app)

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
		t.Fatalf("sections[drift_review] = %d, want 1 (unresolved external-binding drift)", got)
	}
	if got := int(sections["dedup_review"].(float64)); got != 2 {
		t.Fatalf("sections[dedup_review] = %d, want 2 (the colliding source/source_ref pair)", got)
	}
	if got := int(sections["recent_meetings"].(float64)); got != 1 {
		t.Fatalf("sections[recent_meetings] = %d, want 1", got)
	}
}

func seedSidebarCountsFixture(t *testing.T, app *App) {
	t.Helper()

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

	seedDriftReviewFixture(t, app)

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
}

func seedDriftReviewFixture(t *testing.T, app *App) store.ExternalBindingDrift {
	t.Helper()

	account, err := app.store.CreateExternalAccount(store.SphereWork, store.ExternalProviderTodoist, "Todoist", map[string]any{})
	if err != nil {
		t.Fatalf("CreateExternalAccount() error: %v", err)
	}
	driftItem, err := app.store.CreateItem("Drifted task", store.ItemOptions{State: store.ItemStateWaiting})
	if err != nil {
		t.Fatalf("CreateItem(drift) error: %v", err)
	}
	remoteAt := "2026-03-08T10:05:00Z"
	container := "Errands"
	binding, err := app.store.UpsertExternalBinding(store.ExternalBinding{
		AccountID:       account.ID,
		Provider:        account.Provider,
		ObjectType:      "task",
		RemoteID:        "task-1",
		ItemID:          &driftItem.ID,
		ContainerRef:    &container,
		RemoteUpdatedAt: &remoteAt,
	})
	if err != nil {
		t.Fatalf("UpsertExternalBinding() error: %v", err)
	}
	upstream := driftItem
	upstream.State = store.ItemStateDone
	upstream.Title = "Drifted task upstream"
	drift, err := app.store.RecordExternalBindingDrift(binding, driftItem, upstream)
	if err != nil {
		t.Fatalf("RecordExternalBindingDrift() error: %v", err)
	}
	return drift
}

func TestItemReviewDriftQueueAndActions(t *testing.T) {
	app := newAuthedTestApp(t)
	drift := seedDriftReviewFixture(t, app)

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/review?section=drift", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("drift list status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	items, ok := decodeJSONResponse(t, rr)["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("drift list items = %#v, want one row", decodeJSONResponse(t, rr)["items"])
	}
	row, _ := items[0].(map[string]any)
	if row["local_state"] != store.ItemStateWaiting || row["upstream_state"] != store.ItemStateDone {
		t.Fatalf("drift row states = %#v, want local waiting/upstream done", row)
	}
	if row["source_binding"] != "todoist:task:task-1" || row["source_container"] != "Errands" {
		t.Fatalf("drift source metadata = %#v, want binding and container", row)
	}

	rr = doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/drift/"+strconv.FormatInt(drift.ID, 10)+"/take_upstream", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("take upstream status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	item, err := app.store.GetItem(*drift.ItemID)
	if err != nil {
		t.Fatalf("GetItem() error: %v", err)
	}
	if item.State != store.ItemStateDone || item.WorkspaceID != nil {
		t.Fatalf("item after take upstream = state %q workspace %v, want done with workspace unchanged", item.State, item.WorkspaceID)
	}
}

func TestItemDriftActionsResolveQueueEntries(t *testing.T) {
	for _, action := range []string{"keep_local", "dismiss"} {
		t.Run(action, func(t *testing.T) {
			app := newAuthedTestApp(t)
			drift := seedDriftReviewFixture(t, app)

			rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/drift/"+strconv.FormatInt(drift.ID, 10)+"/"+action, nil)
			if rr.Code != http.StatusOK {
				t.Fatalf("%s status = %d, want 200: %s", action, rr.Code, rr.Body.String())
			}
			drifts, err := app.store.ListUnresolvedExternalBindingDrifts(store.ItemListFilter{})
			if err != nil {
				t.Fatalf("ListUnresolvedExternalBindingDrifts() error: %v", err)
			}
			if len(drifts) != 0 {
				t.Fatalf("unresolved drift count after %s = %d, want 0", action, len(drifts))
			}
		})
	}
}

func TestItemDriftReingestRefreshesSourceBeforeResolving(t *testing.T) {
	app := newAuthedTestApp(t)
	account, err := app.store.CreateExternalAccount(store.SphereWork, store.ExternalProviderGmail, "Gmail", nil)
	if err != nil {
		t.Fatalf("CreateExternalAccount() error: %v", err)
	}
	local, err := app.store.CreateItem("Local overlay", store.ItemOptions{State: store.ItemStateWaiting})
	if err != nil {
		t.Fatalf("CreateItem(local) error: %v", err)
	}
	remoteAt := "2026-03-08T10:05:00Z"
	binding, err := app.store.UpsertExternalBinding(store.ExternalBinding{
		AccountID:       account.ID,
		Provider:        account.Provider,
		ObjectType:      emailBindingObjectType,
		RemoteID:        "gmail-drift",
		ItemID:          &local.ID,
		RemoteUpdatedAt: &remoteAt,
	})
	if err != nil {
		t.Fatalf("UpsertExternalBinding() error: %v", err)
	}
	upstream := local
	upstream.Title = "Remote refreshed"
	upstream.State = store.ItemStateInbox
	drift, err := app.store.RecordExternalBindingDrift(binding, local, upstream)
	if err != nil {
		t.Fatalf("RecordExternalBindingDrift() error: %v", err)
	}
	provider := &fakeEmailSyncProvider{
		listFunc: func(opts email.SearchOptions) ([]string, error) {
			if opts.Folder == "INBOX" || !opts.Since.IsZero() {
				return []string{"gmail-drift"}, nil
			}
			return nil, nil
		},
		messages: map[string]*providerdata.EmailMessage{
			"gmail-drift": {
				ID:         "gmail-drift",
				ThreadID:   "thread-gmail-drift",
				Subject:    "Remote refreshed",
				Sender:     "Source <source@example.com>",
				Recipients: []string{"me@example.com"},
				Date:       time.Date(2026, time.March, 8, 10, 5, 0, 0, time.UTC),
				Labels:     []string{"INBOX"},
			},
		},
	}
	app.newEmailSyncProvider = func(context.Context, store.ExternalAccount) (emailSyncProvider, error) {
		return provider, nil
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/drift/"+strconv.FormatInt(drift.ID, 10)+"/reingest_source", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("reingest status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	item, err := app.store.GetItem(local.ID)
	if err != nil {
		t.Fatalf("GetItem() error: %v", err)
	}
	if item.Title != "Remote refreshed" || item.State != store.ItemStateInbox {
		t.Fatalf("item after reingest = title %q state %q, want refreshed inbox", item.Title, item.State)
	}
	drifts, err := app.store.ListUnresolvedExternalBindingDrifts(store.ItemListFilter{})
	if err != nil {
		t.Fatalf("ListUnresolvedExternalBindingDrifts() error: %v", err)
	}
	if len(drifts) != 0 {
		t.Fatalf("unresolved drift count after reingest = %d, want 0", len(drifts))
	}
	if len(provider.listCalls) == 0 {
		t.Fatal("reingest_source did not call the source provider")
	}
}

func TestDismissedDriftReappearsOnlyAfterUpstreamRevisionChanges(t *testing.T) {
	app := newAuthedTestApp(t)
	drift := seedDriftReviewFixture(t, app)

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/items/drift/"+strconv.FormatInt(drift.ID, 10)+"/dismiss", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("dismiss status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	binding, err := app.store.GetBindingByRemote(drift.AccountID, drift.Provider, drift.ObjectType, drift.RemoteID)
	if err != nil {
		t.Fatalf("GetBindingByRemote() error: %v", err)
	}
	item, err := app.store.GetItem(*drift.ItemID)
	if err != nil {
		t.Fatalf("GetItem() error: %v", err)
	}
	upstream := item
	upstream.State = store.ItemStateDone
	if _, err := app.store.RecordExternalBindingDrift(binding, item, upstream); err != nil {
		t.Fatalf("RecordExternalBindingDrift(same revision) error: %v", err)
	}
	drifts, err := app.store.ListUnresolvedExternalBindingDrifts(store.ItemListFilter{})
	if err != nil {
		t.Fatalf("ListUnresolvedExternalBindingDrifts(same revision) error: %v", err)
	}
	if len(drifts) != 0 {
		t.Fatalf("same upstream revision reappeared with %d drifts, want 0", len(drifts))
	}

	nextRemoteAt := "2026-03-08T10:10:00Z"
	binding.RemoteUpdatedAt = &nextRemoteAt
	if _, err := app.store.RecordExternalBindingDrift(binding, item, upstream); err != nil {
		t.Fatalf("RecordExternalBindingDrift(new revision) error: %v", err)
	}
	drifts, err = app.store.ListUnresolvedExternalBindingDrifts(store.ItemListFilter{})
	if err != nil {
		t.Fatalf("ListUnresolvedExternalBindingDrifts(new revision) error: %v", err)
	}
	if len(drifts) != 1 {
		t.Fatalf("new upstream revision drift count = %d, want 1", len(drifts))
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

func TestItemReviewEndpointIncludesDueWaitingAndStalledProjectItems(t *testing.T) {
	app := newAuthedTestApp(t)

	now := time.Now().UTC()
	past := now.Add(-time.Hour).Format(time.RFC3339)
	future := now.Add(time.Hour).Format(time.RFC3339)
	alice, err := app.store.CreateActor("Alice", store.ActorKindHuman)
	if err != nil {
		t.Fatalf("CreateActor() error: %v", err)
	}
	if _, err := app.store.CreateItem("Explicit review", store.ItemOptions{State: store.ItemStateReview}); err != nil {
		t.Fatalf("CreateItem(review) error: %v", err)
	}
	if _, err := app.store.CreateItem("Follow up Alice", store.ItemOptions{
		State:      store.ItemStateWaiting,
		ActorID:    &alice.ID,
		FollowUpAt: &past,
	}); err != nil {
		t.Fatalf("CreateItem(due waiting) error: %v", err)
	}
	if _, err := app.store.CreateItem("Future waiting", store.ItemOptions{
		State:      store.ItemStateWaiting,
		ActorID:    &alice.ID,
		FollowUpAt: &future,
	}); err != nil {
		t.Fatalf("CreateItem(future waiting) error: %v", err)
	}
	if _, err := app.store.CreateItem("Stalled outcome", store.ItemOptions{
		Kind:  store.ItemKindProject,
		State: store.ItemStateNext,
	}); err != nil {
		t.Fatalf("CreateItem(stalled project) error: %v", err)
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/review", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	payload := decodeJSONResponse(t, rr)
	items, ok := payload["items"].([]any)
	if !ok {
		t.Fatalf("items payload = %#v", payload)
	}
	titles := map[string]bool{}
	for _, raw := range items {
		row, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("item row = %#v", raw)
		}
		titles[strFromAny(row["title"])] = true
	}
	for _, title := range []string{"Explicit review", "Follow up Alice", "Stalled outcome"} {
		if !titles[title] {
			t.Fatalf("review endpoint missing %q from titles %#v", title, titles)
		}
	}
	if titles["Future waiting"] {
		t.Fatalf("review endpoint includes future waiting item: %#v", titles)
	}

	rr = doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/counts", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("counts status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	counts, ok := decodeJSONResponse(t, rr)["counts"].(map[string]any)
	if !ok {
		t.Fatalf("counts payload = %#v", decodeJSONResponse(t, rr))
	}
	if got := int(counts[store.ItemStateReview].(float64)); got != 3 {
		t.Fatalf("counts[review] = %d, want 3", got)
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
