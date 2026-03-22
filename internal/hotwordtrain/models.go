package hotwordtrain

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (m *Manager) ListModels() ([]Model, error) {
	models := make([]Model, 0)
	modelDirEntries, err := os.ReadDir(m.modelsDir())
	if err != nil && !isNotFound(err) {
		return nil, err
	}
	for _, entry := range modelDirEntries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".onnx") {
			continue
		}
		path := filepath.Join(m.modelsDir(), entry.Name())
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		models = append(models, Model{
			Name:       strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())),
			FileName:   entry.Name(),
			Path:       path,
			CreatedAt:  info.ModTime().UTC().Format(time.RFC3339),
			SizeBytes:  info.Size(),
			Production: false,
		})
	}
	vendorPath := m.vendorModelPath()
	if info, err := os.Stat(vendorPath); err == nil && !info.IsDir() {
		models = append(models, Model{
			Name:       strings.TrimSuffix(filepath.Base(vendorPath), filepath.Ext(vendorPath)),
			FileName:   filepath.Base(vendorPath),
			Path:       vendorPath,
			CreatedAt:  info.ModTime().UTC().Format(time.RFC3339),
			SizeBytes:  info.Size(),
			Production: true,
		})
	}
	sortModels(models)
	return models, nil
}

func (m *Manager) DeployModel(fileName string) (Model, error) {
	clean := filepath.Base(strings.TrimSpace(fileName))
	if clean == "" {
		return Model{}, fmt.Errorf("missing model name")
	}
	sourcePath := filepath.Join(m.modelsDir(), clean)
	info, err := os.Stat(sourcePath)
	if err != nil || info.IsDir() {
		return Model{}, os.ErrNotExist
	}
	vendorPath := m.vendorModelPath()
	if err := m.ensureDir(filepath.Dir(vendorPath)); err != nil {
		return Model{}, err
	}
	if _, err := os.Stat(vendorPath); err == nil {
		_ = os.Remove(vendorPath + ".bak")
		if renameErr := os.Rename(vendorPath, vendorPath+".bak"); renameErr != nil {
			return Model{}, renameErr
		}
	}
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return Model{}, err
	}
	if err := os.WriteFile(vendorPath, data, 0o644); err != nil {
		return Model{}, err
	}
	model := Model{
		Name:       strings.TrimSuffix(clean, filepath.Ext(clean)),
		FileName:   clean,
		Path:       vendorPath,
		CreatedAt:  nowRFC3339(),
		SizeBytes:  int64(len(data)),
		Production: true,
	}
	return model, nil
}
