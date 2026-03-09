package web

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/krystophny/tabura/internal/store"
)

var (
	workspaceAssignProjectPattern = regexp.MustCompile(`(?i)^(?:assign|link)(?:\s+this)?\s+workspace\s+to\s+(.+?)$`)
	projectCreatePattern          = regexp.MustCompile(`(?i)^(?:create|add)\s+project\s+(.+?)$`)
	projectWorkspacesPattern      = regexp.MustCompile(`(?i)^(?:list|show)\s+(.+?)\s+workspaces$`)
	projectSyncPattern            = regexp.MustCompile(`(?i)^sync(?:\s+project)?\s+(.+?)$`)
)

func parseInlineProjectIntent(text string) *SystemAction {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	normalized := normalizeItemCommandText(trimmed)
	switch normalized {
	case "what project is this", "what project is this workspace", "which project is this", "show project for this workspace":
		return &SystemAction{Action: "show_workspace_project", Params: map[string]interface{}{}}
	}
	if match := workspaceAssignProjectPattern.FindStringSubmatch(trimmed); len(match) == 2 {
		if ref := cleanWorkspaceReference(match[1]); ref != "" {
			return &SystemAction{Action: "assign_workspace_project", Params: map[string]interface{}{"project": ref}}
		}
	}
	if match := projectCreatePattern.FindStringSubmatch(trimmed); len(match) == 2 {
		if name := cleanWorkspaceReference(match[1]); name != "" {
			return &SystemAction{Action: "create_project", Params: map[string]interface{}{"project": name}}
		}
	}
	if match := projectWorkspacesPattern.FindStringSubmatch(trimmed); len(match) == 2 {
		if ref := cleanWorkspaceReference(match[1]); ref != "" {
			return &SystemAction{Action: "list_project_workspaces", Params: map[string]interface{}{"project": ref}}
		}
	}
	if match := projectSyncPattern.FindStringSubmatch(trimmed); len(match) == 2 {
		if ref := cleanWorkspaceReference(match[1]); ref != "" {
			return &SystemAction{Action: "sync_project", Params: map[string]interface{}{"project": ref}}
		}
	}
	return nil
}

