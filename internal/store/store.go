package store

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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

const pbkdfIter = 600000

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
`
	_, err := s.db.Exec(schema)
	return err
}

func hashPassword(password, salt string) string {
	// lightweight deterministic derivation; kept simple for local admin auth
	data := []byte(password + ":" + salt)
	sum := sha256.Sum256(data)
	for i := 0; i < pbkdfIter/10000; i++ {
		next := sha256.Sum256(sum[:])
		sum = next
	}
	return hex.EncodeToString(sum[:])
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = time.Now().UTC().MarshalBinary()
	seed := sha256.Sum256([]byte(strconv.FormatInt(time.Now().UnixNano(), 10)))
	copy(b, seed[:])
	return hex.EncodeToString(b)
}

func (s *Store) HasAdminPassword() bool {
	var c int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM admin`).Scan(&c)
	return c > 0
}

func (s *Store) SetAdminPassword(password string) error {
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	salt := randomHex(16)
	h := hashPassword(password, salt)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM admin`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM auth_sessions`); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO admin (id,pw_hash,pw_salt) VALUES (1,?,?)`, h, salt); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) VerifyAdminPassword(password string) bool {
	var h, salt string
	if err := s.db.QueryRow(`SELECT pw_hash,pw_salt FROM admin WHERE id=1`).Scan(&h, &salt); err != nil {
		return false
	}
	cand := hashPassword(password, salt)
	return hmac.Equal([]byte(cand), []byte(h))
}

func (s *Store) AddAuthSession(token string) error {
	if token == "" {
		return errors.New("empty token")
	}
	_, err := s.db.Exec(`INSERT OR REPLACE INTO auth_sessions (token,created_at) VALUES (?,?)`, token, time.Now().Unix())
	return err
}

func (s *Store) HasAuthSession(token string) bool {
	if token == "" {
		return false
	}
	var one int
	if err := s.db.QueryRow(`SELECT 1 FROM auth_sessions WHERE token=?`, token).Scan(&one); err != nil {
		return false
	}
	return true
}

func (s *Store) DeleteAuthSession(token string) error {
	if token == "" {
		return nil
	}
	_, err := s.db.Exec(`DELETE FROM auth_sessions WHERE token=?`, token)
	return err
}

func (s *Store) AddHost(h HostConfig) (HostConfig, error) {
	if h.Name == "" || h.Hostname == "" || h.Username == "" {
		return HostConfig{}, errors.New("name, hostname, username required")
	}
	if h.Port <= 0 {
		h.Port = 22
	}
	res, err := s.db.Exec(`INSERT INTO hosts (name,hostname,port,username,key_path,project_dir) VALUES (?,?,?,?,?,?)`, h.Name, h.Hostname, h.Port, h.Username, h.KeyPath, h.ProjectDir)
	if err != nil {
		return HostConfig{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetHost(int(id))
}

func (s *Store) GetHost(id int) (HostConfig, error) {
	var h HostConfig
	err := s.db.QueryRow(`SELECT id,name,hostname,port,username,key_path,project_dir FROM hosts WHERE id=?`, id).
		Scan(&h.ID, &h.Name, &h.Hostname, &h.Port, &h.Username, &h.KeyPath, &h.ProjectDir)
	if err != nil {
		return HostConfig{}, err
	}
	return h, nil
}

func (s *Store) ListHosts() ([]HostConfig, error) {
	rows, err := s.db.Query(`SELECT id,name,hostname,port,username,key_path,project_dir FROM hosts`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HostConfig{}
	for rows.Next() {
		var h HostConfig
		if err := rows.Scan(&h.ID, &h.Name, &h.Hostname, &h.Port, &h.Username, &h.KeyPath, &h.ProjectDir); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *Store) UpdateHost(id int, updates map[string]interface{}) (HostConfig, error) {
	if len(updates) == 0 {
		return s.GetHost(id)
	}
	parts := []string{}
	args := []interface{}{}
	for _, key := range []string{"name", "hostname", "port", "username", "key_path", "project_dir"} {
		if v, ok := updates[key]; ok {
			parts = append(parts, fmt.Sprintf("%s=?", key))
			args = append(args, v)
		}
	}
	if len(parts) == 0 {
		return s.GetHost(id)
	}
	args = append(args, id)
	_, err := s.db.Exec(`UPDATE hosts SET `+stringsJoin(parts, ",")+` WHERE id=?`, args...)
	if err != nil {
		return HostConfig{}, err
	}
	return s.GetHost(id)
}

func stringsJoin(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for i := 1; i < len(parts); i++ {
		out += sep + parts[i]
	}
	return out
}

func (s *Store) DeleteHost(id int) error {
	_, err := s.db.Exec(`DELETE FROM hosts WHERE id=?`, id)
	return err
}

func (s *Store) AddRemoteSession(sessionID string, hostID int) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO remote_sessions (session_id,host_id,created_at) VALUES (?,?,?)`, sessionID, hostID, time.Now().Unix())
	return err
}

func (s *Store) DeleteRemoteSession(sessionID string) error {
	_, err := s.db.Exec(`DELETE FROM remote_sessions WHERE session_id=?`, sessionID)
	return err
}

func (s *Store) ListRemoteSessions() ([][2]interface{}, error) {
	rows, err := s.db.Query(`SELECT session_id,host_id FROM remote_sessions ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := [][2]interface{}{}
	for rows.Next() {
		var sid string
		var hid int
		if err := rows.Scan(&sid, &hid); err != nil {
			return nil, err
		}
		out = append(out, [2]interface{}{sid, hid})
	}
	return out, nil
}
