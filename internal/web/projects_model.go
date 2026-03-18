package web

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/krystophny/tabura/internal/modelprofile"
	"github.com/krystophny/tabura/internal/serve"
	"github.com/krystophny/tabura/internal/store"
)

const workspaceServeStartTimeout = 10 * time.Second

func (a *App) canvasSessionIDForWorkspace(project store.Workspace) string {
	sessionID := strings.TrimSpace(project.CanvasSessionID)
	if sessionID != "" {
		return sessionID
	}
	if project.IsDefault {
		return LocalSessionID
	}
	return workspaceIDStr(project.ID)
}

func (a *App) buildWorkspaceAPIModel(project store.Workspace) (workspaceAPIModel, error) {
	session, err := a.chatSessionForWorkspace(project)
	if err != nil {
		return workspaceAPIModel{}, err
	}
	sphere, err := a.workspaceSphere(project)
	if err != nil {
		return workspaceAPIModel{}, err
	}
	alias := a.effectiveWorkspaceChatModelAlias(project)
	effort := strings.TrimSpace(modelprofile.NormalizeReasoningEffort(alias, project.ChatModelReasoningEffort))
	unread, reviewPending := a.workspaceUnreadState(project, session)
	return workspaceAPIModel{
		ID:                       workspaceIDStr(project.ID),
		Name:                     project.Name,
		Kind:                     project.Kind,
		RootPath:                 project.RootPath,
		Sphere:                   sphere,
		WorkspacePath:            project.WorkspacePath,
		MCPURL:                   strings.TrimSpace(project.MCPURL),
		IsDefault:                project.IsDefault,
		ChatSessionID:            session.ID,
		ChatMode:                 session.Mode,
		ChatModel:                alias,
		ChatModelReasoningEffort: effort,
		CanvasSessionID:          a.canvasSessionIDForWorkspace(project),
		RunState:                 a.workspaceRunStateForSession(session.ID),
		Unread:                   unread,
		ReviewPending:            reviewPending,
	}, nil
}

func (a *App) workspaceSphere(project store.Workspace) (string, error) {
	if workspace, err := a.workspaceOfWorkspace(project); err == nil && workspace != nil {
		return workspace.Sphere, nil
	} else if err != nil {
		return "", err
	}
	workspaceID, err := a.store.FindWorkspaceContainingPath(project.RootPath)
	if err != nil || workspaceID == nil {
		return "", err
	}
	workspace, err := a.store.GetWorkspace(*workspaceID)
	if err != nil {
		return "", err
	}
	return workspace.Sphere, nil
}

func (a *App) workspaceSelectionRank(project store.Workspace, activeSphere string) (int, error) {
	workspaceSphere, err := a.workspaceSphere(project)
	if err != nil {
		return 0, err
	}
	cleanProjectSphere := normalizeRuntimeActiveSphere(workspaceSphere)
	cleanActiveSphere := normalizeRuntimeActiveSphere(activeSphere)
	switch {
	case cleanActiveSphere != "" && cleanProjectSphere == cleanActiveSphere:
		return 0, nil
	case cleanProjectSphere == "" && project.IsDefault:
		return 1, nil
	case cleanProjectSphere == "":
		return 2, nil
	default:
		return 4, nil
	}
}

func (a *App) buildWorkspaceActivityItem(project store.Workspace) (workspaceActivityItem, error) {
	session, err := a.chatSessionForWorkspace(project)
	if err != nil {
		return workspaceActivityItem{}, err
	}
	unread, reviewPending := a.workspaceUnreadState(project, session)
	return workspaceActivityItem{
		WorkspaceID:   workspaceIDStr(project.ID),
		WorkspacePath: project.WorkspacePath,
		Name:          project.Name,
		Kind:          project.Kind,
		ChatSessionID: session.ID,
		ChatMode:      session.Mode,
		RunState:      a.workspaceRunStateForSession(session.ID),
		Unread:        unread,
		ReviewPending: reviewPending,
	}, nil
}

