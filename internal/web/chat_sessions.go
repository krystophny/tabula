package web

import (
	"errors"
	"strings"

	"github.com/krystophny/tabura/internal/store"
)

func (a *App) workspaceForChatSession(session store.ChatSession) (store.Workspace, error) {
	if session.WorkspaceID <= 0 {
		return store.Workspace{}, errors.New("chat session workspace is required")
	}
	workspace, err := a.store.GetWorkspace(session.WorkspaceID)
	if err != nil {
		return store.Workspace{}, err
	}
	if strings.TrimSpace(workspace.DirPath) == "" {
		return store.Workspace{}, errors.New("workspace path is required")
	}
	return workspace, nil
}

func (a *App) workspaceDirForChatSession(session store.ChatSession) (string, error) {
	workspace, err := a.workspaceForChatSession(session)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(workspace.DirPath), nil
}

func (a *App) workspaceDirForChatSessionID(sessionID string) (string, error) {
	session, err := a.store.GetChatSession(sessionID)
	if err != nil {
		return "", err
	}
	return a.workspaceDirForChatSession(session)
}

func (a *App) resolveChatSessionTarget(workspacePath string, workspaceID *int64) (store.Workspace, error) {
	if workspaceID != nil {
		return a.store.GetWorkspace(*workspaceID)
	}
	if cleanPath := strings.TrimSpace(workspacePath); cleanPath != "" {
		if workspace, err := a.store.GetWorkspaceByPath(cleanPath); err == nil {
			return workspace, nil
		} else if !isNoRows(err) {
			return store.Workspace{}, err
		}
	}
	return a.ensureStartupWorkspace()
}

func (a *App) chatSessionForWorkspace(workspace store.Workspace) (store.ChatSession, error) {
	return a.store.GetOrCreateChatSessionForWorkspace(workspace.ID)
}

func (a *App) chatSessionForWorkspacePath(workspacePath string) (store.ChatSession, error) {
	workspace, err := a.resolveChatSessionTarget(workspacePath, nil)
	if err != nil {
		return store.ChatSession{}, err
	}
	return a.chatSessionForWorkspace(workspace)
}

func (a *App) chatSessionForProject(project store.Project) (store.ChatSession, error) {
	return a.chatSessionForWorkspacePath(project.WorkspacePath)
}

func (a *App) projectForWorkspace(workspace store.Workspace) (*store.Project, error) {
	project, err := a.store.GetProjectByWorkspacePath(workspace.DirPath)
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return &project, nil
}
