package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/krystophny/tabura/internal/email"
	"github.com/krystophny/tabura/internal/ews"
	"github.com/krystophny/tabura/internal/providerdata"
	"github.com/krystophny/tabura/internal/store"
)

type fakeMailProvider struct {
	labels      []providerdata.Label
	listIDs     []string
	pageIDs     []string
	nextPage    string
	messages    map[string]*providerdata.EmailMessage
	filters     []email.ServerFilter
	resolvedIDs map[string]string
	lastOpts    email.SearchOptions
	lastPage    string
	lastAction  string
	lastIDs     []string
	lastFolder  string
	lastLabel   string
	lastArchive bool
	lastFormat  string
	listErr     error
	getErr      error
	listCalls   int
	pageErr     error
	pageCalls   int
}

func (p *fakeMailProvider) ListLabels(context.Context) ([]providerdata.Label, error) {
	return append([]providerdata.Label(nil), p.labels...), nil
}

func (p *fakeMailProvider) ListMessages(_ context.Context, opts email.SearchOptions) ([]string, error) {
	p.lastOpts = opts
	p.listCalls++
	if p.listErr != nil {
		return nil, p.listErr
	}
	return append([]string(nil), p.listIDs...), nil
}

func (p *fakeMailProvider) ListMessagesPage(_ context.Context, opts email.SearchOptions, pageToken string) (email.MessagePage, error) {
	p.lastOpts = opts
	p.lastPage = pageToken
	p.pageCalls++
	if p.pageErr != nil {
		return email.MessagePage{}, p.pageErr
	}
	return email.MessagePage{IDs: append([]string(nil), p.pageIDs...), NextPageToken: p.nextPage}, nil
}

func (p *fakeMailProvider) GetMessage(_ context.Context, messageID, format string) (*providerdata.EmailMessage, error) {
	p.lastFormat = strings.TrimSpace(format)
	if p.getErr != nil {
		return nil, p.getErr
	}
	return p.messages[messageID], nil
}

func (p *fakeMailProvider) GetMessages(_ context.Context, messageIDs []string, format string) ([]*providerdata.EmailMessage, error) {
	p.lastFormat = strings.TrimSpace(format)
	out := make([]*providerdata.EmailMessage, 0, len(messageIDs))
	for _, id := range messageIDs {
		out = append(out, p.messages[id])
	}
	return out, nil
}

