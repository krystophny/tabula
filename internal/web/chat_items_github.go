package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/krystophny/tabura/internal/store"
)

var (
	githubIssueURLNumberPattern = regexp.MustCompile(`/issues/([0-9]+)`)
)

func parseInlineGitHubIssueActions(text string) []*SystemAction {
	normalized := normalizeItemCommandText(text)
	if normalized == "" {
		return nil
	}
	params := parseGitHubIssueCommandParams(text)
	if isSplitGitHubIssueCommand(normalized) {
		return []*SystemAction{
			{Action: "split_items", Params: map[string]interface{}{"count": 0}},
			{Action: "create_github_issue", Params: params},
		}
	}
	if !isCreateGitHubIssueCommand(normalized) {
		return nil
	}
	return []*SystemAction{{Action: "create_github_issue", Params: params}}
}

func isSplitGitHubIssueCommand(normalized string) bool {
	if !strings.HasPrefix(normalized, "split ") {
		return false
	}
	return strings.Contains(normalized, "local item") && strings.Contains(normalized, "github issue")
}

func isCreateGitHubIssueCommand(normalized string) bool {
	switch {
	case strings.HasPrefix(normalized, "file this as an issue"),
		strings.HasPrefix(normalized, "file it as an issue"),
		strings.HasPrefix(normalized, "file this as a github issue"),
		strings.HasPrefix(normalized, "file it as a github issue"):
		return true
	}
	if strings.Contains(normalized, "github issue") || strings.Contains(normalized, " as an issue") {
		for _, prefix := range []string{"create ", "open ", "file "} {
			if strings.HasPrefix(normalized, prefix) {
				return true
			}
		}
	}
	return false
}

func parseGitHubIssueCommandParams(text string) map[string]interface{} {
	params := map[string]interface{}{}
	if labels := extractGitHubLabels(text); len(labels) > 0 {
		params["labels"] = labels
	}
	if assignees := extractGitHubAssignees(text); len(assignees) > 0 {
		params["assignees"] = assignees
	}
	return params
}

func extractGitHubLabels(text string) []string {
	segment := extractGitHubCommandSegment(text, []string{
		" label it ",
		" labels ",
		" label ",
		" with labels ",
	})
	return parseGitHubTokenList(segment, false)
}

func extractGitHubAssignees(text string) []string {
	segment := extractGitHubCommandSegment(text, []string{
		" assign it to ",
		" assign to ",
		" assignees ",
		" assignee ",
	})
	return parseGitHubTokenList(segment, true)
}

func extractGitHubCommandSegment(text string, markers []string) string {
	lower := " " + strings.ToLower(strings.TrimSpace(text)) + " "
	raw := " " + strings.TrimSpace(text) + " "
	start := -1
	matchedMarker := ""
	for _, marker := range markers {
		idx := strings.Index(lower, marker)
		if idx < 0 {
			continue
		}
		if start == -1 || idx < start {
			start = idx
			matchedMarker = marker
		}
	}
	if start < 0 {
		return ""
	}
	segment := raw[start+len(matchedMarker):]
	end := len(segment)
	for _, marker := range []string{" assign it to ", " assign to ", " assignees ", " assignee ", " label it ", " labels ", " label ", " with labels "} {
		if strings.EqualFold(marker, matchedMarker) {
			continue
		}
		if idx := strings.Index(strings.ToLower(segment), marker); idx >= 0 && idx < end {
			end = idx
		}
	}
	return strings.TrimSpace(segment[:end])
}

func parseGitHubTokenList(raw string, stripAt bool) []string {
	clean := strings.TrimSpace(raw)
	clean = strings.Trim(clean, " \t\r\n.!?,:;")
	if clean == "" {
		return nil
	}
	for _, prefix := range []string{"with ", "as ", "it ", "them "} {
		if strings.HasPrefix(strings.ToLower(clean), prefix) {
			clean = strings.TrimSpace(clean[len(prefix):])
		}
	}
	clean = strings.ReplaceAll(clean, " and ", ",")
	parts := strings.Split(clean, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		token := strings.TrimSpace(part)
		token = strings.Trim(token, " \t\r\n.!?,:;")
		token = strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(token, " and"), "and "))
		if stripAt {
			token = strings.TrimPrefix(token, "@")
		}
		if token == "" {
			continue
		}
		key := strings.ToLower(token)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, token)
	}
	return out
}

