package web

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/krystophny/tabura/internal/store"
)

const (
	reviewDispatchTargetAuto = "auto"

	reviewPolicyAlwaysAgent    = "always_agent"
	reviewPolicyAgentThenHuman = "agent_then_human"
	reviewPolicyAlwaysHuman    = "always_human"
)

var reviewDispatchPRNumberPattern = regexp.MustCompile(`#PR-(\d+)$`)

type itemReviewDispatchRequest struct {
	Target   string `json:"target"`
	Reviewer string `json:"reviewer"`
	Email    string `json:"email"`
}

type reviewDispatchEmailMessage struct {
	To      string
	Subject string
	Body    string
}

type reviewDispatchEmailSender func(context.Context, reviewDispatchEmailMessage) error

type reviewDispatchHandle struct {
	seq    uint64
	target string
	cancel context.CancelFunc
}

type reviewDispatchTracker struct {
	mu      sync.Mutex
	handles map[int64]*reviewDispatchHandle
}

func newReviewDispatchTracker() *reviewDispatchTracker {
	return &reviewDispatchTracker{handles: make(map[int64]*reviewDispatchHandle)}
}

func (t *reviewDispatchTracker) replace(itemID int64, target string, cancel context.CancelFunc) uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	nextSeq := uint64(1)
	if existing := t.handles[itemID]; existing != nil {
		nextSeq = existing.seq + 1
		if existing.cancel != nil {
			existing.cancel()
		}
	}
	t.handles[itemID] = &reviewDispatchHandle{
		seq:    nextSeq,
		target: strings.TrimSpace(target),
		cancel: cancel,
	}
	return nextSeq
}

func (t *reviewDispatchTracker) cancel(itemID int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	existing := t.handles[itemID]
	if existing == nil {
		return
	}
	if existing.cancel != nil {
		existing.cancel()
	}
	delete(t.handles, itemID)
}

func (t *reviewDispatchTracker) finish(itemID int64, seq uint64, target string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	existing := t.handles[itemID]
	if existing == nil || existing.seq != seq || existing.target != strings.TrimSpace(target) {
		return false
	}
	delete(t.handles, itemID)
	return true
}

func normalizeReviewDispatchPolicy(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case reviewPolicyAlwaysAgent:
		return reviewPolicyAlwaysAgent
	case reviewPolicyAgentThenHuman:
		return reviewPolicyAgentThenHuman
	case reviewPolicyAlwaysHuman:
		return reviewPolicyAlwaysHuman
	default:
		return ""
	}
}

func normalizeReviewDispatchTarget(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case store.ItemReviewTargetAgent:
		return store.ItemReviewTargetAgent
	case store.ItemReviewTargetGitHub:
		return store.ItemReviewTargetGitHub
	case store.ItemReviewTargetEmail:
		return store.ItemReviewTargetEmail
	default:
		return ""
	}
}

func reviewDispatchStringPointer(value string) *string {
	clean := strings.TrimSpace(value)
	if clean == "" {
		return nil
	}
	return &clean
}

func reviewDispatchPolicyLabel(policy string) string {
	switch normalizeReviewDispatchPolicy(policy) {
	case reviewPolicyAlwaysAgent:
		return "always agent"
	case reviewPolicyAgentThenHuman:
		return "agent then human"
	case reviewPolicyAlwaysHuman:
		return "always human"
	default:
		return ""
	}
}

