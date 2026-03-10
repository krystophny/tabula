package web

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/krystophny/tabura/internal/appserver"
	"github.com/krystophny/tabura/internal/store"
)

type pendingAppServerApproval struct {
	LocalID    string
	Request    *appserver.ApprovalRequest
	DecisionCh chan string
	CreatedAt  time.Time
}

func approvalPolicyForSession(mode string, yoloMode bool) string {
	policy := executionPolicyForSession(mode, yoloMode)
	return appserver.NormalizeApprovalPolicy(policy.ApprovalPolicy)
}

func mergeApprovalPolicyThreadParams(threadParams map[string]interface{}, mode string, yoloMode bool) map[string]interface{} {
	merged := map[string]interface{}{}
	for key, value := range threadParams {
		if strings.TrimSpace(key) == "" {
			continue
		}
		merged[key] = value
	}
	merged["approvalPolicy"] = approvalPolicyForSession(mode, yoloMode)
	return merged
}

func (a *App) appServerProfileForChatSession(session store.ChatSession, profile appServerModelProfile) appServerModelProfile {
	profile.ThreadParams = mergeApprovalPolicyThreadParams(profile.ThreadParams, session.Mode, a.yoloModeEnabled())
	return profile
}

func formatApprovalRequestDescription(req *appserver.ApprovalRequest) string {
	if req == nil {
		return "Approval required"
	}
	reason := strings.TrimSpace(req.Reason)
	switch req.Kind {
	case "file_change":
		if reason != "" {
			return "Allow file changes: " + reason
		}
		if strings.TrimSpace(req.GrantRoot) != "" {
			return "Allow file changes under " + req.GrantRoot
		}
		return "Allow file changes"
	case "command_execution":
		if reason != "" {
			return "Allow command execution: " + reason
		}
		return "Allow command execution"
	default:
		if reason != "" {
			return "Approval required: " + reason
		}
		return "Approval required"
	}
}

func (a *App) storePendingAppServerApproval(sessionID string, pending *pendingAppServerApproval) {
	if a == nil || strings.TrimSpace(sessionID) == "" || pending == nil || strings.TrimSpace(pending.LocalID) == "" {
		return
	}
	a.approvalMu.Lock()
	defer a.approvalMu.Unlock()
	sessionApprovals := a.pendingApprovals[sessionID]
	if sessionApprovals == nil {
		sessionApprovals = map[string]*pendingAppServerApproval{}
		a.pendingApprovals[sessionID] = sessionApprovals
	}
	sessionApprovals[pending.LocalID] = pending
}

func (a *App) removePendingAppServerApproval(sessionID, requestID string) {
	if a == nil || strings.TrimSpace(sessionID) == "" || strings.TrimSpace(requestID) == "" {
		return
	}
	a.approvalMu.Lock()
	defer a.approvalMu.Unlock()
	sessionApprovals := a.pendingApprovals[sessionID]
	if sessionApprovals == nil {
		return
	}
	delete(sessionApprovals, requestID)
	if len(sessionApprovals) == 0 {
		delete(a.pendingApprovals, sessionID)
	}
}

func (a *App) resolvePendingAppServerApproval(sessionID, requestID, decision string) bool {
	if a == nil || strings.TrimSpace(sessionID) == "" || strings.TrimSpace(requestID) == "" {
		return false
	}
	normalizedDecision := strings.TrimSpace(decision)
	if normalizedDecision == "" {
		normalizedDecision = "cancel"
	}
	a.approvalMu.Lock()
	sessionApprovals := a.pendingApprovals[sessionID]
	pending := sessionApprovals[requestID]
	if pending != nil {
		delete(sessionApprovals, requestID)
		if len(sessionApprovals) == 0 {
			delete(a.pendingApprovals, sessionID)
		}
	}
	a.approvalMu.Unlock()
	if pending == nil {
		return false
	}
	select {
	case pending.DecisionCh <- normalizedDecision:
	default:
	}
	return true
}

func (a *App) requestAppServerApproval(ctx context.Context, sessionID string, ev appserver.StreamEvent) (string, error) {
	if a == nil || ev.Approval == nil {
		return "", fmt.Errorf("approval request is unavailable")
	}
	requestID := randomToken()
	pending := &pendingAppServerApproval{
		LocalID:    requestID,
		Request:    ev.Approval,
		DecisionCh: make(chan string, 1),
		CreatedAt:  time.Now().UTC(),
	}
	a.storePendingAppServerApproval(sessionID, pending)
	a.broadcastChatEvent(sessionID, map[string]interface{}{
		"type":         "approval_request",
		"request_id":   requestID,
		"action":       ev.Approval.Kind,
		"description":  formatApprovalRequestDescription(ev.Approval),
		"reason":       strings.TrimSpace(ev.Approval.Reason),
		"grant_root":   strings.TrimSpace(ev.Approval.GrantRoot),
		"item_id":      strings.TrimSpace(ev.Approval.ItemID),
		"request_kind": ev.Approval.Kind,
	})
	select {
	case <-ctx.Done():
		a.removePendingAppServerApproval(sessionID, requestID)
		return "", ctx.Err()
	case decision := <-pending.DecisionCh:
		a.broadcastChatEvent(sessionID, map[string]interface{}{
			"type":       "approval_resolved",
			"request_id": requestID,
			"decision":   decision,
		})
		return decision, nil
	}
}
