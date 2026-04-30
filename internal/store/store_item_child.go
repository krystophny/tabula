package store

import (
	"database/sql"
	"errors"
)

const itemChildrenTableSchema = `CREATE TABLE IF NOT EXISTS item_children (
  parent_item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  child_item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  role TEXT NOT NULL DEFAULT 'next_action' CHECK (role IN ('next_action', 'support', 'blocked_by')),
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  PRIMARY KEY (parent_item_id, child_item_id)
);
CREATE INDEX IF NOT EXISTS idx_item_children_child_item_id
  ON item_children(child_item_id);`

func (s *Store) migrateItemChildLinkSupport() error {
	_, err := s.db.Exec(itemChildrenTableSchema)
	return err
}

func (s *Store) LinkItemChild(parentItemID, childItemID int64, role string) error {
	cleanRole := normalizeItemLinkRole(role)
	if cleanRole == "" {
		return errors.New("item child role must be next_action, support, or blocked_by")
	}
	if parentItemID <= 0 || childItemID <= 0 {
		return errors.New("parent_item_id and child_item_id must be positive integers")
	}
	if parentItemID == childItemID {
		return errors.New("parent_item_id and child_item_id must differ")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	parent, err := scanItem(tx.QueryRow(
		`SELECT id, title, kind, state, workspace_id, `+scopedContextSelect("context_items", "item_id", "items.id")+` AS sphere, artifact_id, actor_id, visible_after, follow_up_at, due_at, source, source_ref, review_target, reviewer, reviewed_at, created_at, updated_at
		 FROM items
		 WHERE id = ?`,
		parentItemID,
	))
	if err != nil {
		return err
	}
	if parent.Kind != ItemKindProject {
		return errors.New("parent item must be a project")
	}
	if _, err := scanItem(tx.QueryRow(
		`SELECT id, title, kind, state, workspace_id, `+scopedContextSelect("context_items", "item_id", "items.id")+` AS sphere, artifact_id, actor_id, visible_after, follow_up_at, due_at, source, source_ref, review_target, reviewer, reviewed_at, created_at, updated_at
		 FROM items
		 WHERE id = ?`,
		childItemID,
	)); err != nil {
		return err
	}

	if _, err := tx.Exec(
		`INSERT INTO item_children (parent_item_id, child_item_id, role)
		 VALUES (?, ?, ?)
		 ON CONFLICT(parent_item_id, child_item_id) DO UPDATE SET role = excluded.role`,
		parentItemID,
		childItemID,
		cleanRole,
	); err != nil {
		return err
	}
	if err := s.touchItemTx(tx, parentItemID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) UnlinkItemChild(parentItemID, childItemID int64) error {
	if parentItemID <= 0 || childItemID <= 0 {
		return errors.New("parent_item_id and child_item_id must be positive integers")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	item, err := scanItem(tx.QueryRow(
		`SELECT id, title, kind, state, workspace_id, `+scopedContextSelect("context_items", "item_id", "items.id")+` AS sphere, artifact_id, actor_id, visible_after, follow_up_at, due_at, source, source_ref, review_target, reviewer, reviewed_at, created_at, updated_at
		 FROM items
		 WHERE id = ?`,
		parentItemID,
	))
	if err != nil {
		return err
	}
	if item.Kind != ItemKindProject {
		return errors.New("parent item must be a project")
	}
	res, err := tx.Exec(
		`DELETE FROM item_children
		 WHERE parent_item_id = ? AND child_item_id = ?`,
		parentItemID,
		childItemID,
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
	if err := s.touchItemTx(tx, parentItemID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListItemChildLinks(parentItemID int64) ([]ItemChildLink, error) {
	item, err := s.GetItem(parentItemID)
	if err != nil {
		return nil, err
	}
	if item.Kind != ItemKindProject {
		return nil, errors.New("item is not a project")
	}
	rows, err := s.db.Query(
		`SELECT parent_item_id, child_item_id, role, created_at
		 FROM item_children
		 WHERE parent_item_id = ?
		 ORDER BY CASE role WHEN 'next_action' THEN 0 WHEN 'support' THEN 1 ELSE 2 END,
		          datetime(created_at) ASC,
		          child_item_id ASC`,
		parentItemID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ItemChildLink{}
	for rows.Next() {
		var link ItemChildLink
		if err := rows.Scan(&link.ParentItemID, &link.ChildItemID, &link.Role, &link.CreatedAt); err != nil {
			return nil, err
		}
		link.Role = normalizeItemLinkRole(link.Role)
		out = append(out, link)
	}
	return out, rows.Err()
}

func (s *Store) GetProjectItemHealth(itemID int64) (ProjectItemHealth, error) {
	item, err := s.GetItem(itemID)
	if err != nil {
		return ProjectItemHealth{}, err
	}
	if item.Kind != ItemKindProject {
		return ProjectItemHealth{}, errors.New("item is not a project")
	}
	var health ProjectItemHealth
	var nextCount, waitingCount, deferredCount, somedayCount int
	err = s.db.QueryRow(
		`SELECT
		   COALESCE(SUM(CASE WHEN child.state = ? THEN 1 ELSE 0 END), 0) AS next_count,
		   COALESCE(SUM(CASE WHEN child.state = ? THEN 1 ELSE 0 END), 0) AS waiting_count,
		   COALESCE(SUM(CASE WHEN child.state = ? THEN 1 ELSE 0 END), 0) AS deferred_count,
		   COALESCE(SUM(CASE WHEN child.state = ? THEN 1 ELSE 0 END), 0) AS someday_count
		 FROM item_children links
		 JOIN items child ON child.id = links.child_item_id
		 WHERE links.parent_item_id = ?`,
		ItemStateNext,
		ItemStateWaiting,
		ItemStateDeferred,
		ItemStateSomeday,
		itemID,
	).Scan(&nextCount, &waitingCount, &deferredCount, &somedayCount)
	if err != nil {
		return ProjectItemHealth{}, err
	}
	health.HasNextAction = nextCount > 0
	health.HasWaiting = waitingCount > 0
	health.HasDeferred = deferredCount > 0
	health.HasSomeday = somedayCount > 0
	health.Stalled = !health.HasNextAction && !health.HasWaiting && !health.HasDeferred && !health.HasSomeday
	return health, nil
}

func (s *Store) touchItem(id int64) error {
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
	return nil
}
