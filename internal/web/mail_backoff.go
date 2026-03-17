package web

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/krystophny/tabura/internal/ews"
	"github.com/krystophny/tabura/internal/store"
)

type mailBackoffTracker struct {
	mu    sync.Mutex
	until map[int64]time.Time
}

func newMailBackoffTracker() *mailBackoffTracker {
	return &mailBackoffTracker{until: map[int64]time.Time{}}
}

func (t *mailBackoffTracker) note(accountID int64, err error) *ews.BackoffError {
	if t == nil || accountID <= 0 {
		return nil
	}
	var backoffErr *ews.BackoffError
	if !errors.As(err, &backoffErr) || backoffErr == nil {
		return nil
	}
	if backoffErr.Backoff <= 0 {
		backoffErr.Backoff = 5 * time.Minute
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	until := time.Now().Add(backoffErr.Backoff)
	if existing := t.until[accountID]; existing.After(until) {
		until = existing
	}
	t.until[accountID] = until
	return backoffErr
}

func (t *mailBackoffTracker) active(accountID int64) (time.Time, bool) {
	if t == nil || accountID <= 0 {
		return time.Time{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	until := t.until[accountID]
	if until.IsZero() {
		return time.Time{}, false
	}
	if time.Now().After(until) {
		delete(t.until, accountID)
		return time.Time{}, false
	}
	return until, true
}

func (a *App) guardMailAccountBackoff(account store.ExternalAccount) error {
	if a == nil || a.mailBackoffs == nil {
		return nil
	}
	until, ok := a.mailBackoffs.active(account.ID)
	if !ok {
		return nil
	}
	wait := time.Until(until).Round(time.Second)
	if wait < time.Second {
		wait = time.Second
	}
	return fmt.Errorf("exchange server is busy; retry after %s", wait)
}

func (a *App) mailAccountBackoffDelay(account store.ExternalAccount) time.Duration {
	if a == nil || a.mailBackoffs == nil {
		return 0
	}
	until, ok := a.mailBackoffs.active(account.ID)
	if !ok {
		return 0
	}
	wait := time.Until(until)
	if wait < time.Second {
		return time.Second
	}
	return wait
}

func (a *App) writeMailProviderError(w http.ResponseWriter, account store.ExternalAccount, err error) {
	if backoffErr := a.noteMailProviderError(account, err); backoffErr != nil {
		wait := backoffErr.Backoff.Round(time.Second)
		if wait < time.Second {
			wait = time.Second
		}
		writeAPIError(w, http.StatusTooManyRequests, fmt.Sprintf("exchange server is busy; retry after %s", wait))
		return
	}
	writeAPIError(w, http.StatusBadGateway, err.Error())
}

func (a *App) noteMailProviderError(account store.ExternalAccount, err error) *ews.BackoffError {
	if a == nil || a.mailBackoffs == nil {
		return nil
	}
	return a.mailBackoffs.note(account.ID, err)
}
