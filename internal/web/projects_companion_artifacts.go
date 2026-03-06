package web

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/krystophny/tabura/internal/roomstate"
	"github.com/krystophny/tabura/internal/store"
)

const companionArtifactRootDir = ".tabura/artifacts/companion"

type companionTranscriptResponse struct {
	OK         bool                       `json:"ok"`
	ProjectID  string                     `json:"project_id"`
	ProjectKey string                     `json:"project_key"`
	Query      string                     `json:"query,omitempty"`
	Sessions   []store.ParticipantSession `json:"sessions"`
	Session    *store.ParticipantSession  `json:"session,omitempty"`
	Segments   []store.ParticipantSegment `json:"segments"`
}

type companionSummaryResponse struct {
	OK          bool                       `json:"ok"`
	ProjectID   string                     `json:"project_id"`
	ProjectKey  string                     `json:"project_key"`
	Sessions    []store.ParticipantSession `json:"sessions"`
	Session     *store.ParticipantSession  `json:"session,omitempty"`
	SummaryText string                     `json:"summary_text"`
	UpdatedAt   int64                      `json:"updated_at"`
}

type companionReferencesResponse struct {
	OK            bool                       `json:"ok"`
	ProjectID     string                     `json:"project_id"`
	ProjectKey    string                     `json:"project_key"`
	Sessions      []store.ParticipantSession `json:"sessions"`
	Session       *store.ParticipantSession  `json:"session,omitempty"`
	Entities      []string                   `json:"entities"`
	TopicTimeline []any                      `json:"topic_timeline"`
}

func (a *App) resolveProjectCompanionArtifact(w http.ResponseWriter, r *http.Request) (store.Project, []store.ParticipantSession, *store.ParticipantSession, bool) {
	project, err := a.resolveProjectByIDOrActive(chi.URLParam(r, "project_id"))
	if err != nil {
		if isNoRows(err) {
			http.Error(w, "project not found", http.StatusNotFound)
			return store.Project{}, nil, nil, false
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return store.Project{}, nil, nil, false
	}
	sessions, err := a.store.ListParticipantSessions(project.ProjectKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return store.Project{}, nil, nil, false
	}
	selected, err := selectProjectCompanionSession(sessions, r.URL.Query().Get("session_id"))
	if err != nil {
		if isNoRows(err) {
			http.Error(w, "session not found", http.StatusNotFound)
			return store.Project{}, nil, nil, false
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return store.Project{}, nil, nil, false
	}
	return project, sessions, selected, true
}

func selectProjectCompanionSession(sessions []store.ParticipantSession, requested string) (*store.ParticipantSession, error) {
	requestedID := strings.TrimSpace(requested)
	if requestedID != "" {
		for i := range sessions {
			if sessions[i].ID == requestedID {
				return &sessions[i], nil
			}
		}
		return nil, sql.ErrNoRows
	}
	for i := range sessions {
		if sessions[i].EndedAt == 0 {
			return &sessions[i], nil
		}
	}
	if len(sessions) == 0 {
		return nil, nil
	}
	return &sessions[0], nil
}

func parseProjectTranscriptWindow(r *http.Request) (int64, int64) {
	var fromTS, toTS int64
	if v := strings.TrimSpace(r.URL.Query().Get("from")); v != "" {
		fromTS, _ = strconv.ParseInt(v, 10, 64)
	}
	if v := strings.TrimSpace(r.URL.Query().Get("to")); v != "" {
		toTS, _ = strconv.ParseInt(v, 10, 64)
	}
	return fromTS, toTS
}

func parseCompanionEntities(raw string) []string {
	clean := strings.TrimSpace(raw)
	if clean == "" {
		return []string{}
	}
	var values []string
	if err := json.Unmarshal([]byte(clean), &values); err == nil {
		return values
	}
	var generic []any
	if err := json.Unmarshal([]byte(clean), &generic); err != nil {
		return []string{}
	}
	out := make([]string, 0, len(generic))
	for _, item := range generic {
		switch v := item.(type) {
		case string:
			value := strings.TrimSpace(v)
			if value != "" {
				out = append(out, value)
			}
		case map[string]any:
			if name := strings.TrimSpace(fmt.Sprint(v["name"])); name != "" && name != "<nil>" {
				out = append(out, name)
			}
		}
	}
	return out
}

func parseCompanionTopicTimeline(raw string) []any {
	clean := strings.TrimSpace(raw)
	if clean == "" {
		return []any{}
	}
	var out []any
	if err := json.Unmarshal([]byte(clean), &out); err != nil {
		return []any{}
	}
	return out
}

func mergeCompanionEntities(primary, secondary []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(primary)+len(secondary))
	for _, entity := range append(primary, secondary...) {
		clean := strings.TrimSpace(entity)
		if clean == "" {
			continue
		}
		key := strings.ToLower(clean)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, clean)
	}
	return out
}