func (a *App) workspaceUnreadState(project store.Workspace, session store.ChatSession) (bool, bool) {
	lastSeenAt, lastCanvasChangeAt, lastReviewSubmitAt := a.workspaceAttention.snapshot(project.WorkspacePath)
	var dbSeenAt int64
	if t, err := time.Parse("2006-01-02 15:04:05", project.UpdatedAt); err == nil {
		dbSeenAt = t.UnixNano()
	}
	if lastSeenAt < dbSeenAt {
		lastSeenAt = dbSeenAt
	}
	reviewPending := strings.EqualFold(session.Mode, "review") && lastCanvasChangeAt > lastReviewSubmitAt
	unread := lastCanvasChangeAt > lastSeenAt || reviewPending
	activeWorkspaceID, err := a.store.ActiveWorkspaceID()
	if err == nil && strings.TrimSpace(activeWorkspaceID) == workspaceIDStr(project.ID) && !reviewPending {
		unread = false
	}
	return unread, reviewPending
}

func (a *App) markWorkspaceSeen(project store.Workspace) error {
	if err := a.store.TouchWorkspace(workspaceIDStr(project.ID)); err != nil {
		return err
	}
	a.workspaceAttention.markSeen(project.WorkspacePath, time.Now().UnixNano())
	return nil
}

func (a *App) markWorkspaceReviewSubmitted(project store.Workspace) error {
	now := time.Now().UnixNano()
	if err := a.store.TouchWorkspace(workspaceIDStr(project.ID)); err != nil {
		return err
	}
	a.workspaceAttention.markSeen(project.WorkspacePath, now)
	a.workspaceAttention.markReviewSubmitted(project.WorkspacePath, now)
	return nil
}

func (a *App) markWorkspaceOutput(workspacePath string) {
	key := strings.TrimSpace(workspacePath)
	if key == "" {
		return
	}
	now := time.Now().UnixNano()
	a.workspaceAttention.markCanvasChange(key, now)
	project, err := a.store.GetWorkspaceByStoredPath(key)
	if err != nil {
		return
	}
	activeWorkspaceID, err := a.store.ActiveWorkspaceID()
	if err != nil || strings.TrimSpace(activeWorkspaceID) != workspaceIDStr(project.ID) {
		return
	}
	session, err := a.store.GetOrCreateChatSession(project.WorkspacePath)
	if err != nil || strings.EqualFold(session.Mode, "review") {
		return
	}
	a.workspaceAttention.markSeen(project.WorkspacePath, now)
}

func chooseLoopbackPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok || addr.Port <= 0 {
		return 0, errors.New("unable to allocate tcp port")
	}
	return addr.Port, nil
}

func (a *App) startWorkspaceServe(sessionID, projectDir string) error {
	sessionID = strings.TrimSpace(sessionID)
	projectDir = strings.TrimSpace(projectDir)
	if sessionID == "" {
		return errors.New("project session is required")
	}
	if projectDir == "" {
		return errors.New("project path is required")
	}
	if a.tunnels.hasPort(sessionID) {
		return nil
	}

	port, err := chooseLoopbackPort()
	if err != nil {
		return err
	}
	projectApp := serve.NewApp(projectDir, "")
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		_ = projectApp.Start("127.0.0.1", port)
	}()
	deadline := time.Now().Add(workspaceServeStartTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			cancel()
			return errors.New("project serve canceled")
		default:
		}
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				a.tunnels.setProjectServe(sessionID, projectApp, cancel)
				a.tunnels.setPort(sessionID, port)
				a.startCanvasRelay(sessionID, port)
				return nil
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	cancel()
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	_ = projectApp.Stop(stopCtx)
	return errors.New("project tabura MCP listener did not become healthy in time")
}

func (a *App) ensureProjectCanvasReady(project store.Workspace) error {
	sessionID := a.canvasSessionIDForWorkspace(project)
	if a.tunnels.hasPort(sessionID) {
		return nil
	}

	if mcpURL := strings.TrimSpace(project.MCPURL); mcpURL != "" {
		port, err := extractPort(mcpURL)
		if err != nil {
			return err
		}
		a.tunnels.setPort(sessionID, port)
		a.startCanvasRelay(sessionID, port)
		return nil
	}

	if sessionID == LocalSessionID && strings.TrimSpace(a.localProjectDir) != "" {
		if err := a.startLocalServe(); err != nil {
			return err
		}
		if a.tunnels.hasPort(sessionID) {
			return nil
		}
	}

	return a.startWorkspaceServe(sessionID, project.RootPath)
}

