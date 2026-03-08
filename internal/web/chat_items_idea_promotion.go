package web

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/krystophny/tabura/internal/store"
)

const (
	ideaPromotionTargetTask   = "task"
	ideaPromotionTargetItems  = "items"
	ideaPromotionTargetGitHub = "github"

	ideaPromotionDispositionKeep    = "keep"
	ideaPromotionDispositionDone    = "done"
	ideaPromotionDispositionSomeday = "someday"
)

var (
	ideaPromotionSelectionPattern = regexp.MustCompile(`(?i)\b(?:selected|items?)\s+([0-9][0-9,\s]*)`)
	ideaPromotionActionVerbSet    = map[string]struct{}{
		"add": {}, "break": {}, "build": {}, "capture": {}, "clarify": {}, "create": {}, "define": {},
		"document": {}, "draft": {}, "explore": {}, "file": {}, "fix": {}, "implement": {}, "investigate": {},
		"outline": {}, "plan": {}, "prepare": {}, "prototype": {}, "refactor": {}, "review": {}, "ship": {},
		"split": {}, "test": {}, "write": {},
	}
)

type activeIdeaNoteContext struct {
	Item     store.Item
	Artifact store.Artifact
	Meta     ideaNoteMeta
}

func normalizeIdeaPromotionTarget(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case ideaPromotionTargetTask, "single_task", "single-item", "single_item", "item":
		return ideaPromotionTargetTask
	case ideaPromotionTargetItems, "tasks":
		return ideaPromotionTargetItems
	case ideaPromotionTargetGitHub, "github_issue", "github issue", "issue":
		return ideaPromotionTargetGitHub
	default:
		return ""
	}
}

func normalizeIdeaPromotionDisposition(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", ideaPromotionDispositionKeep:
		return ideaPromotionDispositionKeep
	case ideaPromotionDispositionDone:
		return ideaPromotionDispositionDone
	case ideaPromotionDispositionSomeday:
		return ideaPromotionDispositionSomeday
	default:
		return ""
	}
}

func parseInlineIdeaPromotionIntent(text string) *SystemAction {
	normalized := normalizeItemCommandText(text)
	if normalized == "" {
		return nil
	}
	target := ""
	switch {
	case normalized == "make this actionable" || normalized == "turn this idea into a task" || normalized == "turn this idea into an item":
		target = ideaPromotionTargetTask
	case normalized == "turn this idea into items" || normalized == "turn this idea into tasks" || normalized == "split this idea into tasks" || normalized == "split this into tasks" || normalized == "break this down":
		target = ideaPromotionTargetItems
	case normalized == "create a github issue from this idea" || normalized == "file this idea as a github issue":
		target = ideaPromotionTargetGitHub
	}
	if target == "" {
		return nil
	}
	return &SystemAction{
		Action: "promote_idea",
		Params: map[string]interface{}{"target": target},
	}
}

func parseInlineIdeaPromotionApplyIntent(text string) *SystemAction {
	normalized := normalizeItemCommandText(text)
	if normalized == "" {
		return nil
	}
	target := ""
	switch {
	case strings.HasPrefix(normalized, "create this idea task"):
		target = ideaPromotionTargetTask
	case strings.HasPrefix(normalized, "create these idea items"), strings.HasPrefix(normalized, "create selected idea items"), strings.HasPrefix(normalized, "create idea items"):
		target = ideaPromotionTargetItems
	case strings.HasPrefix(normalized, "create this idea github issue"):
		target = ideaPromotionTargetGitHub
	}
	if target == "" {
		return nil
	}
	params := map[string]interface{}{
		"target":      target,
		"disposition": ideaPromotionDispositionFromText(normalized),
	}
	if selection := parseIdeaPromotionSelection(normalized); len(selection) > 0 {
		values := make([]interface{}, 0, len(selection))
		for _, index := range selection {
			values = append(values, index)
		}
		params["selected"] = values
	}
	return &SystemAction{
		Action: "apply_idea_promotion",
		Params: params,
	}
}

func ideaPromotionDispositionFromText(normalized string) string {
	switch {
	case strings.Contains(normalized, "mark this idea done"), strings.Contains(normalized, "mark it done"), strings.Contains(normalized, "mark idea done"):
		return ideaPromotionDispositionDone
	case strings.Contains(normalized, "move this idea to someday"), strings.Contains(normalized, "send this idea to someday"):
		return ideaPromotionDispositionSomeday
	case strings.Contains(normalized, "keep this idea"), strings.Contains(normalized, "keep it"):
		return ideaPromotionDispositionKeep
	default:
		return ideaPromotionDispositionKeep
	}
}

