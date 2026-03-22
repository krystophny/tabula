package hotwordtrain

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func (m *Manager) runGeneration(ctx context.Context, models []string, sampleCount int) {
	jobRoot := filepath.Join(m.generatedDir(), timeSafeStamp())
	_ = m.ensureDir(jobRoot)

	totalGenerated := 0
	modelStates := make([]ModelStatus, 0, len(models))
	for _, model := range models {
		modelStates = append(modelStates, ModelStatus{Name: model, State: statusStateIdle, Target: sampleCount})
	}

	for idx, model := range models {
		outputDir := filepath.Join(jobRoot, model)
		scriptPath := m.generatorPath(model)
		state := modelStates[idx]
		state.State = statusStateRunning
		state.OutputDir = outputDir
		m.updateGenerationModel(idx, state, "generating", statusProgressForIndex(idx, len(models)))

		if err := m.ensureDir(outputDir); err != nil {
			state.State = statusStateFailed
			state.Message = err.Error()
			modelStates[idx] = state
			m.finishGenerationModel(idx, state, totalGenerated, false)
			continue
		}
		if err := requireExecutable(scriptPath); err != nil {
			state.State = statusStateFailed
			state.Message = err.Error()
			modelStates[idx] = state
			m.finishGenerationModel(idx, state, totalGenerated, false)
			continue
		}

		lastLine := ""
		cmd := exec.CommandContext(ctx, scriptPath,
			"--recordings-dir", m.recordingsDir(),
			"--output-dir", outputDir,
			"--count", strconv.Itoa(sampleCount),
			"--model-id", model,
		)
		cmd.Env = append(os.Environ(),
			"TABURA_HOTWORD_RECORDINGS_DIR="+m.recordingsDir(),
			"TABURA_HOTWORD_OUTPUT_DIR="+outputDir,
			"TABURA_HOTWORD_SAMPLE_COUNT="+strconv.Itoa(sampleCount),
			"TABURA_HOTWORD_MODEL_ID="+model,
		)
		err := runLoggedCommand(ctx, cmd, func(line string) {
			lastLine = line
			state.State = statusStateRunning
			state.Message = line
			m.updateGenerationModel(idx, state, "generating", statusProgressForIndex(idx, len(models)))
		})
		if err != nil {
			state.State = statusStateFailed
			state.Message = formatCommandError(err, lastLine)
			modelStates[idx] = state
			m.finishGenerationModel(idx, state, totalGenerated, false)
			continue
		}
		state.State = statusStateCompleted
		state.Count = countWAVFiles(outputDir)
		state.Message = "Generation complete."
		totalGenerated += state.Count
		modelStates[idx] = state
		m.finishGenerationModel(idx, state, totalGenerated, true)
	}

	modelsSnapshot := m.GenerationStatus().Models
	success := false
	for _, model := range modelsSnapshot {
		if model.State == statusStateCompleted {
			success = true
			break
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	next := cloneStatus(m.generation)
	next.Models = append([]ModelStatus(nil), modelsSnapshot...)
	next.GeneratedSamples = totalGenerated
	next.FinishedAt = nowRFC3339()
	if success {
		next.State = statusStateCompleted
		next.Stage = "complete"
		next.Progress = maxStatusProgress
		next.Message = "Generation complete."
		next.Error = ""
	} else {
		next.State = statusStateFailed
		next.Stage = "failed"
		next.Progress = maxStatusProgress
		next.Error = "Generation failed for every selected model."
		if next.Message == "" {
			next.Message = next.Error
		}
	}
	m.setGenerationLocked(next)
}

func (m *Manager) updateGenerationModel(index int, state ModelStatus, stage string, progress int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	next := cloneStatus(m.generation)
	if index < len(next.Models) {
		next.Models[index] = state
	}
	next.State = statusStateRunning
	next.Stage = stage
	next.Progress = progress
	next.Message = state.Message
	m.setGenerationLocked(next)
}

func (m *Manager) finishGenerationModel(index int, state ModelStatus, totalGenerated int, success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	next := cloneStatus(m.generation)
	if index < len(next.Models) {
		next.Models[index] = state
	}
	next.GeneratedSamples = totalGenerated
	next.Stage = "generating"
	next.Progress = statusProgressForIndex(index+1, len(next.Models))
	next.Message = state.Message
	if !success && next.Error == "" {
		next.Error = state.Message
	}
	m.setGenerationLocked(next)
}

func (m *Manager) runTraining(ctx context.Context, req TrainRequest) {
	outputDir := m.modelsDir()
	_ = m.ensureDir(outputDir)

	m.mu.Lock()
	next := cloneStatus(m.training)
	next.Stage = "preparing"
	next.Progress = 5
	next.Message = "Preparing training pipeline."
	m.setTrainingLocked(next)
	m.mu.Unlock()

	scriptPath := m.trainingScriptPath()
	lastLine := ""
	cmd := exec.CommandContext(ctx, scriptPath)
	cmd.Env = append(os.Environ(), "TABURA_HOTWORD_OUTPUT_DIR="+outputDir)
	if req.ConfigPath != "" {
		cmd.Env = append(cmd.Env, "TABURA_HOTWORD_CONFIG="+req.ConfigPath)
	}
	err := runLoggedCommand(ctx, cmd, func(line string) {
		lastLine = line
		m.mu.Lock()
		defer m.mu.Unlock()
		next := cloneStatus(m.training)
		next.State = statusStateRunning
		next.Stage = "training"
		next.Progress = 55
		next.Message = line
		m.setTrainingLocked(next)
	})

	models, listErr := m.ListModels()
	latest := newestModel(models)
	m.mu.Lock()
	defer m.mu.Unlock()
	next = cloneStatus(m.training)
	next.FinishedAt = nowRFC3339()
	next.LatestModel = latest
	if err != nil {
		next.State = statusStateFailed
		next.Stage = "failed"
		next.Progress = maxStatusProgress
		next.Error = formatCommandError(err, lastLine)
		next.Message = next.Error
		m.setTrainingLocked(next)
		return
	}
	if listErr == nil && latest != "" {
		next.LatestModel = latest
	}
	next.State = statusStateCompleted
	next.Stage = "complete"
	next.Progress = maxStatusProgress
	if lastLine != "" {
		next.Message = lastLine
	} else {
		next.Message = "Training complete."
	}
	next.Error = ""
	m.setTrainingLocked(next)
}

func timeSafeStamp() string {
	return strings.NewReplacer(":", "-", ".", "-", "T", "_").Replace(nowRFC3339())
}
