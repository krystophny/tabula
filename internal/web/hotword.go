package web

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

const (
	hotwordPhrase             = "tabura"
	hotwordModelFileName      = "tabura.onnx"
	hotwordTrainScriptRelPath = "scripts/train-hotword.sh"
	hotwordTrainTimeout       = 45 * time.Minute
	maxHotwordJobHistory      = 24
)

var hotwordRuntimeAssetFiles = []string{
	"ort.min.js",
	"melspectrogram.onnx",
	"embedding_model.onnx",
	hotwordModelFileName,
}

type hotwordTrainRunner func(ctx context.Context, cwd string, onProgress func(string)) error

type hotwordTrainJob struct {
	ID           string
	Status       string
	ProgressText string
	ErrorText    string
	StartedAt    time.Time
	FinishedAt   time.Time
}

func (j *hotwordTrainJob) snapshot() map[string]interface{} {
	if j == nil {
		return map[string]interface{}{
			"ok":     false,
			"status": "not_found",
		}
	}
	out := map[string]interface{}{
		"ok":            true,
		"job_id":        j.ID,
		"status":        j.Status,
		"progress_text": j.ProgressText,
	}
	if !j.StartedAt.IsZero() {
		out["started_at"] = j.StartedAt.UTC().Format(time.RFC3339Nano)
	}
	if !j.FinishedAt.IsZero() {
		out["finished_at"] = j.FinishedAt.UTC().Format(time.RFC3339Nano)
	}
	if strings.TrimSpace(j.ErrorText) != "" {
		out["error"] = j.ErrorText
	}
	return out
}

func (a *App) hotwordProjectRoot() string {
	root := strings.TrimSpace(a.localProjectDir)
	if root == "" {
		root = "."
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return root
	}
	return abs
}

func hotwordVendorDir(root string) string {
	return filepath.Join(root, "internal", "web", "static", "vendor", "openwakeword")
}

func hotwordOutputModelPath(root string) string {
	return filepath.Join(root, "models", "hotword", hotwordModelFileName)
}

func hotwordVendorModelPath(root string) string {
	return filepath.Join(hotwordVendorDir(root), hotwordModelFileName)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func checkHotwordStatus(root string) map[string]interface{} {
	vendorDir := hotwordVendorDir(root)
	outputModel := hotwordOutputModelPath(root)
	vendorModel := hotwordVendorModelPath(root)
	trained := fileExists(outputModel) || fileExists(vendorModel)
	missing := make([]string, 0, len(hotwordRuntimeAssetFiles))
	for _, file := range hotwordRuntimeAssetFiles {
		if !fileExists(filepath.Join(vendorDir, file)) {
			missing = append(missing, file)
		}
	}
	runtimeReady := len(missing) == 0
	modelPath := vendorModel
	if !fileExists(modelPath) && fileExists(outputModel) {
		modelPath = outputModel
	}
	return map[string]interface{}{
		"ok":                   true,
		"phrase":               hotwordPhrase,
		"trained":              trained,
		"runtime_assets_ready": runtimeReady,
		"ready":                trained && runtimeReady,
		"model_path":           modelPath,
		"missing":              missing,
	}
}

func appendProgressText(base, line string) string {
	clean := strings.TrimSpace(line)
	if clean == "" {
		return base
	}
	if strings.TrimSpace(base) == "" {
		return clean
	}
	lines := strings.Split(base, "\n")
	lines = append(lines, clean)
	if len(lines) > 32 {
		lines = lines[len(lines)-32:]
	}
	return strings.Join(lines, "\n")
}

func runHotwordTrainer(ctx context.Context, cwd string, onProgress func(string)) error {
	cmd := exec.CommandContext(ctx, "bash", hotwordTrainScriptRelPath)
	if strings.TrimSpace(cwd) != "" {
		cmd.Dir = cwd
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("hotword trainer stdout pipe failed: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("hotword trainer stderr pipe failed: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("hotword trainer start failed: %w", err)
	}

	stream := func(r io.Reader) {
		scanner := bufio.NewScanner(r)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)
		for scanner.Scan() {
			onProgress(scanner.Text())
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		stream(stdout)
	}()
	go func() {
		defer wg.Done()
		stream(stderr)
	}()

	waitErr := cmd.Wait()
	wg.Wait()
	if waitErr != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("hotword training timed out after %s", hotwordTrainTimeout)
		}
		return fmt.Errorf("hotword training failed: %w", waitErr)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
	}()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func (a *App) installHotwordModel(root string, onProgress func(string)) error {
	sourcePath := hotwordOutputModelPath(root)
	if !fileExists(sourcePath) {
		return fmt.Errorf("trained model missing: %s", sourcePath)
	}
	targetDir := hotwordVendorDir(root)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create hotword vendor dir failed: %w", err)
	}
	targetPath := hotwordVendorModelPath(root)
	if sourcePath == targetPath {
		onProgress(fmt.Sprintf("hotword model ready at %s", targetPath))
		return nil
	}
	if err := copyFile(sourcePath, targetPath); err != nil {
		return fmt.Errorf("copy trained model failed: %w", err)
	}
	onProgress(fmt.Sprintf("installed model to %s", targetPath))
	return nil
}

