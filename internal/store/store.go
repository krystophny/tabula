package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type HostConfig struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Hostname   string `json:"hostname"`
	Port       int    `json:"port"`
	Username   string `json:"username"`
	KeyPath    string `json:"key_path"`
	ProjectDir string `json:"project_dir"`
}

type Store struct {
	db *sql.DB
}

type ChatSession struct {
	ID          string `json:"id"`
	ProjectKey  string `json:"project_key"`
	AppThreadID string `json:"app_thread_id"`
	Mode        string `json:"mode"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

type ChatMessage struct {
	ID              int64  `json:"id"`
	SessionID       string `json:"session_id"`
	Role            string `json:"role"`
	ContentMarkdown string `json:"content_markdown"`
	ContentPlain    string `json:"content_plain"`
	RenderFormat    string `json:"render_format"`
	ThreadKey       string `json:"thread_key,omitempty"`
	CreatedAt       int64  `json:"created_at"`
}

type Project struct {
	ID                       string `json:"id"`
	Name                     string `json:"name"`
	ProjectKey               string `json:"project_key"`
	RootPath                 string `json:"root_path"`
	Kind                     string `json:"kind"`
	MCPURL                   string `json:"mcp_url,omitempty"`
	CanvasSessionID          string `json:"canvas_session_id"`
	ChatModel                string `json:"chat_model"`
	ChatModelReasoningEffort string `json:"chat_model_reasoning_effort"`
	IsDefault                bool   `json:"is_default"`
	CreatedAt                int64  `json:"created_at"`
	UpdatedAt                int64  `json:"updated_at"`
	LastOpenedAt             int64  `json:"last_opened_at"`
}

func New(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

// tableColumnNames returns the lowercased column names for a single table.
func (s *Store) tableColumnNames(table string) ([]string, error) {
	rows, err := s.db.Query(fmt.Sprintf(`PRAGMA table_info("%s")`, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols = append(cols, strings.ToLower(strings.TrimSpace(name)))
	}
	return cols, rows.Err()
}

// TableColumns returns a map from table name to the list of column names
// for every user table in the database.
func (s *Store) TableColumns() (map[string][]string, error) {
	rows, err := s.db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make(map[string][]string, len(tables))
	for _, table := range tables {
		cols, err := s.tableColumnNames(table)
		if err != nil {
			return nil, err
		}
		result[table] = cols
	}
	return result, nil
}

func (s *Store) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS hosts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE,
  hostname TEXT NOT NULL,
  port INTEGER NOT NULL DEFAULT 22,
  username TEXT NOT NULL,
  key_path TEXT NOT NULL DEFAULT '',
  project_dir TEXT NOT NULL DEFAULT '~'
);
CREATE TABLE IF NOT EXISTS admin (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  pw_hash TEXT NOT NULL,
  pw_salt TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS auth_sessions (
  token TEXT PRIMARY KEY,
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS remote_sessions (
  session_id TEXT PRIMARY KEY,
  host_id INTEGER NOT NULL,
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS chat_sessions (
  id TEXT PRIMARY KEY,
  project_key TEXT NOT NULL UNIQUE,
  app_thread_id TEXT NOT NULL DEFAULT '',
  mode TEXT NOT NULL DEFAULT 'chat',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS chat_messages (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL,
  role TEXT NOT NULL,
  content_markdown TEXT NOT NULL DEFAULT '',
  content_plain TEXT NOT NULL DEFAULT '',
  render_format TEXT NOT NULL DEFAULT 'markdown',
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_chat_messages_session_created
  ON chat_messages(session_id, created_at, id);
CREATE TABLE IF NOT EXISTS chat_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL,
  turn_id TEXT NOT NULL DEFAULT '',
  event_type TEXT NOT NULL,
  payload_json TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_chat_events_session_created
  ON chat_events(session_id, created_at, id);
CREATE TABLE IF NOT EXISTS projects (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  project_key TEXT NOT NULL UNIQUE,
  root_path TEXT NOT NULL UNIQUE,
  kind TEXT NOT NULL DEFAULT 'managed',
  mcp_url TEXT NOT NULL DEFAULT '',
  canvas_session_id TEXT NOT NULL DEFAULT '',
  chat_model TEXT NOT NULL DEFAULT '',
  chat_model_reasoning_effort TEXT NOT NULL DEFAULT '',
  is_default INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  last_opened_at INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_projects_name_lower
  ON projects(lower(name));
CREATE TABLE IF NOT EXISTS app_state (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS participant_sessions (
  id TEXT PRIMARY KEY,
  project_key TEXT NOT NULL,
  started_at INTEGER NOT NULL,
  ended_at INTEGER NOT NULL DEFAULT 0,
  config_json TEXT NOT NULL DEFAULT '{}'
);
CREATE TABLE IF NOT EXISTS participant_segments (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL,
  start_ts INTEGER NOT NULL,
  end_ts INTEGER NOT NULL DEFAULT 0,
  speaker TEXT NOT NULL DEFAULT '',
  text TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  latency_ms INTEGER NOT NULL DEFAULT 0,
  committed_at INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'final'
);
CREATE INDEX IF NOT EXISTS idx_participant_segments_session
  ON participant_segments(session_id, start_ts);
CREATE TABLE IF NOT EXISTS participant_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL,
  segment_id INTEGER NOT NULL DEFAULT 0,
  event_type TEXT NOT NULL,
  payload_json TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_participant_events_session
  ON participant_events(session_id, created_at);
CREATE TABLE IF NOT EXISTS participant_room_state (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL UNIQUE,
  summary_text TEXT NOT NULL DEFAULT '',
  entities_json TEXT NOT NULL DEFAULT '[]',
  topic_timeline_json TEXT NOT NULL DEFAULT '[]',
  updated_at INTEGER NOT NULL
);
`
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	return s.migrateProjectColumns()
}

