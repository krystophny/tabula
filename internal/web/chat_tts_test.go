package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSynthesizeTTSAudioRejectsEmptyTextAndMissingService(t *testing.T) {
	app := newAuthedTestApp(t)

	if audio, errMsg := app.synthesizeTTSAudio("session-1", 1, "   ", "en"); errMsg != "text is required" || audio != nil {
		t.Fatalf("empty text result = (%v, %q), want (nil, %q)", audio, errMsg, "text is required")
	}

	app.ttsURL = ""
	if audio, errMsg := app.synthesizeTTSAudio("session-1", 2, "hello", "en"); errMsg != "TTS service not configured" || audio != nil {
		t.Fatalf("missing service result = (%v, %q), want (nil, %q)", audio, errMsg, "TTS service not configured")
	}
}

func TestSynthesizeTTSAudioHandlesUpstreamHTTPError(t *testing.T) {
	app := newAuthedTestApp(t)

	ttsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer ttsSrv.Close()
	app.ttsURL = ttsSrv.URL

	audio, errMsg := app.synthesizeTTSAudio("session-1", 3, "hello", "de")
	if audio != nil {
		t.Fatalf("audio = %v, want nil", audio)
	}
	if errMsg != "TTS error: HTTP 502" {
		t.Fatalf("errMsg = %q, want %q", errMsg, "TTS error: HTTP 502")
	}
}

func TestSynthesizeTTSAudioUsesDefaultLanguageAndReturnsAudio(t *testing.T) {
	app := newAuthedTestApp(t)

	var gotPayload map[string]any
	ttsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/audio/speech" {
			t.Fatalf("path = %s, want /v1/audio/speech", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte("RIFFtest"))
	}))
	defer ttsSrv.Close()
	app.ttsURL = ttsSrv.URL

	audio, errMsg := app.synthesizeTTSAudio("session-1", 4, "  hello world  ", "")
	if errMsg != "" {
		t.Fatalf("errMsg = %q, want empty", errMsg)
	}
	if string(audio) != "RIFFtest" {
		t.Fatalf("audio = %q, want %q", string(audio), "RIFFtest")
	}
	if gotPayload["input"] != "hello world" {
		t.Fatalf("input = %#v, want %q", gotPayload["input"], "hello world")
	}
	if gotPayload["voice"] != "en" {
		t.Fatalf("voice = %#v, want %q", gotPayload["voice"], "en")
	}
	if gotPayload["response_format"] != "wav" {
		t.Fatalf("response_format = %#v, want %q", gotPayload["response_format"], "wav")
	}
}