func systemActionStringListParam(params map[string]interface{}, keys ...string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	appendTokens := func(tokens []string) {
		for _, token := range tokens {
			clean := strings.TrimSpace(token)
			if clean == "" {
				continue
			}
			key := strings.ToLower(clean)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, clean)
		}
	}
	for _, key := range keys {
		value, ok := params[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case []string:
			appendTokens(typed)
		case []interface{}:
			tokens := make([]string, 0, len(typed))
			for _, item := range typed {
				if token := strings.TrimSpace(fmt.Sprint(item)); token != "" && token != "<nil>" {
					tokens = append(tokens, token)
				}
			}
			appendTokens(tokens)
		case string:
			appendTokens(parseGitHubTokenList(typed, false))
		default:
			token := strings.TrimSpace(fmt.Sprint(typed))
			if token != "" && token != "<nil>" {
				appendTokens(parseGitHubTokenList(token, false))
			}
		}
	}
	return out
}

func githubIssueActionFailurePrefix(actions []*SystemAction) string {
	if len(actions) > 1 {
		return "I couldn't complete the GitHub issue workflow: "
	}
	return "I couldn't create the GitHub issue: "
}

func conversationGitHubIssueBody(ctx conversationItemContext) string {
	body := cleanConversationItemText(ctx.BodyText)
	if body != "" {
		return body
	}
	return strings.TrimSpace(ctx.Title)
}

func parseGitHubIssueNumberFromURL(raw string) (int, error) {
	trimmed := strings.TrimSpace(raw)
	for _, line := range strings.Split(trimmed, "\n") {
		candidate := strings.TrimSpace(line)
		if candidate == "" {
			continue
		}
		match := githubIssueURLNumberPattern.FindStringSubmatch(candidate)
		if len(match) != 2 {
			continue
		}
		number, err := strconv.Atoi(match[1])
		if err == nil && number > 0 {
			return number, nil
		}
	}
	return 0, fmt.Errorf("could not parse GitHub issue number from %q", trimmed)
}

func (a *App) createGitHubIssueInWorkspace(cwd, title, body string, labels, assignees []string) (ghIssueListItem, error) {
	runner := a.ghCommandRunner
	if runner == nil {
		runner = runGitHubCLI
	}
	ctx, cancel := context.WithTimeout(context.Background(), githubIssueListTimeout)
	defer cancel()

	cleanTitle := strings.TrimSpace(title)
	if cleanTitle == "" {
		return ghIssueListItem{}, errors.New("github issue title is required")
	}
	cleanBody := strings.TrimSpace(body)
	if cleanBody == "" {
		cleanBody = cleanTitle
	}
	args := []string{"issue", "create", "--title", cleanTitle, "--body", cleanBody}
	for _, label := range labels {
		if clean := strings.TrimSpace(label); clean != "" {
			args = append(args, "--label", clean)
		}
	}
	for _, assignee := range assignees {
		if clean := strings.TrimSpace(strings.TrimPrefix(assignee, "@")); clean != "" {
			args = append(args, "--assignee", clean)
		}
	}
	createRaw, err := runner(ctx, cwd, args...)
	if err != nil {
		return ghIssueListItem{}, err
	}
	number, err := parseGitHubIssueNumberFromURL(createRaw)
	if err != nil {
		return ghIssueListItem{}, err
	}
	viewRaw, err := runner(
		ctx,
		cwd,
		"issue", "view", strconv.Itoa(number),
		"--json", "number,title,url,state,labels,assignees",
	)
	if err != nil {
		return ghIssueListItem{}, err
	}
	var issue ghIssueListItem
	if err := json.Unmarshal([]byte(viewRaw), &issue); err != nil {
		return ghIssueListItem{}, fmt.Errorf("invalid github issue response: %w", err)
	}
	if issue.Number <= 0 {
		return ghIssueListItem{}, errors.New("github issue number is required")
	}
	if strings.TrimSpace(issue.Title) == "" {
		return ghIssueListItem{}, fmt.Errorf("github issue #%d title is required", issue.Number)
	}
	return issue, nil
}

func conversationItemCandidateScore(item store.Item, ctx conversationItemContext) int {
	score := 0
	if ctx.ArtifactID != nil && item.ArtifactID != nil && *ctx.ArtifactID == *item.ArtifactID {
		score += 100
	}
	if strings.EqualFold(strings.TrimSpace(item.Title), strings.TrimSpace(ctx.Title)) {
		score += 40
	}
	if ctx.WorkspaceID != nil && item.WorkspaceID != nil && *ctx.WorkspaceID == *item.WorkspaceID {
		score += 20
	}
	if score == 0 {
		return -1
	}
	return score
}

