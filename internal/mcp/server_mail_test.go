package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/krystophny/tabura/internal/email"
	"github.com/krystophny/tabura/internal/providerdata"
	"github.com/krystophny/tabura/internal/store"
)

type fakeMailProvider struct {
	labels     []providerdata.Label
	listIDs    []string
	pageIDs    []string
	nextPage   string
	messages   map[string]*providerdata.EmailMessage
	filters    []email.ServerFilter
	lastAction string
	lastFolder string
	lastLabel  string
}

func (p *fakeMailProvider) ListLabels(_ context.Context) ([]providerdata.Label, error) {
	return append([]providerdata.Label(nil), p.labels...), nil
}

func (p *fakeMailProvider) ListMessages(_ context.Context, _ email.SearchOptions) ([]string, error) {
	return append([]string(nil), p.listIDs...), nil
}

func (p *fakeMailProvider) ListMessagesPage(_ context.Context, _ email.SearchOptions, _ string) (email.MessagePage, error) {
	return email.MessagePage{IDs: append([]string(nil), p.pageIDs...), NextPageToken: p.nextPage}, nil
}

func (p *fakeMailProvider) GetMessage(_ context.Context, messageID, _ string) (*providerdata.EmailMessage, error) {
	return p.messages[messageID], nil
}

func (p *fakeMailProvider) GetMessages(_ context.Context, messageIDs []string, _ string) ([]*providerdata.EmailMessage, error) {
	out := make([]*providerdata.EmailMessage, 0, len(messageIDs))
	for _, id := range messageIDs {
		out = append(out, p.messages[id])
	}
	return out, nil
}

func (p *fakeMailProvider) MarkRead(_ context.Context, _ []string) (int, error) {
	p.lastAction = "mark_read"
	return 1, nil
}
func (p *fakeMailProvider) MarkUnread(_ context.Context, _ []string) (int, error) {
	p.lastAction = "mark_unread"
	return 1, nil
}
func (p *fakeMailProvider) Archive(_ context.Context, _ []string) (int, error) {
	p.lastAction = "archive"
	return 1, nil
}
func (p *fakeMailProvider) MoveToInbox(_ context.Context, _ []string) (int, error) {
	p.lastAction = "move_to_inbox"
	return 1, nil
}
func (p *fakeMailProvider) Trash(_ context.Context, _ []string) (int, error) {
	p.lastAction = "trash"
	return 1, nil
}
func (p *fakeMailProvider) Delete(_ context.Context, _ []string) (int, error) {
	p.lastAction = "delete"
	return 1, nil
}
func (p *fakeMailProvider) ProviderName() string { return "fake" }
func (p *fakeMailProvider) Close() error         { return nil }
func (p *fakeMailProvider) MoveToFolder(_ context.Context, _ []string, folder string) (int, error) {
	p.lastAction = "move_to_folder"
	p.lastFolder = folder
	return 1, nil
}
func (p *fakeMailProvider) ApplyNamedLabel(_ context.Context, _ []string, label string, _ bool) (int, error) {
	p.lastAction = "apply_label"
	p.lastLabel = label
	return 1, nil
}
func (p *fakeMailProvider) ServerFilterCapabilities() email.ServerFilterCapabilities {
	return email.ServerFilterCapabilities{SupportsList: true, SupportsUpsert: true, SupportsDelete: true}
}
func (p *fakeMailProvider) ListServerFilters(context.Context) ([]email.ServerFilter, error) {
	return append([]email.ServerFilter(nil), p.filters...), nil
}
func (p *fakeMailProvider) UpsertServerFilter(_ context.Context, filter email.ServerFilter) (email.ServerFilter, error) {
	if filter.ID == "" {
		filter.ID = "generated"
	}
	p.filters = []email.ServerFilter{filter}
	return filter, nil
}
func (p *fakeMailProvider) DeleteServerFilter(context.Context, string) error {
	p.filters = nil
	return nil
}