func normalizeTimelineKey(item any) string {
	data, err := json.Marshal(item)
	if err != nil {
		return strings.TrimSpace(fmt.Sprint(item))
	}
	return string(data)
}

func mergeCompanionTopicTimeline(primary, secondary []any) []any {
	seen := map[string]struct{}{}
	out := make([]any, 0, len(primary)+len(secondary))
	for _, item := range append(primary, secondary...) {
		key := normalizeTimelineKey(item)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func formatCompanionTopicTimelineItem(item any) string {
	if typed, ok := item.(map[string]any); ok {
		topic := strings.TrimSpace(fmt.Sprint(typed["topic"]))
		speaker := strings.TrimSpace(fmt.Sprint(typed["speaker"]))
		detail := strings.TrimSpace(fmt.Sprint(typed["detail"]))
		if topic == "<nil>" {
			topic = ""
		}
		if speaker == "<nil>" {
			speaker = ""
		}
		if detail == "<nil>" {
			detail = ""
		}
		switch {
		case speaker != "" && detail != "" && detail != topic:
			return fmt.Sprintf("%s: %s (%s)", speaker, topic, detail)
		case speaker != "" && topic != "":
			return fmt.Sprintf("%s: %s", speaker, topic)
		case topic != "" && detail != "" && detail != topic:
			return fmt.Sprintf("%s (%s)", topic, detail)
		case topic != "":
			return topic
		case detail != "":
			return detail
		}
	}
	return strings.TrimSpace(fmt.Sprint(item))
}

type companionRoomMemory struct {
	SummaryText   string
	UpdatedAt     int64
	Entities      []string
	TopicTimeline []any
}

func (a *App) loadCompanionRoomMemory(sessionID string) (companionRoomMemory, error) {
	segments, err := a.store.ListParticipantSegments(sessionID, 0, 0)
	if err != nil {
		return companionRoomMemory{}, err
	}
	events, err := a.store.ListParticipantEvents(sessionID)
	if err != nil {
		return companionRoomMemory{}, err
	}
	derived := roomstate.Derive(segments, events)
	memory := companionRoomMemory{
		SummaryText:   derived.SummaryText,
		UpdatedAt:     derived.UpdatedAt,
		Entities:      derived.Entities,
		TopicTimeline: derived.TopicTimeline,
	}
	state, err := a.store.GetParticipantRoomState(sessionID)
	if err != nil {
		if isNoRows(err) {
			return memory, nil
		}
		return companionRoomMemory{}, err
	}
	if strings.TrimSpace(state.SummaryText) != "" {
		memory.SummaryText = state.SummaryText
	}
	if state.UpdatedAt > memory.UpdatedAt {
		memory.UpdatedAt = state.UpdatedAt
	}
	memory.Entities = mergeCompanionEntities(parseCompanionEntities(state.EntitiesJSON), memory.Entities)
	memory.TopicTimeline = mergeCompanionTopicTimeline(parseCompanionTopicTimeline(state.TopicTimelineJSON), memory.TopicTimeline)
	return memory, nil
}

func respondCompanionArtifact(w http.ResponseWriter, format string, payload any, markdownText, plainText string) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "json":
		writeJSON(w, payload)
	case "md", "markdown":
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		_, _ = w.Write([]byte(markdownText))
	default:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(plainText))
	}
}