func linkedItemSourceDescription(item store.Item) string {
	source := strings.TrimSpace(stringFromPointer(item.Source))
	sourceRef := strings.TrimSpace(stringFromPointer(item.SourceRef))
	switch {
	case source != "" && sourceRef != "":
		return source + " " + sourceRef
	case sourceRef != "":
		return sourceRef
	default:
		return source
	}
}

func stringFromPointer(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func (a *App) findConversationPromotionItem(ctx conversationItemContext) (*store.Item, error) {
	var best *store.Item
	bestScore := -1
	for _, state := range []string{
		store.ItemStateInbox,
		store.ItemStateWaiting,
		store.ItemStateSomeday,
		store.ItemStateDone,
	} {
		items, err := a.store.ListItemsByState(state)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			score := conversationItemCandidateScore(item, ctx)
			if score < 0 {
				continue
			}
			if best == nil || score > bestScore || (score == bestScore && item.ID > best.ID) {
				candidate := item
				best = &candidate
				bestScore = score
			}
		}
	}
	if best == nil {
		return nil, nil
	}
	if source := linkedItemSourceDescription(*best); source != "" {
		return nil, fmt.Errorf("item is already linked to %s", source)
	}
	return best, nil
}

func (a *App) createGitHubIssueFromConversation(sessionID string, session store.ChatSession, action *SystemAction) (string, map[string]interface{}, error) {
	targetProject, err := a.systemActionTargetProject(session)
	if err != nil {
		return "", nil, err
	}
	ctx, err := a.buildConversationItemContext(sessionID, targetProject)
	if err != nil {
		return "", nil, err
	}
	if ctx.WorkspaceID == nil {
		return "", nil, errors.New("no workspace is linked to this conversation")
	}
	workspace, err := a.store.GetWorkspace(*ctx.WorkspaceID)
	if err != nil {
		return "", nil, err
	}
	repo, err := a.store.GitHubRepoForWorkspace(workspace.ID)
	if err != nil {
		return "", nil, err
	}
	if strings.TrimSpace(repo) == "" {
		return "", nil, errors.New("workspace has no GitHub origin remote")
	}

	existingItem, err := a.findConversationPromotionItem(ctx)
	if err != nil {
		return "", nil, err
	}

	title := strings.TrimSpace(ctx.Title)
	body := conversationGitHubIssueBody(ctx)
	labels := systemActionStringListParam(action.Params, "labels", "label")
	assignees := systemActionStringListParam(action.Params, "assignees", "assignee")
	for i, assignee := range assignees {
		assignees[i] = strings.TrimSpace(strings.TrimPrefix(assignee, "@"))
	}

	issue, err := a.createGitHubIssueInWorkspace(workspace.DirPath, title, body, labels, assignees)
	if err != nil {
		return "", nil, err
	}

	source := "github"
	sourceRef := githubIssueSourceRef(repo, issue.Number)
	var item store.Item
	if existingItem == nil {
		opts := store.ItemOptions{
			WorkspaceID: &workspace.ID,
			ArtifactID:  ctx.ArtifactID,
			Source:      &source,
			SourceRef:   &sourceRef,
		}
		item, err = a.store.CreateItem(title, opts)
		if err != nil {
			return "", nil, err
		}
	} else {
		item = *existingItem
		if item.ArtifactID == nil && ctx.ArtifactID != nil {
			if err := a.store.UpdateItemArtifact(item.ID, ctx.ArtifactID); err != nil {
				return "", nil, err
			}
			item.ArtifactID = ctx.ArtifactID
		}
		if err := a.store.UpdateItemSource(item.ID, source, sourceRef); err != nil {
			return "", nil, err
		}
	}
	if err := a.syncGitHubIssueArtifact(item, repo, issue); err != nil {
		return "", nil, err
	}
	updated, err := a.store.GetItem(item.ID)
	if err != nil {
		return "", nil, err
	}

	payload := map[string]interface{}{
		"type":       "github_issue_created",
		"item_id":    updated.ID,
		"issue_no":   issue.Number,
		"issue_url":  strings.TrimSpace(issue.URL),
		"source":     source,
		"source_ref": sourceRef,
		"state":      updated.State,
		"title":      updated.Title,
	}
	if len(labels) > 0 {
		payload["labels"] = labels
	}
	if len(assignees) > 0 {
		payload["assignees"] = assignees
	}
	return fmt.Sprintf("Created GitHub issue #%d: %s", issue.Number, strings.TrimSpace(issue.URL)), payload, nil
}
