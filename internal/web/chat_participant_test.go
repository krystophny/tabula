package web

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sloppy-org/slopshell/internal/store"
)

func enableCompanionForTestProject(t *testing.T, app *App, workspacePath string) {
	t.Helper()
	project, err := app.store.GetWorkspaceByStoredPath(strings.TrimSpace(workspacePath))
	if err != nil {
		t.Fatalf("GetProjectByWorkspacePath(%q): %v", workspacePath, err)
	}
	cfg := app.loadCompanionConfig(project)
	cfg.CompanionEnabled = true
	if err := app.saveCompanionConfig(project.ID, cfg); err != nil {
		t.Fatalf("save companion config: %v", err)
	}
	setLivePolicyForTest(t, app, LivePolicyMeeting)
}

func createParticipantSessionProject(t *testing.T, app *App, key string) store.Workspace {
	t.Helper()
	project, err := app.store.CreateEnrichedWorkspace("Participant "+key, key, filepath.Join(t.TempDir(), key), "managed", "", "", false)
	if err != nil {
		t.Fatalf("CreateProject(%q): %v", key, err)
	}
	return project
}

func newParticipantTestWSConn(t *testing.T) (*chatWSConn, *websocket.Conn, func()) {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	serverConn := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serverConn <- ws
	}))

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		srv.Close()
		t.Fatalf("dial test websocket: %v", err)
	}

	var ws *websocket.Conn
	select {
	case ws = <-serverConn:
	case <-time.After(2 * time.Second):
		_ = clientConn.Close()
		srv.Close()
		t.Fatal("timed out waiting for server websocket")
	}

	cleanup := func() {
		_ = ws.Close()
		_ = clientConn.Close()
		srv.Close()
	}
	return newChatWSConn(ws), clientConn, cleanup
}

func readParticipantMessage(t *testing.T, clientConn *websocket.Conn, timeout time.Duration) participantMessage {
	t.Helper()
	if err := clientConn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	defer func() {
		_ = clientConn.SetReadDeadline(time.Time{})
	}()

	_, data, err := clientConn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	var msg participantMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal participant message: %v", err)
	}
	return msg
}

func assertNoParticipantMessage(t *testing.T, clientConn *websocket.Conn, timeout time.Duration) {
	t.Helper()
	if err := clientConn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	defer func() {
		_ = clientConn.SetReadDeadline(time.Time{})
	}()

	_, _, err := clientConn.ReadMessage()
	if err == nil {
		t.Fatal("unexpected participant message")
	}
	netErr, ok := err.(net.Error)
	if !ok || !netErr.Timeout() {
		t.Fatalf("ReadMessage error = %v, want timeout", err)
	}
}

func TestParticipantStartRequiresCapturePolicy(t *testing.T) {
	app := newAuthedTestApp(t)
	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensureDefaultWorkspace: %v", err)
	}
	projectRecord, err := app.store.GetWorkspaceByStoredPath(project.WorkspacePath)
	if err != nil {
		t.Fatalf("GetProjectByWorkspacePath: %v", err)
	}
	cfg := app.loadCompanionConfig(projectRecord)
	cfg.CompanionEnabled = true
	if err := app.saveCompanionConfig(projectRecord.ID, cfg); err != nil {
		t.Fatalf("save companion config: %v", err)
	}
	setLivePolicyForTest(t, app, LivePolicyDialogue)

	session, err := app.store.GetOrCreateChatSession(project.WorkspacePath)
	if err != nil {
		t.Fatalf("GetOrCreateChatSession: %v", err)
	}
	conn, clientConn, cleanup := newParticipantTestWSConn(t)
	defer cleanup()

	handleParticipantStart(app, conn, session.ID)

	msg := readParticipantMessage(t, clientConn, 2*time.Second)
	if msg.Type != "participant_error" {
		t.Fatalf("message type = %q, want participant_error", msg.Type)
	}
	if msg.Error != "meeting mode is disabled" {
		t.Fatalf("error = %q, want meeting mode is disabled", msg.Error)
	}
}

func TestParticipantConfigGetRequiresAuth(t *testing.T) {
	app := newAuthedTestApp(t)

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/participant/config", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("authed GET status = %d, want 200", rr.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/participant/config", nil)
	unauth := httptest.NewRecorder()
	app.Router().ServeHTTP(unauth, req)
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("unauth GET status = %d, want 401", unauth.Code)
	}
}