func sanitizeCompanionArtifactPathComponent(raw string) string {
	clean := strings.TrimSpace(raw)
	if clean == "" {
		return ""
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", "..", "-")
	clean = replacer.Replace(clean)
	return strings.Trim(clean, "-.")
}

func companionArtifactDir(project store.Project, session *store.ParticipantSession) string {
	if session == nil {
		return ""
	}
	root := strings.TrimSpace(project.RootPath)
	sessionID := sanitizeCompanionArtifactPathComponent(session.ID)
	if root == "" || sessionID == "" {
		return ""
	}
	return filepath.Join(root, filepath.FromSlash(companionArtifactRootDir), sessionID)
}

func writeCompanionArtifactFile(path, content string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func (a *App) syncProjectCompanionArtifacts(project store.Project, session *store.ParticipantSession) error {
	if a == nil || a.store == nil || session == nil {
		return nil
	}
	dir := companionArtifactDir(project, session)
	if dir == "" {
		return nil
	}
	segments, err := a.store.ListParticipantSegments(session.ID, 0, 0)
	if err != nil {
		return err
	}
	memory, err := a.loadCompanionRoomMemory(session.ID)
	if err != nil {
		return err
	}
	files := map[string]string{
		"transcript.md": renderCompanionTranscriptMarkdown(session, segments),
		"summary.md":    renderCompanionSummaryMarkdown(session, memory.SummaryText, memory.UpdatedAt),
		"references.md": renderCompanionReferencesMarkdown(session, memory.Entities, memory.TopicTimeline),
	}
	for name, content := range files {
		if err := writeCompanionArtifactFile(filepath.Join(dir, name), content); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) syncProjectCompanionArtifactsBySessionID(sessionID string) {
	if a == nil || a.store == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	session, err := a.store.GetParticipantSession(sessionID)
	if err != nil {
		log.Printf("companion artifact sync skipped: participant session lookup failed for %s: %v", sessionID, err)
		return
	}
	project, err := a.store.GetProjectByProjectKey(session.ProjectKey)
	if err != nil {
		log.Printf("companion artifact sync skipped: project lookup failed for %s: %v", session.ProjectKey, err)
		return
	}
	if err := a.syncProjectCompanionArtifacts(project, &session); err != nil {
		log.Printf("companion artifact sync failed for %s: %v", sessionID, err)
	}
}

func formatCompanionSessionStamp(session *store.ParticipantSession) string {
	if session == nil || session.StartedAt == 0 {
		return "n/a"
	}
	return time.Unix(session.StartedAt, 0).UTC().Format(time.RFC3339)
}

func renderCompanionTranscriptMarkdown(session *store.ParticipantSession, segments []store.ParticipantSegment) string {
	var b strings.Builder
	b.WriteString("# Companion Transcript\n\n")
	if session == nil {
		b.WriteString("No transcript is available for this project yet.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "Session: `%s`  \nStarted: %s\n\n", session.ID, formatCompanionSessionStamp(session))
	if len(segments) == 0 {
		b.WriteString("_No transcript segments available._\n")
		return b.String()
	}
	for _, seg := range segments {
		speaker := strings.TrimSpace(seg.Speaker)
		if speaker == "" {
			speaker = "Speaker"
		}
		ts := time.Unix(seg.StartTS, 0).UTC().Format("15:04:05")
		fmt.Fprintf(&b, "- **%s** (%s): %s\n", speaker, ts, strings.TrimSpace(seg.Text))
	}
	return b.String()
}

func renderCompanionTranscriptText(session *store.ParticipantSession, segments []store.ParticipantSegment) string {
	if session == nil {
		return "No transcript is available for this project yet.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Session: %s\nStarted: %s\n\n", session.ID, formatCompanionSessionStamp(session))
	if len(segments) == 0 {
		b.WriteString("No transcript segments available.\n")
		return b.String()
	}
	for _, seg := range segments {
		speaker := strings.TrimSpace(seg.Speaker)
		if speaker == "" {
			speaker = "Speaker"
		}
		ts := time.Unix(seg.StartTS, 0).UTC().Format("15:04:05")
		fmt.Fprintf(&b, "[%s] %s: %s\n", ts, speaker, strings.TrimSpace(seg.Text))
	}
	return b.String()
}

func renderCompanionSummaryMarkdown(session *store.ParticipantSession, summary string, updatedAt int64) string {
	var b strings.Builder
	b.WriteString("# Companion Summary\n\n")
	if session == nil {
		b.WriteString("No summary is available for this project yet.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "Session: `%s`  \nStarted: %s\n", session.ID, formatCompanionSessionStamp(session))
	if updatedAt > 0 {
		fmt.Fprintf(&b, "Updated: %s\n", time.Unix(updatedAt, 0).UTC().Format(time.RFC3339))
	}
	b.WriteString("\n")
	text := strings.TrimSpace(summary)
	if text == "" {
		b.WriteString("_No summary text available._\n")
		return b.String()
	}
	b.WriteString(text)
	b.WriteString("\n")
	return b.String()
}

func renderCompanionSummaryText(session *store.ParticipantSession, summary string, updatedAt int64) string {
	if session == nil {
		return "No summary is available for this project yet.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Session: %s\nStarted: %s\n", session.ID, formatCompanionSessionStamp(session))
	if updatedAt > 0 {
		fmt.Fprintf(&b, "Updated: %s\n", time.Unix(updatedAt, 0).UTC().Format(time.RFC3339))
	}
	b.WriteString("\n")
	text := strings.TrimSpace(summary)
	if text == "" {
		b.WriteString("No summary text available.\n")
		return b.String()
	}
	b.WriteString(text)
	b.WriteString("\n")
	return b.String()
}

func renderCompanionReferencesMarkdown(session *store.ParticipantSession, entities []string, topics []any) string {
	var b strings.Builder
	b.WriteString("# Companion References\n\n")
	if session == nil {
		b.WriteString("No references are available for this project yet.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "Session: `%s`  \nStarted: %s\n\n", session.ID, formatCompanionSessionStamp(session))
	b.WriteString("## Entities\n\n")
	if len(entities) == 0 {
		b.WriteString("_No entities captured._\n")
	} else {
		for _, entity := range entities {
			fmt.Fprintf(&b, "- %s\n", entity)
		}
	}
	b.WriteString("\n## Topic Timeline\n\n")
	if len(topics) == 0 {
		b.WriteString("_No topic timeline captured._\n")
		return b.String()
	}
	for _, topic := range topics {
		fmt.Fprintf(&b, "- %s\n", formatCompanionTopicTimelineItem(topic))
	}
	return b.String()
}

func renderCompanionReferencesText(session *store.ParticipantSession, entities []string, topics []any) string {
	if session == nil {
		return "No references are available for this project yet.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Session: %s\nStarted: %s\n\n", session.ID, formatCompanionSessionStamp(session))
	b.WriteString("Entities\n")
	if len(entities) == 0 {
		b.WriteString("- none\n")
	} else {
		for _, entity := range entities {
			fmt.Fprintf(&b, "- %s\n", entity)
		}
	}
	b.WriteString("\nTopic Timeline\n")
	if len(topics) == 0 {
		b.WriteString("- none\n")
		return b.String()
	}
	for _, topic := range topics {
		fmt.Fprintf(&b, "- %s\n", formatCompanionTopicTimelineItem(topic))
	}
	return b.String()
}

func (a *App) handleProjectCompanionTranscript(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	project, sessions, session, ok := a.resolveProjectCompanionArtifact(w, r)
	if !ok {
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	fromTS, toTS := parseProjectTranscriptWindow(r)
	segments := []store.ParticipantSegment{}
	if session != nil {
		var err error
		if query != "" {
			segments, err = a.store.SearchParticipantSegments(session.ID, query)
		} else {
			segments, err = a.store.ListParticipantSegments(session.ID, fromTS, toTS)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	payload := companionTranscriptResponse{
		OK:         true,
		ProjectID:  project.ID,
		ProjectKey: project.ProjectKey,
		Query:      query,
		Sessions:   sessions,
		Session:    session,
		Segments:   segments,
	}
	if err := a.syncProjectCompanionArtifacts(project, session); err != nil {
		log.Printf("companion artifact sync failed for project %s transcript view: %v", project.ID, err)
	}
	respondCompanionArtifact(w, r.URL.Query().Get("format"), payload, renderCompanionTranscriptMarkdown(session, segments), renderCompanionTranscriptText(session, segments))
}

func (a *App) handleProjectCompanionSummary(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	project, sessions, session, ok := a.resolveProjectCompanionArtifact(w, r)
	if !ok {
		return
	}
	summaryText := ""
	updatedAt := int64(0)
	if session != nil {
		memory, err := a.loadCompanionRoomMemory(session.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		summaryText = memory.SummaryText
		updatedAt = memory.UpdatedAt
	}
	payload := companionSummaryResponse{
		OK:          true,
		ProjectID:   project.ID,
		ProjectKey:  project.ProjectKey,
		Sessions:    sessions,
		Session:     session,
		SummaryText: summaryText,
		UpdatedAt:   updatedAt,
	}
	if err := a.syncProjectCompanionArtifacts(project, session); err != nil {
		log.Printf("companion artifact sync failed for project %s summary view: %v", project.ID, err)
	}
	respondCompanionArtifact(w, r.URL.Query().Get("format"), payload, renderCompanionSummaryMarkdown(session, summaryText, updatedAt), renderCompanionSummaryText(session, summaryText, updatedAt))
}

func (a *App) handleProjectCompanionReferences(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	project, sessions, session, ok := a.resolveProjectCompanionArtifact(w, r)
	if !ok {
		return
	}
	entities := []string{}
	topics := []any{}
	if session != nil {
		memory, err := a.loadCompanionRoomMemory(session.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		entities = memory.Entities
		topics = memory.TopicTimeline
	}
	payload := companionReferencesResponse{
		OK:            true,
		ProjectID:     project.ID,
		ProjectKey:    project.ProjectKey,
		Sessions:      sessions,
		Session:       session,
		Entities:      entities,
		TopicTimeline: topics,
	}
	if err := a.syncProjectCompanionArtifacts(project, session); err != nil {
		log.Printf("companion artifact sync failed for project %s references view: %v", project.ID, err)
	}
	respondCompanionArtifact(w, r.URL.Query().Get("format"), payload, renderCompanionReferencesMarkdown(session, entities, topics), renderCompanionReferencesText(session, entities, topics))
}