func resolveReviewDispatchTarget(target, policy, reviewer, email string) (string, error) {
	cleanTarget := normalizeReviewDispatchTarget(target)
	if cleanTarget != "" {
		return cleanTarget, nil
	}
	trimmedTarget := strings.ToLower(strings.TrimSpace(target))
	if trimmedTarget != "" && trimmedTarget != reviewDispatchTargetAuto {
		return "", errors.New("target must be agent, github, email, or auto")
	}
	switch normalizeReviewDispatchPolicy(policy) {
	case reviewPolicyAlwaysAgent, reviewPolicyAgentThenHuman:
		return store.ItemReviewTargetAgent, nil
	case reviewPolicyAlwaysHuman:
		switch {
		case strings.TrimSpace(reviewer) != "":
			return store.ItemReviewTargetGitHub, nil
		case strings.TrimSpace(email) != "":
			return store.ItemReviewTargetEmail, nil
		default:
			return "", errors.New("human review policy requires reviewer or email")
		}
	default:
		return "", errors.New("target is required")
	}
}

func reviewDispatchPRNumber(item store.Item) int {
	match := reviewDispatchPRNumberPattern.FindStringSubmatch(strings.TrimSpace(stringFromPointer(item.SourceRef)))
	if len(match) != 2 {
		return 0
	}
	value, err := strconv.Atoi(match[1])
	if err != nil || value <= 0 {
		return 0
	}
	return value
}

func isReviewDispatchItem(item store.Item, artifact *store.Artifact) bool {
	if reviewDispatchPRNumber(item) > 0 {
		return true
	}
	if artifact == nil {
		return false
	}
	return artifact.Kind == store.ArtifactKindGitHubPR
}

type reviewDispatchContext struct {
	item      store.Item
	itemBrief store.ItemSummary
	workspace store.Workspace
	artifact  *store.Artifact
	cfg       batchWorkConfig
	policy    string
}

func (a *App) loadReviewDispatchContext(itemID int64) (reviewDispatchContext, error) {
	item, err := a.store.GetItem(itemID)
	if err != nil {
		return reviewDispatchContext{}, err
	}
	if item.State == store.ItemStateDone {
		return reviewDispatchContext{}, fmt.Errorf("cannot dispatch review for item in %s state", item.State)
	}
	if item.WorkspaceID == nil || *item.WorkspaceID <= 0 {
		return reviewDispatchContext{}, errors.New("review dispatch requires a workspace-linked item")
	}
	workspace, err := a.store.GetWorkspace(*item.WorkspaceID)
	if err != nil {
		return reviewDispatchContext{}, err
	}
	var artifact *store.Artifact
	if item.ArtifactID != nil && *item.ArtifactID > 0 {
		loaded, err := a.store.GetArtifact(*item.ArtifactID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return reviewDispatchContext{}, err
		}
		if err == nil {
			artifact = &loaded
		}
	}
	if !isReviewDispatchItem(item, artifact) {
		return reviewDispatchContext{}, errors.New("review dispatch requires a GitHub PR item")
	}
	itemBrief, err := a.store.GetItemSummary(itemID)
	if err != nil {
		return reviewDispatchContext{}, err
	}
	cfg, _, err := a.loadWorkspaceBatchConfig(workspace.ID)
	if err != nil {
		return reviewDispatchContext{}, err
	}
	return reviewDispatchContext{
		item:      item,
		itemBrief: itemBrief,
		workspace: workspace,
		artifact:  artifact,
		cfg:       cfg,
		policy:    cfg.ReviewPolicy,
	}, nil
}

func ensureValidDispatchEmail(address string) (string, error) {
	clean := strings.TrimSpace(address)
	if clean == "" {
		return "", errors.New("email is required")
	}
	parsed, err := mail.ParseAddress(clean)
	if err != nil {
		return "", errors.New("email must be a valid address")
	}
	return strings.ToLower(strings.TrimSpace(parsed.Address)), nil
}

func ensureValidGitHubReviewer(raw string) (string, error) {
	clean := strings.TrimSpace(strings.TrimPrefix(raw, "@"))
	if clean == "" {
		return "", errors.New("reviewer is required")
	}
	return clean, nil
}

