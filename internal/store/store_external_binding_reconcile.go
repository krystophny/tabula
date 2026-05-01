package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type ExternalBindingReconcileUpdate struct {
	ObjectType        string
	OldRemoteID       string
	NewRemoteID       string
	ContainerRef      *string
	FollowUpItemState *string
}

func (s *Store) ApplyExternalBindingReconcileUpdates(accountID int64, provider string, updates []ExternalBindingReconcileUpdate) error {
	if _, err := s.validateExternalBindingAccount(accountID, provider); err != nil {
		return err
	}
	if len(updates) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	cleanProvider := normalizeExternalAccountProvider(provider)
	for _, update := range updates {
		if err := applyExternalBindingReconcileUpdateTx(tx, accountID, cleanProvider, update, now); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	tx = nil
	return nil
}

type externalBindingReconcileTarget struct {
	bindingID       int64
	itemID          sql.NullInt64
	artifactID      sql.NullInt64
	currentRemoteID string
	container       sql.NullString
}

func applyExternalBindingReconcileUpdateTx(tx *sql.Tx, accountID int64, provider string, update ExternalBindingReconcileUpdate, now string) error {
	objectType, lookupRemoteID, newRemoteID, err := normalizeExternalBindingReconcileUpdate(update)
	if err != nil || lookupRemoteID == "" {
		return err
	}
	target, err := loadExternalBindingReconcileTarget(tx, accountID, provider, objectType, lookupRemoteID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	targetRemoteID := firstNonEmptyString(newRemoteID, target.currentRemoteID)
	targetContainer := normalizeOptionalString(update.ContainerRef)
	if targetContainer == nil {
		targetContainer = nullStringPointer(target.container)
	}
	if err := updateExternalBindingReconcileTarget(tx, accountID, provider, objectType, target, targetRemoteID, targetContainer, now); err != nil {
		return err
	}
	return updateExternalBindingReconcileItemState(tx, target.itemID, update.FollowUpItemState)
}

func normalizeExternalBindingReconcileUpdate(update ExternalBindingReconcileUpdate) (string, string, string, error) {
	objectType := normalizeExternalBindingObjectType(update.ObjectType)
	if objectType == "" {
		return "", "", "", errors.New("external binding reconcile update object_type is required")
	}
	oldRemoteID := normalizeExternalBindingRemoteID(update.OldRemoteID)
	newRemoteID := normalizeExternalBindingRemoteID(update.NewRemoteID)
	if oldRemoteID != "" {
		return objectType, oldRemoteID, newRemoteID, nil
	}
	return objectType, newRemoteID, newRemoteID, nil
}

func loadExternalBindingReconcileTarget(tx *sql.Tx, accountID int64, provider, objectType, remoteID string) (externalBindingReconcileTarget, error) {
	var out externalBindingReconcileTarget
	var ignoredRemoteAt sql.NullString
	var ignoredSyncedAt string
	err := tx.QueryRow(
		`SELECT id, item_id, artifact_id, remote_id, container_ref, remote_updated_at, last_synced_at
		 FROM external_bindings
		 WHERE account_id = ? AND provider = ? AND object_type = ? AND remote_id = ?`,
		accountID,
		provider,
		objectType,
		remoteID,
	).Scan(&out.bindingID, &out.itemID, &out.artifactID, &out.currentRemoteID, &out.container, &ignoredRemoteAt, &ignoredSyncedAt)
	return out, err
}

func updateExternalBindingReconcileTarget(tx *sql.Tx, accountID int64, provider, objectType string, target externalBindingReconcileTarget, targetRemoteID string, targetContainer any, now string) error {
	if targetRemoteID == target.currentRemoteID {
		_, err := tx.Exec(`UPDATE external_bindings SET container_ref = ?, last_synced_at = ? WHERE id = ?`,
			targetContainer, now, target.bindingID)
		return err
	}
	existingID, err := findExternalBindingReconcileTargetID(tx, accountID, provider, objectType, targetRemoteID)
	if err != nil {
		return err
	}
	if existingID.Valid && existingID.Int64 != target.bindingID {
		return mergeExternalBindingReconcileTarget(tx, target, existingID.Int64, targetContainer, now)
	}
	_, err = tx.Exec(`UPDATE external_bindings SET remote_id = ?, container_ref = ?, last_synced_at = ? WHERE id = ?`,
		targetRemoteID, targetContainer, now, target.bindingID)
	return err
}

func findExternalBindingReconcileTargetID(tx *sql.Tx, accountID int64, provider, objectType, remoteID string) (sql.NullInt64, error) {
	var existingID sql.NullInt64
	err := tx.QueryRow(
		`SELECT id FROM external_bindings WHERE account_id = ? AND provider = ? AND object_type = ? AND remote_id = ?`,
		accountID, provider, objectType, remoteID,
	).Scan(&existingID)
	if errors.Is(err, sql.ErrNoRows) {
		return existingID, nil
	}
	return existingID, err
}

func mergeExternalBindingReconcileTarget(tx *sql.Tx, target externalBindingReconcileTarget, existingID int64, targetContainer any, now string) error {
	if _, err := tx.Exec(
		`UPDATE external_bindings
		 SET item_id = COALESCE(item_id, ?), artifact_id = COALESCE(artifact_id, ?),
		     container_ref = ?, last_synced_at = ?
		 WHERE id = ?`,
		nullablePositiveID(target.itemID.Int64),
		nullablePositiveID(target.artifactID.Int64),
		targetContainer,
		now,
		existingID,
	); err != nil {
		return err
	}
	_, err := tx.Exec(`DELETE FROM external_bindings WHERE id = ?`, target.bindingID)
	return err
}

func updateExternalBindingReconcileItemState(tx *sql.Tx, itemID sql.NullInt64, stateValue *string) error {
	if !itemID.Valid || stateValue == nil {
		return nil
	}
	state := strings.TrimSpace(*stateValue)
	if state == "" {
		return nil
	}
	_, err := tx.Exec(`UPDATE items SET state = ? WHERE id = ?`, state, itemID.Int64)
	return err
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (s *Store) RecordExternalBindingDrift(binding ExternalBinding, local, upstream Item) (ExternalBindingDrift, error) {
	if binding.ID <= 0 || local.ID <= 0 {
		return ExternalBindingDrift{}, errors.New("drift binding and item are required")
	}
	revision := externalBindingDriftRevision(binding, upstream)
	existing, err := s.getExternalBindingDriftByRevision(binding.ID, revision)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ExternalBindingDrift{}, err
	}
	if err == nil && existing.ResolvedAt != nil {
		return existing, nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err == nil {
		return s.updateExternalBindingDrift(existing.ID, binding, local, upstream, revision, now)
	}
	openDrift, err := s.getUnresolvedExternalBindingDriftByBinding(binding.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ExternalBindingDrift{}, err
	}
	if err == nil {
		return s.updateExternalBindingDrift(openDrift.ID, binding, local, upstream, revision, now)
	}
	if _, err := s.db.Exec(
		`INSERT INTO external_binding_drifts (
	binding_id, item_id, account_id, provider, object_type, remote_id, source_container,
	local_state, upstream_state, local_title, upstream_title, local_updated_at,
	upstream_updated_at, upstream_revision, detected_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		binding.ID,
		local.ID,
		binding.AccountID,
		binding.Provider,
		binding.ObjectType,
		binding.RemoteID,
		normalizeOptionalString(binding.ContainerRef),
		local.State,
		upstream.State,
		local.Title,
		upstream.Title,
		local.UpdatedAt,
		normalizeOptionalString(binding.RemoteUpdatedAt),
		revision,
		now,
	); err != nil {
		return ExternalBindingDrift{}, err
	}
	return s.getExternalBindingDriftByRevision(binding.ID, revision)
}

func (s *Store) updateExternalBindingDrift(id int64, binding ExternalBinding, local, upstream Item, revision, detectedAt string) (ExternalBindingDrift, error) {
	if _, err := s.db.Exec(
		`UPDATE external_binding_drifts
SET item_id = ?, account_id = ?, provider = ?, object_type = ?, remote_id = ?,
    source_container = ?, local_state = ?, upstream_state = ?, local_title = ?,
    upstream_title = ?, local_updated_at = ?, upstream_updated_at = ?,
    upstream_revision = ?, detected_at = ?
WHERE id = ? AND resolved_at IS NULL`,
		local.ID,
		binding.AccountID,
		binding.Provider,
		binding.ObjectType,
		binding.RemoteID,
		normalizeOptionalString(binding.ContainerRef),
		local.State,
		upstream.State,
		local.Title,
		upstream.Title,
		local.UpdatedAt,
		normalizeOptionalString(binding.RemoteUpdatedAt),
		revision,
		detectedAt,
		id,
	); err != nil {
		return ExternalBindingDrift{}, err
	}
	return s.GetExternalBindingDrift(id)
}

func (s *Store) ListUnresolvedExternalBindingDrifts(filter ItemListFilter) ([]ExternalBindingDrift, error) {
	scoped := filter
	scoped.Section = ""
	normalizedFilter, err := s.prepareItemListFilter(scoped)
	if err != nil {
		return nil, err
	}
	parts := []string{"d.resolved_at IS NULL"}
	args := []any{}
	parts, args = appendItemFilterClauses(parts, args, normalizedFilter, "i.")
	rows, err := s.db.Query(externalBindingDriftSelect+` WHERE `+stringsJoin(parts, ` AND `)+`
ORDER BY datetime(d.detected_at) DESC, d.id ASC`, args...)
	if err != nil {
		return nil, err
	}
	drifts, err := scanExternalBindingDriftRows(rows)
	closeErr := rows.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return s.withDriftProjectItemLinksForRows(drifts)
}

func (s *Store) CountUnresolvedExternalBindingDrifts(filter ItemListFilter) (int, error) {
	drifts, err := s.ListUnresolvedExternalBindingDrifts(filter)
	if err != nil {
		return 0, err
	}
	return len(drifts), nil
}

func (s *Store) HasLocalExternalBindingDrift(bindingID int64, localState string) (bool, error) {
	state := strings.TrimSpace(localState)
	if bindingID <= 0 || state == "" {
		return false, nil
	}
	var found int
	err := s.db.QueryRow(
		`SELECT 1
		 FROM external_binding_drifts
		 WHERE binding_id = ?
		   AND local_state = ?
		   AND (
		     resolved_at IS NULL
		     OR resolution IN (?, ?)
		   )
		 LIMIT 1`,
		bindingID,
		state,
		ExternalBindingDriftActionKeepLocal,
		ExternalBindingDriftActionDismiss,
	).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) ResolveExternalBindingDrift(id int64, action string) (ExternalBindingDrift, error) {
	cleanAction, err := normalizeExternalBindingDriftAction(action)
	if err != nil {
		return ExternalBindingDrift{}, err
	}
	drift, err := s.GetExternalBindingDrift(id)
	if err != nil {
		return ExternalBindingDrift{}, err
	}
	if drift.ResolvedAt != nil {
		return drift, nil
	}
	if cleanAction == ExternalBindingDriftActionTakeUpstream {
		if err := s.applyDriftUpstreamState(drift); err != nil {
			return ExternalBindingDrift{}, err
		}
	}
	return s.markExternalBindingDriftResolved(id, cleanAction)
}

func (s *Store) MarkExternalBindingDriftReingested(id int64) (ExternalBindingDrift, error) {
	drift, err := s.GetExternalBindingDrift(id)
	if err != nil {
		return ExternalBindingDrift{}, err
	}
	if drift.ResolvedAt != nil {
		return drift, nil
	}
	return s.markExternalBindingDriftResolved(id, ExternalBindingDriftActionReingest)
}

func (s *Store) markExternalBindingDriftResolved(id int64, resolution string) (ExternalBindingDrift, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.Exec(`UPDATE external_binding_drifts SET resolved_at = ?, resolution = ? WHERE id = ?`, now, resolution, id); err != nil {
		return ExternalBindingDrift{}, err
	}
	return s.GetExternalBindingDrift(id)
}

func (s *Store) applyDriftUpstreamState(drift ExternalBindingDrift) error {
	if drift.ItemID == nil || *drift.ItemID <= 0 {
		return errors.New("drift item is missing")
	}
	state := strings.TrimSpace(drift.UpstreamState)
	if normalizeItemState(state) == "" {
		return errors.New("drift upstream_state is invalid")
	}
	title := strings.TrimSpace(drift.UpstreamTitle)
	if title == "" {
		title = strings.TrimSpace(drift.LocalTitle)
	}
	_, err := s.db.Exec(`UPDATE items SET title = ?, state = ?, updated_at = ? WHERE id = ?`,
		title, state, time.Now().UTC().Format(time.RFC3339Nano), *drift.ItemID)
	return err
}

func normalizeExternalBindingDriftAction(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case ExternalBindingDriftActionKeepLocal:
		return ExternalBindingDriftActionKeepLocal, nil
	case ExternalBindingDriftActionTakeUpstream:
		return ExternalBindingDriftActionTakeUpstream, nil
	case ExternalBindingDriftActionDismiss:
		return ExternalBindingDriftActionDismiss, nil
	default:
		return "", errors.New("action must be keep_local, take_upstream, or dismiss")
	}
}

func externalBindingDriftRevision(binding ExternalBinding, upstream Item) string {
	remoteAt := strings.TrimSpace(externalBindingDriftString(binding.RemoteUpdatedAt))
	if remoteAt != "" {
		return remoteAt
	}
	return fmt.Sprintf("%s|%s|%s", strings.TrimSpace(upstream.State), strings.TrimSpace(upstream.Title), strings.TrimSpace(externalBindingDriftString(binding.ContainerRef)))
}

func externalBindingDriftString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

const externalBindingDriftSelect = `SELECT
d.id, d.binding_id, d.item_id, d.account_id, d.provider, d.object_type, d.remote_id,
d.source_container, d.local_state, d.upstream_state, d.local_title, d.upstream_title,
d.local_updated_at, d.upstream_updated_at, d.upstream_revision, d.detected_at,
d.resolved_at, d.resolution, i.workspace_id
FROM external_binding_drifts d
LEFT JOIN items i ON i.id = d.item_id`

func (s *Store) GetExternalBindingDrift(id int64) (ExternalBindingDrift, error) {
	drift, err := scanExternalBindingDrift(s.db.QueryRow(externalBindingDriftSelect+` WHERE d.id = ?`, id))
	if err != nil {
		return ExternalBindingDrift{}, err
	}
	return s.withDriftProjectItemLinks(drift)
}

func (s *Store) getExternalBindingDriftByRevision(bindingID int64, revision string) (ExternalBindingDrift, error) {
	return scanExternalBindingDrift(s.db.QueryRow(externalBindingDriftSelect+` WHERE d.binding_id = ? AND d.upstream_revision = ?`, bindingID, revision))
}

func (s *Store) getUnresolvedExternalBindingDriftByBinding(bindingID int64) (ExternalBindingDrift, error) {
	return scanExternalBindingDrift(s.db.QueryRow(externalBindingDriftSelect+` WHERE d.binding_id = ? AND d.resolved_at IS NULL`, bindingID))
}

func scanExternalBindingDrift(row interface{ Scan(dest ...any) error }) (ExternalBindingDrift, error) {
	var out ExternalBindingDrift
	var itemID, workspaceID sql.NullInt64
	var container, upstreamAt, resolvedAt, resolution sql.NullString
	if err := row.Scan(&out.ID, &out.BindingID, &itemID, &out.AccountID, &out.Provider, &out.ObjectType,
		&out.RemoteID, &container, &out.LocalState, &out.UpstreamState, &out.LocalTitle,
		&out.UpstreamTitle, &out.LocalUpdatedAt, &upstreamAt, &out.UpstreamRevision,
		&out.DetectedAt, &resolvedAt, &resolution, &workspaceID); err != nil {
		return ExternalBindingDrift{}, err
	}
	out.ItemID = nullInt64Pointer(itemID)
	out.WorkspaceID = nullInt64Pointer(workspaceID)
	out.SourceContainer = nullStringPointer(container)
	out.UpstreamUpdatedAt = nullStringPointer(upstreamAt)
	out.ResolvedAt = nullStringPointer(resolvedAt)
	out.Resolution = nullStringPointer(resolution)
	out.SourceBinding = fmt.Sprintf("%s:%s:%s", out.Provider, out.ObjectType, out.RemoteID)
	out.Title = out.LocalTitle
	out.Kind = "drift"
	out.State = ItemStateReview
	return out, nil
}

func scanExternalBindingDriftRows(rows *sql.Rows) ([]ExternalBindingDrift, error) {
	var out []ExternalBindingDrift
	for rows.Next() {
		drift, err := scanExternalBindingDrift(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, drift)
	}
	return out, rows.Err()
}

func (s *Store) withDriftProjectItemLinksForRows(drifts []ExternalBindingDrift) ([]ExternalBindingDrift, error) {
	for i := range drifts {
		next, err := s.withDriftProjectItemLinks(drifts[i])
		if err != nil {
			return nil, err
		}
		drifts[i] = next
	}
	return drifts, nil
}

func (s *Store) withDriftProjectItemLinks(drift ExternalBindingDrift) (ExternalBindingDrift, error) {
	if drift.ItemID == nil {
		return drift, nil
	}
	rows, err := s.db.Query(`SELECT parent.title || ' (' || links.role || ')' FROM item_children links JOIN items parent ON parent.id = links.parent_item_id WHERE links.child_item_id = ? ORDER BY parent.title`, *drift.ItemID)
	if err != nil {
		return ExternalBindingDrift{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var label string
		if err := rows.Scan(&label); err != nil {
			return ExternalBindingDrift{}, err
		}
		drift.ProjectItemLinks = append(drift.ProjectItemLinks, label)
	}
	return drift, rows.Err()
}