func TestParticipantConfigDefaultValues(t *testing.T) {
	app := newAuthedTestApp(t)

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/participant/config", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rr.Code)
	}
	var cfg participantConfig
	if err := json.Unmarshal(rr.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if cfg.AudioPersistence != "none" {
		t.Fatalf("audio_persistence = %q, want none", cfg.AudioPersistence)
	}
	if cfg.CaptureSource != "microphone" {
		t.Fatalf("capture_source = %q, want microphone", cfg.CaptureSource)
	}
	if cfg.Language == "" {
		t.Fatal("language is empty")
	}
	if cfg.MaxSegmentDurationMS <= 0 {
		t.Fatalf("max_segment_duration_ms = %d", cfg.MaxSegmentDurationMS)
	}
	if cfg.CompanionEnabled {
		t.Fatal("companion_enabled = true, want false")
	}
}

func TestParticipantConfigPutAudioPersistenceInvariant(t *testing.T) {
	app := newAuthedTestApp(t)

	payload := map[string]interface{}{
		"companion_enabled": false,
		"language":          "de",
		"stt_model":         "whisper-large",
		"idle_surface":      "black",
		"audio_persistence": "disk",
		"capture_source":    "line-in",
	}
	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPut, "/api/participant/config", payload)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200", rr.Code)
	}

	var cfg participantConfig
	if err := json.Unmarshal(rr.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if cfg.AudioPersistence != "none" {
		t.Fatalf("audio_persistence = %q after PUT with disk, want none", cfg.AudioPersistence)
	}
	if cfg.CaptureSource != "microphone" {
		t.Fatalf("capture_source = %q after PUT, want microphone", cfg.CaptureSource)
	}
	if cfg.Language != "de" {
		t.Fatalf("language = %q, want de", cfg.Language)
	}
	if cfg.IdleSurface != "black" {
		t.Fatalf("idle_surface = %q, want black", cfg.IdleSurface)
	}
	if cfg.CompanionEnabled {
		t.Fatal("companion_enabled = true after PUT, want false")
	}

	rr = doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/participant/config", nil)
	if err := json.Unmarshal(rr.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("decode config after round-trip: %v", err)
	}
	if cfg.AudioPersistence != "none" {
		t.Fatalf("audio_persistence = %q after round-trip, want none", cfg.AudioPersistence)
	}
	if cfg.CaptureSource != "microphone" {
		t.Fatalf("capture_source = %q after round-trip, want microphone", cfg.CaptureSource)
	}
	if cfg.CompanionEnabled {
		t.Fatal("companion_enabled = true after round-trip, want false")
	}
}

func TestParticipantStatusRequiresAuth(t *testing.T) {
	app := newAuthedTestApp(t)

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/participant/status", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("authed GET status = %d, want 200", rr.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/participant/status", nil)
	unauth := httptest.NewRecorder()
	app.Router().ServeHTTP(unauth, req)
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("unauth GET status = %d, want 401", unauth.Code)
	}
}

func TestParticipantStatusReportsZero(t *testing.T) {
	app := newAuthedTestApp(t)

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/participant/status", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rr.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["active_sessions"] != float64(0) {
		t.Fatalf("active_sessions = %v, want 0", resp["active_sessions"])
	}
}