func (s *Store) migrateProjectColumns() error {
	type colDef struct {
		Table string
		Name  string
		SQL   string
	}
	columns := []colDef{
		{Table: "projects", Name: "mcp_url", SQL: `ALTER TABLE projects ADD COLUMN mcp_url TEXT NOT NULL DEFAULT ''`},
		{Table: "projects", Name: "canvas_session_id", SQL: `ALTER TABLE projects ADD COLUMN canvas_session_id TEXT NOT NULL DEFAULT ''`},
		{Table: "projects", Name: "chat_model", SQL: `ALTER TABLE projects ADD COLUMN chat_model TEXT NOT NULL DEFAULT ''`},
		{Table: "projects", Name: "chat_model_reasoning_effort", SQL: `ALTER TABLE projects ADD COLUMN chat_model_reasoning_effort TEXT NOT NULL DEFAULT ''`},
		{Table: "projects", Name: "last_opened_at", SQL: `ALTER TABLE projects ADD COLUMN last_opened_at INTEGER NOT NULL DEFAULT 0`},
		{Table: "chat_messages", Name: "thread_key", SQL: `ALTER TABLE chat_messages ADD COLUMN thread_key TEXT NOT NULL DEFAULT ''`},
	}

	tableColumns := map[string]map[string]bool{}
	for _, table := range []string{"projects", "chat_messages"} {
		cols, err := s.tableColumnNames(table)
		if err != nil {
			return err
		}
		existing := make(map[string]bool, len(cols))
		for _, c := range cols {
			existing[c] = true
		}
		tableColumns[table] = existing
	}

	for _, col := range columns {
		if tableColumns[col.Table][col.Name] {
			continue
		}
		if _, err := s.db.Exec(col.SQL); err != nil {
			return err
		}
	}

	_, _ = s.db.Exec(`UPDATE projects SET canvas_session_id = 'local' WHERE is_default <> 0 AND trim(canvas_session_id) = ''`)
	_, _ = s.db.Exec(`UPDATE projects SET canvas_session_id = id WHERE trim(canvas_session_id) = ''`)
	_, _ = s.db.Exec(`UPDATE projects SET chat_model = lower(trim(chat_model))`)
	_, _ = s.db.Exec(`UPDATE projects SET chat_model_reasoning_effort = lower(trim(chat_model_reasoning_effort))`)
	_, _ = s.db.Exec(`UPDATE projects SET last_opened_at = updated_at WHERE last_opened_at = 0`)
	_, _ = s.db.Exec(`UPDATE chat_messages SET render_format = 'text' WHERE lower(trim(render_format)) = 'canvas'`)
	return nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = time.Now().UTC().MarshalBinary()
	seed := sha256.Sum256([]byte(strconv.FormatInt(time.Now().UnixNano(), 10)))
	copy(b, seed[:])
	return hex.EncodeToString(b)
}