func reviewDispatchArtifactLink(ctx reviewDispatchContext) string {
	switch {
	case ctx.artifact != nil && ctx.artifact.RefURL != nil && strings.TrimSpace(*ctx.artifact.RefURL) != "":
		return strings.TrimSpace(*ctx.artifact.RefURL)
	case strings.TrimSpace(stringFromPointer(ctx.item.SourceRef)) != "":
		return strings.TrimSpace(stringFromPointer(ctx.item.SourceRef))
	}
	return ""
}

func reviewDispatchEmailBody(ctx reviewDispatchContext) string {
	var body strings.Builder
	fmt.Fprintf(&body, "Review requested for: %s\n", ctx.item.Title)
	fmt.Fprintf(&body, "Workspace: %s\n", ctx.workspace.Name)
	if link := reviewDispatchArtifactLink(ctx); link != "" {
		fmt.Fprintf(&body, "Artifact: %s\n", link)
	}
	if ref := strings.TrimSpace(stringFromPointer(ctx.item.SourceRef)); ref != "" {
		fmt.Fprintf(&body, "Source: %s\n", ref)
	}
	return strings.TrimSpace(body.String()) + "\n"
}

func sendReviewDispatchEmail(ctx context.Context, message reviewDispatchEmailMessage) error {
	sendmailPath, err := exec.LookPath("sendmail")
	if err != nil {
		return errors.New("sendmail is not available")
	}
	var payload strings.Builder
	fmt.Fprintf(&payload, "To: %s\n", strings.TrimSpace(message.To))
	fmt.Fprintf(&payload, "Subject: %s\n", strings.TrimSpace(message.Subject))
	payload.WriteString("\n")
	payload.WriteString(strings.TrimSpace(message.Body))
	payload.WriteString("\n")
	cmd := exec.CommandContext(ctx, sendmailPath, "-t", "-i")
	cmd.Stdin = strings.NewReader(payload.String())
	output, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			detail = err.Error()
		}
		return fmt.Errorf("sendmail failed: %s", detail)
	}
	return nil
}

func (a *App) dispatchGitHubReview(ctx context.Context, reviewCtx reviewDispatchContext, reviewer string) error {
	prNumber := reviewDispatchPRNumber(reviewCtx.item)
	if prNumber <= 0 {
		return errors.New("review dispatch requires a GitHub PR source_ref")
	}
	runner := a.ghCommandRunner
	if runner == nil {
		runner = runGitHubCLI
	}
	_, err := runner(ctx, reviewCtx.workspace.DirPath, "pr", "edit", fmt.Sprintf("%d", prNumber), "--add-reviewer", reviewer)
	return err
}

func (a *App) dispatchEmailReview(ctx context.Context, reviewCtx reviewDispatchContext, email string) error {
	sender := a.reviewEmailSender
	if sender == nil {
		sender = sendReviewDispatchEmail
	}
	return sender(ctx, reviewDispatchEmailMessage{
		To:      email,
		Subject: fmt.Sprintf("Review requested: %s", reviewCtx.item.Title),
		Body:    reviewDispatchEmailBody(reviewCtx),
	})
}

func (a *App) markItemWaitingForReview(item store.Item) error {
	if item.State == store.ItemStateWaiting {
		return nil
	}
	return a.store.UpdateItemState(item.ID, store.ItemStateWaiting)
}

func (a *App) launchAgentReviewDispatch(reviewCtx reviewDispatchContext) {
	dispatchCtx, cancel := context.WithCancel(context.Background())
	seq := a.reviewDispatches.replace(reviewCtx.item.ID, store.ItemReviewTargetAgent, cancel)
	a.workerWG.Add(1)
	go func() {
		defer a.workerWG.Done()
		defer a.reviewDispatches.finish(reviewCtx.item.ID, seq, store.ItemReviewTargetAgent)
		if reviewCtx.item.State != store.ItemStateWaiting {
			if err := a.markItemWaitingForReview(reviewCtx.item); err != nil && !errors.Is(err, sql.ErrNoRows) {
				return
			}
		}
		var err error
		if a.workspaceWatchProcessor != nil {
			err = a.workspaceWatchProcessor(dispatchCtx, reviewCtx.workspace, reviewCtx.itemBrief)
		} else {
			err = a.processWorkspaceWatchItem(dispatchCtx, reviewCtx.workspace, reviewCtx.cfg, reviewCtx.itemBrief)
		}
		if errors.Is(dispatchCtx.Err(), context.Canceled) {
			return
		}
		if err != nil {
			_ = a.store.ReturnItemToInbox(reviewCtx.item.ID)
			return
		}
		_ = a.store.TriageItemDone(reviewCtx.item.ID)
	}()
}

