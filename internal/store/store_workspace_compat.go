package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Project struct {
	ID                       string `json:"id"`
	Name                     string `json:"name"`
	WorkspacePath               string `json:"workspace_path"`
	RootPath                 string `json:"root_path"`
	Kind                     string `json:"kind"`
	MCPURL                   string `json:"mcp_url,omitempty"`
	CanvasSessionID          string `json:"canvas_session_id"`
	ChatModel                string `json:"chat_model"`
	ChatModelReasoningEffort string `json:"chat_model_reasoning_effort"`
	CompanionConfigJSON      string `json:"-"`
	IsDefault                bool   `json:"is_default"`
	CreatedAt                int64  `json:"created_at"`
	UpdatedAt                int64  `json:"updated_at"`
	LastOpenedAt             int64  `json:"last_opened_at"`
}

func workspaceIDString(id int64) string {
	return strconv.FormatInt(id, 10)
}

func parseWorkspaceIDString(id string) (int64, error) {
	workspaceID, err := strconv.ParseInt(strings.TrimSpace(id), 10, 64)
	if err != nil || workspaceID <= 0 {
		return 0, sql.ErrNoRows
	}
	return workspaceID, nil
}

func parseWorkspaceTimestamp(value string) int64 {
	if strings.TrimSpace(value) == "" {
		return 0
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.Unix()
	}
	if parsed, err := time.Parse("2006-01-02 15:04:05", value); err == nil {
		return parsed.Unix()
	}
	return 0
}

func projectFromWorkspace(workspace Workspace) Project {
	return Project{
		ID:                       workspaceIDString(workspace.ID),
		Name:                     workspace.Name,
		WorkspacePath:               workspace.DirPath,
		RootPath:                 workspace.DirPath,
		Kind:                     "workspace",
		MCPURL:                   workspace.MCPURL,
		CanvasSessionID:          workspace.CanvasSessionID,
		ChatModel:                normalizeWorkspaceChatModel(workspace.ChatModel),
		ChatModelReasoningEffort: normalizeWorkspaceChatModelReasoningEffort(workspace.ChatModelReasoningEffort),
		CompanionConfigJSON:      workspace.CompanionConfigJSON,
		IsDefault:                workspace.IsActive,
		CreatedAt:                parseWorkspaceTimestamp(workspace.CreatedAt),
		UpdatedAt:                parseWorkspaceTimestamp(workspace.UpdatedAt),
		LastOpenedAt:             parseWorkspaceTimestamp(workspace.UpdatedAt),
	}
}

func (s *Store) ListProjects() ([]Project, error) {
	workspaces, err := s.ListWorkspaces()
	if err != nil {
		return nil, err
	}
	out := make([]Project, 0, len(workspaces))
	for _, workspace := range workspaces {
		out = append(out, projectFromWorkspace(workspace))
	}
	return out, nil
}

func (s *Store) GetProject(id string) (Project, error) {
	workspaceID, err := parseWorkspaceIDString(id)
	if err != nil {
		return Project{}, err
	}
	workspace, err := s.GetWorkspace(workspaceID)
	if err != nil {
		return Project{}, err
	}
	return projectFromWorkspace(workspace), nil
}

func (s *Store) GetProjectByWorkspacePath(workspacePath string) (Project, error) {
	workspace, err := s.GetWorkspaceByPath(workspacePath)
	if err != nil {
		return Project{}, err
	}
	return projectFromWorkspace(workspace), nil
}

func (s *Store) GetProjectByRootPath(rootPath string) (Project, error) {
	return s.GetProjectByWorkspacePath(rootPath)
}

func (s *Store) GetProjectByCanvasSession(canvasSessionID string) (Project, error) {
	workspaces, err := s.ListWorkspaces()
	if err != nil {
		return Project{}, err
	}
	clean := strings.TrimSpace(canvasSessionID)
	for _, workspace := range workspaces {
		if strings.TrimSpace(workspace.CanvasSessionID) == clean {
			return projectFromWorkspace(workspace), nil
		}
	}
	return Project{}, sql.ErrNoRows
}

