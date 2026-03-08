package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/krystophny/tabura/internal/stt"
)

func multipartRequestForTest(t *testing.T, path string, build func(*multipart.Writer)) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	build(writer)
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return httptest.NewRecorder(), req
}

func authedMultipartRequestForTest(t *testing.T, handler http.Handler, path string, build func(*multipart.Writer)) *httptest.ResponseRecorder {
	t.Helper()

	rr, req := multipartRequestForTest(t, path, build)
	req.AddCookie(&http.Cookie{Name: SessionCookie, Value: testAuthToken})
	handler.ServeHTTP(rr, req)
	return rr
}

func addMultipartAudioPart(t *testing.T, writer *multipart.Writer, filename, mimeType string, audio []byte) {
	t.Helper()

	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	if strings.TrimSpace(mimeType) != "" {
		header.Set("Content-Type", mimeType)
	}
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("create multipart audio part: %v", err)
	}
	if _, err := part.Write(audio); err != nil {
		t.Fatalf("write multipart audio: %v", err)
	}
}

func TestReadSTTMultipartAudioRejectsInvalidPayloads(t *testing.T) {
	t.Run("invalid content type", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/stt/transcribe", strings.NewReader("nope"))
		req.Header.Set("Content-Type", "text/plain")

		_, _, err := readSTTMultipartAudio(rr, req)
		if !errors.Is(err, errInvalidMultipartPayload) {
			t.Fatalf("readSTTMultipartAudio() error = %v, want %v", err, errInvalidMultipartPayload)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		rr, req := multipartRequestForTest(t, "/api/stt/transcribe", func(writer *multipart.Writer) {
			if err := writer.WriteField("mime_type", "audio/webm"); err != nil {
				t.Fatalf("WriteField(mime_type): %v", err)
			}
		})

		_, _, err := readSTTMultipartAudio(rr, req)
		if !errors.Is(err, errMissingAudioFile) {
			t.Fatalf("readSTTMultipartAudio() error = %v, want %v", err, errMissingAudioFile)
		}
	})

	t.Run("duplicate file", func(t *testing.T) {
		rr, req := multipartRequestForTest(t, "/api/stt/transcribe", func(writer *multipart.Writer) {
			addMultipartAudioPart(t, writer, "one.webm", "audio/webm", []byte("first"))
			addMultipartAudioPart(t, writer, "two.webm", "audio/webm", []byte("second"))
		})

		_, _, err := readSTTMultipartAudio(rr, req)
		if !errors.Is(err, errDuplicateAudioFile) {
			t.Fatalf("readSTTMultipartAudio() error = %v, want %v", err, errDuplicateAudioFile)
		}
	})

	t.Run("oversized file", func(t *testing.T) {
		rr, req := multipartRequestForTest(t, "/api/stt/transcribe", func(writer *multipart.Writer) {
			addMultipartAudioPart(t, writer, "large.webm", "audio/webm", bytes.Repeat([]byte("a"), stt.MaxAudioBytes+1))
		})

		_, _, err := readSTTMultipartAudio(rr, req)
		if !errors.Is(err, errAudioPayloadTooLarge) {
			t.Fatalf("readSTTMultipartAudio() error = %v, want %v", err, errAudioPayloadTooLarge)
		}
	})
}

func TestReadSTTMultipartAudioUsesMimeTypeFieldOverride(t *testing.T) {
	rr, req := multipartRequestForTest(t, "/api/stt/transcribe", func(writer *multipart.Writer) {
		addMultipartAudioPart(t, writer, "audio.bin", "application/octet-stream", []byte("audio-data"))
		if err := writer.WriteField("mime_type", "audio/wav"); err != nil {
			t.Fatalf("WriteField(mime_type): %v", err)
		}
	})

	audio, mimeType, err := readSTTMultipartAudio(rr, req)
	if err != nil {
		t.Fatalf("readSTTMultipartAudio() error: %v", err)
	}
	if string(audio) != "audio-data" {
		t.Fatalf("audio = %q, want %q", string(audio), "audio-data")
	}
	if mimeType != "audio/wav" {
		t.Fatalf("mimeType = %q, want %q", mimeType, "audio/wav")
	}
}

func TestHandleSTTTranscribeReturnsServiceUnavailableWhenDisabled(t *testing.T) {
	app := newAuthedTestApp(t)
	app.sttURL = ""

	rr := doAuthedMultipartAudioRequest(t, app.Router(), "/api/stt/transcribe", "audio.webm", "audio/webm", []byte("short"))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("POST /api/stt/transcribe status = %d, want %d: %s", rr.Code, http.StatusServiceUnavailable, rr.Body.String())
	}
}