func parseIdeaPromotionSelection(normalized string) []int {
	match := ideaPromotionSelectionPattern.FindStringSubmatch(normalized)
	if len(match) != 2 {
		return nil
	}
	parts := strings.Split(match[1], ",")
	out := make([]int, 0, len(parts))
	seen := map[int]struct{}{}
	for _, part := range parts {
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Ints(out)
	return out
}

func ideaPromotionSourceRef(ideaItemID int64, index int) string {
	return fmt.Sprintf("idea-%d:%d", ideaItemID, index)
}

func splitIdeaPromotionSegments(raw string) []string {
	text := strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for _, rawLine := range lines {
		line := strings.TrimSpace(itemTitlePrefixPattern.ReplaceAllString(rawLine, ""))
		if line == "" {
			continue
		}
		start := 0
		for i, r := range line {
			switch r {
			case '.', ';':
				segment := strings.TrimSpace(line[start:i])
				if segment != "" {
					out = append(out, segment)
				}
				start = i + 1
			}
		}
		if tail := strings.TrimSpace(line[start:]); tail != "" {
			out = append(out, tail)
		}
	}
	return out
}

func looksLikeIdeaPromotionAction(segment string) bool {
	words := strings.Fields(strings.ToLower(strings.TrimSpace(segment)))
	if len(words) == 0 {
		return false
	}
	_, ok := ideaPromotionActionVerbSet[words[0]]
	return ok
}

func normalizeIdeaPromotionTitle(raw string) string {
	title := strings.TrimSpace(itemTitlePrefixPattern.ReplaceAllString(raw, ""))
	title = strings.Trim(title, " \t\r\n-:;,.!?")
	if strings.HasPrefix(strings.ToLower(title), "to ") {
		title = strings.TrimSpace(title[3:])
	}
	title = strings.Join(strings.Fields(title), " ")
	if title == "" {
		return ""
	}
	runes := []rune(title)
	first := strings.ToUpper(string(runes[0]))
	if len(runes) == 1 {
		return first
	}
	return first + string(runes[1:])
}

func buildIdeaPromotionCandidates(meta ideaNoteMeta) []ideaPromotionCandidate {
	lines := append([]string{}, meta.Notes...)
	for _, refinement := range meta.Refinements {
		lines = append(lines, refinement.Body)
	}
	candidates := make([]ideaPromotionCandidate, 0, len(lines))
	seen := map[string]struct{}{}
	for _, line := range lines {
		for _, segment := range splitIdeaPromotionSegments(line) {
			if !looksLikeIdeaPromotionAction(segment) {
				continue
			}
			title := normalizeIdeaPromotionTitle(segment)
			if title == "" {
				continue
			}
			key := strings.ToLower(title)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			candidates = append(candidates, ideaPromotionCandidate{
				Index:   len(candidates) + 1,
				Title:   title,
				Details: strings.TrimSpace(segment),
			})
		}
	}
	if len(candidates) > 0 {
		return candidates
	}
	title := normalizeIdeaPromotionTitle(meta.Title)
	if title == "" {
		title = "Explore this idea"
	}
	if !looksLikeIdeaPromotionAction(title) {
		title = "Prototype " + title
	}
	return []ideaPromotionCandidate{{
		Index:   1,
		Title:   title,
		Details: strings.TrimSpace(meta.Transcript),
	}}
}

func buildIdeaPromotionIssueDraft(meta ideaNoteMeta) *ideaPromotionIssueDraft {
	title := strings.TrimSpace(meta.Title)
	if title == "" {
		title = "Idea follow-up"
	}
	lines := []string{
		"## Idea",
	}
	if meta.Transcript != "" {
		lines = append(lines, "", meta.Transcript)
	}
	if len(meta.Notes) > 0 {
		lines = append(lines, "", "## Notes", "")
		for _, note := range meta.Notes {
			lines = append(lines, "- "+note)
		}
	}
	for _, refinement := range meta.Refinements {
		body := strings.TrimSpace(refinement.Body)
		if body == "" {
			continue
		}
		heading := strings.TrimSpace(refinement.Heading)
		if heading == "" {
			heading = ideaRefinementHeading(refinement.Kind)
		}
		lines = append(lines, "", "## "+heading, "", body)
	}
	body := strings.TrimSpace(strings.Join(lines, "\n"))
	if body == "" {
		body = title
	}
	return &ideaPromotionIssueDraft{
		Title: title,
		Body:  body,
	}
}

func findItemByArtifactAndSource(items []store.Item, artifactID int64, source string) *store.Item {
	for _, item := range items {
		if item.ArtifactID == nil || *item.ArtifactID != artifactID {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(stringFromPointer(item.Source)), source) {
			continue
		}
		candidate := item
		return &candidate
	}
	return nil
}

func (a *App) findIdeaNoteItem(artifactID int64) (*store.Item, error) {
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
		if item := findItemByArtifactAndSource(items, artifactID, "idea"); item != nil {
			return item, nil
		}
	}
	return nil, nil
}