func (a *App) activateWorkspace(workspaceID string) (store.Workspace, error) {
	project, err := a.store.GetEnrichedWorkspace(strings.TrimSpace(workspaceID))
	if err != nil {
		return store.Workspace{}, err
	}
	workspaceSphere, err := a.workspaceSphere(project)
	if err != nil {
		return store.Workspace{}, err
	}
	if cleanSphere := normalizeRuntimeActiveSphere(workspaceSphere); cleanSphere != "" && cleanSphere != a.runtimeActiveSphere() {
		if err := a.store.SetActiveSphere(cleanSphere); err != nil {
			return store.Workspace{}, err
		}
	}
	if err := a.ensureProjectCanvasReady(project); err != nil {
		return store.Workspace{}, err
	}
	if err := a.store.SetActiveWorkspaceID(workspaceIDStr(project.ID)); err != nil {
		return store.Workspace{}, err
	}
	if _, err := a.ensureWorkspaceReady(project, true); err != nil {
		return store.Workspace{}, err
	}
	if err := a.markWorkspaceSeen(project); err != nil {
		return store.Workspace{}, err
	}
	if _, _, err := a.syncTimeTrackingContext("project_switch"); err != nil {
		return store.Workspace{}, err
	}
	return a.store.GetEnrichedWorkspace(workspaceIDStr(project.ID))
}

