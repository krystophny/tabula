package web

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParticipantBinaryChunkTranscribesWAVSegmentImmediately(t *testing.T) {
	app := newAuthedTestApp(t)
	t.Setenv("PATH", t.TempDir())

	type upload struct {
		path     string
		filename string
		body     []byte
	}
	uploads := make(chan upload, 1)
	sttSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(2 * 1024 * 1024); err != nil {
			uploads <- upload{path: "parse-error", filename: err.Error()}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			uploads <- upload{path: "form-file-error", filename: err.Error()}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer file.Close()
		body, err := io.ReadAll(file)
		if err != nil {
			uploads <- upload{path: "read-error", filename: err.Error()}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		uploads <- upload{
			path:     r.URL.Path,
			filename: header.Filename,
			body:     body,
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"participant transcript"}`))
	}))
	defer sttSrv.Close()
	app.sttURL = sttSrv.URL
	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensureDefaultWorkspace: %v", err)
	}
	enableCompanionForTestProject(t, app, project.WorkspacePath)
	chatSession, err := app.store.GetOrCreateChatSession(project.WorkspacePath)
	if err != nil {
		t.Fatalf("GetOrCreateChatSession: %v", err)
	}

	conn, clientConn, cleanup := newParticipantTestWSConn(t)
	defer cleanup()

	handleParticipantStart(app, conn, chatSession.ID)
	started := readParticipantMessage(t, clientConn, 2*time.Second)
	if started.Type != "participant_started" {
		t.Fatalf("start message type = %q, want participant_started", started.Type)
	}

	conn.participantMu.Lock()
	sessionID := conn.participantSessionID
	conn.participantMu.Unlock()
	if sessionID == "" {
		t.Fatal("expected non-empty participant session id")
	}

	wav := buildParticipantSpeechWAV(240, 16000)
	handleParticipantBinaryChunk(app, conn, wav)

	select {
	case req := <-uploads:
		if req.path != "/v1/audio/transcriptions" {
			t.Fatalf("stt path = %q, want /v1/audio/transcriptions", req.path)
		}
		if req.filename != "audio.wav" {
			t.Fatalf("stt filename = %q, want audio.wav", req.filename)
		}
		if !bytes.Equal(req.body, wav) {
			t.Fatal("uploaded WAV payload does not match the participant chunk")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for participant chunk upload")
	}

	segmentMsg := readParticipantMessage(t, clientConn, 2*time.Second)
	if segmentMsg.Type != "participant_segment_text" {
		t.Fatalf("message type = %q, want participant_segment_text", segmentMsg.Type)
	}
	if segmentMsg.SessionID != sessionID {
		t.Fatalf("segment message session_id = %q, want %q", segmentMsg.SessionID, sessionID)
	}
	if segmentMsg.Text != "participant transcript" {
		t.Fatalf("segment message text = %q, want participant transcript", segmentMsg.Text)
	}

	segments, err := app.store.ListParticipantSegments(sessionID, 0, 0)
	if err != nil {
		t.Fatalf("list participant segments: %v", err)
	}
	if len(segments) != 1 {
		t.Fatalf("segments count = %d, want 1", len(segments))
	}
	if segments[0].Text != "participant transcript" {
		t.Fatalf("segment text = %q, want participant transcript", segments[0].Text)
	}
	if segments[0].Model != "whisper-1" {
		t.Fatalf("segment model = %q, want whisper-1", segments[0].Model)
	}

	conn.participantMu.Lock()
	defer conn.participantMu.Unlock()
	if conn.participantBuf != nil {
		t.Fatal("participantBuf should be cleared after immediate chunk transcription")
	}

	artifactDir := filepath.Join(project.RootPath, ".slopshell", "artifacts", "companion", sessionID)
	transcriptPath := filepath.Join(artifactDir, "transcript.md")
	transcriptBody, err := os.ReadFile(transcriptPath)
	if err != nil {
		t.Fatalf("read transcript artifact: %v", err)
	}
	if !strings.Contains(string(transcriptBody), "participant transcript") {
		t.Fatalf("transcript artifact missing committed text: %q", string(transcriptBody))
	}

	referencesPath := filepath.Join(artifactDir, "references.md")
	referencesBody, err := os.ReadFile(referencesPath)
	if err != nil {
		t.Fatalf("read references artifact: %v", err)
	}
	if !strings.Contains(string(referencesBody), "participant transcript") {
		t.Fatalf("references artifact missing transcript detail: %q", string(referencesBody))
	}
}

func TestParticipantBinaryChunkCapturesMeetingNotesThroughGTDIngest(t *testing.T) {
	app := newAuthedTestApp(t)
	t.Setenv("PATH", t.TempDir())

	sttSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"OK, let us go with option B for the timeline. Alice, can you prepare the budget by Friday?"}`))
	}))
	defer sttSrv.Close()
	app.sttURL = sttSrv.URL
	var ingestRequest map[string]any
	mcp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&ingestRequest); err != nil {
			t.Fatalf("decode MCP request: %v", err)
		}
		writeMCPStructuredResult(t, w, map[string]any{
			"count":   1,
			"paths":   []string{".slopshell/artifacts/companion/meeting-actions.md"},
			"created": []string{"brain/commitments/budget.md"},
			"updated": true,
		})
	}))
	defer mcp.Close()
	app.localControlEndpoint = mcpEndpoint{httpURL: mcp.URL}

	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensureDefaultWorkspace: %v", err)
	}
	enableCompanionForTestProject(t, app, project.WorkspacePath)
	chatSession, err := app.store.GetOrCreateChatSession(project.WorkspacePath)
	if err != nil {
		t.Fatalf("GetOrCreateChatSession: %v", err)
	}

	conn, clientConn, cleanup := newParticipantTestWSConn(t)
	defer cleanup()

	handleParticipantStart(app, conn, chatSession.ID)
	started := readParticipantMessage(t, clientConn, 2*time.Second)
	if started.Type != "participant_started" {
		t.Fatalf("start message type = %q, want participant_started", started.Type)
	}

	handleParticipantBinaryChunk(app, conn, buildParticipantSpeechWAV(240, 16000))

	segmentMsg := readParticipantMessage(t, clientConn, 2*time.Second)
	if segmentMsg.Type != "participant_segment_text" {
		t.Fatalf("message type = %q, want participant_segment_text", segmentMsg.Type)
	}

	participantSession, err := app.store.GetParticipantSession(started.SessionID)
	if err != nil {
		t.Fatalf("GetParticipantSession: %v", err)
	}
	events, err := app.store.ListParticipantEvents(participantSession.ID)
	if err != nil {
		t.Fatalf("ListParticipantEvents: %v", err)
	}
	var capturedAction bool
	for _, event := range events {
		if event.EventType == "meeting_action_item_captured" && strings.Contains(event.PayloadJSON, "Prepare the budget by Friday") {
			capturedAction = true
		}
	}
	if !capturedAction {
		t.Fatalf("meeting action capture event missing: %#v", events)
	}
	params, _ := ingestRequest["params"].(map[string]any)
	if params["name"] != "brain.gtd.ingest" {
		t.Fatalf("MCP tool name = %q, want brain.gtd.ingest", params["name"])
	}
	args, _ := params["arguments"].(map[string]any)
	if args["source"] != "meetings" {
		t.Fatalf("ingest source = %q, want meetings", args["source"])
	}

	summaryPath := filepath.Join(project.RootPath, ".slopshell", "artifacts", "companion", started.SessionID, "summary.md")
	summaryBody, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read summary artifact: %v", err)
	}
	summaryText := string(summaryBody)
	if !strings.Contains(summaryText, "## Decisions") || !strings.Contains(summaryText, "Go with option B for the timeline") {
		t.Fatalf("summary artifact missing decision capture: %q", summaryText)
	}
	if !strings.Contains(summaryText, "## Action Items") || !strings.Contains(summaryText, "Alice: Prepare the budget by Friday") {
		t.Fatalf("summary artifact missing action-item capture: %q", summaryText)
	}
}