func (p *fakeMailProvider) MarkRead(_ context.Context, ids []string) (int, error) {
	return p.record("mark_read", ids), nil
}
func (p *fakeMailProvider) MarkUnread(_ context.Context, ids []string) (int, error) {
	return p.record("mark_unread", ids), nil
}
func (p *fakeMailProvider) Archive(_ context.Context, ids []string) (int, error) {
	return p.record("archive", ids), nil
}
func (p *fakeMailProvider) ArchiveResolved(_ context.Context, ids []string) ([]email.ActionResolution, error) {
	p.record("archive", ids)
	return p.resolutions(ids), nil
}
func (p *fakeMailProvider) MoveToInbox(_ context.Context, ids []string) (int, error) {
	return p.record("move_to_inbox", ids), nil
}
func (p *fakeMailProvider) MoveToInboxResolved(_ context.Context, ids []string) ([]email.ActionResolution, error) {
	p.record("move_to_inbox", ids)
	return p.resolutions(ids), nil
}
func (p *fakeMailProvider) Trash(_ context.Context, ids []string) (int, error) {
	return p.record("trash", ids), nil
}
func (p *fakeMailProvider) TrashResolved(_ context.Context, ids []string) ([]email.ActionResolution, error) {
	p.record("trash", ids)
	return p.resolutions(ids), nil
}
func (p *fakeMailProvider) Delete(_ context.Context, ids []string) (int, error) {
	return p.record("delete", ids), nil
}
func (p *fakeMailProvider) ProviderName() string { return "fake" }
func (p *fakeMailProvider) Close() error         { return nil }
func (p *fakeMailProvider) MoveToFolder(_ context.Context, ids []string, folder string) (int, error) {
	p.lastIDs = append([]string(nil), ids...)
	p.lastAction = "move_to_folder"
	p.lastFolder = folder
	return len(ids), nil
}
func (p *fakeMailProvider) MoveToFolderResolved(_ context.Context, ids []string, folder string) ([]email.ActionResolution, error) {
	p.lastIDs = append([]string(nil), ids...)
	p.lastAction = "move_to_folder"
	p.lastFolder = folder
	return p.resolutions(ids), nil
}
func (p *fakeMailProvider) ApplyNamedLabel(_ context.Context, ids []string, label string, archive bool) (int, error) {
	p.lastIDs = append([]string(nil), ids...)
	p.lastAction = "apply_label"
	p.lastLabel = label
	p.lastArchive = archive
	return len(ids), nil
}
func (p *fakeMailProvider) ServerFilterCapabilities() email.ServerFilterCapabilities {
	return email.ServerFilterCapabilities{SupportsList: true, SupportsUpsert: true, SupportsDelete: true}
}
func (p *fakeMailProvider) ListServerFilters(context.Context) ([]email.ServerFilter, error) {
	return append([]email.ServerFilter(nil), p.filters...), nil
}
func (p *fakeMailProvider) UpsertServerFilter(_ context.Context, filter email.ServerFilter) (email.ServerFilter, error) {
	if strings.TrimSpace(filter.ID) == "" {
		filter.ID = "generated"
	}
	p.filters = []email.ServerFilter{filter}
	return filter, nil
}
func (p *fakeMailProvider) DeleteServerFilter(context.Context, string) error {
	p.filters = nil
	return nil
}

func (p *fakeMailProvider) record(action string, ids []string) int {
	p.lastAction = action
	p.lastIDs = append([]string(nil), ids...)
	return len(ids)
}

func (p *fakeMailProvider) resolutions(ids []string) []email.ActionResolution {
	out := make([]email.ActionResolution, 0, len(ids))
	for _, id := range ids {
		resolved := id
		if p.resolvedIDs != nil {
			if mapped := strings.TrimSpace(p.resolvedIDs[id]); mapped != "" {
				resolved = mapped
			}
		}
		out = append(out, email.ActionResolution{
			OriginalMessageID: id,
			ResolvedMessageID: resolved,
		})
	}
	return out
}

func TestMailAPIListsEnabledEmailAccounts(t *testing.T) {
	app := newAuthedTestApp(t)
	if _, err := app.store.CreateExternalAccount(store.SphereWork, store.ExternalProviderGmail, "Work Gmail", map[string]any{}); err != nil {
		t.Fatalf("CreateExternalAccount(gmail): %v", err)
	}
	private, err := app.store.CreateExternalAccount(store.SpherePrivate, store.ExternalProviderGoogleCalendar, "Calendar", map[string]any{})
	if err != nil {
		t.Fatalf("CreateExternalAccount(calendar): %v", err)
	}
	if err := app.store.UpdateExternalAccount(private.ID, store.ExternalAccountUpdate{Enabled: boolPointer(false)}); err != nil {
		t.Fatalf("UpdateExternalAccount(disable): %v", err)
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/mail/accounts", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	data := decodeJSONDataResponse(t, rr)
	accounts, _ := data["accounts"].([]any)
	if len(accounts) != 1 {
		t.Fatalf("accounts len = %d, want 1", len(accounts))
	}
}

func TestMailAPIListsMessagesAndGetsMessage(t *testing.T) {
	app := newAuthedTestApp(t)
	account, err := app.store.CreateExternalAccount(store.SphereWork, store.ExternalProviderGmail, "Work Gmail", map[string]any{})
	if err != nil {
		t.Fatalf("CreateExternalAccount: %v", err)
	}
	now := time.Date(2026, time.March, 16, 12, 0, 0, 0, time.UTC)
	provider := &fakeMailProvider{
		labels:   []providerdata.Label{{ID: "inbox", Name: "Inbox"}},
		pageIDs:  []string{"m2"},
		nextPage: "next-2",
		messages: map[string]*providerdata.EmailMessage{
			"m1": {ID: "m1", Subject: "Older", Date: now.Add(-time.Hour)},
			"m2": {ID: "m2", Subject: "Newer", Date: now},
		},
	}
	app.newEmailProvider = func(context.Context, store.ExternalAccount) (email.EmailProvider, error) {
		return provider, nil
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/external-accounts/"+itoaMail(account.ID)+"/mail/messages?page_token=next-1", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	data := decodeJSONDataResponse(t, rr)
	if got := data["next_page_token"]; got != "next-2" {
		t.Fatalf("next_page_token = %#v", got)
	}
	messages, _ := data["messages"].([]any)
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(messages))
	}

	getRR := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/external-accounts/"+itoaMail(account.ID)+"/mail/messages/m2", nil)
	if getRR.Code != http.StatusOK {
		t.Fatalf("get status = %d body=%s", getRR.Code, getRR.Body.String())
	}
	getData := decodeJSONDataResponse(t, getRR)
	message, _ := getData["message"].(map[string]any)
	if message["ID"] != "m2" {
		t.Fatalf("message id = %#v", message["ID"])
	}
}

