package store

import "strings"

func (s *Store) migrateProjectRemovalSupport() error {
	tableColumns, err := s.tableColumnSet("workspaces", "items", "time_entries", "external_container_mappings")
	if err != nil {
		return err
	}
	if !tableColumns["workspaces"]["project_id"] &&
		!tableColumns["items"]["project_id"] &&
		!tableColumns["time_entries"]["project_id"] &&
		!tableColumns["external_container_mappings"]["project_id"] {
		_, _ = s.db.Exec(`DROP TABLE IF EXISTS projects`)
		_, _ = s.db.Exec(`DELETE FROM app_state WHERE key = 'active_project_id'`)
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`ALTER TABLE workspaces RENAME TO workspaces_project_legacy`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE TABLE workspaces (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  dir_path TEXT NOT NULL UNIQUE,
  is_active INTEGER NOT NULL DEFAULT 0,
  is_daily INTEGER NOT NULL DEFAULT 0,
  daily_date TEXT,
  mcp_url TEXT NOT NULL DEFAULT '',
  canvas_session_id TEXT NOT NULL DEFAULT '',
  chat_model TEXT NOT NULL DEFAULT '',
  chat_model_reasoning_effort TEXT NOT NULL DEFAULT '',
  companion_config_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO workspaces (
id, name, dir_path, is_active, is_daily, daily_date, mcp_url, canvas_session_id, chat_model, chat_model_reasoning_effort, companion_config_json, created_at, updated_at
)
SELECT
id, name, dir_path, is_active, COALESCE(is_daily, 0), daily_date, COALESCE(mcp_url, ''), COALESCE(canvas_session_id, ''), COALESCE(chat_model, ''), COALESCE(chat_model_reasoning_effort, ''), COALESCE(companion_config_json, '{}'), created_at, updated_at
FROM workspaces_project_legacy`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE workspaces_project_legacy`); err != nil {
		return err
	}

	if _, err := tx.Exec(`ALTER TABLE items RENAME TO items_project_legacy`); err != nil {
		return err
	}
	if _, err := tx.Exec(strings.Replace(itemsTableSchema, "IF NOT EXISTS ", "", 1)); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO items (
id, title, state, workspace_id, artifact_id, actor_id, visible_after, follow_up_at, source, source_ref, review_target, reviewer, reviewed_at, created_at, updated_at
)
SELECT
id, title, state, workspace_id, artifact_id, actor_id, visible_after, follow_up_at, source, source_ref, review_target, reviewer, reviewed_at, created_at, updated_at
FROM items_project_legacy`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE items_project_legacy`); err != nil {
		return err
	}

	if _, err := tx.Exec(`ALTER TABLE time_entries RENAME TO time_entries_project_legacy`); err != nil {
		return err
	}
	if _, err := tx.Exec(strings.Replace(timeEntriesTableSchema, "IF NOT EXISTS ", "", 1)); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO time_entries (
id, workspace_id, started_at, ended_at, activity, notes
)
SELECT
id, workspace_id, started_at, ended_at, activity, notes
FROM time_entries_project_legacy`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE time_entries_project_legacy`); err != nil {
		return err
	}

	if _, err := tx.Exec(`ALTER TABLE external_container_mappings RENAME TO external_container_mappings_project_legacy`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE TABLE external_container_mappings (
  id INTEGER PRIMARY KEY,
  provider TEXT NOT NULL,
  container_type TEXT NOT NULL,
  container_ref TEXT NOT NULL,
  workspace_id INTEGER REFERENCES workspaces(id) ON DELETE SET NULL
)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO external_container_mappings (
id, provider, container_type, container_ref, workspace_id
)
SELECT
id, provider, container_type, container_ref, workspace_id
FROM external_container_mappings_project_legacy`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE external_container_mappings_project_legacy`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE IF EXISTS projects`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM app_state WHERE key = 'active_project_id'`); err != nil {
		return err
	}
	return tx.Commit()
}
