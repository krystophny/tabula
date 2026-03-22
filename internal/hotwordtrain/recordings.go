package hotwordtrain

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (m *Manager) SaveRecording(kind string, reader io.Reader) (Recording, error) {
	if reader == nil {
		return Recording{}, errors.New("missing recording body")
	}
	if err := m.ensureDir(m.recordingsDir()); err != nil {
		return Recording{}, err
	}
	recording := Recording{
		ID:        time.Now().UTC().Format("20060102T150405") + "-" + randomID(),
		Kind:      normalizeKind(kind),
		CreatedAt: nowRFC3339(),
	}
	recording.FileName = recording.ID + ".wav"
	audioPath := filepath.Join(m.recordingsDir(), recording.FileName)
	file, err := os.Create(audioPath)
	if err != nil {
		return Recording{}, err
	}
	size, copyErr := io.Copy(file, reader)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(audioPath)
		return Recording{}, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(audioPath)
		return Recording{}, closeErr
	}
	recording.SizeBytes = size
	recording.DurationMS = wavDurationMS(audioPath)
	if err := writeJSONFile(metadataPathForAudio(audioPath), recording); err != nil {
		_ = os.Remove(audioPath)
		return Recording{}, err
	}
	return recording, nil
}

func (m *Manager) ListRecordings() ([]Recording, error) {
	entries, err := os.ReadDir(m.recordingsDir())
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]Recording, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		var recording Recording
		if err := decodeJSONFile(filepath.Join(m.recordingsDir(), entry.Name()), &recording); err != nil {
			continue
		}
		out = append(out, recording)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt != out[j].CreatedAt {
			return out[i].CreatedAt > out[j].CreatedAt
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}

func (m *Manager) DeleteRecording(id string) error {
	recording, err := m.recordingByID(id)
	if err != nil {
		return err
	}
	audioPath := filepath.Join(m.recordingsDir(), recording.FileName)
	_ = os.Remove(metadataPathForAudio(audioPath))
	_ = os.Remove(audioPath)
	return nil
}

func (m *Manager) RecordingPath(id string) (string, Recording, error) {
	recording, err := m.recordingByID(id)
	if err != nil {
		return "", Recording{}, err
	}
	return filepath.Join(m.recordingsDir(), recording.FileName), recording, nil
}

func (m *Manager) recordingByID(id string) (Recording, error) {
	recordings, err := m.ListRecordings()
	if err != nil {
		return Recording{}, err
	}
	for _, recording := range recordings {
		if recording.ID == strings.TrimSpace(id) {
			return recording, nil
		}
	}
	return Recording{}, os.ErrNotExist
}

func wavDurationMS(path string) int {
	file, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer file.Close()
	header := make([]byte, 44)
	if _, err := io.ReadFull(file, header); err != nil {
		return 0
	}
	if string(header[:4]) != "RIFF" || string(header[8:12]) != "WAVE" {
		return 0
	}
	sampleRate := binary.LittleEndian.Uint32(header[24:28])
	byteRate := binary.LittleEndian.Uint32(header[28:32])
	dataSize := binary.LittleEndian.Uint32(header[40:44])
	if sampleRate == 0 || byteRate == 0 || dataSize == 0 {
		return 0
	}
	durationSeconds := float64(dataSize) / float64(byteRate)
	return int(durationSeconds * 1000)
}
