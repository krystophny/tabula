package web

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/krystophny/tabura/internal/eou"
)

func makeEOUCheckRequest(t *testing.T, withAuth bool, silenceMS, elapsedMS int) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("audio", "sample.webm")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	_, _ = part.Write([]byte("fake-webm-audio"))
	_ = writer.WriteField("mime_type", "audio/webm")
	_ = writer.WriteField("silence_ms", strconv.Itoa(silenceMS))
	_ = writer.WriteField("elapsed_ms", strconv.Itoa(elapsedMS))
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/stt/eou-check", bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if withAuth {
		req.AddCookie(&http.Cookie{Name: SessionCookie, Value: testAuthToken})
	}
	return req
}

func TestHandleSTTEOUCheck(t *testing.T) {
	orig := sttTranscribe
	t.Cleanup(func() { sttTranscribe = orig })

	t.Run("requires auth", func(t *testing.T) {
		app := newAuthedTestApp(t)
		req := makeEOUCheckRequest(t, false, 900, 1500)
		rr := httptest.NewRecorder()
		app.Router().ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("high confidence end commits", func(t *testing.T) {
		sttTranscribe = func(_ string, _ []byte) (string, error) { return "hello world", nil }
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"p_end": 0.92, "label": "end"})
		}))
		defer ts.Close()

		app := newAuthedTestApp(t)
		app.eouEnabled = true
		app.eouClient = eou.NewClient(ts.URL, 1*time.Second)

		req := makeEOUCheckRequest(t, true, 900, 1500)
		rr := httptest.NewRecorder()
		app.Router().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		var out sttEOUResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if !out.ShouldCommit || out.Reason != eou.ReasonHighConfidenceEnd {
			t.Fatalf("unexpected response: %+v", out)
		}
	})

	t.Run("eou failure falls back to VAD commit", func(t *testing.T) {
		sttTranscribe = func(_ string, _ []byte) (string, error) { return "hello world", nil }
		app := newAuthedTestApp(t)
		app.eouEnabled = true
		app.eouClient = eou.NewClient("http://127.0.0.1:1", 25*time.Millisecond)

		req := makeEOUCheckRequest(t, true, 1200, 1800)
		rr := httptest.NewRecorder()
		app.Router().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		var out sttEOUResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if !out.ShouldCommit || out.Reason != eou.ReasonFallbackVADCommit {
			t.Fatalf("unexpected response: %+v", out)
		}
	})
}
