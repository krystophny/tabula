package web

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func listWorkspaceMarkdownNoteFiles(root string, fileCap int) ([]string, bool, error) {
	files, capped, err := listWorkspaceMarkdownNoteFilesWithRG(root, fileCap)
	if err == nil {
		return files, capped, nil
	}
	return listWorkspaceMarkdownNoteFilesByWalk(root, fileCap)
}

func listWorkspaceMarkdownNoteFilesWithRG(root string, fileCap int) ([]string, bool, error) {
	if _, err := exec.LookPath("rg"); err != nil {
		return nil, false, err
	}
	cmd := exec.Command("rg", "--files", "--glob", "*.md")
	cmd.Dir = root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil && stdout.Len() == 0 {
		return nil, false, err
	}
	files := []string{}
	capped := false
	for _, line := range strings.Split(stdout.String(), "\n") {
		rel := strings.TrimSpace(line)
		if rel == "" {
			continue
		}
		path := filepath.Clean(filepath.Join(root, filepath.FromSlash(rel)))
		if !pathInsideOrEqual(path, root) || pathInWorkPersonalGuardrail(path) {
			continue
		}
		if len(files) >= fileCap {
			capped = true
			break
		}
		files = append(files, path)
	}
	return files, capped, nil
}

func listWorkspaceMarkdownNoteFilesByWalk(root string, fileCap int) ([]string, bool, error) {
	files := []string{}
	capped := false
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() {
			if pathInWorkPersonalGuardrail(path) {
				return filepath.SkipDir
			}
			base := entry.Name()
			if base != "." && strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			return nil
		}
		if len(files) >= fileCap {
			capped = true
			return filepath.SkipAll
		}
		files = append(files, path)
		return nil
	})
	return files, capped, err
}