func TestMailToolsListReadActAndFilter(t *testing.T) {
	s, st, _ := newDomainServerForTest(t)
	account, err := st.CreateExternalAccount(store.SphereWork, store.ExternalProviderExchangeEWS, "TU Graz", map[string]any{})
	if err != nil {
		t.Fatalf("CreateExternalAccount: %v", err)
	}
	now := time.Date(2026, time.March, 16, 15, 0, 0, 0, time.UTC)
	provider := &fakeMailProvider{
		labels:   []providerdata.Label{{ID: "inbox", Name: "Inbox"}},
		pageIDs:  []string{"m1"},
		nextPage: "next-2",
		messages: map[string]*providerdata.EmailMessage{
			"m1": {ID: "m1", Subject: "Subject", Date: now},
		},
		filters: []email.ServerFilter{{
			ID:      "f1",
			Name:    "Known sender",
			Enabled: true,
			Action:  email.ServerFilterAction{Archive: true},
		}},
	}
	s.newEmailProvider = func(context.Context, store.ExternalAccount) (email.EmailProvider, error) {
		return provider, nil
	}

	listed, err := s.callTool("mail_account_list", map[string]interface{}{})
	if err != nil {
		t.Fatalf("mail_account_list failed: %v", err)
	}
	accounts, _ := listed["accounts"].([]store.ExternalAccount)
	if len(accounts) != 1 || accounts[0].ID != account.ID {
		t.Fatalf("accounts = %+v", accounts)
	}

	messages, err := s.callTool("mail_message_list", map[string]interface{}{
		"account_id": account.ID,
		"page_token": "next-1",
	})
	if err != nil {
		t.Fatalf("mail_message_list failed: %v", err)
	}
	if got := messages["next_page_token"]; got != "next-2" {
		t.Fatalf("next_page_token = %#v", got)
	}

	message, err := s.callTool("mail_message_get", map[string]interface{}{
		"account_id": account.ID,
		"message_id": "m1",
	})
	if err != nil {
		t.Fatalf("mail_message_get failed: %v", err)
	}
	gotMessage, _ := message["message"].(*providerdata.EmailMessage)
	if gotMessage == nil || gotMessage.ID != "m1" {
		t.Fatalf("message = %#v", message["message"])
	}

	acted, err := s.callTool("mail_action", map[string]interface{}{
		"account_id":  account.ID,
		"action":      "archive_label",
		"message_ids": []interface{}{"m1"},
		"label":       "project-x",
	})
	if err != nil {
		t.Fatalf("mail_action failed: %v", err)
	}
	if succeeded, _ := acted["succeeded"].(int); succeeded != 1 {
		t.Fatalf("succeeded = %#v", acted["succeeded"])
	}
	if provider.lastAction != "move_to_folder" {
		t.Fatalf("lastAction = %q", provider.lastAction)
	}

	filters, err := s.callTool("mail_server_filter_list", map[string]interface{}{
		"account_id": account.ID,
	})
	if err != nil {
		t.Fatalf("mail_server_filter_list failed: %v", err)
	}
	gotFilters, _ := filters["filters"].([]email.ServerFilter)
	if len(gotFilters) != 1 || gotFilters[0].ID != "f1" {
		t.Fatalf("filters = %+v", gotFilters)
	}

	upserted, err := s.callTool("mail_server_filter_upsert", map[string]interface{}{
		"account_id": account.ID,
		"filter": map[string]interface{}{
			"name":    "Archive updates",
			"enabled": true,
			"action": map[string]interface{}{
				"archive": true,
			},
		},
	})
	if err != nil {
		t.Fatalf("mail_server_filter_upsert failed: %v", err)
	}
	gotFilter, _ := upserted["filter"].(email.ServerFilter)
	if gotFilter.ID == "" || gotFilter.Name != "Archive updates" {
		t.Fatalf("filter = %+v", gotFilter)
	}

	deleted, err := s.callTool("mail_server_filter_delete", map[string]interface{}{
		"account_id": account.ID,
		"filter_id":  "generated",
	})
	if err != nil {
		t.Fatalf("mail_server_filter_delete failed: %v", err)
	}
	if ok, _ := deleted["deleted"].(bool); !ok {
		t.Fatalf("deleted = %#v", deleted["deleted"])
	}
}
