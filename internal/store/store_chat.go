package store

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

func normalizeChatMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "plan":
		return "plan"
	default:
		return "chat"
	}
}

func normalizeChatRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "assistant":
		return "assistant"
	case "system":
		return "system"
	default:
		return "user"
	}
}

func normalizeRenderFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "text":
		return "text"
	case "canvas":
		return "text"
	default:
		return "markdown"
	}
}

func (s *Store) GetOrCreateChatSession(projectKey string) (ChatSession, error) {
	key := strings.TrimSpace(projectKey)
	if key == "" {
		key = "default"
	}
	if existing, err := s.GetChatSessionByProjectKey(key); err == nil {
		return existing, nil
	}
	now := time.Now().Unix()
	id := fmt.Sprintf("chat-%s", randomHex(8))
	_, err := s.db.Exec(
		`INSERT INTO chat_sessions (id, project_key, app_thread_id, mode, created_at, updated_at) VALUES (?,?,?,?,?,?)`,
		id, key, "", "chat", now, now,
	)
	if err != nil {
		return ChatSession{}, err
	}
	return s.GetChatSession(id)
}

func (s *Store) GetChatSessionByProjectKey(projectKey string) (ChatSession, error) {
	key := strings.TrimSpace(projectKey)
	if key == "" {
		key = "default"
	}
	var out ChatSession
	err := s.db.QueryRow(
		`SELECT id, project_key, app_thread_id, mode, created_at, updated_at FROM chat_sessions WHERE project_key = ?`,
		key,
	).Scan(&out.ID, &out.ProjectKey, &out.AppThreadID, &out.Mode, &out.CreatedAt, &out.UpdatedAt)
	if err != nil {
		return ChatSession{}, err
	}
	out.Mode = normalizeChatMode(out.Mode)
	return out, nil
}

func (s *Store) GetChatSession(id string) (ChatSession, error) {
	var out ChatSession
	err := s.db.QueryRow(
		`SELECT id, project_key, app_thread_id, mode, created_at, updated_at FROM chat_sessions WHERE id = ?`,
		strings.TrimSpace(id),
	).Scan(&out.ID, &out.ProjectKey, &out.AppThreadID, &out.Mode, &out.CreatedAt, &out.UpdatedAt)
	if err != nil {
		return ChatSession{}, err
	}
	out.Mode = normalizeChatMode(out.Mode)
	return out, nil
}

