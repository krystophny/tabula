package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/sloppy-org/slopshell/internal/store"
	"github.com/sloppy-org/slopshell/internal/todoist"
	"github.com/sloppy-org/sloptools/pkg/taskgtd"
)

var (
	todoistMapWorkspacePattern = regexp.MustCompile(`(?i)^map\s+todoist\s+project\s+(.+?)\s+to\s+workspace\s+(.+?)$`)
	todoistCreateTaskPattern   = regexp.MustCompile(`(?i)^create\s+todoist\s+task\s*:?\s+(.+?)$`)
	todoistDueStringPattern    = regexp.MustCompile(`(?i)^(.*?)\s+(?:by|due)\s+(.+?)$`)
)

type todoistAccountConfig struct {
	BaseURL     string `json:"base_url"`
	MoveBaseURL string `json:"move_base_url"`
}

func parseInlineTodoistIntent(text string) *SystemAction {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	normalized := normalizeItemCommandText(trimmed)
	if normalized == "sync todoist" {
		return &SystemAction{Action: "sync_todoist", Params: map[string]interface{}{}}
	}
	if match := todoistMapWorkspacePattern.FindStringSubmatch(trimmed); len(match) == 3 {
		projectRef := strings.TrimSpace(match[1])
		workspaceRef := cleanWorkspaceReference(match[2])
		if projectRef != "" && workspaceRef != "" {
			return &SystemAction{
				Action: "map_todoist_project",
				Params: map[string]interface{}{
					"project":   projectRef,
					"workspace": workspaceRef,
				},
			}
		}
	}
	if match := todoistCreateTaskPattern.FindStringSubmatch(trimmed); len(match) == 2 {
		taskText := strings.TrimSpace(match[1])
		if taskText != "" {
			return &SystemAction{
				Action: "create_todoist_task",
				Params: map[string]interface{}{"text": taskText},
			}
		}
	}
	return nil
}

func todoistActionFailurePrefix(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "map_todoist_project":
		return "I couldn't save the Todoist mapping: "
	case "sync_todoist":
		return "I couldn't sync Todoist: "
	case "create_todoist_task":
		return "I couldn't create the Todoist task: "
	default:
		return "I couldn't resolve the Todoist request: "
	}
}

func todoistTaskSourceRef(taskID string) string {
	return "task:" + strings.TrimSpace(taskID)
}

func decodeTodoistAccountConfig(account store.ExternalAccount) (todoistAccountConfig, error) {
	var cfg todoistAccountConfig
	raw := strings.TrimSpace(account.ConfigJSON)
	if raw == "" || raw == "{}" {
		return cfg, nil
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return todoistAccountConfig{}, fmt.Errorf("decode todoist config: %w", err)
	}
	return cfg, nil
}

func todoistClientForAccount(account store.ExternalAccount) (*todoist.Client, error) {
	cfg, err := decodeTodoistAccountConfig(account)
	if err != nil {
		return nil, err
	}
	opts := []todoist.Option{}
	if strings.TrimSpace(cfg.BaseURL) != "" {
		opts = append(opts, todoist.WithBaseURL(cfg.BaseURL))
	}
	if strings.TrimSpace(cfg.MoveBaseURL) != "" {
		opts = append(opts, todoist.WithMoveBaseURL(cfg.MoveBaseURL))
	}
	return todoist.NewClientFromEnv(account.AccountName, opts...)
}

func todoistTaskFollowUpAt(task todoist.Task) *string {
	if task.Due == nil {
		return nil
	}
	if task.Due.DateTime != nil {
		value := strings.TrimSpace(*task.Due.DateTime)
		if value != "" {
			return normalizedTodoistTimestamp(value)
		}
	}
	if value := strings.TrimSpace(task.Due.Date); value != "" {
		followUp := value + "T09:00:00Z"
		return &followUp
	}
	return nil
}

func todoistTaskDueAt(task todoist.Task) *string {
	if task.Deadline == nil {
		return nil
	}
	value := strings.TrimSpace(task.Deadline.Date)
	if value == "" {
		return nil
	}
	due := value + "T23:59:59Z"
	return &due
}

func normalizedTodoistTimestamp(raw string) *string {
	clean := strings.TrimSpace(raw)
	if clean == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		parsed, err := time.Parse(layout, clean)
		if err == nil {
			value := parsed.UTC().Format(time.RFC3339Nano)
			return &value
		}
	}
	return &clean
}

