package store

import (
	"database/sql"
	"strings"
)

var itemChildIndexSQL = []string{
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_external_bindings_identity
  ON external_bindings(account_id, provider, object_type, remote_id)`,
	`CREATE INDEX IF NOT EXISTS idx_external_bindings_stale
  ON external_bindings(provider, last_synced_at)`,
	`CREATE INDEX IF NOT EXISTS idx_batch_run_items_batch_status
  ON batch_run_items(batch_id, status, item_id)`,
}

func (s *Store) migrateItemTableStateSupport() error {
	if err := s.repairItemLegacyForeignKeys(); err != nil {
		return err
	}
	var schema sql.NullString
	if err := s.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'items'`).Scan(&schema); err != nil {
		return err
	}
	if strings.Contains(strings.ToLower(schema.String), "'review'") {
		return nil
	}
	if _, err := s.db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	defer func() {
		_, _ = s.db.Exec(`PRAGMA legacy_alter_table = OFF`)
		_, _ = s.db.Exec(`PRAGMA foreign_keys = ON`)
	}()
	if _, err := s.db.Exec(`PRAGMA legacy_alter_table = ON`); err != nil {
		return err
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

func (s *Store) repairItemLegacyForeignKeys() error {
	rows, err := s.db.Query(`SELECT name, sql FROM sqlite_master WHERE type = 'table' AND sql LIKE '%items_legacy%'`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type brokenTable struct {
		name string
		sql  string
	}
	var broken []brokenTable
	for rows.Next() {
		var table brokenTable
		if err := rows.Scan(&table.name, &table.sql); err != nil {
			return err
		}
		if table.name == "items" || table.name == "items_legacy" {
			continue
		}
		broken = append(broken, table)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, table := range broken {
		if err := s.rebuildItemChildTable(table.name, table.sql); err != nil {
			return err
		}
	}
	for _, stmt := range itemChildIndexSQL {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) rebuildItemChildTable(name, createSQL string) error {
	columns, err := s.tableColumnNames(name)
	if err != nil {
		return err
	}
	fixedSQL := strings.ReplaceAll(createSQL, `REFERENCES "items_legacy"`, `REFERENCES items`)
	fixedSQL = strings.ReplaceAll(fixedSQL, `REFERENCES items_legacy`, `REFERENCES items`)
	legacyName := name + "_item_fk_legacy"
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`ALTER TABLE ` + quoteIdent(name) + ` RENAME TO ` + quoteIdent(legacyName)); err != nil {
		return err
	}
	if _, err := tx.Exec(fixedSQL); err != nil {
		return err
	}
	columnList := stringsJoin(quoteIdentList(columns), ", ")
	if _, err := tx.Exec(`INSERT INTO ` + quoteIdent(name) + ` (` + columnList + `)
SELECT ` + columnList + `
FROM ` + quoteIdent(legacyName)); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE ` + quoteIdent(legacyName)); err != nil {
		return err
	}
	return tx.Commit()
}

func quoteIdentList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, quoteIdent(value))
	}
	return out
}

func quoteIdent(value string) string {
	clean := strings.TrimSpace(value)
	if clean == "" {
		panic("empty SQL identifier")
	}
	return `"` + strings.ReplaceAll(clean, `"`, `""`) + `"`
}
