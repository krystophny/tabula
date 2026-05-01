package sync_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/sloppy-org/slopshell/internal/store"
	tabsync "github.com/sloppy-org/slopshell/internal/sync"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(filepath.Join(t.TempDir(), "slopshell.db"))
	if err != nil {
		t.Fatalf("store.New() error: %v", err)
	}
	t.Cleanup(func() {
		_ = s.Close()
	})
	return s
}

func TestStoreSinkUpsertItemUsesContainerMappingAndBinding(t *testing.T) {
	s := newTestStore(t)
	sink := tabsync.NewStoreSink(s)

	account, err := s.CreateExternalAccount(store.SphereWork, store.ExternalProviderTodoist, "todo", map[string]any{})
	if err != nil {
		t.Fatalf("CreateExternalAccount() error: %v", err)
	}
	workspace, err := s.CreateWorkspace("sync-target", filepath.Join(t.TempDir(), "workspace"), store.SphereWork)
	if err != nil {
		t.Fatalf("CreateWorkspace() error: %v", err)
	}
	if _, err := s.SetContainerMapping(account.Provider, "project", "alpha", &workspace.ID, nil); err != nil {
		t.Fatalf("SetContainerMapping() error: %v", err)
	}

	item, err := sink.UpsertItem(context.Background(), store.Item{
		Title: "Follow up with provider",
	}, store.ExternalBinding{
		AccountID:    account.ID,
		Provider:     account.Provider,
		ObjectType:   "task",
		RemoteID:     "remote-1",
		ContainerRef: stringPtr("alpha"),
	})
	if err != nil {
		t.Fatalf("UpsertItem(create) error: %v", err)
	}
	if item.WorkspaceID == nil || *item.WorkspaceID != workspace.ID {
		t.Fatalf("item.WorkspaceID = %v, want %d", item.WorkspaceID, workspace.ID)
	}
	if item.Sphere != store.SphereWork {
		t.Fatalf("item.Sphere = %q, want %q", item.Sphere, store.SphereWork)
	}

	binding, err := s.GetBindingByRemote(account.ID, account.Provider, "task", "remote-1")
	if err != nil {
		t.Fatalf("GetBindingByRemote() error: %v", err)
	}
	if binding.ItemID == nil || *binding.ItemID != item.ID {
		t.Fatalf("binding.ItemID = %v, want %d", binding.ItemID, item.ID)
	}

	updated, err := sink.UpsertItem(context.Background(), store.Item{
		Title: "Updated provider title",
		State: store.ItemStateWaiting,
	}, store.ExternalBinding{
		AccountID:    account.ID,
		Provider:     account.Provider,
		ObjectType:   "task",
		RemoteID:     "remote-1",
		ContainerRef: stringPtr("alpha"),
	})
	if err != nil {
		t.Fatalf("UpsertItem(update) error: %v", err)
	}
	if updated.ID != item.ID {
		t.Fatalf("updated.ID = %d, want %d", updated.ID, item.ID)
	}
	if updated.Title != "Updated provider title" {
		t.Fatalf("updated.Title = %q, want updated title", updated.Title)
	}
	if updated.State != store.ItemStateWaiting {
		t.Fatalf("updated.State = %q, want %q", updated.State, store.ItemStateWaiting)
	}
}

func TestStoreSinkUpsertArtifactLinksWorkspaceAndTracksBinding(t *testing.T) {
	s := newTestStore(t)
	sink := tabsync.NewStoreSink(s)

	account, err := s.CreateExternalAccount(store.SpherePrivate, store.ExternalProviderIMAP, "mail", map[string]any{})
	if err != nil {
		t.Fatalf("CreateExternalAccount() error: %v", err)
	}
	workspace, err := s.CreateWorkspace("mailbox", filepath.Join(t.TempDir(), "mailbox"), store.SpherePrivate)
	if err != nil {
		t.Fatalf("CreateWorkspace() error: %v", err)
	}
	if _, err := s.SetContainerMapping(account.Provider, "folder", "inbox", &workspace.ID, nil); err != nil {
		t.Fatalf("SetContainerMapping() error: %v", err)
	}

	title := "Subject line"
	artifact, err := sink.UpsertArtifact(context.Background(), store.Artifact{
		Kind:  store.ArtifactKindEmail,
		Title: &title,
	}, store.ExternalBinding{
		AccountID:    account.ID,
		Provider:     account.Provider,
		ObjectType:   "message",
		RemoteID:     "msg-1",
		ContainerRef: stringPtr("inbox"),
	})
	if err != nil {
		t.Fatalf("UpsertArtifact() error: %v", err)
	}

	links, err := s.ListArtifactWorkspaceLinks(workspace.ID)
	if err != nil {
		t.Fatalf("ListArtifactWorkspaceLinks() error: %v", err)
	}
	if len(links) != 1 || links[0].ArtifactID != artifact.ID {
		t.Fatalf("workspace links = %#v, want artifact %d", links, artifact.ID)
	}

	binding, err := s.GetBindingByRemote(account.ID, account.Provider, "message", "msg-1")
	if err != nil {
		t.Fatalf("GetBindingByRemote() error: %v", err)
	}
	if binding.ArtifactID == nil || *binding.ArtifactID != artifact.ID {
		t.Fatalf("binding.ArtifactID = %v, want %d", binding.ArtifactID, artifact.ID)
	}
}

