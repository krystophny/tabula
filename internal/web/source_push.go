package web

import (
	"context"
	"log"
	"time"

	"github.com/sloppy-org/slopshell/internal/store"
)

const (
	sourcePushReconcileInterval = 15 * time.Second
	sourcePushReconnectDelay    = 5 * time.Second
	sourcePushErrorRetryDelay   = 5 * time.Minute
)

type sourcePushManager struct {
	app       *App
	providers map[string]*accountSyncProvider
	watchers  map[int64]sourcePushWatcher
}

type sourcePushWatcher struct {
	account store.ExternalAccount
	cancel  context.CancelFunc
}

func (m *sourcePushManager) goWorker(run func()) {
	if m != nil && m.app != nil {
		m.app.workerWG.Add(1)
		go func() {
			defer m.app.workerWG.Done()
			run()
		}()
		return
	}
	go run()
}

func newSourcePushManager(app *App, providers []*accountSyncProvider) sourcePushRunner {
	pushProviders := make(map[string]*accountSyncProvider)
	for _, provider := range providers {
		if provider == nil || provider.watchAccount == nil {
			continue
		}
		pushProviders[provider.name] = provider
	}
	if len(pushProviders) == 0 {
		return nil
	}
	return &sourcePushManager{
		app:       app,
		providers: pushProviders,
		watchers:  map[int64]sourcePushWatcher{},
	}
}

func (m *sourcePushManager) Run(ctx context.Context) {
	if m == nil || m.app == nil || m.app.store == nil {
		return
	}
	m.reconcile(ctx)
	ticker := time.NewTicker(sourcePushReconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			m.stopAll()
			return
		case <-ticker.C:
			m.reconcile(ctx)
		}
	}
}

func (m *sourcePushManager) reconcile(ctx context.Context) {
	accounts, err := m.app.store.ListExternalAccounts("")
	if err != nil {
		log.Printf("source push: list external accounts: %v", err)
		return
	}
	desired := make(map[int64]store.ExternalAccount, len(accounts))
	for _, account := range accounts {
		if !account.Enabled {
			continue
		}
		provider := m.providers[account.Provider]
		if provider == nil || provider.watchAccount == nil {
			continue
		}
		policy, err := provider.SyncPolicy(ctx, account)
		if err != nil {
			log.Printf("source push: account %d sync policy: %v", account.ID, err)
			continue
		}
		if !policy.DisablePoll {
			continue
		}
		desired[account.ID] = account
	}
	for accountID, watcher := range m.watchers {
		account, ok := desired[accountID]
		if ok && sourcePushAccountEqual(watcher.account, account) {
			delete(desired, accountID)
			continue
		}
		watcher.cancel()
		delete(m.watchers, accountID)
	}
	for accountID, account := range desired {
		provider := m.providers[account.Provider]
		if provider == nil {
			continue
		}
		watchCtx, cancel := context.WithCancel(ctx)
		m.watchers[accountID] = sourcePushWatcher{account: account, cancel: cancel}
		m.goWorker(func() {
			m.runWatcher(watchCtx, account, provider)
		})
	}
}

func (m *sourcePushManager) runWatcher(ctx context.Context, account store.ExternalAccount, provider *accountSyncProvider) {
	triggerCh := make(chan struct{}, 1)
	triggerSync := func() {
		select {
		case triggerCh <- struct{}{}:
		default:
		}
	}
	triggerSync()
	m.goWorker(func() {
		m.consumeSyncTriggers(ctx, account, provider, triggerCh)
	})
	for {
		if !m.waitForAccountRetry(ctx, account, nil) {
			return
		}
		err := provider.watchAccount(ctx, account, triggerSync)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Printf("source push: account %d watch failed: %v", account.ID, err)
			if !m.waitForAccountRetry(ctx, account, err) {
				return
			}
			continue
		} else {
			log.Printf("source push: account %d watch exited; reconnecting", account.ID)
		}
		if !sleepSourcePush(ctx, sourcePushReconnectDelay) {
			return
		}
	}
}

func (m *sourcePushManager) consumeSyncTriggers(ctx context.Context, account store.ExternalAccount, provider *accountSyncProvider, triggerCh chan struct{}) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-triggerCh:
			if !m.waitForAccountRetry(ctx, account, nil) {
				return
			}
			count, err := provider.syncAccount(ctx, account)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("source push: account %d sync failed: %v", account.ID, err)
				if !m.waitForAccountRetry(ctx, account, err) {
					return
				}
				continue
			}
			if count > 0 && provider.onSynced != nil {
				provider.onSynced(account, count)
			}
			if ctx.Err() != nil {
				return
			}
			if provider.continueSync == nil {
				continue
			}
			if ctx.Err() != nil {
				return
			}
			fresh, freshErr := m.app.store.GetExternalAccount(account.ID)
			if freshErr != nil {
				log.Printf("source push: account %d continuation state failed: %v", account.ID, freshErr)
				continue
			}
			delay, ok := provider.continueSync(ctx, fresh)
			if !ok {
				continue
			}
			scheduleSourcePushTrigger(ctx, delay, triggerCh)
		}
	}
}

func (m *sourcePushManager) waitForAccountRetry(ctx context.Context, account store.ExternalAccount, err error) bool {
	delay := m.retryDelayForAccount(account, err)
	if delay <= 0 {
		return true
	}
	return sleepSourcePush(ctx, delay)
}

func (m *sourcePushManager) retryDelayForAccount(account store.ExternalAccount, err error) time.Duration {
	delay := time.Duration(0)
	if m != nil && m.app != nil {
		if err != nil {
			m.app.noteMailProviderError(account, err)
		}
		if backoffDelay := m.app.mailAccountBackoffDelay(account); backoffDelay > delay {
			delay = backoffDelay
		}
	}
	if err != nil && delay < sourcePushErrorRetryDelay {
		delay = sourcePushErrorRetryDelay
	}
	return delay
}

func (m *sourcePushManager) stopAll() {
	for accountID, watcher := range m.watchers {
		watcher.cancel()
		delete(m.watchers, accountID)
	}
}

func sourcePushAccountEqual(left, right store.ExternalAccount) bool {
	return left.ID == right.ID &&
		left.Provider == right.Provider &&
		left.Enabled == right.Enabled &&
		left.AccountName == right.AccountName
}

func sleepSourcePush(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func scheduleSourcePushTrigger(ctx context.Context, delay time.Duration, triggerCh chan<- struct{}) {
	if delay > 0 && !sleepSourcePush(ctx, delay) {
		return
	}
	select {
	case <-ctx.Done():
	case triggerCh <- struct{}{}:
	default:
	}
}