func (a *App) dispatchReview(itemID int64, req itemReviewDispatchRequest) (store.Item, error) {
	reviewCtx, err := a.loadReviewDispatchContext(itemID)
	if err != nil {
		return store.Item{}, err
	}
	target, err := resolveReviewDispatchTarget(req.Target, reviewCtx.policy, req.Reviewer, req.Email)
	if err != nil {
		return store.Item{}, err
	}
	switch target {
	case store.ItemReviewTargetGitHub:
		reviewer, err := ensureValidGitHubReviewer(req.Reviewer)
		if err != nil {
			return store.Item{}, err
		}
		a.reviewDispatches.cancel(itemID)
		ctx, cancel := context.WithTimeout(context.Background(), githubPRCommandTimeout)
		defer cancel()
		if err := a.dispatchGitHubReview(ctx, reviewCtx, reviewer); err != nil {
			return store.Item{}, err
		}
		if err := a.markItemWaitingForReview(reviewCtx.item); err != nil {
			return store.Item{}, err
		}
		if err := a.store.UpdateItemReviewDispatch(itemID, reviewDispatchStringPointer(target), &reviewer); err != nil {
			return store.Item{}, err
		}
	case store.ItemReviewTargetEmail:
		email, err := ensureValidDispatchEmail(req.Email)
		if err != nil {
			return store.Item{}, err
		}
		a.reviewDispatches.cancel(itemID)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := a.dispatchEmailReview(ctx, reviewCtx, email); err != nil {
			return store.Item{}, err
		}
		if err := a.markItemWaitingForReview(reviewCtx.item); err != nil {
			return store.Item{}, err
		}
		if err := a.store.UpdateItemReviewDispatch(itemID, reviewDispatchStringPointer(target), &email); err != nil {
			return store.Item{}, err
		}
	case store.ItemReviewTargetAgent:
		if a.workspaceWatchProcessor == nil && a.appServerClient == nil {
			return store.Item{}, errors.New("agent review requires workspace watch processing")
		}
		if err := a.markItemWaitingForReview(reviewCtx.item); err != nil {
			return store.Item{}, err
		}
		reviewCtx.item.State = store.ItemStateWaiting
		if refreshed, err := a.store.GetItemSummary(itemID); err == nil {
			reviewCtx.itemBrief = refreshed
		}
		if err := a.store.UpdateItemReviewDispatch(itemID, reviewDispatchStringPointer(target), nil); err != nil {
			return store.Item{}, err
		}
		a.launchAgentReviewDispatch(reviewCtx)
	default:
		return store.Item{}, errors.New("unsupported review target")
	}
	return a.store.GetItem(itemID)
}

func (a *App) handleItemReviewDispatch(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	itemID, err := parseItemIDParam(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req itemReviewDispatchRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	item, err := a.dispatchReview(itemID, req)
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, sql.ErrNoRows):
			status = http.StatusNotFound
		case strings.Contains(strings.ToLower(err.Error()), "not available"):
			status = http.StatusBadGateway
		case strings.Contains(strings.ToLower(err.Error()), "gh pr edit"):
			status = http.StatusBadGateway
		}
		writeAPIError(w, status, err.Error())
		return
	}
	writeAPIData(w, http.StatusOK, map[string]any{
		"item": item,
	})
}
