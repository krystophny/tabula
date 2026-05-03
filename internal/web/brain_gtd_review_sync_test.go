package web

import (
	"context"
	"testing"

	"github.com/sloppy-org/slopshell/internal/store"
)

func TestSyncBrainGTDReviewListsImportsCanonicalMarkdownAndGoogleTasks(t *testing.T) {
	app := newAuthedTestApp(t)
	t.Setenv("SLOPSHELL_BRAIN_GTD_SYNC", "on")
	app.localControlEndpoint = mcpEndpoint{httpURL: "http://mcp.test"}
	restoreBrainGTDFetchers(t)
	fetchBrainGTDCommitmentList = func(_ *App, _ context.Context, _ string) (brainGTDCommitmentList, error) {
		return brainGTDCommitmentList{}, nil
	}
	fetchBrainGTDReviewList = func(_ *App, _ context.Context, sphere string) (brainGTDReviewList, error) {
		if sphere == store.SpherePrivate {
			return brainGTDReviewList{Items: []brainGTDReviewItem{{
				ID:        "google_tasks:todo.old/task-1",
				Source:    store.ExternalProviderGoogleTasks,
				SourceRef: "todo.old/task-1",
				Title:     "Submit ChatGPT invoices",
				Queue:     "next",
				Track:     "private-admin",
				Due:       "2026-05-06",
			}}}, nil
		}
		return brainGTDReviewList{Items: []brainGTDReviewItem{{
			ID:       "markdown:brain/commitments/work.md",
			Source:   store.ExternalProviderMarkdown,
			Title:    "Reply to grant mail",
			Queue:    "next",
			Track:    "admin",
			Path:     "brain/commitments/work.md",
			FollowUp: "2026-05-04",
			Due:      "2026-05-10",
		}}}, nil
	}

	result, err := app.syncBrainGTDReviewLists(context.Background())
	if err != nil {
		t.Fatalf("syncBrainGTDReviewLists: %v", err)
	}
	if result.Imported != 2 {
		t.Fatalf("imported = %d, want 2", result.Imported)
	}
	markdown, err := app.store.GetItemBySource(store.ExternalProviderMarkdown, "brain/commitments/work.md")
	if err != nil {
		t.Fatalf("GetItemBySource(markdown): %v", err)
	}
	if markdown.Sphere != store.SphereWork || markdown.FollowUpAt == nil || markdown.DueAt == nil {
		t.Fatalf("markdown item timing/sphere = %#v", markdown)
	}
	if markdown.Track != "admin" {
		t.Fatalf("markdown track = %q, want admin", markdown.Track)
	}
	task, err := app.store.GetItemBySource(store.ExternalProviderGoogleTasks, "todo.old/task-1")
	if err != nil {
		t.Fatalf("GetItemBySource(google_tasks): %v", err)
	}
	if task.Sphere != store.SpherePrivate || task.DueAt == nil || *task.DueAt != "2026-05-06T23:59:59Z" {
		t.Fatalf("google task = %#v, want private hard due end of day", task)
	}
	if task.Track != "private-admin" {
		t.Fatalf("google task track = %q, want private-admin", task.Track)
	}
}

func TestSyncBrainGTDReviewListsMigratesTodoistItemsToCanonicalMarkdown(t *testing.T) {
	app := newAuthedTestApp(t)
	t.Setenv("SLOPSHELL_BRAIN_GTD_SYNC", "on")
	app.localControlEndpoint = mcpEndpoint{httpURL: "http://mcp.test"}
	restoreBrainGTDFetchers(t)
	account, err := app.store.CreateExternalAccount(store.SphereWork, store.ExternalProviderTodoist, "Todoist", nil)
	if err != nil {
		t.Fatalf("CreateExternalAccount: %v", err)
	}
	source := store.ExternalProviderTodoist
	firstRef := "task:task-1"
	secondRef := "task:task-2"
	first, err := app.store.CreateItem("Send alpha budget", store.ItemOptions{Sphere: stringPtr(store.SphereWork), Source: &source, SourceRef: &firstRef})
	if err != nil {
		t.Fatalf("CreateItem(first): %v", err)
	}
	second, err := app.store.CreateItem("send alpha budget", store.ItemOptions{Sphere: stringPtr(store.SphereWork), Source: &source, SourceRef: &secondRef})
	if err != nil {
		t.Fatalf("CreateItem(second): %v", err)
	}
	if _, err := app.store.UpsertExternalBinding(store.ExternalBinding{
		AccountID: account.ID, Provider: account.Provider, ObjectType: "task", RemoteID: "task-1", ItemID: &first.ID,
	}); err != nil {
		t.Fatalf("binding first: %v", err)
	}
	if _, err := app.store.UpsertExternalBinding(store.ExternalBinding{
		AccountID: account.ID, Provider: account.Provider, ObjectType: "task", RemoteID: "task-2", ItemID: &second.ID,
	}); err != nil {
		t.Fatalf("binding second: %v", err)
	}
	fetchBrainGTDCommitmentList = func(_ *App, _ context.Context, _ string) (brainGTDCommitmentList, error) {
		return brainGTDCommitmentList{Items: []brainGTDCommitmentItem{{
			Path:     "brain/commitments/alpha.md",
			Title:    "Send alpha budget",
			Status:   "next",
			Bindings: []string{"todoist:work:3:project:task-1", "todoist:work:3:project:task-2"},
		}}}, nil
	}
	fetchBrainGTDReviewList = func(_ *App, _ context.Context, _ string) (brainGTDReviewList, error) {
		return brainGTDReviewList{Items: []brainGTDReviewItem{{
			Source: store.ExternalProviderMarkdown,
			Title:  "Send alpha budget",
			Queue:  "next",
			Path:   "brain/commitments/alpha.md",
		}}}, nil
	}

	result, err := app.syncBrainGTDReviewLists(context.Background())
	if err != nil {
		t.Fatalf("syncBrainGTDReviewLists: %v", err)
	}
	if result.Migrated != 1 || result.Merged != 1 {
		t.Fatalf("result = %#v, want one migrated and one merged", result)
	}
	canonical, err := app.store.GetItemBySource(store.ExternalProviderMarkdown, "brain/commitments/alpha.md")
	if err != nil {
		t.Fatalf("GetItemBySource(markdown): %v", err)
	}
	if canonical.State != store.ItemStateNext {
		t.Fatalf("canonical state = %q, want next", canonical.State)
	}
	if _, err := app.store.GetItemBySource(store.ExternalProviderTodoist, firstRef); err == nil {
		t.Fatal("first Todoist source still exists")
	}
	if _, err := app.store.GetItemBySource(store.ExternalProviderTodoist, secondRef); err == nil {
		t.Fatal("second Todoist source still exists")
	}
	binding, err := app.store.GetBindingByRemote(account.ID, store.ExternalProviderTodoist, "task", "task-2")
	if err != nil {
		t.Fatalf("GetBindingByRemote(task-2): %v", err)
	}
	if binding.ItemID == nil || *binding.ItemID != canonical.ID {
		t.Fatalf("task-2 binding item = %#v, want canonical %d", binding.ItemID, canonical.ID)
	}
}

