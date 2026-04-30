package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/sloppy-org/slopshell/internal/store"
)

// The unified next-actions view must surface only executable next actions and
// must keep project items, deferred items, and source-specific items
// distinguishable through filters.

func TestItemNextAPIExcludesProjectItemsByDefault(t *testing.T) {
	app := newAuthedTestApp(t)

	project, err := app.store.CreateItem("Plan release", store.ItemOptions{
		Kind:  store.ItemKindProject,
		State: store.ItemStateNext,
	})
	if err != nil {
		t.Fatalf("CreateItem(project) error: %v", err)
	}
	action, err := app.store.CreateItem("Send draft", store.ItemOptions{
		State: store.ItemStateNext,
	})
	if err != nil {
		t.Fatalf("CreateItem(action) error: %v", err)
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/next", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	payload := decodeJSONResponse(t, rr)
	items, ok := payload["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("default next payload = %#v, want only the action item", payload)
	}
	row := items[0].(map[string]any)
	if int64(row["id"].(float64)) != action.ID {
		t.Fatalf("default item id = %v, want %d", row["id"], action.ID)
	}
	if row["kind"] != store.ItemKindAction {
		t.Fatalf("default item kind = %v, want %q", row["kind"], store.ItemKindAction)
	}

	rrInclude := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/next?include_project_items=true", nil)
	if rrInclude.Code != http.StatusOK {
		t.Fatalf("include status = %d, want 200: %s", rrInclude.Code, rrInclude.Body.String())
	}
	includeItems, _ := decodeJSONResponse(t, rrInclude)["items"].([]any)
	if len(includeItems) != 2 {
		t.Fatalf("include payload = %#v, want 2", includeItems)
	}

	rrSection := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/next?section=project_items", nil)
	if rrSection.Code != http.StatusOK {
		t.Fatalf("section status = %d, want 200: %s", rrSection.Code, rrSection.Body.String())
	}
	sectionItems, _ := decodeJSONResponse(t, rrSection)["items"].([]any)
	if len(sectionItems) != 1 {
		t.Fatalf("section payload = %#v, want 1", sectionItems)
	}
	sectionRow := sectionItems[0].(map[string]any)
	if int64(sectionRow["id"].(float64)) != project.ID {
		t.Fatalf("section drill id = %v, want %d", sectionRow["id"], project.ID)
	}
}

func TestItemNextAPIScopesToProjectItemChildren(t *testing.T) {
	app := newAuthedTestApp(t)

	project, err := app.store.CreateItem("Outcome A", store.ItemOptions{
		Kind:  store.ItemKindProject,
		State: store.ItemStateNext,
	})
	if err != nil {
		t.Fatalf("CreateItem(project) error: %v", err)
	}
	otherProject, err := app.store.CreateItem("Outcome B", store.ItemOptions{
		Kind:  store.ItemKindProject,
		State: store.ItemStateNext,
	})
	if err != nil {
		t.Fatalf("CreateItem(other project) error: %v", err)
	}
	childA, err := app.store.CreateItem("Child action A", store.ItemOptions{State: store.ItemStateNext})
	if err != nil {
		t.Fatalf("CreateItem(child A) error: %v", err)
	}
	childB, err := app.store.CreateItem("Child action B", store.ItemOptions{State: store.ItemStateNext})
	if err != nil {
		t.Fatalf("CreateItem(child B) error: %v", err)
	}
	if _, err := app.store.CreateItem("Standalone", store.ItemOptions{State: store.ItemStateNext}); err != nil {
		t.Fatalf("CreateItem(standalone) error: %v", err)
	}
	if err := app.store.LinkItemChild(project.ID, childA.ID, store.ItemLinkRoleNextAction); err != nil {
		t.Fatalf("LinkItemChild(A) error: %v", err)
	}
	if err := app.store.LinkItemChild(otherProject.ID, childB.ID, store.ItemLinkRoleNextAction); err != nil {
		t.Fatalf("LinkItemChild(B) error: %v", err)
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/next?project_item_id="+itoa(project.ID), nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	items, ok := decodeJSONResponse(t, rr)["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("payload = %#v, want only child A", items)
	}
	row := items[0].(map[string]any)
	if int64(row["id"].(float64)) != childA.ID {
		t.Fatalf("scoped item id = %v, want %d", row["id"], childA.ID)
	}

	rrBad := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/next?project_item_id=bad", nil)
	if rrBad.Code != http.StatusBadRequest {
		t.Fatalf("invalid project_item_id status = %d, want 400", rrBad.Code)
	}
}

func TestItemNextAPIFiltersBySource(t *testing.T) {
	app := newAuthedTestApp(t)

	todoist := store.ExternalProviderTodoist
	github := "github"
	if _, err := app.store.CreateItem("From Todoist", store.ItemOptions{
		State:  store.ItemStateNext,
		Source: &todoist,
	}); err != nil {
		t.Fatalf("CreateItem(todoist) error: %v", err)
	}
	if _, err := app.store.CreateItem("From GitHub", store.ItemOptions{
		State:  store.ItemStateNext,
		Source: &github,
	}); err != nil {
		t.Fatalf("CreateItem(github) error: %v", err)
	}
	if _, err := app.store.CreateItem("Local capture", store.ItemOptions{
		State: store.ItemStateNext,
	}); err != nil {
		t.Fatalf("CreateItem(local) error: %v", err)
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/next?source=todoist", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	items, _ := decodeJSONResponse(t, rr)["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1 (only todoist-backed)", len(items))
	}
	row := items[0].(map[string]any)
	if row["source"] != todoist {
		t.Fatalf("source = %v, want %q", row["source"], todoist)
	}
}

func TestItemNextAPIFiltersByActor(t *testing.T) {
	app := newAuthedTestApp(t)

	actor, err := app.store.CreateActor("Bob", store.ActorKindHuman)
	if err != nil {
		t.Fatalf("CreateActor() error: %v", err)
	}
	delegated, err := app.store.CreateItem("Delegated draft", store.ItemOptions{
		State:   store.ItemStateNext,
		ActorID: &actor.ID,
	})
	if err != nil {
		t.Fatalf("CreateItem(delegated) error: %v", err)
	}
	if _, err := app.store.CreateItem("Self action", store.ItemOptions{
		State: store.ItemStateNext,
	}); err != nil {
		t.Fatalf("CreateItem(self) error: %v", err)
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/next?actor_id="+itoa(actor.ID), nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	items, _ := decodeJSONResponse(t, rr)["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	row := items[0].(map[string]any)
	if int64(row["id"].(float64)) != delegated.ID {
		t.Fatalf("actor scope id = %v, want %d", row["id"], delegated.ID)
	}

	rrBad := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/next?actor_id=0", nil)
	if rrBad.Code != http.StatusBadRequest {
		t.Fatalf("invalid actor_id status = %d, want 400", rrBad.Code)
	}
}

func TestItemNextAPIDoesNotIncludeDeferredFutureItems(t *testing.T) {
	app := newAuthedTestApp(t)

	future := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	deferred, err := app.store.CreateItem("Deferred future", store.ItemOptions{
		State:        store.ItemStateDeferred,
		VisibleAfter: &future,
		FollowUpAt:   &future,
	})
	if err != nil {
		t.Fatalf("CreateItem(deferred) error: %v", err)
	}
	action, err := app.store.CreateItem("Now action", store.ItemOptions{
		State: store.ItemStateNext,
	})
	if err != nil {
		t.Fatalf("CreateItem(action) error: %v", err)
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/next", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	items, _ := decodeJSONResponse(t, rr)["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1 (deferred future must not appear)", len(items))
	}
	row := items[0].(map[string]any)
	if int64(row["id"].(float64)) != action.ID {
		t.Fatalf("returned id = %v, want %d", row["id"], action.ID)
	}

	stored, err := app.store.GetItem(deferred.ID)
	if err != nil {
		t.Fatalf("GetItem(deferred) error: %v", err)
	}
	if stored.State != store.ItemStateDeferred {
		t.Fatalf("deferred state = %q, want %q (next view must not redefine state)", stored.State, store.ItemStateDeferred)
	}
}

func TestItemNextAPISurfacesOverdueWithoutChangingState(t *testing.T) {
	app := newAuthedTestApp(t)

	past := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	future := time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339)
	overdue, err := app.store.CreateItem("Overdue deadline", store.ItemOptions{
		State: store.ItemStateNext,
		DueAt: &past,
	})
	if err != nil {
		t.Fatalf("CreateItem(overdue) error: %v", err)
	}
	if _, err := app.store.CreateItem("Future hard deadline", store.ItemOptions{
		State: store.ItemStateNext,
		DueAt: &future,
	}); err != nil {
		t.Fatalf("CreateItem(future) error: %v", err)
	}
	if _, err := app.store.CreateItem("No deadline", store.ItemOptions{
		State: store.ItemStateNext,
	}); err != nil {
		t.Fatalf("CreateItem(no deadline) error: %v", err)
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/next", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	payload := decodeJSONResponse(t, rr)
	items, _ := payload["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("len(items) = %d, want 3", len(items))
	}
	overdueRaw, ok := payload["overdue"].([]any)
	if !ok {
		t.Fatalf("payload missing overdue list: %#v", payload)
	}
	if len(overdueRaw) != 1 {
		t.Fatalf("len(overdue) = %d, want 1", len(overdueRaw))
	}
	if int64(overdueRaw[0].(float64)) != overdue.ID {
		t.Fatalf("overdue id = %v, want %d", overdueRaw[0], overdue.ID)
	}

	stored, err := app.store.GetItem(overdue.ID)
	if err != nil {
		t.Fatalf("GetItem(overdue) error: %v", err)
	}
	if stored.State != store.ItemStateNext {
		t.Fatalf("overdue state = %q, want %q (visibility must not redefine state)", stored.State, store.ItemStateNext)
	}
	if stored.DueAt == nil || strings.TrimSpace(*stored.DueAt) == "" {
		t.Fatalf("overdue due_at = %v, want preserved", stored.DueAt)
	}
}

// Hard deadlines (due_at) and follow-up/start dates (follow_up_at) are
// independent: a follow-up date in the past must not surface the row as a
// missed hard deadline, and clearing one must not affect the other.
func TestItemNextAPIDistinguishesDueAtFromFollowUpAt(t *testing.T) {
	app := newAuthedTestApp(t)

	past := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	hardDeadlineOnly, err := app.store.CreateItem("Hard deadline only", store.ItemOptions{
		State: store.ItemStateNext,
		DueAt: &past,
	})
	if err != nil {
		t.Fatalf("CreateItem(hard deadline only) error: %v", err)
	}
	followUpOnly, err := app.store.CreateItem("Follow-up only", store.ItemOptions{
		State:      store.ItemStateNext,
		FollowUpAt: &past,
	})
	if err != nil {
		t.Fatalf("CreateItem(follow-up only) error: %v", err)
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/next", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	payload := decodeJSONResponse(t, rr)
	overdueRaw, _ := payload["overdue"].([]any)
	overdueIDs := map[int64]bool{}
	for _, raw := range overdueRaw {
		overdueIDs[int64(raw.(float64))] = true
	}
	if !overdueIDs[hardDeadlineOnly.ID] {
		t.Fatalf("overdue list missing hard deadline item %d: %v", hardDeadlineOnly.ID, overdueIDs)
	}
	if overdueIDs[followUpOnly.ID] {
		t.Fatalf("follow_up_at must not be treated as a hard deadline: %v", overdueIDs)
	}
}

func TestItemNextAPIFiltersByDueAndFollowUpWindows(t *testing.T) {
	app := newAuthedTestApp(t)

	dueSoon := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)
	dueLater := time.Now().UTC().Add(72 * time.Hour).Format(time.RFC3339)
	followSoon := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)
	followLater := time.Now().UTC().Add(72 * time.Hour).Format(time.RFC3339)

	soon, err := app.store.CreateItem("Due soon", store.ItemOptions{
		State:      store.ItemStateNext,
		DueAt:      &dueSoon,
		FollowUpAt: &followSoon,
	})
	if err != nil {
		t.Fatalf("CreateItem(due soon) error: %v", err)
	}
	if _, err := app.store.CreateItem("Due later", store.ItemOptions{
		State:      store.ItemStateNext,
		DueAt:      &dueLater,
		FollowUpAt: &followLater,
	}); err != nil {
		t.Fatalf("CreateItem(due later) error: %v", err)
	}
	if _, err := app.store.CreateItem("No dates", store.ItemOptions{
		State: store.ItemStateNext,
	}); err != nil {
		t.Fatalf("CreateItem(no dates) error: %v", err)
	}

	cutoff := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)

	cases := []struct {
		name  string
		query string
	}{
		{"due_before", "/api/items/next?due_before=" + url.QueryEscape(cutoff)},
		{"follow_up_before", "/api/items/next?follow_up_before=" + url.QueryEscape(cutoff)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, tc.query, nil)
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
			}
			items, _ := decodeJSONResponse(t, rr)["items"].([]any)
			if len(items) != 1 {
				t.Fatalf("%s len(items) = %d, want 1", tc.name, len(items))
			}
			row := items[0].(map[string]any)
			if int64(row["id"].(float64)) != soon.ID {
				t.Fatalf("%s id = %v, want %d", tc.name, row["id"], soon.ID)
			}
		})
	}

	rrAfter := doAuthedJSONRequest(t, app.Router(), http.MethodGet,
		"/api/items/next?due_after="+url.QueryEscape(cutoff), nil)
	if rrAfter.Code != http.StatusOK {
		t.Fatalf("due_after status = %d, want 200: %s", rrAfter.Code, rrAfter.Body.String())
	}
	afterItems, _ := decodeJSONResponse(t, rrAfter)["items"].([]any)
	if len(afterItems) != 1 {
		t.Fatalf("due_after len(items) = %d, want 1", len(afterItems))
	}

	rrBad := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/items/next?due_before=not-a-date", nil)
	if rrBad.Code != http.StatusBadRequest {
		t.Fatalf("invalid due_before status = %d, want 400", rrBad.Code)
	}
}

