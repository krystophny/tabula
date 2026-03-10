package web

import (
	"context"
	"testing"
	"time"

	"github.com/krystophny/tabura/internal/appserver"
)

func TestApprovalPolicyForSession(t *testing.T) {
	tests := []struct {
		name string
		mode string
		yolo bool
		want string
	}{
		{name: "yolo", mode: "chat", yolo: true, want: appserver.ApprovalPolicyNever},
		{name: "plan", mode: "plan", yolo: false, want: appserver.ApprovalPolicyUnlessTrusted},
		{name: "review", mode: "review", yolo: false, want: appserver.ApprovalPolicyUnlessTrusted},
		{name: "default", mode: "chat", yolo: false, want: appserver.ApprovalPolicyOnRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := approvalPolicyForSession(tc.mode, tc.yolo); got != tc.want {
				t.Fatalf("approvalPolicyForSession(%q, %t) = %q, want %q", tc.mode, tc.yolo, got, tc.want)
			}
		})
	}
}

func TestRequestAppServerApprovalWaitsForDecision(t *testing.T) {
	app := newAuthedTestApp(t)
	project, err := app.ensureDefaultProjectRecord()
	if err != nil {
		t.Fatalf("ensure default project: %v", err)
	}
	session, err := app.store.GetOrCreateChatSession(project.ProjectKey)
	if err != nil {
		t.Fatalf("GetOrCreateChatSession: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resultCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		decision, reqErr := app.requestAppServerApproval(ctx, session.ID, appserver.StreamEvent{
			Approval: &appserver.ApprovalRequest{
				Kind:   "command_execution",
				Reason: "run git status",
				ItemID: "item-42",
			},
		})
		if reqErr != nil {
			errCh <- reqErr
			return
		}
		resultCh <- decision
	}()

	var requestID string
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		app.approvalMu.Lock()
		for id := range app.pendingApprovals[session.ID] {
			requestID = id
		}
		app.approvalMu.Unlock()
		if requestID != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if requestID == "" {
		t.Fatal("expected pending approval request")
	}
	if ok := app.resolvePendingAppServerApproval(session.ID, requestID, "decline"); !ok {
		t.Fatal("expected approval resolution to succeed")
	}

	select {
	case err := <-errCh:
		t.Fatalf("requestAppServerApproval error: %v", err)
	case decision := <-resultCh:
		if decision != "decline" {
			t.Fatalf("decision = %q, want %q", decision, "decline")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for approval resolution")
	}
}
