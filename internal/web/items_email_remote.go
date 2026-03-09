package web

import (
	"context"
	"fmt"
	"strings"

	"github.com/krystophny/tabura/internal/store"
)

type emailArchiveProvider interface {
	Archive(context.Context, []string) (int, error)
}

type emailMoveToInboxProvider interface {
	MoveToInbox(context.Context, []string) (int, error)
}

func emailBackedItem(item store.Item) bool {
	source := strings.ToLower(strings.TrimSpace(stringFromPointer(item.Source)))
	switch source {
	case store.ExternalProviderGmail, store.ExternalProviderIMAP, store.ExternalProviderExchange:
		return true
	default:
		return false
	}
}

func emailMessageBindingForItem(bindings []store.ExternalBinding, item store.Item) *store.ExternalBinding {
	source := strings.ToLower(strings.TrimSpace(stringFromPointer(item.Source)))
	for i := range bindings {
		binding := bindings[i]
		if binding.ObjectType != emailBindingObjectType {
			continue
		}
		if source != "" && strings.ToLower(strings.TrimSpace(binding.Provider)) != source {
			continue
		}
		return &binding
	}
	for i := range bindings {
		binding := bindings[i]
		if binding.ObjectType == emailBindingObjectType {
			return &binding
		}
	}
	return nil
}

func (a *App) syncRemoteEmailItemState(ctx context.Context, item store.Item, nextState string) error {
	if a == nil || !emailBackedItem(item) {
		return nil
	}
	normalizedState := strings.ToLower(strings.TrimSpace(nextState))
	if normalizedState != store.ItemStateDone && normalizedState != store.ItemStateInbox {
		return nil
	}
	bindings, err := a.store.GetBindingsByItem(item.ID)
	if err != nil {
		return err
	}
	binding := emailMessageBindingForItem(bindings, item)
	if binding == nil || strings.TrimSpace(binding.RemoteID) == "" {
		return nil
	}
	account, err := a.store.GetExternalAccount(binding.AccountID)
	if err != nil {
		return err
	}
	cfg, err := decodeEmailSyncAccountConfig(account)
	if err != nil {
		return err
	}
	provider, err := a.emailSyncProviderForAccount(ctx, account, cfg)
	if err != nil {
		return err
	}
	defer provider.Close()

	messageIDs := []string{strings.TrimSpace(binding.RemoteID)}
	switch normalizedState {
	case store.ItemStateDone:
		archiver, ok := provider.(emailArchiveProvider)
		if !ok {
			return fmt.Errorf("email provider %s does not support archive", account.Provider)
		}
		_, err = archiver.Archive(ctx, messageIDs)
		return err
	case store.ItemStateInbox:
		restorer, ok := provider.(emailMoveToInboxProvider)
		if !ok {
			return fmt.Errorf("email provider %s does not support move to inbox", account.Provider)
		}
		_, err = restorer.MoveToInbox(ctx, messageIDs)
		return err
	default:
		return nil
	}
}
