package web

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sloppy-org/slopshell/internal/ews"
	"github.com/sloppy-org/slopshell/internal/store"
	tabsync "github.com/sloppy-org/slopshell/internal/sync"
)

func TestSourcePushManagerStartsWatcherAndTriggersSync(t *testing.T) {
	app := newAuthedTestApp(t)
	_, err := app.store.CreateExternalAccount(store.SphereWork, store.ExternalProviderExchangeEWS, "TU Graz", map[string]any{
		"endpoint": "https://exchange.tugraz.at/EWS/Exchange.asmx",
		"username": "ert",
	})
	if err != nil {
		t.Fatalf("CreateExternalAccount(exchange_ews) error: %v", err)
	}

	var syncCalls atomic.Int32
	watchStarted := make(chan struct{}, 1)
	provider := &accountSyncProvider{
		name: store.ExternalProviderExchangeEWS,
		syncAccount: func(context.Context, store.ExternalAccount) (int, error) {
			syncCalls.Add(1)
			return 1, nil
		},
		syncPolicy: func(context.Context, store.ExternalAccount) (tabsync.SyncPolicy, error) {
			return tabsync.SyncPolicy{DisablePoll: true}, nil
		},
		watchAccount: func(ctx context.Context, _ store.ExternalAccount, trigger func()) error {
			select {
			case watchStarted <- struct{}{}:
			default:
			}
			trigger()
			<-ctx.Done()
			return ctx.Err()
		},
	}

	manager, ok := newSourcePushManager(app, []*accountSyncProvider{provider}).(*sourcePushManager)
	if !ok || manager == nil {
		t.Fatal("expected source push manager")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.reconcile(ctx)

	select {
	case <-watchStarted:
	case <-time.After(time.Second):
		t.Fatal("push watcher did not start")
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if syncCalls.Load() > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("syncCalls = %d, want at least 1", syncCalls.Load())
}

func TestSourcePushManagerContinuesSyncWhileBackfillPending(t *testing.T) {
	app := newAuthedTestApp(t)
	account, err := app.store.CreateExternalAccount(store.SphereWork, store.ExternalProviderExchangeEWS, "TU Graz", map[string]any{
		"history_sync": map[string]any{
			"current_container": "folder-inbox",
			"cursor":            "2",
			"complete":          false,
		},
	})
	if err != nil {
		t.Fatalf("CreateExternalAccount(exchange_ews) error: %v", err)
	}

	var syncCalls atomic.Int32
	errCh := make(chan error, 1)
	provider := &accountSyncProvider{
		name: store.ExternalProviderExchangeEWS,
		syncAccount: func(context.Context, store.ExternalAccount) (int, error) {
			call := syncCalls.Add(1)
			if call == 1 {
				return 1, nil
			}
			if updateErr := app.store.UpdateExternalAccount(account.ID, store.ExternalAccountUpdate{Config: map[string]any{
				"history_sync": map[string]any{
					"complete": true,
				},
			}}); updateErr != nil {
				select {
				case errCh <- updateErr:
				default:
				}
			}
			return 1, nil
		},
		syncPolicy: func(context.Context, store.ExternalAccount) (tabsync.SyncPolicy, error) {
			return tabsync.SyncPolicy{DisablePoll: true}, nil
		},
		watchAccount: func(ctx context.Context, _ store.ExternalAccount, trigger func()) error {
			trigger()
			<-ctx.Done()
			return ctx.Err()
		},
		continueSync: func(context.Context, store.ExternalAccount) (time.Duration, bool) {
			fresh, freshErr := app.store.GetExternalAccount(account.ID)
			if freshErr != nil {
				select {
				case errCh <- freshErr:
				default:
				}
				return 0, false
			}
			if !app.emailHistoryPending(fresh) {
				return 0, false
			}
			return 10 * time.Millisecond, true
		},
	}

	manager, ok := newSourcePushManager(app, []*accountSyncProvider{provider}).(*sourcePushManager)
	if !ok || manager == nil {
		t.Fatal("expected source push manager")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.reconcile(ctx)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		select {
		case asyncErr := <-errCh:
			t.Fatalf("async source push error: %v", asyncErr)
		default:
		}
		if syncCalls.Load() >= 2 {
			cancel()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("syncCalls = %d, want at least 2", syncCalls.Load())
}

func TestSourcePushManagerWaitForAccountRetryHonorsBackoff(t *testing.T) {
	app := newAuthedTestApp(t)
	manager := &sourcePushManager{app: app}
	account := store.ExternalAccount{ID: 2, Provider: store.ExternalProviderExchangeEWS}
	if delay := manager.retryDelayForAccount(account, &ews.BackoffError{Backoff: 150 * time.Millisecond}); delay < sourcePushErrorRetryDelay {
		t.Fatalf("delay = %v, want at least %v minimum backoff", delay, sourcePushErrorRetryDelay)
	}
}

func TestSourcePushManagerWaitForAccountRetryCoolsDownGenericErrors(t *testing.T) {
	app := newAuthedTestApp(t)
	manager := &sourcePushManager{app: app}
	account := store.ExternalAccount{ID: 2, Provider: store.ExternalProviderExchangeEWS}
	if delay := manager.retryDelayForAccount(account, errors.New("auth failed")); delay != sourcePushErrorRetryDelay {
		t.Fatalf("delay = %v, want %v", delay, sourcePushErrorRetryDelay)
	}
	if delay := manager.retryDelayForAccount(account, nil); delay != 0 {
		t.Fatalf("delay without error = %v, want 0", delay)
	}
}
