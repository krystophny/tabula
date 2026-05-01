package store

import (
	"database/sql"
	"errors"
	"strings"
)

// ItemGestureUndo describes the prior state captured by a swipe gesture so the
// caller can revert the local overlay verbatim. Unlike UpdateItem this does
// not enforce the forward-only state machine — undo is an explicit revert and
// must be allowed to move the item back from done to its previous state.
type ItemGestureUndo struct {
	State        string
	ActorID      *int64
	VisibleAfter *string
	FollowUpAt   *string
}

// RestoreItemFromGestureUndo writes the snapshot fields onto the item row.
// State is required; the time and actor fields are written verbatim with nil
// meaning "clear that column" (matching how the gesture captured them).
func (s *Store) RestoreItemFromGestureUndo(id int64, undo ItemGestureUndo) error {
	if strings.TrimSpace(undo.State) == "" {
		return errors.New("item state is required")
	}
	state := normalizeItemState(undo.State)
	if state == "" {
		return errors.New("invalid item state")
	}
	if _, err := s.GetItem(id); err != nil {
		return err
	}
	parts := []string{"state = ?", "updated_at = datetime('now')"}
	args := []any{state}

	parts = append(parts, "actor_id = ?")
	args = append(args, nullablePositiveID(int64Or(undo.ActorID, 0)))

	visible, err := normalizeOptionalRFC3339String(undo.VisibleAfter)
	if err != nil {
		return errors.New("visible_after must be valid RFC3339")
	}
	parts = append(parts, "visible_after = ?")
	args = append(args, visible)

	follow, err := normalizeOptionalRFC3339String(undo.FollowUpAt)
	if err != nil {
		return errors.New("follow_up_at must be valid RFC3339")
	}
	parts = append(parts, "follow_up_at = ?")
	args = append(args, follow)

	args = append(args, id)
	res, err := s.db.Exec(`UPDATE items SET `+strings.Join(parts, ", ")+` WHERE id = ?`, args...)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func int64Or(value *int64, fallback int64) int64 {
	if value == nil {
		return fallback
	}
	return *value
}