func (s *Store) CreateProject(name, workspacePath, rootPath, kind, mcpURL, canvasSessionID string, isDefault bool) (Project, error) {
	sphere := SpherePrivate
	if activeSphere, err := s.ActiveSphere(); err == nil && strings.TrimSpace(activeSphere) != "" {
		sphere = activeSphere
	}
	workspace, err := s.CreateWorkspace(name, rootPath, sphere)
	if err != nil {
		return Project{}, err
	}
	if strings.TrimSpace(mcpURL) != "" {
		if updated, updateErr := s.UpdateWorkspaceMCPURL(workspace.ID, mcpURL); updateErr == nil {
			workspace = updated
		} else {
			return Project{}, updateErr
		}
	}
	if strings.TrimSpace(canvasSessionID) != "" {
		if updated, updateErr := s.UpdateWorkspaceCanvasSession(workspace.ID, canvasSessionID); updateErr == nil {
			workspace = updated
		} else {
			return Project{}, updateErr
		}
	}
	if isDefault {
		if err := s.SetActiveWorkspace(workspace.ID); err != nil {
			return Project{}, err
		}
		workspace, err = s.GetWorkspace(workspace.ID)
		if err != nil {
			return Project{}, err
		}
	}
	return projectFromWorkspace(workspace), nil
}

func (s *Store) UpdateWorkspaceMCPURL(id int64, mcpURL string) (Workspace, error) {
	_, err := s.db.Exec(`UPDATE workspaces SET mcp_url = ?, updated_at = datetime('now') WHERE id = ?`, strings.TrimSpace(mcpURL), id)
	if err != nil {
		return Workspace{}, err
	}
	return s.GetWorkspace(id)
}

func (s *Store) UpdateWorkspaceCanvasSession(id int64, canvasSessionID string) (Workspace, error) {
	_, err := s.db.Exec(`UPDATE workspaces SET canvas_session_id = ?, updated_at = datetime('now') WHERE id = ?`, strings.TrimSpace(canvasSessionID), id)
	if err != nil {
		return Workspace{}, err
	}
	return s.GetWorkspace(id)
}

func (s *Store) SetActiveWorkspaceID(workspaceID string) error {
	workspaceNumericID, err := parseWorkspaceIDString(workspaceID)
	if err != nil {
		return errors.New("workspace id is required")
	}
	return s.SetActiveWorkspace(workspaceNumericID)
}

func (s *Store) ActiveWorkspaceID() (string, error) {
	workspace, err := s.ActiveWorkspace()
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return workspaceIDString(workspace.ID), nil
}