func (a *App) resolveActiveIdeaNoteContext(session store.ChatSession) (*activeIdeaNoteContext, error) {
	artifact, err := a.resolveActiveIdeaNoteArtifact(session.ProjectKey)
	if err != nil {
		return nil, err
	}
	item, err := a.findIdeaNoteItem(artifact.ID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, errors.New("idea note is not linked to an item")
	}
	return &activeIdeaNoteContext{
		Item:     *item,
		Artifact: *artifact,
		Meta:     parseIdeaNoteMeta(artifact.MetaJSON, ideaNoteString(artifact.Title)),
	}, nil
}

func ideaPromotionSelectionFromParams(params map[string]interface{}) []int {
	raw, ok := params["selected"]
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []int:
		return typed
	case []interface{}:
		out := make([]int, 0, len(typed))
		seen := map[int]struct{}{}
		for _, entry := range typed {
			value, err := strconv.Atoi(strings.TrimSpace(fmt.Sprint(entry)))
			if err != nil || value <= 0 {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			out = append(out, value)
		}
		sort.Ints(out)
		return out
	default:
		value, err := strconv.Atoi(strings.TrimSpace(fmt.Sprint(typed)))
		if err != nil || value <= 0 {
			return nil
		}
		return []int{value}
	}
}

func selectIdeaPromotionCandidates(candidates []ideaPromotionCandidate, selected []int) ([]ideaPromotionCandidate, error) {
	if len(candidates) == 0 {
		return nil, errors.New("idea promotion preview has no candidates")
	}
	if len(selected) == 0 {
		return candidates, nil
	}
	byIndex := make(map[int]ideaPromotionCandidate, len(candidates))
	for _, candidate := range candidates {
		byIndex[candidate.Index] = candidate
	}
	out := make([]ideaPromotionCandidate, 0, len(selected))
	for _, index := range selected {
		candidate, ok := byIndex[index]
		if !ok {
			return nil, fmt.Errorf("idea proposal %d is not available", index)
		}
		out = append(out, candidate)
	}
	return out, nil
}

func (a *App) persistIdeaNoteMeta(session store.ChatSession, artifact store.Artifact, meta ideaNoteMeta) error {
	metaJSON, err := encodeIdeaNoteMeta(meta)
	if err != nil {
		return err
	}
	if err := a.store.UpdateArtifact(artifact.ID, store.ArtifactUpdate{MetaJSON: metaJSON}); err != nil {
		return err
	}
	return a.renderIdeaNoteOnCanvas(session.ProjectKey, meta.Title, meta)
}