func TestParticipantBinaryChunkTranscribeFailureSendsParticipantError(t *testing.T) {
	app := newAuthedTestApp(t)
	t.Setenv("PATH", t.TempDir())
	sttSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forced sidecar failure", http.StatusBadGateway)
	}))
	defer sttSrv.Close()
	app.sttURL = sttSrv.URL

	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensureDefaultWorkspace: %v", err)
	}
	enableCompanionForTestProject(t, app, project.WorkspacePath)
	chatSession, err := app.store.GetOrCreateChatSession(project.WorkspacePath)
	if err != nil {
		t.Fatalf("GetOrCreateChatSession: %v", err)
	}

	conn, clientConn, cleanup := newParticipantTestWSConn(t)
	defer cleanup()

	handleParticipantStart(app, conn, chatSession.ID)
	started := readParticipantMessage(t, clientConn, 2*time.Second)
	if started.Type != "participant_started" {
		t.Fatalf("start message type = %q, want participant_started", started.Type)
	}

	handleParticipantBinaryChunk(app, conn, buildParticipantSpeechWAV(240, 16000))

	msg := readParticipantMessage(t, clientConn, 2*time.Second)
	if msg.Type != "participant_error" {
		t.Fatalf("message type = %q, want participant_error", msg.Type)
	}
	if !strings.Contains(msg.Error, "transcription failed") {
		t.Fatalf("participant error = %q, want transcription failure", msg.Error)
	}

	segments, err := app.store.ListParticipantSegments(started.SessionID, 0, 0)
	if err != nil {
		t.Fatalf("ListParticipantSegments: %v", err)
	}
	if len(segments) != 0 {
		t.Fatalf("segments count = %d, want 0 after failed transcription", len(segments))
	}
}

