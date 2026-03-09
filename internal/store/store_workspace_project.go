package store

import (
	"os"
	"path/filepath"
	"strings"
)

func normalizeProjectMatchToken(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		}
	}
	return b.String()
}

func projectInferenceTokens(raw string) []string {
	trimmed := strings.TrimSpace(strings.ToLower(raw))
	if trimmed == "" {
		return nil
	}
	parts := strings.FieldsFunc(trimmed, func(r rune) bool {
		switch {
		case r >= 'a' && r <= 'z':
			return false
		case r >= '0' && r <= '9':
			return false
		default:
			return true
		}
	})
	seen := map[string]struct{}{}
	out := make([]string, 0, len(parts)+1)
	if whole := normalizeProjectMatchToken(trimmed); whole != "" {
		seen[whole] = struct{}{}
		out = append(out, whole)
	}
	for _, part := range parts {
		token := normalizeProjectMatchToken(part)
		if token == "" {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		out = append(out, token)
	}
	return out
}

func (s *Store) inferProjectIDForWorkspacePath(dirPath string) (*string, error) {
	cleanPath := normalizeWorkspacePath(dirPath)
	if cleanPath == "" {
		return nil, nil
	}
	projects, err := s.ListProjects()
	if err != nil {
		return nil, err
	}
	if len(projects) == 0 {
		return nil, nil
	}

	candidates := map[string]struct{}{}
	for _, raw := range []string{
		filepath.Base(cleanPath),
		filepath.Base(filepath.Dir(cleanPath)),
	} {
		for _, token := range projectInferenceTokens(raw) {
			candidates[token] = struct{}{}
		}
	}
	if info, err := os.Stat(cleanPath); err == nil && info.IsDir() {
		ownerRepo, err := workspaceGitRemoteOwnerRepo(cleanPath)
		if err != nil {
			return nil, err
		}
		for _, token := range projectInferenceTokens(ownerRepo) {
			candidates[token] = struct{}{}
		}
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	matches := map[string]Project{}
	for _, project := range projects {
		projectToken := normalizeProjectMatchToken(project.Name)
		if projectToken == "" {
			continue
		}
		if _, ok := candidates[projectToken]; ok {
			matches[project.ID] = project
		}
	}
	if len(matches) != 1 {
		return nil, nil
	}
	for _, project := range matches {
		id := project.ID
		return &id, nil
	}
	return nil, nil
}
