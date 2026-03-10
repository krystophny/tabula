package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/krystophny/tabura/internal/store"
)

const githubIssueListTimeout = 60 * time.Second

type itemGitHubSyncRequest struct {
	WorkspaceID int64 `json:"workspace_id"`
}

type itemGitHubSyncResponse struct {
	OK          bool   `json:"ok"`
	WorkspaceID int64  `json:"workspace_id"`
	Repo        string `json:"repo"`
	Synced      int    `json:"synced"`
	Open        int    `json:"open"`
	Closed      int    `json:"closed"`
}

type ghIssueListLabel struct {
	Name string `json:"name"`
}

type ghIssueListAssignee struct {
	Login string `json:"login"`
}

type ghIssueListItem struct {
	Number    int                   `json:"number"`
	Title     string                `json:"title"`
	URL       string                `json:"url"`
	State     string                `json:"state"`
	Labels    []ghIssueListLabel    `json:"labels"`
	Assignees []ghIssueListAssignee `json:"assignees"`
}

func optionalTrimmedString(raw string) *string {
	clean := strings.TrimSpace(raw)
	if clean == "" {
		return nil
	}
	return &clean
}

func legacyBugReportIssueSourceRef(number int) string {
	return fmt.Sprintf("issue:%d", number)
}

func githubIssueSourceRef(ownerRepo string, number int) string {
	return fmt.Sprintf("%s#%d", strings.TrimSpace(ownerRepo), number)
}

func githubIssueItemState(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "open":
		return store.ItemStateInbox, nil
	case "closed":
		return store.ItemStateDone, nil
	default:
		return "", fmt.Errorf("unsupported github issue state %q", raw)
	}
}