func TestHandleSTTTranscribeReturnsShortRecordingReason(t *testing.T) {
	app := newAuthedTestApp(t)
	app.sttURL = "http://127.0.0.1:1"

	rr := doAuthedMultipartAudioRequest(t, app.Router(), "/api/stt/transcribe", "audio.webm", "audio/webm", bytes.Repeat([]byte("a"), 512))
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/stt/transcribe status = %d, want 200: %s", rr.Code, rr.Body.String())
	}

	var payload map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["text"] != "" {
		t.Fatalf("text = %q, want empty", payload["text"])
	}
	if payload["reason"] != "recording_too_short" {
		t.Fatalf("reason = %q, want %q", payload["reason"], "recording_too_short")
	}
}

func TestHandleSTTTranscribeRejectsInvalidMimeType(t *testing.T) {
	app := newAuthedTestApp(t)
	app.sttURL = "http://127.0.0.1:1"

	rr := doAuthedMultipartAudioRequest(t, app.Router(), "/api/stt/transcribe", "audio.txt", "text/plain", bytes.Repeat([]byte("a"), 2048))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/stt/transcribe status = %d, want %d: %s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "mime_type must be audio/*") {
		t.Fatalf("response body = %q, want mime_type validation error", rr.Body.String())
	}
}

func TestHandleSTTTranscribeRejectsMultipartWithoutFile(t *testing.T) {
	app := newAuthedTestApp(t)
	app.sttURL = "http://127.0.0.1:1"

	rr := authedMultipartRequestForTest(t, app.Router(), "/api/stt/transcribe", func(writer *multipart.Writer) {
		if err := writer.WriteField("mime_type", "audio/webm"); err != nil {
			t.Fatalf("WriteField(mime_type): %v", err)
		}
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/stt/transcribe status = %d, want %d: %s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), errMissingAudioFile.Error()) {
		t.Fatalf("response body = %q, want %q", rr.Body.String(), errMissingAudioFile.Error())
	}
}

func TestHandleSTTTranscribeReturnsLikelyNoiseReason(t *testing.T) {
	app := newAuthedTestApp(t)

	sttSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"text": "okay"}); err != nil {
			t.Fatalf("encode stt response: %v", err)
		}
	}))
	defer sttSrv.Close()
	app.sttURL = sttSrv.URL

	rr := doAuthedMultipartAudioRequest(t, app.Router(), "/api/stt/transcribe", "audio.wav", "audio/wav", buildSpeechLikeWAV(16000, 240, 0.75))
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/stt/transcribe status = %d, want 200: %s", rr.Code, rr.Body.String())
	}

	var payload map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["reason"] != "likely_noise" {
		t.Fatalf("reason = %q, want %q", payload["reason"], "likely_noise")
	}
}

func TestHandleSTTTranscribeReturnsNoSpeechDetectedForEmptyTranscript(t *testing.T) {
	app := newAuthedTestApp(t)

	sttSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"text": "   "}); err != nil {
			t.Fatalf("encode stt response: %v", err)
		}
	}))
	defer sttSrv.Close()
	app.sttURL = sttSrv.URL

	rr := doAuthedMultipartAudioRequest(t, app.Router(), "/api/stt/transcribe", "audio.wav", "audio/wav", buildSpeechLikeWAV(16000, 240, 0.75))
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/stt/transcribe status = %d, want 200: %s", rr.Code, rr.Body.String())
	}

	var payload map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["reason"] != "no_speech_detected" {
		t.Fatalf("reason = %q, want %q", payload["reason"], "no_speech_detected")
	}
}

func TestHandleSTTTranscribeReturnsBadGatewayForUpstreamFailure(t *testing.T) {
	app := newAuthedTestApp(t)

	sttSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "backend unavailable", http.StatusBadGateway)
	}))
	defer sttSrv.Close()
	app.sttURL = sttSrv.URL

	rr := doAuthedMultipartAudioRequest(t, app.Router(), "/api/stt/transcribe", "audio.wav", "audio/wav", buildSpeechLikeWAV(16000, 240, 0.75))
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("POST /api/stt/transcribe status = %d, want %d: %s", rr.Code, http.StatusBadGateway, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "transcription failed: stt HTTP 502") {
		t.Fatalf("response body = %q, want upstream transcription failure", rr.Body.String())
	}
}