func (s *Store) ListChatSessions() ([]ChatSession, error) {
	rows, err := s.db.Query(
		`SELECT id, project_key, app_thread_id, mode, created_at, updated_at
		 FROM chat_sessions ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ChatSession, 0, 32)
	for rows.Next() {
		var item ChatSession
		if err := rows.Scan(
			&item.ID,
			&item.ProjectKey,
			&item.AppThreadID,
			&item.Mode,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.Mode = normalizeChatMode(item.Mode)
		out = append(out, item)
	}
	return out, nil
}

func (s *Store) UpdateChatSessionMode(id, mode string) (ChatSession, error) {
	normalizedMode := normalizeChatMode(mode)
	now := time.Now().Unix()
	_, err := s.db.Exec(
		`UPDATE chat_sessions SET mode = ?, updated_at = ? WHERE id = ?`,
		normalizedMode, now, strings.TrimSpace(id),
	)
	if err != nil {
		return ChatSession{}, err
	}
	return s.GetChatSession(id)
}

func (s *Store) UpdateChatSessionThread(id, appThreadID string) error {
	_, err := s.db.Exec(
		`UPDATE chat_sessions SET app_thread_id = ?, updated_at = ? WHERE id = ?`,
		strings.TrimSpace(appThreadID), time.Now().Unix(), strings.TrimSpace(id),
	)
	return err
}

func (s *Store) AddChatMessage(sessionID, role, contentMarkdown, contentPlain, renderFormat string, opts ...ChatMessageOption) (ChatMessage, error) {
	role = normalizeChatRole(role)
	renderFormat = normalizeRenderFormat(renderFormat)
	var o chatMessageOpts
	for _, fn := range opts {
		fn(&o)
	}
	threadKey := strings.TrimSpace(o.threadKey)
	now := time.Now().Unix()
	res, err := s.db.Exec(
		`INSERT INTO chat_messages (session_id, role, content_markdown, content_plain, render_format, thread_key, created_at) VALUES (?,?,?,?,?,?,?)`,
		strings.TrimSpace(sessionID),
		role,
		contentMarkdown,
		contentPlain,
		renderFormat,
		threadKey,
		now,
	)
	if err != nil {
		return ChatMessage{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return ChatMessage{}, err
	}
	return ChatMessage{
		ID:              id,
		SessionID:       strings.TrimSpace(sessionID),
		Role:            role,
		ContentMarkdown: contentMarkdown,
		ContentPlain:    contentPlain,
		RenderFormat:    renderFormat,
		ThreadKey:       threadKey,
		CreatedAt:       now,
	}, nil
}

type chatMessageOpts struct {
	threadKey string
}

type ChatMessageOption func(*chatMessageOpts)

func WithThreadKey(key string) ChatMessageOption {
	return func(o *chatMessageOpts) {
		o.threadKey = key
	}
}

func (s *Store) UpdateChatMessageContent(id int64, contentMarkdown, contentPlain, renderFormat string) error {
	if id <= 0 {
		return errors.New("message id is required")
	}
	renderFormat = normalizeRenderFormat(renderFormat)
	_, err := s.db.Exec(
		`UPDATE chat_messages SET content_markdown = ?, content_plain = ?, render_format = ? WHERE id = ?`,
		contentMarkdown,
		contentPlain,
		renderFormat,
		id,
	)
	return err
}

func (s *Store) ListChatMessages(sessionID string, limit int, opts ...ChatMessageOption) ([]ChatMessage, error) {
	if limit <= 0 {
		limit = 200
	}
	var o chatMessageOpts
	for _, fn := range opts {
		fn(&o)
	}
	threadKey := strings.TrimSpace(o.threadKey)
	rows, err := s.db.Query(
		`SELECT id, session_id, role, content_markdown, content_plain, render_format, thread_key, created_at
		 FROM chat_messages WHERE session_id = ? AND thread_key = ? ORDER BY id ASC LIMIT ?`,
		strings.TrimSpace(sessionID), threadKey, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ChatMessage, 0, limit)
	for rows.Next() {
		var item ChatMessage
		if err := rows.Scan(
			&item.ID,
			&item.SessionID,
			&item.Role,
			&item.ContentMarkdown,
			&item.ContentPlain,
			&item.RenderFormat,
			&item.ThreadKey,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		item.Role = normalizeChatRole(item.Role)
		item.RenderFormat = normalizeRenderFormat(item.RenderFormat)
		out = append(out, item)
	}
	return out, nil
}

func (s *Store) ClearChatMessages(sessionID string) error {
	_, err := s.db.Exec("DELETE FROM chat_messages WHERE session_id = ?", sessionID)
	return err
}

func (s *Store) ClearAllChatMessages() error {
	_, err := s.db.Exec("DELETE FROM chat_messages")
	return err
}

func (s *Store) ClearAllChatEvents() error {
	_, err := s.db.Exec("DELETE FROM chat_events")
	return err
}

func (s *Store) ResetChatSessionThread(sessionID string) error {
	_, err := s.db.Exec("UPDATE chat_sessions SET app_thread_id = '' WHERE id = ?", sessionID)
	return err
}

func (s *Store) ResetAllChatSessionThreads() error {
	_, err := s.db.Exec("UPDATE chat_sessions SET app_thread_id = '', updated_at = ?", time.Now().Unix())
	return err
}

func (s *Store) AddChatEvent(sessionID, turnID, eventType, payloadJSON string) error {
	_, err := s.db.Exec(
		`INSERT INTO chat_events (session_id, turn_id, event_type, payload_json, created_at) VALUES (?,?,?,?,?)`,
		strings.TrimSpace(sessionID),
		strings.TrimSpace(turnID),
		strings.TrimSpace(eventType),
		payloadJSON,
		time.Now().Unix(),
	)
	return err
}