func githubIssueArtifactMeta(ownerRepo string, issue ghIssueListItem) (*string, error) {
	labels := make([]string, 0, len(issue.Labels))
	for _, label := range issue.Labels {
		if clean := strings.TrimSpace(label.Name); clean != "" {
			labels = append(labels, clean)
		}
	}
	assignees := make([]string, 0, len(issue.Assignees))
	for _, assignee := range issue.Assignees {
		if clean := strings.TrimSpace(assignee.Login); clean != "" {
			assignees = append(assignees, clean)
		}
	}
	payload := map[string]any{
		"owner_repo": ownerRepo,
		"number":     issue.Number,
		"state":      strings.ToLower(strings.TrimSpace(issue.State)),
		"url":        strings.TrimSpace(issue.URL),
		"labels":     labels,
		"assignees":  assignees,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	text := string(raw)
	return &text, nil
}

func (a *App) listGitHubIssues(cwd string) ([]ghIssueListItem, error) {
	runner := a.ghCommandRunner
	if runner == nil {
		runner = runGitHubCLI
	}
	ctx, cancel := context.WithTimeout(context.Background(), githubIssueListTimeout)
	defer cancel()

	raw, err := runner(
		ctx,
		cwd,
		"issue", "list",
		"--state", "all",
		"--limit", "500",
		"--json", "number,title,url,state,labels,assignees",
	)
	if err != nil {
		return nil, err
	}
	var issues []ghIssueListItem
	if err := json.Unmarshal([]byte(raw), &issues); err != nil {
		return nil, fmt.Errorf("invalid github issue response: %w", err)
	}
	return issues, nil
}

func (a *App) syncGitHubIssueArtifact(item store.Item, ownerRepo string, issue ghIssueListItem) error {
	title := strings.TrimSpace(issue.Title)
	kind := store.ArtifactKindGitHubIssue
	refURL := optionalTrimmedString(issue.URL)
	metaJSON, err := githubIssueArtifactMeta(ownerRepo, issue)
	if err != nil {
		return err
	}

	createArtifact := func() (store.Artifact, error) {
		return a.store.CreateArtifact(kind, nil, refURL, optionalTrimmedString(title), metaJSON)
	}

	if item.ArtifactID == nil {
		artifact, err := createArtifact()
		if err != nil {
			return err
		}
		return a.store.UpdateItemArtifact(item.ID, &artifact.ID)
	}

	err = a.store.UpdateArtifact(*item.ArtifactID, store.ArtifactUpdate{
		Kind:     &kind,
		RefURL:   refURL,
		Title:    optionalTrimmedString(title),
		MetaJSON: metaJSON,
	})
	if errors.Is(err, sql.ErrNoRows) {
		artifact, createErr := createArtifact()
		if createErr != nil {
			return createErr
		}
		return a.store.UpdateItemArtifact(item.ID, &artifact.ID)
	}
	return err
}

func migrateLegacyBugReportIssue(a *App, workspaceID int64, ownerRepo string, issue ghIssueListItem) error {
	legacy, err := a.store.GetItemBySource("bug_report", legacyBugReportIssueSourceRef(issue.Number))
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if legacy.WorkspaceID == nil || *legacy.WorkspaceID != workspaceID {
		return nil
	}
	return a.store.UpdateItemSource(legacy.ID, "github", githubIssueSourceRef(ownerRepo, issue.Number))
}

func (a *App) syncGitHubIssues(workspaceID int64) (itemGitHubSyncResponse, error) {
	workspace, err := a.store.GetWorkspace(workspaceID)
	if err != nil {
		return itemGitHubSyncResponse{}, err
	}
	repo, err := a.store.GitHubRepoForWorkspace(workspaceID)
	if err != nil {
		return itemGitHubSyncResponse{}, err
	}
	if strings.TrimSpace(repo) == "" {
		return itemGitHubSyncResponse{}, errors.New("workspace has no GitHub origin remote")
	}

	issues, err := a.listGitHubIssues(workspace.DirPath)
	if err != nil {
		return itemGitHubSyncResponse{}, err
	}

	result := itemGitHubSyncResponse{
		OK:          true,
		WorkspaceID: workspace.ID,
		Repo:        repo,
		Synced:      len(issues),
	}
	for _, issue := range issues {
		if issue.Number <= 0 {
			return itemGitHubSyncResponse{}, errors.New("github issue number is required")
		}
		if strings.TrimSpace(issue.Title) == "" {
			return itemGitHubSyncResponse{}, fmt.Errorf("github issue #%d title is required", issue.Number)
		}
		if err := migrateLegacyBugReportIssue(a, workspace.ID, repo, issue); err != nil {
			return itemGitHubSyncResponse{}, err
		}

		item, err := a.store.UpsertItemFromSource("github", githubIssueSourceRef(repo, issue.Number), issue.Title, &workspace.ID)
		if err != nil {
			return itemGitHubSyncResponse{}, err
		}
		if err := a.syncGitHubIssueArtifact(item, repo, issue); err != nil {
			return itemGitHubSyncResponse{}, err
		}

		desiredState, err := githubIssueItemState(issue.State)
		if err != nil {
			return itemGitHubSyncResponse{}, err
		}
		switch desiredState {
		case store.ItemStateDone:
			result.Closed++
			if item.State != store.ItemStateDone {
				if err := a.store.CompleteItemBySource("github", githubIssueSourceRef(repo, issue.Number)); err != nil {
					return itemGitHubSyncResponse{}, err
				}
			}
		case store.ItemStateInbox:
			result.Open++
			if item.State == store.ItemStateDone {
				if err := a.store.SyncItemStateBySource("github", githubIssueSourceRef(repo, issue.Number), store.ItemStateInbox); err != nil {
					return itemGitHubSyncResponse{}, err
				}
			}
		}
	}
	return result, nil
}

func (a *App) handleGitHubIssueSync(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	var req itemGitHubSyncRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.WorkspaceID <= 0 {
		http.Error(w, "workspace_id is required", http.StatusBadRequest)
		return
	}

	result, err := a.syncGitHubIssues(req.WorkspaceID)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			http.Error(w, err.Error(), http.StatusNotFound)
		case strings.Contains(strings.ToLower(err.Error()), "no github origin remote"):
			http.Error(w, err.Error(), http.StatusBadRequest)
		default:
			http.Error(w, err.Error(), http.StatusBadGateway)
		}
		return
	}
	writeJSON(w, result)
}