func TestParticipantStatusIncludesMeetingDiagnosticsAndReplayEval(t *testing.T) {
	app := newAuthedTestApp(t)
	project := createParticipantSessionProject(t, app, "meeting-status")
	workspace, err := app.ensureWorkspaceReady(project, false)
	if err != nil {
		t.Fatalf("ensureWorkspaceReady: %v", err)
	}
	if err := app.store.SetActiveWorkspace(workspace.ID); err != nil {
		t.Fatalf("SetActiveWorkspace: %v", err)
	}
	enableCompanionForTestProject(t, app, project.WorkspacePath)
	workspace, err = app.store.GetWorkspace(workspace.ID)
	if err != nil {
		t.Fatalf("GetWorkspace: %v", err)
	}
	cfg := app.loadCompanionConfig(workspace)
	cfg.DirectedSpeechGateEnabled = true
	if err := app.saveCompanionConfig(project.ID, cfg); err != nil {
		t.Fatalf("save companion config: %v", err)
	}

	sess, err := app.store.AddParticipantSession(project.WorkspacePath, "{}")
	if err != nil {
		t.Fatalf("AddParticipantSession: %v", err)
	}
	if err := app.store.AddParticipantEvent(sess.ID, 0, "session_started", "{}"); err != nil {
		t.Fatalf("AddParticipantEvent session_started: %v", err)
	}
	first, err := app.store.AddParticipantSegment(store.ParticipantSegment{
		SessionID:   sess.ID,
		Speaker:     "Alice",
		Text:        "Computer, summarize that.",
		CommittedAt: 100,
		Status:      "final",
	})
	if err != nil {
		t.Fatalf("AddParticipantSegment(first): %v", err)
	}
	if err := app.store.AddParticipantEvent(sess.ID, first.ID, "segment_committed", `{"text":"Computer, summarize that."}`); err != nil {
		t.Fatalf("AddParticipantEvent segment_committed(first): %v", err)
	}
	if err := app.store.AddParticipantEvent(sess.ID, first.ID, "assistant_triggered", `{"chat_session_id":"chat-1"}`); err != nil {
		t.Fatalf("AddParticipantEvent assistant_triggered: %v", err)
	}
	second, err := app.store.AddParticipantSegment(store.ParticipantSegment{
		SessionID:   sess.ID,
		Speaker:     "Bob",
		Text:        "Can you include the appendix?",
		CommittedAt: 102,
		Status:      "final",
	})
	if err != nil {
		t.Fatalf("AddParticipantSegment(second): %v", err)
	}
	if err := app.store.AddParticipantEvent(sess.ID, second.ID, "segment_committed", `{"text":"Can you include the appendix?"}`); err != nil {
		t.Fatalf("AddParticipantEvent segment_committed(second): %v", err)
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/participant/status", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rr.Code)
	}
	var resp companionStateResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ActiveSessions != 1 {
		t.Fatalf("active_sessions = %d, want 1", resp.ActiveSessions)
	}
	if resp.ActiveSessionID != sess.ID {
		t.Fatalf("active_session_id = %q, want %q", resp.ActiveSessionID, sess.ID)
	}
	if resp.InteractionPolicy.Decision != companionInteractionDecisionSuppressed {
		t.Fatalf("interaction_policy.decision = %q, want %q", resp.InteractionPolicy.Decision, companionInteractionDecisionSuppressed)
	}
	if resp.InteractionPolicy.Reason != "overlap_other_speaker" {
		t.Fatalf("interaction_policy.reason = %q, want overlap_other_speaker", resp.InteractionPolicy.Reason)
	}
	if resp.DecisionSummary.Pickup == "" || resp.DecisionSummary.Overlap == "" {
		t.Fatalf("decision_summary = %#v, want non-empty pickup and overlap text", resp.DecisionSummary)
	}
	if resp.ReplayEval.CorpusVersion != "meeting-v1" {
		t.Fatalf("replay_eval.corpus_version = %q, want meeting-v1", resp.ReplayEval.CorpusVersion)
	}
	if resp.ReplayEval.Metrics.FalseBargeIns != 0 {
		t.Fatalf("replay_eval.false_barge_ins = %d, want 0", resp.ReplayEval.Metrics.FalseBargeIns)
	}
	if resp.ReplayEval.Metrics.OverlapYields != 2 {
		t.Fatalf("replay_eval.overlap_yields = %d, want 2", resp.ReplayEval.Metrics.OverlapYields)
	}
}

func TestParticipantSessionsListRequiresAuth(t *testing.T) {
	app := newAuthedTestApp(t)

	req := httptest.NewRequest(http.MethodGet, "/api/participant/sessions", nil)
	unauth := httptest.NewRecorder()
	app.Router().ServeHTTP(unauth, req)
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("unauth GET status = %d, want 401", unauth.Code)
	}
}

func TestParticipantSessionsListEmpty(t *testing.T) {
	app := newAuthedTestApp(t)

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/participant/sessions", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rr.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	sessions, ok := resp["sessions"].([]interface{})
	if !ok {
		t.Fatal("sessions not an array")
	}
	if len(sessions) != 0 {
		t.Fatalf("sessions count = %d, want 0", len(sessions))
	}
}