func systemActionProjectRef(params map[string]interface{}) string {
	for _, key := range []string{"project", "name", "target"} {
		value := strings.TrimSpace(fmt.Sprint(params[key]))
		if value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func (a *App) resolveCurrentWorkspace(session store.ChatSession) (store.Workspace, error) {
	workspace, err := a.fallbackWorkspaceForProjectKey(session.ProjectKey)
	if err != nil {
		return store.Workspace{}, err
	}
	if workspace == nil {
		return store.Workspace{}, fmt.Errorf("no active workspace")
	}
	return *workspace, nil
}

type projectWorkspaceSyncResult struct {
	WorkspaceID int64  `json:"workspace_id"`
	Name        string `json:"name"`
	DirPath     string `json:"dir_path"`
	Status      string `json:"status"`
	Output      string `json:"output,omitempty"`
}

func syncWorkspaceGitPull(dirPath string) (string, string) {
	if err := exec.Command("git", "-C", dirPath, "rev-parse", "--is-inside-work-tree").Run(); err != nil {
		return "skipped", "not a git workspace"
	}
	if err := exec.Command("git", "-C", dirPath, "remote", "get-url", "origin").Run(); err != nil {
		return "skipped", "no git remote configured"
	}
	result := executeShellCommand("git pull --ff-only", dirPath)
	output := strings.TrimSpace(result.Output)
	if result.TimedOut {
		return "failed", "git pull timed out"
	}
	if result.RunErr != nil || result.ExitCode != 0 {
		return "failed", output
	}
	if output == "" || output == "(no output)" {
		output = "Already up to date."
	}
	return "synced", output
}

func (a *App) executeProjectAction(session store.ChatSession, action *SystemAction) (string, map[string]interface{}, error) {
	switch strings.ToLower(strings.TrimSpace(action.Action)) {
	case "assign_workspace_project":
		workspace, err := a.resolveCurrentWorkspace(session)
		if err != nil {
			return "", nil, err
		}
		project, err := a.resolveProjectReference(systemActionProjectRef(action.Params))
		if err != nil {
			return "", nil, err
		}
		updated, err := a.store.SetWorkspaceProject(workspace.ID, &project.ID)
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("Assigned workspace %s to project %s.", updated.Name, project.Name), map[string]interface{}{
			"type":         "workspace_project_updated",
			"workspace_id": updated.ID,
			"project_id":   project.ID,
		}, nil
	case "show_workspace_project":
		workspace, err := a.resolveCurrentWorkspace(session)
		if err != nil {
			return "", nil, err
		}
		if workspace.ProjectID == nil || strings.TrimSpace(*workspace.ProjectID) == "" {
			return fmt.Sprintf("Workspace %s is not assigned to a project.", workspace.Name), map[string]interface{}{
				"type":         "workspace_project",
				"workspace_id": workspace.ID,
				"project_id":   nil,
			}, nil
		}
		project, err := a.store.GetProject(*workspace.ProjectID)
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("Workspace %s belongs to project %s.", workspace.Name, project.Name), map[string]interface{}{
			"type":         "workspace_project",
			"workspace_id": workspace.ID,
			"project_id":   project.ID,
			"project_name": project.Name,
		}, nil
	case "create_project":
		name := systemActionProjectRef(action.Params)
		project, created, err := a.createProject(projectCreateRequest{Name: name})
		if err != nil {
			return "", nil, err
		}
		message := fmt.Sprintf("Created project %s.", project.Name)
		if !created {
			message = fmt.Sprintf("Project %s already exists.", project.Name)
		}
		return message, map[string]interface{}{
			"type":       "create_project",
			"project_id": project.ID,
			"name":       project.Name,
			"dir_path":   filepath.Clean(project.RootPath),
			"created":    created,
		}, nil
	case "list_project_workspaces":
		project, err := a.resolveProjectReference(systemActionProjectRef(action.Params))
		if err != nil {
			return "", nil, err
		}
		workspaces, err := a.store.ListWorkspacesForProject(project.ID)
		if err != nil {
			return "", nil, err
		}
		names := make([]string, 0, len(workspaces))
		payloadWorkspaces := make([]map[string]interface{}, 0, len(workspaces))
		for _, workspace := range workspaces {
			names = append(names, workspace.Name)
			payloadWorkspaces = append(payloadWorkspaces, map[string]interface{}{
				"workspace_id": workspace.ID,
				"name":         workspace.Name,
				"dir_path":     workspace.DirPath,
			})
		}
		message := fmt.Sprintf("Project %s has %d workspace(s).", project.Name, len(workspaces))
		if len(names) > 0 {
			message += " " + strings.Join(names, ", ") + "."
		}
		return message, map[string]interface{}{
			"type":       "list_project_workspaces",
			"project_id": project.ID,
			"workspaces": payloadWorkspaces,
		}, nil
	case "sync_project":
		project, err := a.resolveProjectReference(systemActionProjectRef(action.Params))
		if err != nil {
			return "", nil, err
		}
		workspaces, err := a.store.ListWorkspacesForProject(project.ID)
		if err != nil {
			return "", nil, err
		}
		results := make([]projectWorkspaceSyncResult, 0, len(workspaces))
		synced := 0
		for _, workspace := range workspaces {
			status, output := syncWorkspaceGitPull(workspace.DirPath)
			if status == "synced" {
				synced++
			}
			results = append(results, projectWorkspaceSyncResult{
				WorkspaceID: workspace.ID,
				Name:        workspace.Name,
				DirPath:     workspace.DirPath,
				Status:      status,
				Output:      output,
			})
		}
		message := fmt.Sprintf("Synced %d of %d workspace(s) for project %s.", synced, len(workspaces), project.Name)
		return message, map[string]interface{}{
			"type":       "sync_project",
			"project_id": project.ID,
			"results":    results,
		}, nil
	default:
		return "", nil, fmt.Errorf("unsupported project action: %s", action.Action)
	}
}
