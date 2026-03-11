package web

import (
	"context"
	"strings"
	"time"

	"github.com/krystophny/tabura/internal/store"
)

type deterministicFastPathContext struct {
	Now         time.Time
	CaptureMode string
	Cursor      *chatCursorContext
}

type deterministicFastPathMatch struct {
	Name           string
	Actions        []*SystemAction
	TitledItem     *titledItemIntent
	FailureMessage func(userText string, enforced []*SystemAction, err error) string
}

type deterministicFastPathParser struct {
	Name  string
	Parse func(text string, ctx deterministicFastPathContext) *deterministicFastPathMatch
}

var deterministicFastPaths = []deterministicFastPathParser{
	{
		Name: "source_sync",
		Parse: func(text string, _ deterministicFastPathContext) *deterministicFastPathMatch {
			action := parseInlineSourceSyncIntent(text)
			if action == nil {
				return nil
			}
			return fastPathSingleAction("source_sync", action, fixedFastPathFailure(sourceSyncActionFailurePrefix(action.Action)))
		},
	},
	{
		Name: "calendar",
		Parse: func(text string, ctx deterministicFastPathContext) *deterministicFastPathMatch {
			action := parseInlineCalendarIntent(text, ctx.Now)
			if action == nil {
				return nil
			}
			return fastPathSingleAction("calendar", action, fixedFastPathFailure(calendarActionFailurePrefix(action.Action)))
		},
	},
	{
		Name: "briefing",
		Parse: func(text string, ctx deterministicFastPathContext) *deterministicFastPathMatch {
			action := parseInlineBriefingIntent(text, ctx.Now)
			if action == nil {
				return nil
			}
			return fastPathSingleAction("briefing", action, fixedFastPathFailure(briefingActionFailurePrefix(action.Action)))
		},
	},
	{
		Name: "todoist",
		Parse: func(text string, _ deterministicFastPathContext) *deterministicFastPathMatch {
			action := parseInlineTodoistIntent(text)
			if action == nil {
				return nil
			}
			return fastPathSingleAction("todoist", action, fixedFastPathFailure(todoistActionFailurePrefix(action.Action)))
		},
	},
	{
		Name: "evernote",
		Parse: func(text string, _ deterministicFastPathContext) *deterministicFastPathMatch {
			action := parseInlineEvernoteIntent(text)
			if action == nil {
				return nil
			}
			return fastPathSingleAction("evernote", action, fixedFastPathFailure(evernoteActionFailurePrefix(action.Action)))
		},
	},
	{
		Name: "bear",
		Parse: func(text string, _ deterministicFastPathContext) *deterministicFastPathMatch {
			action := parseInlineBearIntent(text)
			if action == nil {
				return nil
			}
			return fastPathSingleAction("bear", action, fixedFastPathFailure(bearActionFailurePrefix(action.Action)))
		},
	},
	{
		Name: "zotero",
		Parse: func(text string, _ deterministicFastPathContext) *deterministicFastPathMatch {
			action := parseInlineZoteroIntent(text)
			if action == nil {
				return nil
			}
			return fastPathSingleAction("zotero", action, fixedFastPathFailure(zoteroActionFailurePrefix(action.Action)))
		},
	},
	{
		Name: "cursor",
		Parse: func(text string, ctx deterministicFastPathContext) *deterministicFastPathMatch {
			action := parseInlineCursorIntent(text, ctx.Cursor)
			if action == nil {
				return nil
			}
			return fastPathSingleAction("cursor", action, fixedFastPathFailure("I couldn't resolve the pointed selection: "))
		},
	},
	{
		Name: "titled_item",
		Parse: func(text string, _ deterministicFastPathContext) *deterministicFastPathMatch {
			intent := parseInlineTitledItemIntent(text)
			if intent == nil {
				return nil
			}
			return &deterministicFastPathMatch{
				Name:       "titled_item",
				TitledItem: intent,
				FailureMessage: func(_ string, _ []*SystemAction, err error) string {
					return "I couldn't resolve the named item: " + err.Error()
				},
			}
		},
	},
	{
		Name: "item",
		Parse: func(text string, ctx deterministicFastPathContext) *deterministicFastPathMatch {
			action := parseInlineItemIntentWithCaptureMode(text, ctx.Now, ctx.CaptureMode)
			if action == nil {
				return nil
			}
			return fastPathSingleAction("item", action, itemFastPathFailure)
		},
	},
	{
		Name: "github_issue",
		Parse: func(text string, _ deterministicFastPathContext) *deterministicFastPathMatch {
			actions := parseInlineGitHubIssueActions(text)
			if len(actions) == 0 {
				return nil
			}
			return fastPathActionPlan("github_issue", actions, githubFastPathFailure)
		},
	},
	{
		Name: "artifact_link",
		Parse: func(text string, _ deterministicFastPathContext) *deterministicFastPathMatch {
			action := parseInlineArtifactLinkIntent(text)
			if action == nil {
				return nil
			}
			return fastPathSingleAction("artifact_link", action, fixedFastPathFailure("I couldn't resolve the artifact linking request: "))
		},
	},
	{
		Name: "batch",
		Parse: func(text string, _ deterministicFastPathContext) *deterministicFastPathMatch {
			action := parseInlineBatchIntent(text)
			if action == nil {
				return nil
			}
			return fastPathSingleAction("batch", action, fixedFastPathFailure("I couldn't resolve the batch request: "))
		},
	},
	{
		Name: "workspace",
		Parse: func(text string, _ deterministicFastPathContext) *deterministicFastPathMatch {
			action := parseInlineWorkspaceIntent(text)
			if action == nil {
				return nil
			}
			return fastPathSingleAction("workspace", action, fixedFastPathFailure("I couldn't resolve the workspace request: "))
		},
	},
	{
		Name: "project",
		Parse: func(text string, _ deterministicFastPathContext) *deterministicFastPathMatch {
			action := parseInlineProjectIntent(text)
			if action == nil {
				return nil
			}
			return fastPathSingleAction("project", action, fixedFastPathFailure("I couldn't resolve the project request: "))
		},
	},
}