func TestParticipantSessionsListWithData(t *testing.T) {
	app := newAuthedTestApp(t)
	project := createParticipantSessionProject(t, app, "test-key")

	_, err := app.store.AddParticipantSession(project.WorkspacePath, "{}")
	if err != nil {
		t.Fatalf("add session: %v", err)
	}
	_, err = app.store.AddParticipantSession(project.WorkspacePath, "{}")
	if err != nil {
		t.Fatalf("add session 2: %v", err)
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/participant/sessions", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rr.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	sessions := resp["sessions"].([]interface{})
	if len(sessions) != 2 {
		t.Fatalf("sessions count = %d, want 2", len(sessions))
	}
}

func TestParticipantSessionsListFilterByWorkspacePath(t *testing.T) {
	app := newAuthedTestApp(t)
	projectA := createParticipantSessionProject(t, app, "key-a")
	projectB := createParticipantSessionProject(t, app, "key-b")

	_, _ = app.store.AddParticipantSession(projectA.WorkspacePath, "{}")
	_, _ = app.store.AddParticipantSession(projectB.WorkspacePath, "{}")

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/participant/sessions?workspace_path=key-a", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rr.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	sessions := resp["sessions"].([]interface{})
	if len(sessions) != 1 {
		t.Fatalf("sessions count = %d, want 1", len(sessions))
	}
}

func TestParticipantTranscriptRequiresAuth(t *testing.T) {
	app := newAuthedTestApp(t)

	req := httptest.NewRequest(http.MethodGet, "/api/participant/sessions/test-id/transcript", nil)
	unauth := httptest.NewRecorder()
	app.Router().ServeHTTP(unauth, req)
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("unauth GET status = %d, want 401", unauth.Code)
	}
}

func TestParticipantTranscript(t *testing.T) {
	app := newAuthedTestApp(t)
	project := createParticipantSessionProject(t, app, "proj-t")

	sess, err := app.store.AddParticipantSession(project.WorkspacePath, "{}")
	if err != nil {
		t.Fatalf("add session: %v", err)
	}
	_, _ = app.store.AddParticipantSegment(store.ParticipantSegment{
		SessionID: sess.ID,
		StartTS:   100,
		EndTS:     110,
		Text:      "hello meeting",
		Status:    "final",
	})
	_, _ = app.store.AddParticipantSegment(store.ParticipantSegment{
		SessionID: sess.ID,
		StartTS:   200,
		EndTS:     210,
		Text:      "goodbye meeting",
		Status:    "final",
	})

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/participant/sessions/"+sess.ID+"/transcript", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rr.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	segments := resp["segments"].([]interface{})
	if len(segments) != 2 {
		t.Fatalf("segments count = %d, want 2", len(segments))
	}
}

func TestParticipantTranscriptTimeFilter(t *testing.T) {
	app := newAuthedTestApp(t)
	project := createParticipantSessionProject(t, app, "proj-tf")

	sess, _ := app.store.AddParticipantSession(project.WorkspacePath, "{}")
	_, _ = app.store.AddParticipantSegment(store.ParticipantSegment{SessionID: sess.ID, StartTS: 100, Text: "early"})
	_, _ = app.store.AddParticipantSegment(store.ParticipantSegment{SessionID: sess.ID, StartTS: 200, Text: "late"})

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/participant/sessions/"+sess.ID+"/transcript?from=150", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rr.Code)
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	segments := resp["segments"].([]interface{})
	if len(segments) != 1 {
		t.Fatalf("filtered segments = %d, want 1", len(segments))
	}
}

func TestParticipantSearch(t *testing.T) {
	app := newAuthedTestApp(t)
	project := createParticipantSessionProject(t, app, "proj-s")

	sess, _ := app.store.AddParticipantSession(project.WorkspacePath, "{}")
	_, _ = app.store.AddParticipantSegment(store.ParticipantSegment{SessionID: sess.ID, StartTS: 100, Text: "hello world"})
	_, _ = app.store.AddParticipantSegment(store.ParticipantSegment{SessionID: sess.ID, StartTS: 200, Text: "goodbye world"})

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/participant/sessions/"+sess.ID+"/search?q=hello", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rr.Code)
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	segments := resp["segments"].([]interface{})
	if len(segments) != 1 {
		t.Fatalf("search results = %d, want 1", len(segments))
	}
}

func TestParticipantExportTxt(t *testing.T) {
	app := newAuthedTestApp(t)
	project := createParticipantSessionProject(t, app, "proj-e")

	sess, _ := app.store.AddParticipantSession(project.WorkspacePath, "{}")
	_, _ = app.store.AddParticipantSegment(store.ParticipantSegment{SessionID: sess.ID, StartTS: 100, Speaker: "Alice", Text: "hello"})

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/participant/sessions/"+sess.ID+"/export?format=txt", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Alice") || !strings.Contains(body, "hello") {
		t.Fatalf("txt export missing content: %q", body)
	}
}