func TestMailAPIListsMessagesUsesPagingFromFirstPage(t *testing.T) {
	app := newAuthedTestApp(t)
	account, err := app.store.CreateExternalAccount(store.SphereWork, store.ExternalProviderGmail, "Work Gmail", map[string]any{})
	if err != nil {
		t.Fatalf("CreateExternalAccount: %v", err)
	}
	now := time.Date(2026, time.March, 16, 12, 0, 0, 0, time.UTC)
	provider := &fakeMailProvider{
		listIDs:  []string{"legacy-list-id"},
		pageIDs:  []string{"m2"},
		nextPage: "next-1",
		messages: map[string]*providerdata.EmailMessage{"m2": {ID: "m2", Subject: "Paged", Date: now}},
	}
	app.newEmailProvider = func(context.Context, store.ExternalAccount) (email.EmailProvider, error) {
		return provider, nil
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/external-accounts/"+itoaMail(account.ID)+"/mail/messages?limit=25", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if provider.lastPage != "" {
		t.Fatalf("lastPage = %q, want empty first-page token", provider.lastPage)
	}
	if provider.lastOpts.MaxResults != 25 {
		t.Fatalf("MaxResults = %d, want 25", provider.lastOpts.MaxResults)
	}
	data := decodeJSONDataResponse(t, rr)
	if got := data["next_page_token"]; got != "next-1" {
		t.Fatalf("next_page_token = %#v", got)
	}
}

func TestMailAPIListsMessagesUsesRequestedFormat(t *testing.T) {
	app := newAuthedTestApp(t)
	account, err := app.store.CreateExternalAccount(store.SphereWork, store.ExternalProviderGmail, "Work Gmail", map[string]any{})
	if err != nil {
		t.Fatalf("CreateExternalAccount: %v", err)
	}
	now := time.Date(2026, time.March, 16, 12, 0, 0, 0, time.UTC)
	provider := &fakeMailProvider{
		pageIDs: []string{"m2"},
		messages: map[string]*providerdata.EmailMessage{
			"m2": {ID: "m2", Subject: "Paged", Date: now},
		},
	}
	app.newEmailProvider = func(context.Context, store.ExternalAccount) (email.EmailProvider, error) {
		return provider, nil
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/external-accounts/"+itoaMail(account.ID)+"/mail/messages?limit=25&format=metadata", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if provider.lastFormat != "metadata" {
		t.Fatalf("lastFormat = %q, want metadata", provider.lastFormat)
	}
}