func (a *App) lookupHotwordJob(jobID string) *hotwordTrainJob {
	a.mu.Lock()
	defer a.mu.Unlock()
	job := a.hotwordTrainJobs[jobID]
	if job == nil {
		return nil
	}
	copied := *job
	return &copied
}

func (a *App) trimHotwordJobsLocked() {
	if len(a.hotwordTrainJobs) <= maxHotwordJobHistory {
		return
	}
	jobs := make([]*hotwordTrainJob, 0, len(a.hotwordTrainJobs))
	for _, job := range a.hotwordTrainJobs {
		jobs = append(jobs, job)
	}
	slices.SortFunc(jobs, func(left, right *hotwordTrainJob) int {
		return left.StartedAt.Compare(right.StartedAt)
	})
	removeCount := len(jobs) - maxHotwordJobHistory
	for i := 0; i < removeCount; i++ {
		if jobs[i] == nil || strings.TrimSpace(jobs[i].ID) == "" {
			continue
		}
		if jobs[i].ID == a.hotwordTrainActive {
			continue
		}
		delete(a.hotwordTrainJobs, jobs[i].ID)
	}
}

func (a *App) runHotwordTrainJob(jobID string) {
	root := a.hotwordProjectRoot()
	updateProgress := func(line string) {
		a.mu.Lock()
		defer a.mu.Unlock()
		job := a.hotwordTrainJobs[jobID]
		if job == nil {
			return
		}
		job.ProgressText = appendProgressText(job.ProgressText, line)
	}

	a.mu.Lock()
	job := a.hotwordTrainJobs[jobID]
	if job != nil {
		job.Status = "running"
		job.ProgressText = appendProgressText(job.ProgressText, "starting hotword training...")
	}
	ctx, cancel := context.WithTimeout(context.Background(), hotwordTrainTimeout)
	a.hotwordTrainCancel = cancel
	a.mu.Unlock()

	var runErr error
	runner := a.hotwordTrainRunner
	if runner == nil {
		runner = runHotwordTrainer
	}
	runErr = runner(ctx, root, updateProgress)
	if runErr == nil {
		runErr = a.installHotwordModel(root, updateProgress)
	}
	cancel()

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.hotwordTrainCancel != nil {
		a.hotwordTrainCancel = nil
	}
	if a.hotwordTrainActive == jobID {
		a.hotwordTrainActive = ""
	}
	job = a.hotwordTrainJobs[jobID]
	if job == nil {
		return
	}
	job.FinishedAt = time.Now().UTC()
	if runErr != nil {
		job.Status = "failed"
		job.ErrorText = strings.TrimSpace(runErr.Error())
		job.ProgressText = appendProgressText(job.ProgressText, fmt.Sprintf("training failed: %s", job.ErrorText))
		return
	}
	job.Status = "succeeded"
	job.ErrorText = ""
	job.ProgressText = appendProgressText(job.ProgressText, "training completed")
}

func (a *App) handleHotwordStatus(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	root := a.hotwordProjectRoot()
	payload := checkHotwordStatus(root)

	a.mu.Lock()
	activeID := strings.TrimSpace(a.hotwordTrainActive)
	activeJob := a.hotwordTrainJobs[activeID]
	activeStatus := ""
	if activeJob != nil {
		activeStatus = activeJob.Status
	}
	var lastError string
	var lastErrorAt time.Time
	for _, job := range a.hotwordTrainJobs {
		if job == nil || strings.TrimSpace(job.ErrorText) == "" {
			continue
		}
		if lastError == "" || job.FinishedAt.After(lastErrorAt) {
			lastError = strings.TrimSpace(job.ErrorText)
			lastErrorAt = job.FinishedAt
		}
	}
	a.mu.Unlock()

	trainInProgress := activeStatus == "queued" || activeStatus == "running"
	payload["train_in_progress"] = trainInProgress
	if strings.TrimSpace(lastError) != "" {
		payload["last_error"] = lastError
	}
	writeJSON(w, payload)
}

func (a *App) handleHotwordTrainStart(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	a.mu.Lock()
	activeID := strings.TrimSpace(a.hotwordTrainActive)
	activeJob := a.hotwordTrainJobs[activeID]
	if activeJob != nil && (activeJob.Status == "queued" || activeJob.Status == "running") {
		snapshot := activeJob.snapshot()
		snapshot["status"] = "already_running"
		a.mu.Unlock()
		writeJSON(w, snapshot)
		return
	}

	jobID := strconv.FormatInt(time.Now().UnixNano(), 16)
	job := &hotwordTrainJob{
		ID:           jobID,
		Status:       "queued",
		ProgressText: "queued",
		StartedAt:    time.Now().UTC(),
	}
	a.hotwordTrainJobs[jobID] = job
	a.hotwordTrainActive = jobID
	a.trimHotwordJobsLocked()
	a.mu.Unlock()

	go a.runHotwordTrainJob(jobID)
	writeJSON(w, map[string]interface{}{
		"ok":     true,
		"job_id": jobID,
		"status": "started",
	})
}

func (a *App) handleHotwordTrainStatus(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	jobID := strings.TrimSpace(chi.URLParam(r, "job_id"))
	if jobID == "" {
		http.Error(w, "missing job_id", http.StatusBadRequest)
		return
	}
	job := a.lookupHotwordJob(jobID)
	if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	writeJSON(w, job.snapshot())
}