func (s *Store) TouchProject(id string) error {
	workspaceID, err := parseWorkspaceIDString(id)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE workspaces SET updated_at = datetime('now') WHERE id = ?`, workspaceID)
	return err
}

func (s *Store) UpdateProjectTransport(id, mcpURL, canvasSessionID string) error {
	workspaceID, err := parseWorkspaceIDString(id)
	if err != nil {
		return err
	}
	if _, err := s.UpdateWorkspaceMCPURL(workspaceID, mcpURL); err != nil {
		return err
	}
	_, err = s.UpdateWorkspaceCanvasSession(workspaceID, canvasSessionID)
	return err
}

func (s *Store) UpdateProjectRuntime(id, mcpURL, canvasSessionID string) error {
	return s.UpdateProjectTransport(id, mcpURL, canvasSessionID)
}

func (s *Store) UpdateProjectChatModel(id, model string) error {
	workspaceID, err := parseWorkspaceIDString(id)
	if err != nil {
		return err
	}
	return s.UpdateWorkspaceChatModel(workspaceID, model)
}

func (s *Store) UpdateProjectChatModelReasoningEffort(id, effort string) error {
	workspaceID, err := parseWorkspaceIDString(id)
	if err != nil {
		return err
	}
	return s.UpdateWorkspaceChatModelReasoningEffort(workspaceID, effort)
}

func (s *Store) UpdateProjectKind(string, string) error {
	return nil
}

func (s *Store) RenameProject(id, name, workspacePath, rootPath, kind string) error {
	workspaceID, err := parseWorkspaceIDString(id)
	if err != nil {
		return err
	}
	if strings.TrimSpace(rootPath) != "" {
		_, err = s.UpdateWorkspaceLocation(workspaceID, name, rootPath)
		return err
	}
	_, err = s.UpdateWorkspaceName(workspaceID, name)
	return err
}

func (s *Store) UpdateProjectLocation(id, name, workspacePath, rootPath, kind string) error {
	return s.RenameProject(id, name, workspacePath, rootPath, kind)
}

func (s *Store) DeleteProject(workspaceID string) error {
	workspaceNumericID, err := parseWorkspaceIDString(workspaceID)
	if err != nil {
		return err
	}
	return s.DeleteWorkspace(workspaceNumericID)
}

func (s *Store) UpdateProjectCompanionConfig(id, configJSON string) error {
	workspaceID, err := parseWorkspaceIDString(id)
	if err != nil {
		return err
	}
	return s.UpdateWorkspaceCompanionConfig(workspaceID, configJSON)
}

func normalizeProjectName(name string) string {
	return normalizeWorkspaceName(name)
}

func normalizeProjectPath(path string) string {
	return normalizeWorkspacePath(path)
}

func normalizeProjectChatModel(raw string) string {
	return normalizeWorkspaceChatModel(raw)
}

func normalizeProjectChatModelReasoningEffort(raw string) string {
	return normalizeWorkspaceChatModelReasoningEffort(raw)
}

func (s *Store) workspaceForProject(project Project) (Workspace, error) {
	workspaceID, err := parseWorkspaceIDString(project.ID)
	if err != nil {
		return Workspace{}, err
	}
	return s.GetWorkspace(workspaceID)
}

func (s *Store) projectForWorkspace(workspace Workspace) (*Project, error) {
	project := projectFromWorkspace(workspace)
	return &project, nil
}

func (s *Store) ensureWorkspaceForLegacyProject(project Project) (Workspace, error) {
	return s.workspaceForProject(project)
}

func (s *Store) ensureWorkspaceForProject(project Project) (Workspace, error) {
	return s.workspaceForProject(project)
}

func (s *Store) ListWorkspacesForProject(workspaceID string) ([]Workspace, error) {
	project, err := s.GetProject(workspaceID)
	if err != nil {
		return nil, err
	}
	workspace, err := s.workspaceForProject(project)
	if err != nil {
		return nil, err
	}
	return []Workspace{workspace}, nil
}

func (s *Store) SetWorkspaceProject(id int64, _ *string) (Workspace, error) {
	return s.GetWorkspace(id)
}

func (s *Store) FindWorkspaceByProjectPath(path string) (*int64, error) {
	return s.FindWorkspaceContainingPath(path)
}

func (s *Store) inferWorkspaceIDForWorkspacePath(dirPath string) (*string, error) {
	if workspace, err := s.GetWorkspaceByPath(dirPath); err == nil {
		id := workspaceIDString(workspace.ID)
		return &id, nil
	}
	return nil, nil
}

func (s *Store) migrateLegacyProjectData() error {
	return nil
}

func (s *Store) purgeLegacyHubData() error {
	return nil
}

func sameWorkspaceID(current *string, want string) bool {
	return current != nil && strings.TrimSpace(*current) == strings.TrimSpace(want)
}

func (s *Store) copyLegacyProjectRuntimeConfigToWorkspace(Project) error {
	return nil
}

func (s *Store) linkContextToLegacyProject(int64, Project) error {
	return nil
}

func (s *Store) projectForWorkspaceID(workspaceID int64) (Project, error) {
	workspace, err := s.GetWorkspace(workspaceID)
	if err != nil {
		return Project{}, err
	}
	return projectFromWorkspace(workspace), nil
}

func (s *Store) appServerModelProfileForProject(project Project) string {
	return normalizeWorkspaceChatModel(project.ChatModel)
}

func (s *Store) appServerModelProfileForWorkspacePath(workspacePath string) string {
	project, err := s.GetProjectByWorkspacePath(workspacePath)
	if err != nil {
		return ""
	}
	return s.appServerModelProfileForProject(project)
}

func (s *Store) UpdateProjectCanvasSession(id, canvasSessionID string) error {
	workspaceID, err := parseWorkspaceIDString(id)
	if err != nil {
		return err
	}
	_, err = s.UpdateWorkspaceCanvasSession(workspaceID, canvasSessionID)
	return err
}

func (s *Store) UpdateProjectMCPURL(id, mcpURL string) error {
	workspaceID, err := parseWorkspaceIDString(id)
	if err != nil {
		return err
	}
	_, err = s.UpdateWorkspaceMCPURL(workspaceID, mcpURL)
	return err
}

func (s *Store) projectByPath(path string) (Project, error) {
	return s.GetProjectByWorkspacePath(path)
}

func (s *Store) activeProject() (Project, error) {
	id, err := s.ActiveWorkspaceID()
	if err != nil {
		return Project{}, err
	}
	return s.GetProject(id)
}

func (s *Store) workspaceIDForWorkspace(workspaceID int64) string {
	return workspaceIDString(workspaceID)
}

func invalidWorkspaceIDError(id string) error {
	return fmt.Errorf("invalid workspace-backed project id: %s", strings.TrimSpace(id))
}