func TestStoreSinkRecordsStateDriftInsteadOfOverwritingLocalOverlay(t *testing.T) {
	s := newTestStore(t)
	sink := tabsync.NewStoreSink(s)

	account, err := s.CreateExternalAccount(store.SphereWork, store.ExternalProviderTodoist, "todo", map[string]any{})
	if err != nil {
		t.Fatalf("CreateExternalAccount() error: %v", err)
	}
	remoteAt := "2026-03-08T10:00:00Z"
	item, err := sink.UpsertItem(context.Background(), store.Item{
		Title: "Close upstream task",
		State: store.ItemStateNext,
	}, store.ExternalBinding{
		AccountID:       account.ID,
		Provider:        account.Provider,
		ObjectType:      "task",
		RemoteID:        "remote-drift",
		RemoteUpdatedAt: &remoteAt,
	})
	if err != nil {
		t.Fatalf("UpsertItem(create) error: %v", err)
	}

	time.Sleep(1100 * time.Millisecond)
	localState := store.ItemStateWaiting
	if err := s.UpdateItem(item.ID, store.ItemUpdate{State: &localState}); err != nil {
		t.Fatalf("UpdateItem(local overlay) error: %v", err)
	}
	nextRemoteAt := "2026-03-08T10:05:00Z"
	if _, err := sink.UpsertItem(context.Background(), store.Item{
		Title: "Closed upstream task",
		State: store.ItemStateDone,
	}, store.ExternalBinding{
		AccountID:       account.ID,
		Provider:        account.Provider,
		ObjectType:      "task",
		RemoteID:        "remote-drift",
		RemoteUpdatedAt: &nextRemoteAt,
	}); err != nil {
		t.Fatalf("UpsertItem(conflict) error: %v", err)
	}

	got, err := s.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem() error: %v", err)
	}
	if got.State != store.ItemStateWaiting {
		t.Fatalf("local item state = %q, want waiting preserved", got.State)
	}
	drifts, err := s.ListUnresolvedExternalBindingDrifts(store.ItemListFilter{})
	if err != nil {
		t.Fatalf("ListUnresolvedExternalBindingDrifts() error: %v", err)
	}
	if len(drifts) != 1 {
		t.Fatalf("drift count = %d, want 1", len(drifts))
	}
	if drifts[0].LocalState != store.ItemStateWaiting || drifts[0].UpstreamState != store.ItemStateDone {
		t.Fatalf("drift states = local %q upstream %q, want waiting/done", drifts[0].LocalState, drifts[0].UpstreamState)
	}
}

func TestStoreSinkReopensDismissedStateDriftOnNewRemoteRevision(t *testing.T) {
	s := newTestStore(t)
	sink := tabsync.NewStoreSink(s)

	account, item := createSyncedTodoistItem(t, s, sink, "remote-redrift")

	time.Sleep(1100 * time.Millisecond)
	localState := store.ItemStateWaiting
	if err := s.UpdateItem(item.ID, store.ItemUpdate{State: &localState}); err != nil {
		t.Fatalf("UpdateItem(local overlay) error: %v", err)
	}
	upsertTodoistRemoteState(t, sink, account, "remote-redrift", "Closed upstream task", store.ItemStateDone, "2026-03-08T10:05:00Z")
	drifts := unresolvedDrifts(t, s)
	if len(drifts) != 1 {
		t.Fatalf("first drift count = %d, want 1", len(drifts))
	}
	if _, err := s.ResolveExternalBindingDrift(drifts[0].ID, store.ExternalBindingDriftActionDismiss); err != nil {
		t.Fatalf("ResolveExternalBindingDrift(dismiss) error: %v", err)
	}

	upsertTodoistRemoteState(t, sink, account, "remote-redrift", "Closed upstream task", store.ItemStateDone, "2026-03-08T10:05:00Z")
	got, err := s.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem(same revision) error: %v", err)
	}
	if got.State != store.ItemStateWaiting {
		t.Fatalf("same-revision local item state = %q, want waiting preserved", got.State)
	}
	if drifts = unresolvedDrifts(t, s); len(drifts) != 0 {
		t.Fatalf("same-revision drift count = %d, want 0", len(drifts))
	}

	upsertTodoistRemoteState(t, sink, account, "remote-redrift", "Deferred upstream task", store.ItemStateDeferred, "2026-03-08T10:10:00Z")
	got, err = s.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem() error: %v", err)
	}
	if got.State != store.ItemStateWaiting {
		t.Fatalf("local item state = %q, want waiting preserved", got.State)
	}
	drifts = unresolvedDrifts(t, s)
	if len(drifts) != 1 {
		t.Fatalf("second drift count = %d, want 1", len(drifts))
	}
	if drifts[0].LocalState != store.ItemStateWaiting || drifts[0].UpstreamState != store.ItemStateDeferred {
		t.Fatalf("drift states = local %q upstream %q, want waiting/deferred", drifts[0].LocalState, drifts[0].UpstreamState)
	}
}