func TestParticipantStopDropsLateTranscriptCommit(t *testing.T) {
	app := newAuthedTestApp(t)
	t.Setenv("PATH", t.TempDir())

	requestStarted := make(chan struct{}, 1)
	releaseResponse := make(chan struct{})
	sttSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case requestStarted <- struct{}{}:
		default:
		}
		<-releaseResponse
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"late participant transcript"}`))
	}))
	defer sttSrv.Close()
	app.sttURL = sttSrv.URL

	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensureDefaultWorkspace: %v", err)
	}
	enableCompanionForTestProject(t, app, project.WorkspacePath)
	chatSession, err := app.store.GetOrCreateChatSession(project.WorkspacePath)
	if err != nil {
		t.Fatalf("GetOrCreateChatSession: %v", err)
	}

	conn, clientConn, cleanup := newParticipantTestWSConn(t)
	defer cleanup()

	handleParticipantStart(app, conn, chatSession.ID)
	started := readParticipantMessage(t, clientConn, 2*time.Second)
	if started.Type != "participant_started" {
		t.Fatalf("start message type = %q, want participant_started", started.Type)
	}

	handleParticipantBinaryChunk(app, conn, buildParticipantSpeechWAV(240, 16000))

	select {
	case <-requestStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for participant STT request")
	}

	handleParticipantStop(app, conn)
	stopped := readParticipantMessage(t, clientConn, 2*time.Second)
	if stopped.Type != "participant_stopped" {
		t.Fatalf("stop message type = %q, want participant_stopped", stopped.Type)
	}

	close(releaseResponse)

	deadline := time.Now().Add(2 * time.Second)
	for {
		segments, err := app.store.ListParticipantSegments(started.SessionID, 0, 0)
		if err != nil {
			t.Fatalf("ListParticipantSegments: %v", err)
		}
		if len(segments) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("segments count = %d, want 0 after stop", len(segments))
		}
		time.Sleep(10 * time.Millisecond)
	}

	assertNoParticipantMessage(t, clientConn, 250*time.Millisecond)
}

func buildParticipantSpeechWAV(durationMS, sampleRate int) []byte {
	numSamples := sampleRate * durationMS / 1000
	dataSize := numSamples * 2
	buf := bytes.NewBuffer(make([]byte, 0, 44+dataSize))

	buf.WriteString("RIFF")
	_ = binary.Write(buf, binary.LittleEndian, uint32(36+dataSize))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	_ = binary.Write(buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(buf, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(buf, binary.LittleEndian, uint32(sampleRate*2))
	_ = binary.Write(buf, binary.LittleEndian, uint16(2))
	_ = binary.Write(buf, binary.LittleEndian, uint16(16))
	buf.WriteString("data")
	_ = binary.Write(buf, binary.LittleEndian, uint32(dataSize))

	for i := 0; i < numSamples; i++ {
		sample := int16(12000)
		if i%8 < 4 {
			sample = -12000
		}
		_ = binary.Write(buf, binary.LittleEndian, sample)
	}

	return buf.Bytes()
}
