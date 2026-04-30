package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s *Store) CreateItem(title string, opts ItemOptions) (Item, error) {
	cleanTitle := strings.TrimSpace(title)
	cleanKind := normalizeItemKind(opts.Kind)
	cleanState := normalizeItemState(opts.State)
	if cleanTitle == "" {
		return Item{}, errors.New("item title is required")
	}
	if cleanKind == "" {
		return Item{}, errors.New("invalid item kind")
	}
	if cleanState == "" {
		return Item{}, errors.New("invalid item state")
	}
	if opts.WorkspaceID == nil && opts.ArtifactID != nil {
		artifact, err := s.GetArtifact(*opts.ArtifactID)
		if err != nil {
			return Item{}, err
		}
		inferredWorkspaceID, err := s.InferWorkspaceForArtifact(artifact)
		if err != nil {
			return Item{}, err
		}
		opts.WorkspaceID = inferredWorkspaceID
	}
	if opts.WorkspaceID != nil && *opts.WorkspaceID > 0 {
		if _, err := s.GetWorkspace(*opts.WorkspaceID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return Item{}, errors.New("foreign key constraint failed: workspace_id")
			}
			return Item{}, err
		}
	}
	itemSphere, err := s.resolveItemSphere(opts.WorkspaceID, opts.Sphere)
	if err != nil {
		return Item{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return Item{}, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`INSERT INTO items (
			title, kind, state, workspace_id, artifact_id, actor_id, visible_after, follow_up_at, due_at, source, source_ref, review_target, reviewer, reviewed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		cleanTitle,
		cleanKind,
		cleanState,
		opts.WorkspaceID,
		opts.ArtifactID,
		opts.ActorID,
		normalizeOptionalString(opts.VisibleAfter),
		normalizeOptionalString(opts.FollowUpAt),
		normalizeOptionalString(opts.DueAt),
		normalizeOptionalString(opts.Source),
		normalizeOptionalString(opts.SourceRef),
		normalizeOptionalString(normalizedReviewTargetPointer(opts.ReviewTarget)),
		normalizeOptionalString(normalizedReviewerPointer(opts.Reviewer)),
		normalizeOptionalString(reviewTimestampPointer(opts.ReviewTarget, opts.Reviewer)),
	)
	if err != nil {
		return Item{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Item{}, err
	}
	if err := s.syncPrimaryItemArtifactTx(tx, id, opts.ArtifactID); err != nil {
		return Item{}, err
	}
	if err := tx.Commit(); err != nil {
		return Item{}, err
	}
	if err := s.syncScopedContextLink("context_items", "item_id", id, itemSphere); err != nil {
		return Item{}, err
	}
	if opts.WorkspaceID != nil && *opts.WorkspaceID > 0 {
		if err := s.syncItemDateContext(id, opts.WorkspaceID); err != nil {
			return Item{}, err
		}
	}
	return s.GetItem(id)
}

func (s *Store) GetItem(id int64) (Item, error) {
	return scanItem(s.db.QueryRow(
		`SELECT id, title, kind, state, workspace_id, `+scopedContextSelect("context_items", "item_id", "items.id")+` AS sphere, artifact_id, actor_id, visible_after, follow_up_at, due_at, source, source_ref, review_target, reviewer, reviewed_at, created_at, updated_at
		 FROM items
		 WHERE id = ?`,
		id,
	))
}

func (s *Store) GetItemBySource(source, sourceRef string) (Item, error) {
	cleanSource := strings.TrimSpace(source)
	cleanSourceRef := strings.TrimSpace(sourceRef)
	if cleanSource == "" || cleanSourceRef == "" {
		return Item{}, errors.New("item source and source_ref are required")
	}
	return scanItem(s.db.QueryRow(
		`SELECT id, title, kind, state, workspace_id, `+scopedContextSelect("context_items", "item_id", "items.id")+` AS sphere, artifact_id, actor_id, visible_after, follow_up_at, due_at, source, source_ref, review_target, reviewer, reviewed_at, created_at, updated_at
		 FROM items
		 WHERE source = ? AND source_ref = ?`,
		cleanSource,
		cleanSourceRef,
	))
}

func (s *Store) UpsertItemFromSource(source, sourceRef, title string, workspaceID *int64) (Item, error) {
	cleanSource := strings.TrimSpace(source)
	cleanSourceRef := strings.TrimSpace(sourceRef)
	cleanTitle := strings.TrimSpace(title)
	if cleanSource == "" || cleanSourceRef == "" {
		return Item{}, errors.New("item source and source_ref are required")
	}
	if cleanTitle == "" {
		return Item{}, errors.New("item title is required")
	}

	existing, err := s.GetItemBySource(cleanSource, cleanSourceRef)
	switch {
	case err == nil:
		itemSphere, err := s.resolveItemSphere(workspaceID, &existing.Sphere)
		if err != nil {
			return Item{}, err
		}
		res, err := s.db.Exec(
			`UPDATE items
			 SET title = ?, workspace_id = ?, updated_at = datetime('now')
		 WHERE id = ?`,
			cleanTitle,
			workspaceID,
			existing.ID,
		)
		if err != nil {
			return Item{}, err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return Item{}, err
		}
		if affected == 0 {
			return Item{}, sql.ErrNoRows
		}
		if err := s.syncScopedContextLink("context_items", "item_id", existing.ID, itemSphere); err != nil {
			return Item{}, err
		}
		if workspaceID != nil || existing.WorkspaceID != nil {
			if err := s.syncItemDateContext(existing.ID, workspaceID); err != nil {
				return Item{}, err
			}
		}
		return s.GetItem(existing.ID)
	case !errors.Is(err, sql.ErrNoRows):
		return Item{}, err
	}

	return s.CreateItem(cleanTitle, ItemOptions{
		WorkspaceID: workspaceID,
		Source:      &cleanSource,
		SourceRef:   &cleanSourceRef,
	})
}

func (s *Store) UpdateItemArtifact(id int64, artifactID *int64) error {
	return s.syncPrimaryItemArtifact(id, artifactID)
}

func (s *Store) UpdateItemSource(id int64, source, sourceRef string) error {
	cleanSource := strings.TrimSpace(source)
	cleanSourceRef := strings.TrimSpace(sourceRef)
	if cleanSource == "" || cleanSourceRef == "" {
		return errors.New("item source and source_ref are required")
	}
	existing, err := s.GetItemBySource(cleanSource, cleanSourceRef)
	switch {
	case err == nil && existing.ID != id:
		return fmt.Errorf("item source %s:%s is already linked to item %d", cleanSource, cleanSourceRef, existing.ID)
	case err != nil && !errors.Is(err, sql.ErrNoRows):
		return err
	}
	res, err := s.db.Exec(
		`UPDATE items
		 SET source = ?, source_ref = ?, updated_at = datetime('now')
		 WHERE id = ?`,
		cleanSource,
		cleanSourceRef,
		id,
	)
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

func normalizedReviewTargetPointer(value *string) *string {
	if value == nil {
		return nil
	}
	clean := normalizeItemReviewTarget(*value)
	if clean == "" {
		return nil
	}
	return &clean
}

func validateReviewTargetPointer(value *string) error {
	if value == nil {
		return nil
	}
	if strings.TrimSpace(*value) == "" {
		return nil
	}
	if normalizedReviewTargetPointer(value) == nil {
		return errors.New("review target must be agent, github, or email")
	}
	return nil
}

func normalizedReviewerPointer(value *string) *string {
	if value == nil {
		return nil
	}
	clean := strings.TrimSpace(*value)
	if clean == "" {
		return nil
	}
	return &clean
}

func reviewTimestampPointer(target, reviewer *string) *string {
	if normalizedReviewTargetPointer(target) == nil && normalizedReviewerPointer(reviewer) == nil {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return &now
}

func (s *Store) UpdateItemReviewDispatch(id int64, target, reviewer *string) error {
	if err := validateReviewTargetPointer(target); err != nil {
		return err
	}
	cleanTarget := normalizedReviewTargetPointer(target)
	cleanReviewer := normalizedReviewerPointer(reviewer)
	if cleanTarget == nil && cleanReviewer != nil {
		return errors.New("review target is required when reviewer is set")
	}
	res, err := s.db.Exec(
		`UPDATE items
		 SET review_target = ?, reviewer = ?, reviewed_at = ?, updated_at = datetime('now')
		 WHERE id = ?`,
		normalizeOptionalString(cleanTarget),
		normalizeOptionalString(cleanReviewer),
		normalizeOptionalString(reviewTimestampPointer(target, reviewer)),
		id,
	)
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

func (s *Store) UpdateItem(id int64, updates ItemUpdate) error {
	item, err := s.GetItem(id)
	if err != nil {
		return err
	}
	plan, err := s.buildItemUpdatePlan(id, item, updates)
	if err != nil {
		return err
	}
	if len(plan.parts) == 0 && !plan.artifactUpdated && !plan.scopeUpdated {
		return nil
	}
	if err := s.applyItemUpdateParts(id, plan.parts, plan.args); err != nil {
		return err
	}
	if plan.artifactUpdated {
		if err := s.syncPrimaryItemArtifact(id, plan.artifactID); err != nil {
			return err
		}
	}
	if plan.item.Sphere != "" {
		if err := s.syncScopedContextLink("context_items", "item_id", id, plan.item.Sphere); err != nil {
			return err
		}
	}
	if updates.WorkspaceID != nil || item.WorkspaceID != nil {
		return s.syncItemDateContext(id, plan.targetWorkspaceID)
	}
	return nil
}

func (s *Store) SetItemSphere(id int64, sphere string) error {
	item, err := s.GetItem(id)
	if err != nil {
		return err
	}
	if item.WorkspaceID != nil {
		return errors.New("item sphere is derived from workspace")
	}
	cleanSphere := normalizeRequiredSphere(sphere)
	if cleanSphere == "" {
		return errors.New("item sphere must be work or private")
	}
	res, err := s.db.Exec(`UPDATE items SET updated_at = datetime('now') WHERE id = ?`, id)
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
	return s.syncScopedContextLink("context_items", "item_id", id, cleanSphere)
}

func (s *Store) ListSphereAccounts(sphere string) ([]ExternalAccount, error) {
	return s.ListExternalAccounts(sphere)
}

func (s *Store) AddSphereAccount(sphere, kind, label string, config map[string]any) (ExternalAccount, error) {
	return s.CreateExternalAccount(sphere, kind, label, config)
}

func (s *Store) RemoveSphereAccount(id int64) error {
	return s.DeleteExternalAccount(id)
}

func (s *Store) SyncItemStateBySource(source, sourceRef, state string) error {
	cleanSource := strings.TrimSpace(source)
	cleanSourceRef := strings.TrimSpace(sourceRef)
	cleanState := normalizeItemState(state)
	if cleanSource == "" || cleanSourceRef == "" {
		return errors.New("item source and source_ref are required")
	}
	if cleanState == "" {
		return errors.New("invalid item state")
	}
	query := `UPDATE items
		 SET state = ?, updated_at = datetime('now')`
	args := []any{cleanState}
	if cleanState == ItemStateInbox {
		query += `, visible_after = NULL, follow_up_at = NULL`
	}
	query += `
		 WHERE source = ? AND source_ref = ?`
	args = append(args, cleanSource, cleanSourceRef)
	res, err := s.db.Exec(query, args...)
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

func (s *Store) UpdateItemState(id int64, state string) error {
	item, err := s.GetItem(id)
	if err != nil {
		return err
	}
	next := normalizeItemState(state)
	if err := validateItemTransition(item.State, next); err != nil {
		return err
	}
	query := `UPDATE items SET state = ?, updated_at = datetime('now')`
	args := []any{next}
	if next == ItemStateInbox || next == ItemStateNext {
		query += `, visible_after = NULL, follow_up_at = NULL`
	}
	query += ` WHERE id = ?`
	args = append(args, id)
	res, err := s.db.Exec(query, args...)
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

func (s *Store) triageableItem(id int64) (Item, error) {
	item, err := s.GetItem(id)
	if err != nil {
		return Item{}, err
	}
	if item.State == ItemStateDone {
		return Item{}, fmt.Errorf("cannot triage item in %s state", item.State)
	}
	return item, nil
}

func normalizeRFC3339String(value string) (string, error) {
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
	if err != nil {
		return "", err
	}
	return parsed.UTC().Format(time.RFC3339Nano), nil
}

func (s *Store) TriageItemDone(id int64) error {
	if _, err := s.triageableItem(id); err != nil {
		return err
	}
	res, err := s.db.Exec(
		`UPDATE items
		 SET state = ?, updated_at = datetime('now')
		 WHERE id = ?`,
		ItemStateDone,
		id,
	)
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

func (s *Store) TriageItemLater(id int64, visibleAfter string) error {
	if _, err := s.triageableItem(id); err != nil {
		return err
	}
	normalized, err := normalizeRFC3339String(visibleAfter)
	if err != nil {
		return errors.New("visible_after must be a valid RFC3339 timestamp")
	}
	res, err := s.db.Exec(
		`UPDATE items
		 SET state = ?, visible_after = ?, updated_at = datetime('now')
		 WHERE id = ?`,
		ItemStateDeferred,
		normalized,
		id,
	)
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

func (s *Store) TriageItemDelegate(id, actorID int64) error {
	if _, err := s.triageableItem(id); err != nil {
		return err
	}
	if _, err := s.GetActor(actorID); err != nil {
		return err
	}
	res, err := s.db.Exec(
		`UPDATE items
		 SET actor_id = ?, state = ?, visible_after = NULL, updated_at = datetime('now')
		 WHERE id = ?`,
		actorID,
		ItemStateWaiting,
		id,
	)
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

func (s *Store) TriageItemDelete(id int64) error {
	if _, err := s.triageableItem(id); err != nil {
		return err
	}
	return s.DeleteItem(id)
}

func (s *Store) TriageItemSomeday(id int64) error {
	if _, err := s.triageableItem(id); err != nil {
		return err
	}
	res, err := s.db.Exec(
		`UPDATE items
		 SET state = ?, visible_after = NULL, follow_up_at = NULL, updated_at = datetime('now')
		 WHERE id = ?`,
		ItemStateSomeday,
		id,
	)
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

func (s *Store) AssignItem(id, actorID int64) error {
	if _, err := s.GetActor(actorID); err != nil {
		return err
	}
	item, err := s.GetItem(id)
	if err != nil {
		return err
	}
	if item.State == ItemStateDone {
		return fmt.Errorf("cannot assign item in %s state", item.State)
	}
	res, err := s.db.Exec(
		`UPDATE items
		 SET actor_id = ?, state = ?, updated_at = datetime('now')
		 WHERE id = ?`,
		actorID,
		ItemStateWaiting,
		id,
	)
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

func (s *Store) UnassignItem(id int64) error {
	item, err := s.GetItem(id)
	if err != nil {
		return err
	}
	if item.State == ItemStateDone {
		return fmt.Errorf("cannot unassign item in %s state", item.State)
	}
	if item.ActorID == nil {
		return errors.New("item is not assigned")
	}
	res, err := s.db.Exec(
		`UPDATE items
		 SET actor_id = NULL, state = ?, visible_after = NULL, follow_up_at = NULL, updated_at = datetime('now')
		 WHERE id = ?`,
		ItemStateInbox,
		id,
	)
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

func (s *Store) CompleteItemByActor(id, actorID int64) error {
	item, err := s.GetItem(id)
	if err != nil {
		return err
	}
	if item.State == ItemStateDone {
		return fmt.Errorf("cannot complete item in %s state", item.State)
	}
	if item.ActorID == nil {
		return errors.New("item is not assigned")
	}
	if *item.ActorID != actorID {
		return errors.New("item actor does not match")
	}
	res, err := s.db.Exec(
		`UPDATE items
		 SET state = ?, updated_at = datetime('now')
		 WHERE id = ?`,
		ItemStateDone,
		id,
	)
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

func (s *Store) ReturnItemToInbox(id int64) error {
	item, err := s.GetItem(id)
	if err != nil {
		return err
	}
	if item.State == ItemStateDone {
		return fmt.Errorf("cannot return item from %s state", item.State)
	}
	res, err := s.db.Exec(
		`UPDATE items
		 SET state = ?, visible_after = NULL, follow_up_at = NULL, updated_at = datetime('now')
		 WHERE id = ?`,
		ItemStateInbox,
		id,
	)
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

func (s *Store) CompleteItemBySource(source, sourceRef string) error {
	cleanSource := strings.TrimSpace(source)
	cleanSourceRef := strings.TrimSpace(sourceRef)
	if cleanSource == "" || cleanSourceRef == "" {
		return errors.New("item source and source_ref are required")
	}
	res, err := s.db.Exec(
		`UPDATE items
		 SET state = ?, updated_at = datetime('now')
		 WHERE source = ? AND source_ref = ? AND state != ?`,
		ItemStateDone,
		cleanSource,
		cleanSourceRef,
		ItemStateDone,
	)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected > 0 {
		return nil
	}
	var existingCount int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM items WHERE source = ? AND source_ref = ?`,
		cleanSource,
		cleanSourceRef,
	).Scan(&existingCount); err != nil {
		return err
	}
	if existingCount == 0 {
		return sql.ErrNoRows
	}
	return fmt.Errorf("cannot complete item from source %s:%s", cleanSource, cleanSourceRef)
}