func TestMailAPIListsMessagesReturnsFriendlyBackoffError(t *testing.T) {
	app := newAuthedTestApp(t)
	account, err := app.store.CreateExternalAccount(store.SphereWork, store.ExternalProviderExchangeEWS, "TU Graz", map[string]any{})
	if err != nil {
		t.Fatalf("CreateExternalAccount: %v", err)
	}
	provider := &fakeMailProvider{
		pageErr: &ews.BackoffError{
			Operation:    "FindItem",
			ResponseCode: "ErrorServerBusy",
			Message:      "The server cannot service this request right now. Try again later.",
			Backoff:      2 * time.Minute,
		},
	}
	app.newEmailProvider = func(context.Context, store.ExternalAccount) (email.EmailProvider, error) {
		return provider, nil
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/external-accounts/"+itoaMail(account.ID)+"/mail/messages", nil)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "retry after 2m0s") {
		t.Fatalf("body = %s, want retry-after message", rr.Body.String())
	}

	rr = doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/external-accounts/"+itoaMail(account.ID)+"/mail/messages", nil)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d body=%s", rr.Code, rr.Body.String())
	}
	if provider.pageCalls != 1 {
		t.Fatalf("provider page calls = %d, want 1", provider.pageCalls)
	}
}

func TestMailAPIActionArchiveLabelUsesExchangeArchiveFolder(t *testing.T) {
	app := newAuthedTestApp(t)
	account, err := app.store.CreateExternalAccount(store.SphereWork, store.ExternalProviderExchangeEWS, "TU Graz", map[string]any{})
	if err != nil {
		t.Fatalf("CreateExternalAccount: %v", err)
	}
	provider := &fakeMailProvider{}
	app.newEmailProvider = func(context.Context, store.ExternalAccount) (email.EmailProvider, error) {
		return provider, nil
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/external-accounts/"+itoaMail(account.ID)+"/mail/actions", map[string]any{
		"action":      "archive_label",
		"message_ids": []string{"m1", "m2"},
		"label":       "padova2023",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if provider.lastAction != "move_to_folder" {
		t.Fatalf("lastAction = %q", provider.lastAction)
	}
	if provider.lastFolder != "Archive/padova2023" {
		t.Fatalf("lastFolder = %q", provider.lastFolder)
	}
}

func TestMailAPIActionLogsResolvedMovesReturnBeforeBackgroundReconcile(t *testing.T) {
	app := newAuthedTestApp(t)
	account, err := app.store.CreateExternalAccount(store.SphereWork, store.ExternalProviderExchangeEWS, "TU Graz", map[string]any{})
	if err != nil {
		t.Fatalf("CreateExternalAccount: %v", err)
	}
	provider := &fakeMailProvider{
		messages: map[string]*providerdata.EmailMessage{
			"m1": {
				ID:      "m1",
				Subject: "Need action",
				Sender:  "alice@example.com",
				Labels:  []string{"Posteingang"},
			},
		},
	}
	syncCalls := 0
	reconcileStarted := make(chan struct{}, 1)
	reconcileRelease := make(chan struct{})
	app.newEmailProvider = func(context.Context, store.ExternalAccount) (email.EmailProvider, error) {
		return provider, nil
	}
	app.syncMailAccountNow = func(context.Context, store.ExternalAccount) (int, error) {
		syncCalls++
		select {
		case reconcileStarted <- struct{}{}:
		default:
		}
		<-reconcileRelease
		return 1, nil
	}

	resultCh := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		resultCh <- doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/external-accounts/"+itoaMail(account.ID)+"/mail/actions", map[string]any{
			"action":      "trash",
			"message_ids": []string{"m1"},
		})
	}()

	var rr *httptest.ResponseRecorder
	select {
	case rr = <-resultCh:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("mail action request blocked on reconcile")
	}
	close(reconcileRelease)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if provider.lastAction != "trash" {
		t.Fatalf("lastAction = %q, want trash", provider.lastAction)
	}
	select {
	case <-reconcileStarted:
	case <-time.After(time.Second):
		t.Fatal("background reconcile did not start")
	}
	if syncCalls != 1 {
		t.Fatalf("syncCalls = %d, want 1", syncCalls)
	}
	logs, err := app.store.ListMailActionLogs(account.ID, 10)
	if err != nil {
		t.Fatalf("ListMailActionLogs() error: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("logs len = %d, want 1", len(logs))
	}
	if logs[0].Status != store.MailActionLogApplied {
		t.Fatalf("log status = %q, want %q", logs[0].Status, store.MailActionLogApplied)
	}
	if logs[0].FolderFrom != "Posteingang" {
		t.Fatalf("log folder_from = %q, want Posteingang", logs[0].FolderFrom)
	}
	if logs[0].FolderTo != "Gelöschte Elemente" {
		t.Fatalf("log folder_to = %q, want Gelöschte Elemente", logs[0].FolderTo)
	}
}