func todoistEnabledAccounts(accounts []store.ExternalAccount, sphere string) []store.ExternalAccount {
	out := make([]store.ExternalAccount, 0, len(accounts))
	for _, account := range accounts {
		if !account.Enabled {
			continue
		}
		if sphere != "" && !strings.EqualFold(account.Sphere, sphere) {
			continue
		}
		out = append(out, account)
	}
	return out
}

func todoistProjectMapping(mappings []store.ExternalContainerMapping, projectName string) *store.ExternalContainerMapping {
	cleanName := strings.ToLower(strings.TrimSpace(projectName))
	if cleanName == "" {
		return nil
	}
	for i := range mappings {
		mapping := &mappings[i]
		if !strings.EqualFold(mapping.Provider, store.ExternalProviderTodoist) {
			continue
		}
		if !strings.EqualFold(mapping.ContainerType, "project") {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(mapping.ContainerRef), cleanName) {
			return mapping
		}
	}
	return nil
}

func (a *App) todoistProjectMappingForAccount(account store.ExternalAccount, mappings []store.ExternalContainerMapping, projectName string) (*store.ExternalContainerMapping, error) {
	mapping := todoistProjectMapping(mappings, projectName)
	if mapping == nil {
		return nil, nil
	}
	if mapping.Sphere != nil && !strings.EqualFold(strings.TrimSpace(*mapping.Sphere), account.Sphere) {
		return nil, nil
	}
	if mapping.WorkspaceID == nil {
		return mapping, nil
	}
	workspace, err := a.store.GetWorkspace(*mapping.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(workspace.Sphere, account.Sphere) {
		return nil, nil
	}
	return mapping, nil
}

func todoistProjectNameByID(projects []todoist.Project) map[string]string {
	out := make(map[string]string, len(projects))
	for _, project := range projects {
		projectID := strings.TrimSpace(project.ID)
		if projectID == "" {
			continue
		}
		out[projectID] = strings.TrimSpace(project.Name)
	}
	return out
}

func todoistProjectIDByName(projects []todoist.Project) map[string]string {
	out := make(map[string]string, len(projects))
	for _, project := range projects {
		name := strings.ToLower(strings.TrimSpace(project.Name))
		projectID := strings.TrimSpace(project.ID)
		if name == "" || projectID == "" {
			continue
		}
		out[name] = projectID
	}
	return out
}

func activeWorkspaceID(workspaces []store.Workspace) *int64 {
	for i := range workspaces {
		if workspaces[i].IsActive {
			id := workspaces[i].ID
			return &id
		}
	}
	return nil
}

func (a *App) activeTodoistAccounts() ([]store.ExternalAccount, error) {
	accounts, err := a.store.ListExternalAccountsByProvider(store.ExternalProviderTodoist)
	if err != nil {
		return nil, err
	}
	activeSphere, err := a.store.ActiveSphere()
	if err != nil {
		return nil, err
	}
	enabled := todoistEnabledAccounts(accounts, activeSphere)
	if len(enabled) > 0 {
		return enabled, nil
	}
	enabled = todoistEnabledAccounts(accounts, "")
	if len(enabled) == 0 {
		return nil, errors.New("no enabled Todoist account is configured")
	}
	return enabled, nil
}

func (a *App) persistTodoistTask(account store.ExternalAccount, task todoist.Task, comments []todoist.Comment, mappings []store.ExternalContainerMapping, projectNames map[string]string, kind string) (store.Item, error) {
	projectName := ""
	if task.ProjectID != nil {
		projectName = strings.TrimSpace(projectNames[strings.TrimSpace(*task.ProjectID)])
	}
	mapping, err := a.todoistProjectMappingForAccount(account, mappings, projectName)
	if err != nil {
		return store.Item{}, err
	}
	source := store.ExternalProviderTodoist
	sourceRef := todoistTaskSourceRef(task.ID)
	title := strings.TrimSpace(task.Content)
	if title == "" {
		return store.Item{}, errors.New("todoist task content is required")
	}
	followUpAt := todoistTaskFollowUpAt(task)
	dueAt := todoistTaskDueAt(task)
	desiredState := todoistItemState(task, time.Now())
	if existing, err := a.store.GetItemBySource(source, sourceRef); err == nil {
		return a.updatePersistedTodoistTask(existing, account, task, comments, mapping, projectName, projectNames, title, source, sourceRef, desiredState, followUpAt, dueAt, kind)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return store.Item{}, err
	}
	return a.createPersistedTodoistTask(account, task, comments, mapping, projectName, projectNames, title, source, sourceRef, desiredState, followUpAt, dueAt, kind)
}

func (a *App) updatePersistedTodoistTask(existing store.Item, account store.ExternalAccount, task todoist.Task, comments []todoist.Comment, mapping *store.ExternalContainerMapping, projectName string, projectNames map[string]string, title, source, sourceRef, desiredState string, followUpAt, dueAt *string, kind string) (store.Item, error) {
	state := desiredState
	updates := store.ItemUpdate{
		Title:        &title,
		Kind:         todoistKindUpdate(existing.Kind, kind),
		State:        todoistStateUpdate(existing.State, state),
		VisibleAfter: todoistTimeUpdate(followUpAt),
		FollowUpAt:   todoistTimeUpdate(followUpAt),
		DueAt:        todoistTimeUpdate(dueAt),
	}
	if mapping != nil {
		updates.WorkspaceID = mappedWorkspaceUpdate(mapping)
	} else if existing.WorkspaceID != nil {
		clear := int64(0)
		updates.WorkspaceID = &clear
	}
	if mapping == nil || mapping.WorkspaceID == nil {
		sphere := account.Sphere
		updates.Sphere = &sphere
	}
	if err := a.store.UpdateItem(existing.ID, updates); err != nil {
		return store.Item{}, err
	}
	item, err := a.store.GetItem(existing.ID)
	if err != nil {
		return store.Item{}, err
	}
	if existing.State == store.ItemStateDone && desiredState != store.ItemStateDone {
		if err := a.store.SyncItemStateBySource(source, sourceRef, desiredState); err != nil {
			return store.Item{}, err
		}
		item, err = a.store.GetItem(existing.ID)
		if err != nil {
			return store.Item{}, err
		}
	}
	if err := a.syncTodoistTaskArtifact(item, task, projectNames, comments); err != nil {
		return store.Item{}, err
	}
	item, err = a.store.GetItem(existing.ID)
	if err != nil {
		return store.Item{}, err
	}
	if err := a.upsertTodoistTaskBinding(account.ID, task.ID, item.ID, projectName); err != nil {
		return store.Item{}, err
	}
	return item, nil
}

func (a *App) createPersistedTodoistTask(account store.ExternalAccount, task todoist.Task, comments []todoist.Comment, mapping *store.ExternalContainerMapping, projectName string, projectNames map[string]string, title, source, sourceRef, desiredState string, followUpAt, dueAt *string, kind string) (store.Item, error) {
	opts := store.ItemOptions{
		State:        desiredState,
		Kind:         kind,
		Sphere:       &account.Sphere,
		VisibleAfter: followUpAt,
		FollowUpAt:   followUpAt,
		DueAt:        dueAt,
		Source:       &source,
		SourceRef:    &sourceRef,
	}
	if mapping != nil && mapping.WorkspaceID != nil {
		opts.WorkspaceID = mapping.WorkspaceID
	}
	item, err := a.store.CreateItem(title, opts)
	if err != nil {
		return store.Item{}, err
	}
	if err := a.syncTodoistTaskArtifact(item, task, projectNames, comments); err != nil {
		return store.Item{}, err
	}
	item, err = a.store.GetItem(item.ID)
	if err != nil {
		return store.Item{}, err
	}
	if err := a.upsertTodoistTaskBinding(account.ID, task.ID, item.ID, projectName); err != nil {
		return store.Item{}, err
	}
	return item, nil
}

func todoistKindUpdate(current, desired string) *string {
	if desired != store.ItemKindProject || current == store.ItemKindProject {
		return nil
	}
	kind := store.ItemKindProject
	return &kind
}

func todoistTimeUpdate(value *string) *string {
	if value != nil {
		return value
	}
	empty := ""
	return &empty
}

func todoistStateUpdate(current, desired string) *string {
	if desired == "" || current == desired {
		return nil
	}
	switch desired {
	case store.ItemStateDone:
		return &desired
	case store.ItemStateNext, store.ItemStateDeferred:
		switch current {
		case store.ItemStateInbox, store.ItemStateNext, store.ItemStateDeferred:
			return &desired
		}
	case store.ItemStateInbox:
		if current == store.ItemStateDone {
			return &desired
		}
	}
	return nil
}

func (a *App) upsertTodoistTaskBinding(accountID int64, taskID string, itemID int64, projectName string) error {
	_, err := a.store.UpsertExternalBinding(store.ExternalBinding{
		AccountID:    accountID,
		Provider:     store.ExternalProviderTodoist,
		ObjectType:   "task",
		RemoteID:     strings.TrimSpace(taskID),
		ItemID:       &itemID,
		ContainerRef: optionalStringPointer(projectName),
	})
	return err
}

func mappedWorkspaceUpdate(mapping *store.ExternalContainerMapping) *int64 {
	if mapping == nil {
		return nil
	}
	if mapping.WorkspaceID != nil {
		workspaceID := *mapping.WorkspaceID
		return &workspaceID
	}
	clear := int64(0)
	return &clear
}

func optionalStringPointer(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func (a *App) executeMapTodoistProjectAction(session store.ChatSession, action *SystemAction) (string, map[string]interface{}, error) {
	projectRef := strings.TrimSpace(systemActionStringParam(action.Params, "project"))
	if projectRef == "" {
		return "", nil, errors.New("todoist project name is required")
	}
	var workspaceID *int64
	if workspaceRef := systemActionWorkspaceRef(action.Params); workspaceRef != "" {
		workspace, err := a.resolveWorkspaceReference(session.WorkspacePath, workspaceRef)
		if err != nil {
			return "", nil, err
		}
		workspaceID = &workspace.ID
		mapping, err := a.store.SetContainerMapping(store.ExternalProviderTodoist, "project", projectRef, workspaceID, nil)
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("Mapped Todoist project %s to workspace %s.", projectRef, workspace.Name), map[string]interface{}{
			"type":          "todoist_container_mapping",
			"mapping_id":    mapping.ID,
			"container_ref": mapping.ContainerRef,
			"workspace_id":  workspace.ID,
		}, nil
	}
	return "", nil, errors.New("todoist mapping target workspace is required")
}

func (a *App) executeSyncTodoistAction() (string, map[string]interface{}, error) {
	accounts, err := a.activeTodoistAccounts()
	if err != nil {
		return "", nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	syncedCount := 0
	for _, account := range accounts {
		count, err := a.syncTodoistAccount(ctx, account)
		if err != nil {
			return "", nil, err
		}
		syncedCount += count
	}

	return fmt.Sprintf("Synced %d Todoist task(s).", syncedCount), map[string]interface{}{
		"type":  "sync_todoist",
		"count": syncedCount,
	}, nil
}

func (a *App) syncTodoistAccount(ctx context.Context, account store.ExternalAccount) (int, error) {
	if a != nil && a.accountSyncs != nil {
		return a.accountSyncs.Do(ctx, account.ID, func(ctx context.Context) (int, error) {
			return a.syncTodoistAccountDirect(ctx, account)
		})
	}
	return a.syncTodoistAccountDirect(ctx, account)
}

func (a *App) syncTodoistAccountDirect(ctx context.Context, account store.ExternalAccount) (int, error) {
	mappings, err := a.store.ListContainerMappings(store.ExternalProviderTodoist)
	if err != nil {
		return 0, err
	}
	client, err := todoistClientForAccount(account)
	if err != nil {
		return 0, err
	}
	projects, err := client.ListProjects(ctx)
	if err != nil {
		return 0, err
	}
	projectNames := todoistProjectNameByID(projects)
	tasks, err := client.ListTasks(ctx, todoist.ListTasksOptions{})
	if err != nil {
		return 0, err
	}
	parentIDs := taskgtd.ParentTaskIDs(todoistTaskRefs(tasks))
	itemsByTaskID := make(map[string]store.Item, len(tasks))
	syncedCount := 0
	for _, task := range tasks {
		comments := []todoist.Comment(nil)
		if task.CommentCount > 0 {
			detail, err := client.GetTask(ctx, task.ID)
			if err != nil {
				return 0, err
			}
			if strings.TrimSpace(detail.Task.ID) != "" {
				task = detail.Task
			}
			comments = append(comments, detail.Comments...)
		}
		kind := store.ItemKindAction
		if parentIDs[strings.TrimSpace(task.ID)] {
			kind = store.ItemKindProject
		}
		item, err := a.persistTodoistTask(account, task, comments, mappings, projectNames, kind)
		if err != nil {
			return 0, err
		}
		itemsByTaskID[strings.TrimSpace(task.ID)] = item
		syncedCount++
	}
	if err := a.linkSyncedTodoistSubtasks(tasks, itemsByTaskID); err != nil {
		return 0, err
	}
	return syncedCount, nil
}

func todoistTaskRefs(tasks []todoist.Task) []taskgtd.Task {
	out := make([]taskgtd.Task, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, taskgtd.Task{
			ID:       strings.TrimSpace(task.ID),
			ParentID: strings.TrimSpace(optionalStringValue(task.ParentID)),
		})
	}
	return out
}

func (a *App) linkSyncedTodoistSubtasks(tasks []todoist.Task, itemsByTaskID map[string]store.Item) error {
	for _, task := range tasks {
		parentID := strings.TrimSpace(optionalStringValue(task.ParentID))
		if parentID == "" {
			continue
		}
		parent, parentOK := itemsByTaskID[parentID]
		child, childOK := itemsByTaskID[strings.TrimSpace(task.ID)]
		if !parentOK || !childOK {
			continue
		}
		if err := a.store.LinkItemChild(parent.ID, child.ID, "next_action"); err != nil {
			return err
		}
	}
	return nil
}

func parseTodoistTaskDraft(raw string) (string, string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", ""
	}
	if match := todoistDueStringPattern.FindStringSubmatch(trimmed); len(match) == 3 {
		content := strings.TrimSpace(match[1])
		dueString := strings.TrimSpace(match[2])
		if content != "" && dueString != "" {
			return content, dueString
		}
	}
	return trimmed, ""
}

func (a *App) createTodoistTaskProjectTarget(client *todoist.Client) (string, error) {
	mappings, err := a.store.ListContainerMappings(store.ExternalProviderTodoist)
	if err != nil {
		return "", err
	}
	workspaces, err := a.store.ListWorkspaces()
	if err != nil {
		return "", err
	}
	activeWorkspaceID := activeWorkspaceID(workspaces)
	targetName := ""
	for _, mapping := range mappings {
		if !strings.EqualFold(mapping.Provider, store.ExternalProviderTodoist) || !strings.EqualFold(mapping.ContainerType, "project") {
			continue
		}
		if mapping.WorkspaceID != nil && activeWorkspaceID != nil && *mapping.WorkspaceID == *activeWorkspaceID {
			targetName = mapping.ContainerRef
			break
		}
	}
	if targetName == "" {
		return "", nil
	}
	projects, err := client.ListProjects(context.Background())
	if err != nil {
		return "", err
	}
	projectIDs := todoistProjectIDByName(projects)
	return projectIDs[strings.ToLower(strings.TrimSpace(targetName))], nil
}

func (a *App) executeCreateTodoistTaskAction(action *SystemAction) (string, map[string]interface{}, error) {
	accounts, err := a.activeTodoistAccounts()
	if err != nil {
		return "", nil, err
	}
	account := accounts[0]
	client, err := todoistClientForAccount(account)
	if err != nil {
		return "", nil, err
	}
	return a.executeCreateTodoistTaskActionWith(account, client, action)
}

func (a *App) executeCreateTodoistTaskActionWith(account store.ExternalAccount, client *todoist.Client, action *SystemAction) (string, map[string]interface{}, error) {
	taskText := strings.TrimSpace(systemActionStringParam(action.Params, "text"))
	if taskText == "" {
		return "", nil, errors.New("todoist task text is required")
	}
	content, dueString := parseTodoistTaskDraft(taskText)
	if content == "" {
		return "", nil, errors.New("todoist task content is required")
	}
	targetProjectID, err := a.createTodoistTaskProjectTarget(client)
	if err != nil {
		return "", nil, err
	}
	req := todoist.CreateTaskRequest{
		Content:   content,
		ProjectID: targetProjectID,
	}
	if dueString != "" {
		req.DueString = dueString
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	task, err := client.CreateTask(ctx, req)
	if err != nil {
		return "", nil, err
	}
	mappings, err := a.store.ListContainerMappings(store.ExternalProviderTodoist)
	if err != nil {
		return "", nil, err
	}
	projects, err := client.ListProjects(ctx)
	if err != nil {
		return "", nil, err
	}
	item, err := a.persistTodoistTask(account, task, nil, mappings, todoistProjectNameByID(projects), store.ItemKindAction)
	if err != nil {
		return "", nil, err
	}
	return fmt.Sprintf("Created Todoist task %q.", task.Content), map[string]interface{}{
		"type":       "todoist_task_created",
		"task_id":    strings.TrimSpace(task.ID),
		"item_id":    item.ID,
		"title":      item.Title,
		"project_id": targetProjectID,
	}, nil
}

func (a *App) executeTodoistAction(session store.ChatSession, action *SystemAction) (string, map[string]interface{}, error) {
	switch strings.ToLower(strings.TrimSpace(action.Action)) {
	case "map_todoist_project":
		return a.executeMapTodoistProjectAction(session, action)
	case "sync_todoist":
		return a.executeSyncTodoistAction()
	case "create_todoist_task":
		return a.executeCreateTodoistTaskAction(action)
	default:
		return "", nil, fmt.Errorf("unsupported todoist action: %s", action.Action)
	}
}
