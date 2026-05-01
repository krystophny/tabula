package web

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/sloppy-org/slopshell/internal/store"
)

func (a *App) updateItemState(ctx context.Context, item store.Item, state string) error {
	if todoistBackedItem(item) && todoistDoneState(state) && item.State != store.ItemStateDone {
		if err := a.syncTodoistItemCompletion(item); err != nil {
			return itemRemoteStateUpdateError{err: err}
		}
	}
	if item.State != strings.TrimSpace(state) {
		if err := a.syncRemoteEmailItemState(ctx, item, state); err != nil {
			return itemRemoteStateUpdateError{err: err}
		}
	}
	return a.store.UpdateItemState(item.ID, state)
}

type itemRemoteStateUpdateError struct {
	err error
}

func (e itemRemoteStateUpdateError) Error() string {
	return e.err.Error()
}

func (e itemRemoteStateUpdateError) Unwrap() error {
	return e.err
}

func writeItemStateUpdateError(w http.ResponseWriter, err error) {
	var remoteErr itemRemoteStateUpdateError
	if errors.As(err, &remoteErr) {
		writeAPIError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeItemStoreError(w, err)
}