func TestStoreSinkUpdatesUnresolvedStateDriftOnNewRemoteRevision(t *testing.T) {
	s := newTestStore(t)
	sink := tabsync.NewStoreSink(s)

	account, item := createSyncedTodoistItem(t, s, sink, "remote-open-redrift")

	time.Sleep(1100 * time.Millisecond)
	localState := store.ItemStateWaiting
	if err := s.UpdateItem(item.ID, store.ItemUpdate{State: &localState}); err != nil {
		t.Fatalf("UpdateItem(local overlay) error: %v", err)
	}
	upsertTodoistRemoteState(t, sink, account, "remote-open-redrift", "Closed upstream task", store.ItemStateDone, "2026-03-08T10:05:00Z")
	firstDrifts := unresolvedDrifts(t, s)
	if len(firstDrifts) != 1 {
		t.Fatalf("first drift count = %d, want 1", len(firstDrifts))
	}

	upsertTodoistRemoteState(t, sink, account, "remote-open-redrift", "Deferred upstream task", store.ItemStateDeferred, "2026-03-08T10:10:00Z")
	got, err := s.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem() error: %v", err)
	}
	if got.State != store.ItemStateWaiting {
		t.Fatalf("local item state = %q, want waiting preserved", got.State)
	}
	secondDrifts := unresolvedDrifts(t, s)
	if len(secondDrifts) != 1 {
		t.Fatalf("second drift count = %d, want 1", len(secondDrifts))
	}
	if secondDrifts[0].ID != firstDrifts[0].ID {
		t.Fatalf("second drift ID = %d, want existing drift %d", secondDrifts[0].ID, firstDrifts[0].ID)
	}
	if secondDrifts[0].LocalState != store.ItemStateWaiting || secondDrifts[0].UpstreamState != store.ItemStateDeferred {
		t.Fatalf("drift states = local %q upstream %q, want waiting/deferred", secondDrifts[0].LocalState, secondDrifts[0].UpstreamState)
	}
}

func createSyncedTodoistItem(t *testing.T, s *store.Store, sink *tabsync.StoreSink, remoteID string) (store.ExternalAccount, store.Item) {
	t.Helper()
	account, err := s.CreateExternalAccount(store.SphereWork, store.ExternalProviderTodoist, "todo", map[string]any{})
	if err != nil {
		t.Fatalf("CreateExternalAccount() error: %v", err)
	}
	item := upsertTodoistRemoteState(t, sink, account, remoteID, "Keep local task", store.ItemStateNext, "2026-03-08T10:00:00Z")
	return account, item
}

func upsertTodoistRemoteState(t *testing.T, sink *tabsync.StoreSink, account store.ExternalAccount, remoteID, title, state, remoteAt string) store.Item {
	t.Helper()
	item, err := sink.UpsertItem(context.Background(), store.Item{
		Title: title,
		State: state,
	}, store.ExternalBinding{
		AccountID:       account.ID,
		Provider:        account.Provider,
		ObjectType:      "task",
		RemoteID:        remoteID,
		RemoteUpdatedAt: &remoteAt,
	})
	if err != nil {
		t.Fatalf("UpsertItem(%s) error: %v", remoteID, err)
	}
	return item
}

func unresolvedDrifts(t *testing.T, s *store.Store) []store.ExternalBindingDrift {
	t.Helper()
	drifts, err := s.ListUnresolvedExternalBindingDrifts(store.ItemListFilter{})
	if err != nil {
		t.Fatalf("ListUnresolvedExternalBindingDrifts() error: %v", err)
	}
	return drifts
}

func stringPtr(value string) *string {
	return &value
}