func TestItemNextAPIFiltersBySourceContainer(t *testing.T) {
	app := newAuthedTestApp(t)

	gmail := store.ExternalProviderGmail
	inboxRef := "msg-inbox-1"
	cabinetRef := "msg-cabinet-1"
	inboxItem, err := app.store.CreateItem("Inbox-bound message", store.ItemOptions{
		State:     store.ItemStateNext,
		Source:    &gmail,
		SourceRef: &inboxRef,
	})
	if err != nil {
		t.Fatalf("CreateItem(inbox) error: %v", err)
	}
	if _, err := app.store.CreateItem("Filed message", store.ItemOptions{
		State:     store.ItemStateNext,
		Source:    &gmail,
		SourceRef: &cabinetRef,
	}); err != nil {
		t.Fatalf("CreateItem(filed) error: %v", err)
	}

	account, err := app.store.CreateExternalAccount(store.SpherePrivate, gmail, "primary", map[string]any{})
	if err != nil {
		t.Fatalf("CreateExternalAccount() error: %v", err)
	}
	inboxContainer := "INBOX"
	cabinetContainer := "Cabinet"
	if _, err := app.store.UpsertExternalBinding(store.ExternalBinding{
		AccountID:    account.ID,
		Provider:     gmail,
		ObjectType:   "email",
		RemoteID:     inboxRef,
		ItemID:       &inboxItem.ID,
		ContainerRef: &inboxContainer,
	}); err != nil {
		t.Fatalf("UpsertExternalBinding(inbox) error: %v", err)
	}
	otherID := inboxItem.ID + 1
	if _, err := app.store.UpsertExternalBinding(store.ExternalBinding{
		AccountID:    account.ID,
		Provider:     gmail,
		ObjectType:   "email",
		RemoteID:     cabinetRef,
		ItemID:       &otherID,
		ContainerRef: &cabinetContainer,
	}); err != nil {
		t.Fatalf("UpsertExternalBinding(cabinet) error: %v", err)
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet,
		"/api/items/next?source_container="+url.QueryEscape(inboxContainer), nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	items, _ := decodeJSONResponse(t, rr)["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	row := items[0].(map[string]any)
	if int64(row["id"].(float64)) != inboxItem.ID {
		t.Fatalf("source_container id = %v, want %d", row["id"], inboxItem.ID)
	}
}