func TestParticipantExportJSON(t *testing.T) {
	app := newAuthedTestApp(t)
	project := createParticipantSessionProject(t, app, "proj-ej")

	sess, _ := app.store.AddParticipantSession(project.WorkspacePath, "{}")
	_, _ = app.store.AddParticipantSegment(store.ParticipantSegment{SessionID: sess.ID, StartTS: 100, Text: "hello json"})

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/participant/sessions/"+sess.ID+"/export?format=json", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rr.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode json export: %v", err)
	}
	if resp["ok"] != true {
		t.Fatal("expected ok=true")
	}
	segments := resp["segments"].([]interface{})
	if len(segments) != 1 {
		t.Fatalf("segments = %d, want 1", len(segments))
	}
}

func TestParticipantExportMarkdown(t *testing.T) {
	app := newAuthedTestApp(t)
	project := createParticipantSessionProject(t, app, "proj-em")

	sess, _ := app.store.AddParticipantSession(project.WorkspacePath, "{}")
	_, _ = app.store.AddParticipantSegment(store.ParticipantSegment{SessionID: sess.ID, StartTS: 100, Speaker: "Bob", Text: "hello md"})

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/participant/sessions/"+sess.ID+"/export?format=md", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "# Meeting Transcript") {
		t.Fatalf("md export missing header: %q", body)
	}
	if !strings.Contains(body, "**Bob**") {
		t.Fatalf("md export missing speaker: %q", body)
	}
}

func TestPrivacyParticipantConfigNeverStoresAudioPersistence(t *testing.T) {
	app := newAuthedTestApp(t)
	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensureDefaultWorkspace: %v", err)
	}

	cfg := app.loadCompanionConfig(project)
	if cfg.AudioPersistence != "none" {
		t.Fatalf("default audio_persistence = %q, want none", cfg.AudioPersistence)
	}

	cfg.AudioPersistence = "disk"
	if err := app.saveCompanionConfig(project.ID, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	reloadedProject, err := app.store.GetEnrichedWorkspace(workspaceIDStr(project.ID))
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	loaded := app.loadCompanionConfig(reloadedProject)
	if loaded.AudioPersistence != "none" {
		t.Fatalf("loaded audio_persistence = %q after save with disk, want none", loaded.AudioPersistence)
	}
}

func TestPrivacyParticipantMessageNoAudioFields(t *testing.T) {
	msg := participantMessage{
		Type:      "participant_segment_text",
		SessionID: "test",
		Text:      "hello world",
		SegmentID: 1,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	banned := []string{"audio", "wav", "pcm", "recording", "sound_blob", "buffer", "bytes"}
	for key := range decoded {
		lower := strings.ToLower(key)
		for _, b := range banned {
			if lower == b {
				t.Errorf("participant message contains banned field %q", key)
			}
		}
	}
}

func TestPrivacyParticipantBufferCleanupOnStop(t *testing.T) {
	conn, cleanup := newTestWSConn(t)
	defer cleanup()

	conn.participantMu.Lock()
	conn.participantActive = true
	conn.participantSessionID = "test-sess"
	conn.participantBuf = make([]byte, 512)
	conn.participantMu.Unlock()

	app := newAuthedTestApp(t)

	handleParticipantStop(app, conn)

	conn.participantMu.Lock()
	defer conn.participantMu.Unlock()
	if conn.participantBuf != nil {
		t.Error("participantBuf should be nil after stop")
	}
	if conn.participantActive {
		t.Error("participantActive should be false after stop")
	}
	if conn.participantSessionID != "" {
		t.Error("participantSessionID should be empty after stop")
	}
}

func TestParticipantStartUsesChatSessionWorkspacePath(t *testing.T) {
	app := newAuthedTestApp(t)
	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensureDefaultWorkspace: %v", err)
	}
	session, err := app.store.GetOrCreateChatSession(project.WorkspacePath)
	if err != nil {
		t.Fatalf("GetOrCreateChatSession: %v", err)
	}
	enableCompanionForTestProject(t, app, project.WorkspacePath)
	conn, cleanup := newTestWSConn(t)
	defer cleanup()

	handleParticipantStart(app, conn, session.ID)

	conn.participantMu.Lock()
	participantSessionID := conn.participantSessionID
	conn.participantMu.Unlock()
	if participantSessionID == "" {
		t.Fatal("expected participant session id")
	}
	participantSession, err := app.store.GetParticipantSession(participantSessionID)
	if err != nil {
		t.Fatalf("GetParticipantSession: %v", err)
	}
	if participantSession.WorkspacePath != project.WorkspacePath {
		t.Fatalf("participant session workspace_path = %q, want %q", participantSession.WorkspacePath, project.WorkspacePath)
	}
	if participantSession.WorkspaceID == 0 {
		t.Fatal("participant session workspace_id should be set")
	}
}

