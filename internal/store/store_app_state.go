package store

import (
	"database/sql"
	"errors"
	"strings"
)

func (s *Store) SetAppState(key, value string) error {
	cleanKey := strings.TrimSpace(key)
	if cleanKey == "" {
		return errors.New("app state key is required")
	}
	_, err := s.db.Exec(
		`INSERT INTO app_state (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		cleanKey,
		strings.TrimSpace(value),
	)
	return err
}

func (s *Store) AppState(key string) (string, error) {
	cleanKey := strings.TrimSpace(key)
	if cleanKey == "" {
		return "", errors.New("app state key is required")
	}
	var value string
	if err := s.db.QueryRow(`SELECT value FROM app_state WHERE key = ?`, cleanKey).Scan(&value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func normalizeWorkspaceChatModel(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "spark":
		return "spark"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func normalizeWorkspaceChatModelReasoningEffort(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "low", "medium", "high", "xhigh":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
	}
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
