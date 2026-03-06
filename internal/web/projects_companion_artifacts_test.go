package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/krystophny/tabura/internal/store"
)

func seedProjectCompanionSession(t *testing.T, app *App) (store.Project, store.ParticipantSession) {
	t.Helper()
	project, err := app.ensureDefaultProjectRecord()
	if err != nil {
		t.Fatalf("ensureDefaultProjectRecord: %v", err)
	}
	session, err := app.store.AddParticipantSession(project.ProjectKey, "{}")
	if err != nil {
		t.Fatalf("AddParticipantSession: %v", err)
	}
	return project, session
}

func TestProjectCompanionTranscriptAPIAndExports(t *testing.T) {
	app := newAuthedTestApp(t)
	project, session := seedProjectCompanionSession(t, app)

	_, _ = app.store.AddParticipantSegment(store.ParticipantSegment{
		SessionID: session.ID,
		StartTS:   100,
		EndTS:     110,
		Speaker:   "Alice",
		Text:      "alpha note",
		Status:    "final",
	})
	_, _ = app.store.AddParticipantSegment(store.ParticipantSegment{
		SessionID: session.ID,
		StartTS:   200,
		EndTS:     210,
		Speaker:   "Bob",
		Text:      "beta note",
		Status:    "final",
	})

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/projects/"+project.ID+"/transcript?q=beta", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET transcript status = %d, want 200", rr.Code)
	}
	var payload companionTranscriptResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode transcript payload: %v", err)
	}
	if payload.ProjectID != project.ID {
		t.Fatalf("project_id = %q, want %q", payload.ProjectID, project.ID)
	}
	if payload.Session == nil || payload.Session.ID != session.ID {
		t.Fatalf("selected session = %#v, want %q", payload.Session, session.ID)
	}
	if len(payload.Segments) != 1 {
		t.Fatalf("segments = %d, want 1", len(payload.Segments))
	}
	if payload.Segments[0].Text != "beta note" {
		t.Fatalf("segment text = %q, want beta note", payload.Segments[0].Text)
	}
	body := rr.Body.String()
	if strings.Contains(strings.ToLower(body), "audio") {
		t.Fatalf("transcript payload must remain text-only: %s", body)
	}

	rr = doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/projects/"+project.ID+"/transcript?format=md", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET transcript markdown status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "# Companion Transcript") {
		t.Fatalf("transcript markdown missing header: %q", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "alpha note") || !strings.Contains(rr.Body.String(), "beta note") {
		t.Fatalf("transcript markdown missing segment text: %q", rr.Body.String())
	}

	rr = doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/projects/"+project.ID+"/transcript?format=txt", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET transcript text status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Alice: alpha note") || !strings.Contains(rr.Body.String(), "Bob: beta note") {
		t.Fatalf("transcript text missing export content: %q", rr.Body.String())
	}
}

func TestProjectCompanionSummaryAndReferencesAPIAndExports(t *testing.T) {
	app := newAuthedTestApp(t)
	project, session := seedProjectCompanionSession(t, app)
	if err := app.store.UpsertParticipantRoomState(session.ID, "Decision summary", `["Acme","Budget"]`, `[{"topic":"Status"},{"topic":"Risks"}]`); err != nil {
		t.Fatalf("UpsertParticipantRoomState: %v", err)
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/projects/"+project.ID+"/summary", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET summary status = %d, want 200", rr.Code)
	}
	var summary companionSummaryResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &summary); err != nil {
		t.Fatalf("decode summary payload: %v", err)
	}
	if summary.Session == nil || summary.Session.ID != session.ID {
		t.Fatalf("summary session = %#v, want %q", summary.Session, session.ID)
	}
	if summary.SummaryText != "Decision summary" {
		t.Fatalf("summary_text = %q, want Decision summary", summary.SummaryText)
	}
	if strings.Contains(strings.ToLower(rr.Body.String()), "audio") {
		t.Fatalf("summary payload must remain text-only: %s", rr.Body.String())
	}

	rr = doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/projects/"+project.ID+"/summary?format=md", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET summary markdown status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "# Companion Summary") || !strings.Contains(rr.Body.String(), "Decision summary") {
		t.Fatalf("summary markdown missing expected content: %q", rr.Body.String())
	}

	rr = doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/projects/"+project.ID+"/references", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET references status = %d, want 200", rr.Code)
	}
	var refs companionReferencesResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &refs); err != nil {
		t.Fatalf("decode references payload: %v", err)
	}
	if len(refs.Entities) != 2 {
		t.Fatalf("entities = %d, want 2", len(refs.Entities))
	}
	if len(refs.TopicTimeline) != 2 {
		t.Fatalf("topic_timeline = %d, want 2", len(refs.TopicTimeline))
	}
	if strings.Contains(strings.ToLower(rr.Body.String()), "audio") {
		t.Fatalf("references payload must remain text-only: %s", rr.Body.String())
	}

	rr = doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/projects/"+project.ID+"/references?format=md", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET references markdown status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "# Companion References") {
		t.Fatalf("references markdown missing header: %q", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Acme") || !strings.Contains(rr.Body.String(), "Status") {
		t.Fatalf("references markdown missing captured metadata: %q", rr.Body.String())
	}
}
