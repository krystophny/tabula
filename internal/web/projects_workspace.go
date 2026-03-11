package web

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"

	"github.com/krystophny/tabura/internal/store"
)

func (a *App) workspaceForProject(project store.Project) (*store.Workspace, error) {
	rootPath := filepath.Clean(strings.TrimSpace(project.RootPath))
	if rootPath == "" || isHubProject(project) {
		return nil, nil
	}
	if workspace, err := a.store.GetWorkspaceByPath(rootPath); err == nil {
		return &workspace, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	workspaces, err := a.store.ListWorkspacesForProject(project.ID)
	if err != nil {
		return nil, err
	}
	for i := range workspaces {
		if filepath.Clean(workspaces[i].DirPath) == rootPath {
			return &workspaces[i], nil
		}
	}
	if workspaceID, err := a.store.FindWorkspaceContainingPath(rootPath); err == nil && workspaceID != nil {
		workspace, getErr := a.store.GetWorkspace(*workspaceID)
		if getErr != nil {
			return nil, getErr
		}
		return &workspace, nil
	} else if err != nil {
		return nil, err
	}
	return nil, nil
}

func (a *App) ensureWorkspaceForProject(project store.Project, activate bool) (store.Workspace, error) {
	if isHubProject(project) {
		return store.Workspace{}, errors.New("hub project has no workspace")
	}
	rootPath := filepath.Clean(strings.TrimSpace(project.RootPath))
	if rootPath == "" {
		return store.Workspace{}, errors.New("project path is required")
	}
	workspaceRef, err := a.workspaceForProject(project)
	if err != nil {
		return store.Workspace{}, err
	}
	if workspaceRef == nil {
		workspace, createErr := a.store.CreateWorkspace(project.Name, rootPath, a.runtimeActiveSphere())
		if createErr != nil {
			return store.Workspace{}, createErr
		}
		workspaceRef = &workspace
	}
	workspace := *workspaceRef
	if workspace.ProjectID == nil || strings.TrimSpace(*workspace.ProjectID) != project.ID {
		workspace, err = a.store.SetWorkspaceProject(workspace.ID, &project.ID)
		if err != nil {
			return store.Workspace{}, err
		}
	}
	if activate {
		if err := a.setActiveWorkspaceTracked(workspace.ID, "workspace_switch"); err != nil {
			return store.Workspace{}, err
		}
		workspace, err = a.store.GetWorkspace(workspace.ID)
		if err != nil {
			return store.Workspace{}, err
		}
	}
	if _, err := a.store.GetOrCreateChatSessionForWorkspace(workspace.ID); err != nil {
		return store.Workspace{}, err
	}
	return workspace, nil
}

func (a *App) ensureStartupProjectWithWorkspace() error {
	workspaces, err := a.store.ListWorkspaces()
	if err != nil {
		return err
	}
	if len(workspaces) > 0 {
		return nil
	}
	projects, err := a.store.ListProjects()
	if err != nil {
		return err
	}
	for _, project := range projects {
		if !isTemporaryProject(project) {
			continue
		}
		_, err := a.activateProject(project.ID)
		return err
	}
	project, _, err := a.createProject(projectCreateRequest{Kind: "task"})
	if err != nil {
		return err
	}
	_, err = a.activateProject(project.ID)
	return err
}