func TestMailAPIActionRewritesExchangeBindingToResolvedID(t *testing.T) {
	app := newAuthedTestApp(t)
	account, err := app.store.CreateExternalAccount(store.SphereWork, store.ExternalProviderExchangeEWS, "TU Graz", map[string]any{})
	if err != nil {
		t.Fatalf("CreateExternalAccount: %v", err)
	}
	item, err := app.store.CreateItem("Follow up", store.ItemOptions{})
	if err != nil {
		t.Fatalf("CreateItem() error: %v", err)
	}
	title := "Mail"
	artifact, err := app.store.CreateArtifact(store.ArtifactKindEmail, nil, nil, &title, nil)
	if err != nil {
		t.Fatalf("CreateArtifact() error: %v", err)
	}
	containerRef := "Posteingang"
	if _, err := app.store.UpsertExternalBinding(store.ExternalBinding{
		AccountID:    account.ID,
		Provider:     store.ExternalProviderExchangeEWS,
		ObjectType:   "email",
		RemoteID:     "m1",
		ItemID:       &item.ID,
		ArtifactID:   &artifact.ID,
		ContainerRef: &containerRef,
	}); err != nil {
		t.Fatalf("UpsertExternalBinding() error: %v", err)
	}
	provider := &fakeMailProvider{
		resolvedIDs: map[string]string{"m1": "m1-trash"},
		messages: map[string]*providerdata.EmailMessage{
			"m1": {
				ID:      "m1",
				Subject: "Need action",
				Sender:  "alice@example.com",
				Labels:  []string{"Posteingang", "INBOX"},
			},
		},
	}
	app.newEmailProvider = func(context.Context, store.ExternalAccount) (email.EmailProvider, error) {
		return provider, nil
	}
	app.syncMailAccountNow = func(context.Context, store.ExternalAccount) (int, error) { return 1, nil }

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/external-accounts/"+itoaMail(account.ID)+"/mail/actions", map[string]any{
		"action":      "trash",
		"message_ids": []string{"m1"},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if _, err := app.store.GetBindingByRemote(account.ID, store.ExternalProviderExchangeEWS, "email", "m1"); err == nil {
		t.Fatal("old binding still exists")
	}
	binding, err := app.store.GetBindingByRemote(account.ID, store.ExternalProviderExchangeEWS, "email", "m1-trash")
	if err != nil {
		t.Fatalf("GetBindingByRemote(new) error: %v", err)
	}
	if binding.ContainerRef == nil || *binding.ContainerRef != "Gelöschte Elemente" {
		t.Fatalf("binding container_ref = %v, want Gelöschte Elemente", binding.ContainerRef)
	}
	updatedItem, err := app.store.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem() error: %v", err)
	}
	if updatedItem.State != store.ItemStateDone {
		t.Fatalf("item state = %q, want done", updatedItem.State)
	}
	logs, err := app.store.ListMailActionLogs(account.ID, 10)
	if err != nil {
		t.Fatalf("ListMailActionLogs() error: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("logs len = %d, want 1", len(logs))
	}
	if logs[0].ResolvedMessageID != "m1-trash" {
		t.Fatalf("resolved_message_id = %q, want m1-trash", logs[0].ResolvedMessageID)
	}
}

func boolPointer(value bool) *bool {
	return &value
}

func itoaMail(value int64) string {
	return strconv.FormatInt(value, 10)
}
