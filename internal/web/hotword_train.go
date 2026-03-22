package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/krystophny/tabura/internal/hotwordtrain"
)

type hotwordTrainDeployRequest struct {
	Model string `json:"model"`
}

func (a *App) serveHotwordTrain(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	var data []byte
	var err error
	if a.devRuntime {
		data, err = os.ReadFile(filepath.Join(a.localProjectDir, "internal", "web", "static", "hotword-train.html"))
	} else {
		data, err = staticFiles.ReadFile("static/hotword-train.html")
	}
	if err != nil {
		http.Error(w, "hotword training client not found", http.StatusNotFound)
		return
	}
	page := string(data)
	baseHref := html.EscapeString(appBasePath(r))
	page = strings.Replace(page, "<head>", fmt.Sprintf("<head>\n  <base href=\"%s\">", baseHref), 1)
	boot := strings.TrimSpace(a.bootID)
	if boot != "" {
		page = strings.Replace(page, `href="./static/hotword-train.css"`, fmt.Sprintf(`href="./static/hotword-train.css?v=%s"`, url.QueryEscape(boot)), 1)
		page = strings.Replace(page, `src="./static/hotword-train.js"`, fmt.Sprintf(`src="./static/hotword-train.js?v=%s"`, url.QueryEscape(boot)), 1)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(page))
}

func (a *App) handleHotwordTrainRecordingsList(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	recordings, err := a.hotwordTrainer.ListRecordings()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{
		"ok":         true,
		"recordings": recordingPayloads(recordings),
	})
}

func (a *App) handleHotwordTrainRecordingUpload(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid multipart payload")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()
	recording, err := a.hotwordTrainer.SaveRecording(r.FormValue("kind"), file)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSONStatus(w, http.StatusCreated, map[string]any{
		"ok":        true,
		"recording": recordingPayload(recording),
	})
}

func (a *App) handleHotwordTrainRecordingDelete(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	if err := a.hotwordTrainer.DeleteRecording(chi.URLParam(r, "recording_id")); err != nil {
		if os.IsNotExist(err) {
			writeAPIError(w, http.StatusNotFound, "recording not found")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeNoContent(w)
}

func (a *App) handleHotwordTrainRecordingAudio(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	path, recording, err := a.hotwordTrainer.RecordingPath(chi.URLParam(r, "recording_id"))
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, recording.FileName, time.Now(), file)
}

func (a *App) handleHotwordTrainGenerateStart(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	var req hotwordtrain.GenerateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := a.hotwordTrainer.StartGeneration(a.shutdownCtx, req); err != nil {
		writeAPIError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSONStatus(w, http.StatusAccepted, map[string]any{
		"ok":     true,
		"status": a.hotwordTrainer.GenerationStatus(),
	})
}

func (a *App) handleHotwordTrainGenerateStatus(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	streamHotwordTrainStatus(w, r, a.hotwordTrainer.WatchGeneration)
}

func (a *App) handleHotwordTrainStart(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	var req hotwordtrain.TrainRequest
	if err := decodeJSON(r, &req); err != nil && !errors.Is(err, io.EOF) {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := a.hotwordTrainer.StartTraining(a.shutdownCtx, req); err != nil {
		writeAPIError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSONStatus(w, http.StatusAccepted, map[string]any{
		"ok":     true,
		"status": a.hotwordTrainer.TrainingStatus(),
	})
}

func (a *App) handleHotwordTrainStatus(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	streamHotwordTrainStatus(w, r, a.hotwordTrainer.WatchTraining)
}

func (a *App) handleHotwordTrainModels(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	models, err := a.hotwordTrainer.ListModels()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{
		"ok":     true,
		"models": models,
	})
}

func (a *App) handleHotwordTrainDeploy(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	var req hotwordTrainDeployRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	model, err := a.hotwordTrainer.DeployModel(req.Model)
	if err != nil {
		if os.IsNotExist(err) {
			writeAPIError(w, http.StatusNotFound, "model not found")
			return
		}
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{
		"ok":    true,
		"model": model,
	})
}

func streamHotwordTrainStatus(w http.ResponseWriter, r *http.Request, subscribe func() (<-chan hotwordtrain.Status, func())) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeAPIError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}
	statuses, unsubscribe := subscribe()
	defer unsubscribe()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")

	encoder := json.NewEncoder(w)
	for {
		select {
		case <-r.Context().Done():
			return
		case status, ok := <-statuses:
			if !ok {
				return
			}
			_, _ = fmt.Fprint(w, "event: status\n")
			_, _ = fmt.Fprint(w, "data: ")
			if err := encoder.Encode(status); err != nil {
				return
			}
			_, _ = fmt.Fprint(w, "\n")
			flusher.Flush()
		}
	}
}

func recordingPayloads(recordings []hotwordtrain.Recording) []map[string]any {
	out := make([]map[string]any, 0, len(recordings))
	for _, recording := range recordings {
		out = append(out, recordingPayload(recording))
	}
	return out
}

func recordingPayload(recording hotwordtrain.Recording) map[string]any {
	return map[string]any{
		"id":          recording.ID,
		"kind":        recording.Kind,
		"created_at":  recording.CreatedAt,
		"file_name":   recording.FileName,
		"size_bytes":  recording.SizeBytes,
		"duration_ms": recording.DurationMS,
		"audio_url":   "./api/hotword/train/recordings/" + url.PathEscape(recording.ID) + "/audio",
	}
}