func (s *Store) UpdateItemTimes(id int64, visibleAfter, followUpAt *string) error {
	res, err := s.db.Exec(
		`UPDATE items
		 SET visible_after = ?, follow_up_at = ?, updated_at = datetime('now')
		 WHERE id = ?`,
		normalizeOptionalString(visibleAfter),
		normalizeOptionalString(followUpAt),
		id,
	)
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

func (s *Store) ResurfaceDueItems(now time.Time) (int, error) {
	cutoff := now.UTC().Format(time.RFC3339Nano)
	res, err := s.db.Exec(
		`UPDATE items
		 SET state = ?, updated_at = datetime('now')
		 WHERE state = ?
		   AND (
		     (visible_after IS NOT NULL AND trim(visible_after) <> '' AND datetime(visible_after) <= datetime(?))
		     OR
		     (follow_up_at IS NOT NULL AND trim(follow_up_at) <> '' AND datetime(follow_up_at) <= datetime(?))
		   )`,
		ItemStateNext,
		ItemStateDeferred,
		cutoff,
		cutoff,
	)
	if err != nil {
		return 0, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}

func (s *Store) DeleteItem(id int64) error {
	res, err := s.db.Exec(`DELETE FROM items WHERE id = ?`, id)
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

func nullablePositiveID(id int64) any {
	if id <= 0 {
		return nil
	}
	return id
}

func nullStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func normalizeOptionalRFC3339String(value *string) (any, error) {
	if value == nil {
		return nil, nil
	}
	clean := strings.TrimSpace(*value)
	if clean == "" {
		return nil, nil
	}
	normalized, err := normalizeRFC3339String(clean)
	if err != nil {
		return nil, errors.New("timestamps must be valid RFC3339")
	}
	return normalized, nil
}
