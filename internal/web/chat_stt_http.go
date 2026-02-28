package web

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/krystophny/tabura/internal/stt"
)

func (a *App) handleSTTTranscribe(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	if a.sttURL == "" {
		http.Error(w, "stt sidecar is not configured", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseMultipartForm(stt.MaxAudioBytes + (1 * 1024 * 1024)); err != nil {
		http.Error(w, "invalid multipart payload", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing audio file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, stt.MaxAudioBytes+1))
	if err != nil {
		http.Error(w, "failed to read audio payload", http.StatusBadRequest)
		return
	}
	if len(data) > stt.MaxAudioBytes {
		http.Error(w, "audio payload exceeds max size", http.StatusRequestEntityTooLarge)
		return
	}
	if len(data) < 1024 {
		writeJSON(w, map[string]string{"text": "", "reason": "recording_too_short"})
		return
	}

	mimeType := strings.TrimSpace(r.FormValue("mime_type"))
	if mimeType == "" && header != nil {
		mimeType = strings.TrimSpace(header.Header.Get("Content-Type"))
	}
	mimeType = stt.NormalizeMimeType(mimeType)
	if !stt.IsAllowedMimeType(mimeType) {
		http.Error(w, "mime_type must be audio/* or application/octet-stream", http.StatusBadRequest)
		return
	}

	replacements := a.loadSTTReplacements()
	text, transcribeErr := stt.Transcribe(a.sttURL, mimeType, data, replacements)
	if transcribeErr != nil {
		if errors.Is(transcribeErr, stt.ErrLikelyNoise) {
			writeJSON(w, map[string]string{"text": "", "reason": "likely_noise"})
			return
		}
		if stt.IsRetryableNoSpeechError(transcribeErr) {
			writeJSON(w, map[string]string{"text": "", "reason": "no_speech_detected"})
			return
		}
		http.Error(w, fmt.Sprintf("transcription failed: %v", transcribeErr), http.StatusBadGateway)
		return
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		writeJSON(w, map[string]string{"text": "", "reason": "empty_transcript"})
		return
	}
	writeJSON(w, map[string]string{"text": trimmed})
}
