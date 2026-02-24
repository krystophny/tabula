package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/krystophny/tabura/internal/eou"
	"github.com/krystophny/tabura/internal/stt"
)

var sttTranscribe = stt.TranscribeWithVoxType

type sttEOUResponse struct {
	Transcript   string  `json:"transcript"`
	PEnd         float64 `json:"p_end"`
	ShouldCommit bool    `json:"should_commit"`
	Reason       string  `json:"reason"`
	LatencyMS    int64   `json:"latency_ms"`
	Error        string  `json:"error,omitempty"`
}

func intFromFormValue(v string) int {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return n
}

func readMultipartAudio(r *http.Request) (data []byte, mimeType string, err error) {
	if err := r.ParseMultipartForm(int64(stt.MaxAudioBytes + 1024*1024)); err != nil {
		return nil, "", fmt.Errorf("invalid multipart body: %w", err)
	}
	file, header, err := r.FormFile("audio")
	if err != nil {
		return nil, "", errors.New("missing audio file")
	}
	defer file.Close()
	limited := io.LimitReader(file, int64(stt.MaxAudioBytes+1))
	data, err = io.ReadAll(limited)
	if err != nil {
		return nil, "", fmt.Errorf("read audio: %w", err)
	}
	if len(data) == 0 {
		return nil, "", errors.New("empty audio payload")
	}
	if len(data) > stt.MaxAudioBytes {
		return nil, "", errors.New("audio payload exceeds max size")
	}
	mimeType = stt.NormalizeMimeType(r.FormValue("mime_type"))
	if strings.TrimSpace(mimeType) == "" || mimeType == "audio/webm" {
		// Fall back to uploaded part content type when provided.
		partType := strings.TrimSpace(header.Header.Get("Content-Type"))
		if partType != "" {
			mimeType = stt.NormalizeMimeType(partType)
		}
	}
	if !stt.IsAllowedMimeType(mimeType) {
		return nil, "", errors.New("mime_type must be audio/* or application/octet-stream")
	}
	return data, mimeType, nil
}

func writeEOUJSON(w http.ResponseWriter, status int, payload sttEOUResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (a *App) handleSTTEOUCheck(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	started := time.Now()

	data, mimeType, err := readMultipartAudio(r)
	if err != nil {
		writeEOUJSON(w, http.StatusBadRequest, sttEOUResponse{
			Reason:    eou.ReasonEmptyTranscriptCont,
			LatencyMS: time.Since(started).Milliseconds(),
			Error:     err.Error(),
		})
		return
	}

	silenceMS := intFromFormValue(r.FormValue("silence_ms"))
	elapsedMS := intFromFormValue(r.FormValue("elapsed_ms"))

	transcript, transcribeErr := sttTranscribe(mimeType, data)
	if transcribeErr != nil {
		writeEOUJSON(w, http.StatusOK, sttEOUResponse{
			Reason:       eou.ReasonNoSemanticScore,
			ShouldCommit: false,
			LatencyMS:    time.Since(started).Milliseconds(),
			Error:        fmt.Sprintf("transcription failed: %v", transcribeErr),
		})
		return
	}
	transcript = strings.TrimSpace(transcript)
	decisionInput := eou.DecisionInput{
		Transcript:         transcript,
		SilenceMS:          silenceMS,
		ElapsedMS:          elapsedMS,
		CommitThreshold:    a.eouCommitThreshold,
		CandidateSilenceMS: a.eouCandidateSilenceMS,
		HardSilenceMS:      a.eouHardSilenceMS,
		MaxRecordingMS:     a.eouMaxRecordingMS,
	}

	resp := sttEOUResponse{
		Transcript: transcript,
		LatencyMS:  time.Since(started).Milliseconds(),
	}

	if a.eouEnabled && a.eouClient != nil && transcript != "" {
		ctx, cancel := context.WithTimeout(r.Context(), a.eouTimeout)
		pred, err := a.eouClient.Predict(ctx, eou.PredictRequest{Text: transcript})
		cancel()
		if err != nil {
			decisionInput.FallbackToVAD = true
			resp.Error = fmt.Sprintf("semantic EOU unavailable: %v", err)
		} else {
			decisionInput.HasSemanticScore = true
			decisionInput.PEnd = pred.PEnd
			resp.PEnd = pred.PEnd
		}
	}

	decision := eou.Decide(decisionInput)
	resp.ShouldCommit = decision.ShouldCommit
	resp.Reason = decision.Reason
	resp.LatencyMS = time.Since(started).Milliseconds()
	writeEOUJSON(w, http.StatusOK, resp)
}