func (a *App) handleWorkspaceActivate(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	workspaceID := strings.TrimSpace(chi.URLParam(r, "workspace_id"))
	if workspaceID == "" {
		http.Error(w, "workspace_id is required", http.StatusBadRequest)
		return
	}
	project, err := a.activateWorkspace(workspaceID)
	if err != nil {
		if isNoRows(err) {
			http.Error(w, "project not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	item, err := a.buildWorkspaceAPIModel(project)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{
		"ok":                  true,
		"active_workspace_id": workspaceIDStr(project.ID),
		"active_sphere":       a.runtimeActiveSphere(),
		"workspace":           item,
	})
}

func (a *App) updateWorkspaceChatModel(workspaceID, rawModel, rawReasoningEffort string) (store.Workspace, error) {
	project, err := a.store.GetEnrichedWorkspace(strings.TrimSpace(workspaceID))
	if err != nil {
		return store.Workspace{}, err
	}
	requestedModel := strings.TrimSpace(rawModel)
	modelAlias := modelprofile.ResolveAlias(requestedModel, "")
	if requestedModel != "" && modelAlias == "" {
		return store.Workspace{}, fmt.Errorf("unsupported model alias: %s", requestedModel)
	}
	if modelAlias != "" && modelAlias != modelprofile.AliasSpark {
		return store.Workspace{}, fmt.Errorf("persistent project model switching is disabled; spark is the only default dialogue model")
	}
	if modelAlias == "" {
		modelAlias = a.effectiveWorkspaceChatModelAlias(project)
	}
	if modelAlias == "" {
		modelAlias = modelprofile.AliasSpark
	}
	reasoningEffort := strings.TrimSpace(modelprofile.NormalizeReasoningEffort(modelAlias, rawReasoningEffort))
	if reasoningEffort == "" {
		reasoningEffort = strings.TrimSpace(modelprofile.MainThreadReasoningEffort(modelAlias))
	}
	if err := a.store.UpdateEnrichedWorkspaceChatModel(workspaceIDStr(project.ID), modelAlias); err != nil {
		return store.Workspace{}, err
	}
	if err := a.store.UpdateEnrichedWorkspaceChatModelReasoningEffort(workspaceIDStr(project.ID), reasoningEffort); err != nil {
		return store.Workspace{}, err
	}
	_ = a.store.SetAppState(appStateDefaultChatModelKey, modelAlias)
	updated, err := a.store.GetEnrichedWorkspace(workspaceIDStr(project.ID))
	if err != nil {
		return store.Workspace{}, err
	}
	a.resetWorkspaceChatAppSession(updated.WorkspacePath)
	return updated, nil
}

func (a *App) handleWorkspaceChatModelUpdate(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	workspaceID := strings.TrimSpace(chi.URLParam(r, "workspace_id"))
	if workspaceID == "" {
		http.Error(w, "workspace_id is required", http.StatusBadRequest)
		return
	}
	var req workspaceChatModelRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	project, err := a.updateWorkspaceChatModel(workspaceID, req.Model, req.ReasoningEffort)
	if err != nil {
		if isNoRows(err) {
			http.Error(w, "project not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	item, err := a.buildWorkspaceAPIModel(project)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{
		"ok":        true,
		"workspace": item,
	})
}

func (a *App) resolveRuntimeWorkspaceByIDOrActive(workspaceID string) (store.Workspace, error) {
	id := strings.TrimSpace(workspaceID)
	if id == "" || strings.EqualFold(id, "active") {
		projects, defaultProject, err := a.listProjectsWithDefault()
		if err != nil {
			return store.Workspace{}, err
		}
		return a.chooseActiveProject(projects, defaultProject)
	}
	return a.store.GetEnrichedWorkspace(id)
}

func normalizeProjectListPath(raw string) (string, error) {
	cleaned := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	cleaned = strings.Trim(cleaned, "/")
	if cleaned == "" || cleaned == "." {
		return "", nil
	}
	if strings.ContainsRune(cleaned, '\x00') {
		return "", errors.New("invalid path")
	}
	parts := strings.Split(cleaned, "/")
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part {
		case "", ".":
			continue
		case "..":
			return "", errors.New("invalid path")
		default:
			normalized = append(normalized, part)
		}
	}
	return strings.Join(normalized, "/"), nil
}

func pathWithinRoot(path, root string) bool {
	cleanPath := filepath.Clean(path)
	cleanRoot := filepath.Clean(root)
	if cleanPath == cleanRoot {
		return true
	}
	return strings.HasPrefix(cleanPath, cleanRoot+string(os.PathSeparator))
}

func (a *App) handleWorkspaceContext(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	workspaceID := strings.TrimSpace(chi.URLParam(r, "workspace_id"))
	project, err := a.resolveRuntimeWorkspaceByIDOrActive(workspaceID)
	if err != nil {
		if isNoRows(err) {
			http.Error(w, "project not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	project, err = a.activateWorkspace(workspaceIDStr(project.ID))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	item, err := a.buildWorkspaceAPIModel(project)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{
		"ok":                  true,
		"active_workspace_id": workspaceIDStr(project.ID),
		"workspace":           item,
	})
}

func (a *App) handleWorkspaceFilesList(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	workspace, err := a.resolveRuntimeWorkspaceByIDOrActive(chi.URLParam(r, "workspace_id"))
	if err != nil {
		if isNoRows(err) {
			http.Error(w, "workspace not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	relPath, err := normalizeProjectListPath(r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	rootPath := filepath.Clean(strings.TrimSpace(workspace.DirPath))
	targetPath := rootPath
	if relPath != "" {
		targetPath = filepath.Join(rootPath, filepath.FromSlash(relPath))
	}
	targetPath = filepath.Clean(targetPath)
	if !pathWithinRoot(targetPath, rootPath) {
		http.Error(w, "invalid path", http.StatusForbidden)
		return
	}
	info, err := os.Stat(targetPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, "path not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !info.IsDir() {
		http.Error(w, "path is not a directory", http.StatusBadRequest)
		return
	}
	entries, err := os.ReadDir(targetPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items := make([]workspaceFileEntry, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if name == "" || name == "." || name == ".." {
			continue
		}
		entryPath := name
		if relPath != "" {
			entryPath = relPath + "/" + name
		}
		items = append(items, workspaceFileEntry{
			Name:  name,
			Path:  entryPath,
			IsDir: entry.IsDir(),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].IsDir != items[j].IsDir {
			return items[i].IsDir
		}
		leftLower := strings.ToLower(items[i].Name)
		rightLower := strings.ToLower(items[j].Name)
		if leftLower != rightLower {
			return leftLower < rightLower
		}
		return items[i].Name < items[j].Name
	})
	writeJSON(w, map[string]interface{}{
		"ok":           true,
		"workspace_id": workspace.ID,
		"path":         relPath,
		"is_root":      relPath == "",
		"entries":      items,
	})
}