func TestParticipantReleaseSessionEndsPersistedSession(t *testing.T) {
	app := newAuthedTestApp(t)
	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensureDefaultWorkspace: %v", err)
	}
	session, err := app.store.GetOrCreateChatSession(project.WorkspacePath)
	if err != nil {
		t.Fatalf("GetOrCreateChatSession: %v", err)
	}
	enableCompanionForTestProject(t, app, project.WorkspacePath)
	conn, cleanup := newTestWSConn(t)
	defer cleanup()

	handleParticipantStart(app, conn, session.ID)

	conn.participantMu.Lock()
	participantSessionID := conn.participantSessionID
	conn.participantMu.Unlock()
	if participantSessionID == "" {
		t.Fatal("expected participant session id")
	}

	releasedSessionID, ok := releaseParticipantSession(app, conn)
	if !ok {
		t.Fatal("releaseParticipantSession() = false, want true")
	}
	if releasedSessionID != participantSessionID {
		t.Fatalf("released session id = %q, want %q", releasedSessionID, participantSessionID)
	}

	persisted, err := app.store.GetParticipantSession(participantSessionID)
	if err != nil {
		t.Fatalf("GetParticipantSession: %v", err)
	}
	if persisted.EndedAt == 0 {
		t.Fatal("persisted session should be ended after release")
	}

	conn.participantMu.Lock()
	defer conn.participantMu.Unlock()
	if conn.participantActive {
		t.Fatal("participantActive should be false after release")
	}
	if conn.participantSessionID != "" {
		t.Fatalf("participantSessionID = %q, want empty", conn.participantSessionID)
	}
	if conn.participantBuf != nil {
		t.Fatal("participantBuf should be nil after release")
	}
}

func TestParticipantWSStartStop(t *testing.T) {
	app := newAuthedTestApp(t)
	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensureDefaultWorkspace: %v", err)
	}
	enableCompanionForTestProject(t, app, project.WorkspacePath)
	session, err := app.store.GetOrCreateChatSession(project.WorkspacePath)
	if err != nil {
		t.Fatalf("GetOrCreateChatSession: %v", err)
	}
	conn, cleanup := newTestWSConn(t)
	defer cleanup()

	handleParticipantStart(app, conn, session.ID)

	conn.participantMu.Lock()
	active := conn.participantActive
	sessID := conn.participantSessionID
	conn.participantMu.Unlock()

	if !active {
		t.Fatal("expected participantActive=true after start")
	}
	if sessID == "" {
		t.Fatal("expected non-empty participantSessionID after start")
	}

	handleParticipantStop(app, conn)

	conn.participantMu.Lock()
	active = conn.participantActive
	sessID = conn.participantSessionID
	conn.participantMu.Unlock()

	if active {
		t.Fatal("expected participantActive=false after stop")
	}
	if sessID != "" {
		t.Fatalf("expected empty sessionID after stop, got %q", sessID)
	}
}

func TestParticipantDoubleStartReturnsError(t *testing.T) {
	app := newAuthedTestApp(t)
	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensureDefaultWorkspace: %v", err)
	}
	enableCompanionForTestProject(t, app, project.WorkspacePath)
	session, err := app.store.GetOrCreateChatSession(project.WorkspacePath)
	if err != nil {
		t.Fatalf("GetOrCreateChatSession: %v", err)
	}
	conn, cleanup := newTestWSConn(t)
	defer cleanup()

	handleParticipantStart(app, conn, session.ID)
	handleParticipantStart(app, conn, session.ID)

	conn.participantMu.Lock()
	defer conn.participantMu.Unlock()
	if !conn.participantActive {
		t.Fatal("expected participantActive=true")
	}
}