func fastPathSingleAction(name string, action *SystemAction, failure func(string, []*SystemAction, error) string) *deterministicFastPathMatch {
	if action == nil {
		return nil
	}
	return fastPathActionPlan(name, []*SystemAction{action}, failure)
}

func fastPathActionPlan(name string, actions []*SystemAction, failure func(string, []*SystemAction, error) string) *deterministicFastPathMatch {
	if len(actions) == 0 {
		return nil
	}
	return &deterministicFastPathMatch{
		Name:           name,
		Actions:        actions,
		FailureMessage: failure,
	}
}

func fixedFastPathFailure(prefix string) func(string, []*SystemAction, error) string {
	return func(_ string, _ []*SystemAction, err error) string {
		return prefix + err.Error()
	}
}

func itemFastPathFailure(userText string, enforced []*SystemAction, err error) string {
	action := ""
	copied := copySystemActions(enforced)
	if len(copied) == 1 {
		action = copied[0].Action
		if normalized := normalizeSystemActionForExecution(copied[0], userText); normalized != nil {
			action = normalized.Action
		}
	}
	if action == "" {
		action = "make_item"
	}
	return itemActionFailurePrefix(action) + err.Error()
}

func githubFastPathFailure(_ string, enforced []*SystemAction, err error) string {
	return githubIssueActionFailurePrefix(enforced) + err.Error()
}

func tryDeterministicFastPath(text string, ctx deterministicFastPathContext) *deterministicFastPathMatch {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	for _, parser := range deterministicFastPaths {
		if parser.Parse == nil {
			continue
		}
		if match := parser.Parse(trimmed, ctx); match != nil {
			if strings.TrimSpace(match.Name) == "" {
				match.Name = parser.Name
			}
			return match
		}
	}
	return nil
}

func (a *App) executeDeterministicFastPath(ctx context.Context, sessionID string, session store.ChatSession, userText string, match *deterministicFastPathMatch) (string, []map[string]interface{}, bool) {
	if match == nil {
		return "", nil, false
	}
	if match.TitledItem != nil {
		message, payload, err := a.executeTitledItemIntent(ctx, session, match.TitledItem)
		if err != nil {
			if match.FailureMessage != nil {
				return match.FailureMessage(userText, nil, err), nil, true
			}
			return err.Error(), nil, true
		}
		if payload == nil {
			return message, nil, true
		}
		return message, []map[string]interface{}{payload}, true
	}
	enforced := enforceRoutingPolicy(userText, match.Actions)
	if len(enforced) == 0 {
		return "", nil, false
	}
	message, payloads, err := a.executeSystemActionPlan(sessionID, session, userText, enforced)
	if err != nil {
		if match.FailureMessage != nil {
			return match.FailureMessage(userText, enforced, err), nil, true
		}
		return err.Error(), nil, true
	}
	return message, payloads, true
}