func TestSyncBrainGTDReviewListsRepairsStaleMarkdownBinding(t *testing.T) {
	app := newAuthedTestApp(t)
	t.Setenv("SLOPSHELL_BRAIN_GTD_SYNC", "on")
	app.localControlEndpoint = mcpEndpoint{httpURL: "http://mcp.test"}
	restoreBrainGTDFetchers(t)

	account, err := app.store.CreateExternalAccount(store.SphereWork, store.ExternalProviderMarkdown, "GTD Markdown work", nil)
	if err != nil {
		t.Fatalf("CreateExternalAccount: %v", err)
	}
	source := store.ExternalProviderMarkdown
	path := "brain/commitments/alpha.md"
	canonical, err := app.store.CreateItem("Send alpha budget", store.ItemOptions{
		Sphere:    stringPtr(store.SphereWork),
		Source:    &source,
		SourceRef: &path,
	})
	if err != nil {
		t.Fatalf("CreateItem(canonical): %v", err)
	}
	duplicate, err := app.store.CreateItem("Old duplicate", store.ItemOptions{Sphere: stringPtr(store.SphereWork)})
	if err != nil {
		t.Fatalf("CreateItem(duplicate): %v", err)
	}
	if _, err := app.store.UpsertExternalBinding(store.ExternalBinding{
		AccountID:  account.ID,
		Provider:   account.Provider,
		ObjectType: "commitment",
		RemoteID:   path,
		ItemID:     &duplicate.ID,
	}); err != nil {
		t.Fatalf("binding duplicate: %v", err)
	}

	fetchBrainGTDCommitmentList = func(_ *App, _ context.Context, _ string) (brainGTDCommitmentList, error) {
		return brainGTDCommitmentList{}, nil
	}
	fetchBrainGTDReviewList = func(_ *App, _ context.Context, _ string) (brainGTDReviewList, error) {
		return brainGTDReviewList{Items: []brainGTDReviewItem{{
			Source: store.ExternalProviderMarkdown,
			Title:  "Send alpha budget",
			Queue:  "next",
			Path:   path,
			Due:    "2026-05-10",
		}}}, nil
	}

	result, err := app.syncBrainGTDReviewLists(context.Background())
	if err != nil {
		t.Fatalf("syncBrainGTDReviewLists: %v", err)
	}
	if result.Imported != 1 {
		t.Fatalf("imported = %d, want 1", result.Imported)
	}
	binding, err := app.store.GetBindingByRemote(account.ID, store.ExternalProviderMarkdown, "commitment", path)
	if err != nil {
		t.Fatalf("GetBindingByRemote(markdown): %v", err)
	}
	if binding.ItemID == nil || *binding.ItemID != canonical.ID {
		t.Fatalf("binding item = %#v, want canonical %d", binding.ItemID, canonical.ID)
	}
	updated, err := app.store.GetItem(canonical.ID)
	if err != nil {
		t.Fatalf("GetItem(canonical): %v", err)
	}
	if updated.State != store.ItemStateNext || updated.DueAt == nil {
		t.Fatalf("updated canonical = %#v, want next with due date", updated)
	}
}

func restoreBrainGTDFetchers(t *testing.T) {
	t.Helper()
	oldReview := fetchBrainGTDReviewList
	oldCommitments := fetchBrainGTDCommitmentList
	t.Cleanup(func() {
		fetchBrainGTDReviewList = oldReview
		fetchBrainGTDCommitmentList = oldCommitments
	})
}