func TestParticipantStopWithoutStartReturnsError(t *testing.T) {
	app := newAuthedTestApp(t)
	conn, cleanup := newTestWSConn(t)
	defer cleanup()

	handleParticipantStop(app, conn)

	conn.participantMu.Lock()
	defer conn.participantMu.Unlock()
	if conn.participantActive {
		t.Fatal("should not be active after stop-without-start")
	}
}

func TestParticipantStartRequiresCompanionEnabled(t *testing.T) {
	app := newAuthedTestApp(t)
	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensureDefaultWorkspace: %v", err)
	}
	conn, cleanup := newTestWSConn(t)
	defer cleanup()

	handleParticipantStart(app, conn, "test-session")

	conn.participantMu.Lock()
	active := conn.participantActive
	sessionID := conn.participantSessionID
	conn.participantMu.Unlock()
	if active {
		t.Fatal("participantActive = true, want false when companion is disabled")
	}
	if sessionID != "" {
		t.Fatalf("participantSessionID = %q, want empty", sessionID)
	}
	sessions, err := app.store.ListParticipantSessions(project.WorkspacePath)
	if err != nil {
		t.Fatalf("ListParticipantSessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("participant sessions = %d, want 0", len(sessions))
	}
}

func TestParticipantConfigPutDisableStopsActiveSession(t *testing.T) {
	app := newAuthedTestApp(t)
	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensureDefaultWorkspace: %v", err)
	}
	enableCompanionForTestProject(t, app, project.WorkspacePath)
	session, err := app.store.GetOrCreateChatSession(project.WorkspacePath)
	if err != nil {
		t.Fatalf("GetOrCreateChatSession: %v", err)
	}
	conn, cleanup := newTestWSConn(t)
	defer cleanup()
	app.hub.registerChat(session.ID, conn)
	defer app.hub.unregisterChat(session.ID, conn)

	handleParticipantStart(app, conn, session.ID)

	conn.participantMu.Lock()
	participantSessionID := conn.participantSessionID
	conn.participantMu.Unlock()
	if participantSessionID == "" {
		t.Fatal("expected participant session id")
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPut, "/api/participant/config", map[string]any{
		"companion_enabled": false,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200", rr.Code)
	}

	conn.participantMu.Lock()
	active := conn.participantActive
	currentSessionID := conn.participantSessionID
	conn.participantMu.Unlock()
	if active {
		t.Fatal("participantActive = true, want false after disabling companion")
	}
	if currentSessionID != "" {
		t.Fatalf("participantSessionID = %q, want empty after disabling companion", currentSessionID)
	}

	persisted, err := app.store.GetParticipantSession(participantSessionID)
	if err != nil {
		t.Fatalf("GetParticipantSession: %v", err)
	}
	if persisted.EndedAt == 0 {
		t.Fatal("participant session should be ended after disabling companion")
	}
}

func TestLivePolicyPostStopsActiveParticipantSession(t *testing.T) {
	app := newAuthedTestApp(t)
	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensureDefaultWorkspace: %v", err)
	}
	enableCompanionForTestProject(t, app, project.WorkspacePath)
	session, err := app.store.GetOrCreateChatSession(project.WorkspacePath)
	if err != nil {
		t.Fatalf("GetOrCreateChatSession: %v", err)
	}
	conn, cleanup := newTestWSConn(t)
	defer cleanup()
	app.hub.registerChat(session.ID, conn)
	defer app.hub.unregisterChat(session.ID, conn)

	handleParticipantStart(app, conn, session.ID)

	conn.participantMu.Lock()
	participantSessionID := conn.participantSessionID
	conn.participantMu.Unlock()
	if participantSessionID == "" {
		t.Fatal("expected participant session id")
	}

	rr := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/live-policy", map[string]any{
		"policy": "dialogue",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("POST live-policy status = %d, want 200", rr.Code)
	}

	conn.participantMu.Lock()
	active := conn.participantActive
	currentSessionID := conn.participantSessionID
	conn.participantMu.Unlock()
	if active {
		t.Fatal("participantActive = true, want false after switching to dialogue")
	}
	if currentSessionID != "" {
		t.Fatalf("participantSessionID = %q, want empty after switching to dialogue", currentSessionID)
	}

	persisted, err := app.store.GetParticipantSession(participantSessionID)
	if err != nil {
		t.Fatalf("GetParticipantSession: %v", err)
	}
	if persisted.EndedAt == 0 {
		t.Fatal("participant session should be ended after leaving meeting mode")
	}
}
