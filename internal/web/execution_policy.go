package web

import "strings"

const (
	executionPolicyDefault    = "default"
	executionPolicyReviewed   = "reviewed"
	executionPolicyAutonomous = "autonomous"
)

type executionPolicy struct {
	Name           string
	ApprovalPolicy string
}

func executionPolicyForSession(mode string, autonomous bool) executionPolicy {
	if autonomous {
		return executionPolicy{
			Name:           executionPolicyAutonomous,
			ApprovalPolicy: "never",
		}
	}
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "plan", "review":
		return executionPolicy{
			Name:           executionPolicyReviewed,
			ApprovalPolicy: "unlessTrusted",
		}
	default:
		return executionPolicy{
			Name:           executionPolicyDefault,
			ApprovalPolicy: "on-request",
		}
	}
}