func (a *App) previewIdeaPromotion(session store.ChatSession, action *SystemAction) (string, map[string]interface{}, error) {
	ctx, err := a.resolveActiveIdeaNoteContext(session)
	if err != nil {
		return "", nil, err
	}
	if len(ctx.Meta.Promotions) > 0 {
		return "", nil, errors.New("idea has already been promoted")
	}
	target := normalizeIdeaPromotionTarget(systemActionStringParam(action.Params, "target"))
	if target == "" {
		return "", nil, errors.New("idea promotion target is required")
	}
	preview := &ideaPromotionPreview{
		Target:    target,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	switch target {
	case ideaPromotionTargetTask:
		candidates := buildIdeaPromotionCandidates(ctx.Meta)
		preview.Candidates = []ideaPromotionCandidate{candidates[0]}
	case ideaPromotionTargetItems:
		preview.Candidates = buildIdeaPromotionCandidates(ctx.Meta)
	case ideaPromotionTargetGitHub:
		preview.Issue = buildIdeaPromotionIssueDraft(ctx.Meta)
	default:
		return "", nil, errors.New("unsupported idea promotion target")
	}
	ctx.Meta.PromotionPreview = preview
	if err := a.persistIdeaNoteMeta(session, ctx.Artifact, ctx.Meta); err != nil {
		return "", nil, err
	}
	message := ""
	switch target {
	case ideaPromotionTargetTask:
		message = `Drafted a task promotion on canvas. Say "create this idea task" to confirm.`
	case ideaPromotionTargetItems:
		message = `Drafted idea item proposals on canvas. Say "create these idea items" or "create selected idea items 1,2" to confirm.`
	case ideaPromotionTargetGitHub:
		message = `Drafted a GitHub issue on canvas. Say "create this idea GitHub issue" to confirm.`
	}
	payload := map[string]interface{}{
		"type":        "idea_promotion_preview",
		"target":      target,
		"artifact_id": ctx.Artifact.ID,
	}
	if len(preview.Candidates) > 0 {
		payload["candidate_count"] = len(preview.Candidates)
	}
	if preview.Issue != nil {
		payload["issue_title"] = preview.Issue.Title
	}
	return message, payload, nil
}

func (a *App) applyIdeaPromotionDisposition(itemID int64, disposition string) (string, error) {
	switch normalizeIdeaPromotionDisposition(disposition) {
	case ideaPromotionDispositionKeep:
		item, err := a.store.GetItem(itemID)
		if err != nil {
			return "", err
		}
		return item.State, nil
	case ideaPromotionDispositionDone:
		if err := a.store.TriageItemDone(itemID); err != nil {
			return "", err
		}
	case ideaPromotionDispositionSomeday:
		if err := a.store.TriageItemSomeday(itemID); err != nil {
			return "", err
		}
	default:
		return "", errors.New("invalid idea disposition")
	}
	item, err := a.store.GetItem(itemID)
	if err != nil {
		return "", err
	}
	return item.State, nil
}

func (a *App) applyIdeaPromotion(session store.ChatSession, action *SystemAction) (string, map[string]interface{}, error) {
	ctx, err := a.resolveActiveIdeaNoteContext(session)
	if err != nil {
		return "", nil, err
	}
	preview := normalizeIdeaPromotionPreview(ctx.Meta.PromotionPreview)
	if preview == nil {
		return "", nil, errors.New("review the promotion on canvas first")
	}
	target := normalizeIdeaPromotionTarget(systemActionStringParam(action.Params, "target"))
	if target == "" {
		target = preview.Target
	}
	if target != preview.Target {
		return "", nil, fmt.Errorf("active promotion is for %s", preview.Target)
	}

	disposition := normalizeIdeaPromotionDisposition(systemActionStringParam(action.Params, "disposition"))
	if disposition == "" {
		disposition = ideaPromotionDispositionKeep
	}

	payload := map[string]interface{}{
		"type":         "idea_promotion_applied",
		"target":       target,
		"idea_item_id": ctx.Item.ID,
	}
	record := ideaPromotionRecord{
		Target:    target,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	switch target {
	case ideaPromotionTargetTask, ideaPromotionTargetItems:
		selected, err := selectIdeaPromotionCandidates(preview.Candidates, ideaPromotionSelectionFromParams(action.Params))
		if err != nil {
			return "", nil, err
		}
		itemIDs := make([]int64, 0, len(selected))
		refs := make([]string, 0, len(selected))
		source := "idea"
		for _, candidate := range selected {
			sourceRef := ideaPromotionSourceRef(ctx.Item.ID, candidate.Index)
			item, err := a.store.CreateItem(candidate.Title, store.ItemOptions{
				WorkspaceID: ctx.Item.WorkspaceID,
				ArtifactID:  &ctx.Artifact.ID,
				Source:      &source,
				SourceRef:   &sourceRef,
			})
			if err != nil {
				return "", nil, err
			}
			itemIDs = append(itemIDs, item.ID)
			refs = append(refs, sourceRef)
		}
		payload["created_item_ids"] = itemIDs
		record.Count = len(itemIDs)
		record.Refs = refs
	case ideaPromotionTargetGitHub:
		if ctx.Item.WorkspaceID == nil {
			return "", nil, errors.New("idea is not linked to a workspace")
		}
		workspace, err := a.store.GetWorkspace(*ctx.Item.WorkspaceID)
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
		issue, err := a.createGitHubIssueInWorkspace(
			workspace.DirPath,
			preview.Issue.Title,
			preview.Issue.Body,
			nil,
			nil,
		)
		if err != nil {
			return "", nil, err
		}
		payload["issue_no"] = issue.Number
		payload["issue_url"] = strings.TrimSpace(issue.URL)
		record.Count = 1
		record.Refs = []string{strings.TrimSpace(issue.URL)}
	default:
		return "", nil, errors.New("unsupported idea promotion target")
	}

	ideaState, err := a.applyIdeaPromotionDisposition(ctx.Item.ID, disposition)
	if err != nil {
		return "", nil, err
	}
	payload["idea_state"] = ideaState
	ctx.Meta.PromotionPreview = nil
	ctx.Meta.Promotions = append(ctx.Meta.Promotions, record)
	if err := a.persistIdeaNoteMeta(session, ctx.Artifact, ctx.Meta); err != nil {
		return "", nil, err
	}

	switch target {
	case ideaPromotionTargetTask:
		ids, _ := payload["created_item_ids"].([]int64)
		if len(ids) == 1 {
			return "Created 1 task from the idea.", payload, nil
		}
		return fmt.Sprintf("Created %d tasks from the idea.", len(ids)), payload, nil
	case ideaPromotionTargetItems:
		ids, _ := payload["created_item_ids"].([]int64)
		return fmt.Sprintf("Created %d items from the idea.", len(ids)), payload, nil
	case ideaPromotionTargetGitHub:
		return fmt.Sprintf("Created GitHub issue #%v from the idea: %s", payload["issue_no"], payload["issue_url"]), payload, nil
	default:
		return "Applied the idea promotion.", payload, nil
	}
}
