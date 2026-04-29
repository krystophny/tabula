package web

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/sloppy-org/slopshell/internal/store"
)

const workPersonalGuardrailMessage = "Access to the work personal subtree is blocked by default. It contains local-only personal material; choose another workspace or request an explicit override."

type workPersonalGuardrailError struct{}

func (e workPersonalGuardrailError) Error() string {
	return workPersonalGuardrailMessage
}

func isWorkPersonalGuardrailError(err error) bool {
	var guardrailErr workPersonalGuardrailError
	return errors.As(err, &guardrailErr)
}

func workPersonalGuardrailRoot() string {
	if workBrain := strings.TrimSpace(os.Getenv("SLOPSHELL_BRAIN_WORK_ROOT")); workBrain != "" {
		clean := filepath.Clean(expandWorkspacePathReference(workBrain))
		if filepath.Base(clean) == "brain" {
			return filepath.Join(filepath.Dir(clean), "personal")
		}
		return filepath.Join(clean, "personal")
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}
	return filepath.Join(home, "Nextcloud", "personal")
}

func absoluteCleanPath(path string) string {
	clean := strings.TrimSpace(expandWorkspacePathReference(path))
	if clean == "" {
		return ""
	}
	abs, err := filepath.Abs(clean)
	if err != nil {
		return filepath.Clean(clean)
	}
	return filepath.Clean(abs)
}

func pathInsideOrEqual(path, root string) bool {
	cleanPath := absoluteCleanPath(path)
	cleanRoot := absoluteCleanPath(root)
	if cleanPath == "" || cleanRoot == "" {
		return false
	}
	if cleanPath == cleanRoot {
		return true
	}
	rel, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func pathInWorkPersonalGuardrail(path string) bool {
	return pathInsideOrEqual(path, workPersonalGuardrailRoot())
}

func enforceWorkPersonalPath(path string) error {
	if pathInWorkPersonalGuardrail(path) {
		return workPersonalGuardrailError{}
	}
	return nil
}

func enforceWorkPersonalWorkspace(workspace store.Workspace) error {
	for _, path := range []string{workspace.DirPath, workspace.RootPath, workspace.WorkspacePath} {
		if pathInWorkPersonalGuardrail(path) {
			return workPersonalGuardrailError{}
		}
	}
	return nil
}

func filterWorkPersonalGuardrailWorkspaces(workspaces []store.Workspace) []store.Workspace {
	filtered := workspaces[:0]
	for _, workspace := range workspaces {
		if enforceWorkPersonalWorkspace(workspace) != nil {
			continue
		}
		filtered = append(filtered, workspace)
	}
	return filtered
}
