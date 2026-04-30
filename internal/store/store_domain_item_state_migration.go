package store

import (
	"database/sql"
	"strings"
)

func (s *Store) migrateItemTableStateSupport() error {
	var schema sql.NullString
	if err := s.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'items'`).Scan(&schema); err != nil {
		return err
	}
	if strings.Contains(strings.ToLower(schema.String), "'review'") {
		return nil
	}
	columns, err := s.tableColumnNames("items")
	if err != nil {
		return err
	}
	preserve := make(map[string]bool, len(columns))
	for _, column := range columns {
		preserve[column] = true
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`ALTER TABLE items RENAME TO items_legacy`); err != nil {
		return err
	}
	if _, err := tx.Exec(strings.Replace(itemsTableSchema, "IF NOT EXISTS ", "", 1)); err != nil {
		return err
	}
	copyColumns := []string{
		"id", "title", "state", "workspace_id", "artifact_id", "actor_id", "visible_after", "follow_up_at",
		"source", "source_ref", "review_target", "reviewer", "reviewed_at", "created_at", "updated_at",
	}
	var kept []string
	for _, column := range copyColumns {
		if preserve[column] {
			kept = append(kept, column)
		}
	}
	columnList := stringsJoin(kept, ", ")
	if _, err := tx.Exec(`INSERT INTO items (` + columnList + `)
SELECT ` + columnList + `
FROM items_legacy`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE items_legacy`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE IF EXISTS context_items`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE TABLE context_items (
  context_id INTEGER NOT NULL REFERENCES contexts(id) ON DELETE CASCADE,
  item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  PRIMARY KEY (context_id, item_id)
)`); err != nil {
		return err
	}
	return tx.Commit()
}
