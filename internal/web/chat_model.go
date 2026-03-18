package web

import (
	"strings"

	"github.com/krystophny/tabura/internal/modelprofile"
	"github.com/krystophny/tabura/internal/store"
)

type appServerModelProfile struct {
	Alias        string
	Model        string
	ThreadParams map[string]interface{}
	TurnParams   map[string]interface{}
}

func (a *App) effectiveWorkspaceChatModelAlias(project store.Workspace) string {
	if alias := modelprofile.ResolveAlias(project.ChatModel, ""); alias != "" {
		return alias
	}
	return modelprofile.AliasSpark
}

func (a *App) effectiveWorkspaceChatModelReasoningEffort(project store.Workspace) string {
	alias := a.effectiveWorkspaceChatModelAlias(project)
	effort := modelprofile.NormalizeReasoningEffort(alias, project.ChatModelReasoningEffort)
	if effort == "" {
		return modelprofile.MainThreadReasoningEffort(alias)
	}
	return effort
}

func (a *App) appServerModelProfileForWorkspace(project store.Workspace) appServerModelProfile {
	alias := a.effectiveWorkspaceChatModelAlias(project)
	effort := a.effectiveWorkspaceChatModelReasoningEffort(project)
	model := modelprofile.ModelForAlias(alias)
	if model == "" {
		model = strings.TrimSpace(a.appServerModel)
	}
	if model == "" {
		model = modelprofile.ModelForAlias(modelprofile.AliasSpark)
	}
	reasoning := modelprofile.MainThreadReasoningParamsForEffort(alias, effort)
	return appServerModelProfile{
		Alias:        alias,
		Model:        model,
		ThreadParams: nil,
		TurnParams:   reasoning,
	}

}

func (a *App) appServerModelProfileForWorkspacePath(workspacePath string) appServerModelProfile {
	cleanKey := strings.TrimSpace(workspacePath)
	if cleanKey != "" {
		if project, err := a.store.GetWorkspaceByStoredPath(cleanKey); err == nil {
			return a.appServerModelProfileForWorkspace(project)
		}
	}
	alias := modelprofile.AliasSpark
	legacyModel := modelprofile.ModelForAlias(alias)
	legacyReasoning := modelprofile.MainThreadReasoningParamsForEffort(alias, modelprofile.MainThreadReasoningEffort(alias))
	return appServerModelProfile{
		Alias:        alias,
		Model:        legacyModel,
		ThreadParams: nil,
		TurnParams:   legacyReasoning,
	}
}

func (a *App) resetWorkspaceChatAppSession(workspacePath string) {
	key := strings.TrimSpace(workspacePath)
	if key == "" {
		return
	}
	session, err := a.chatSessionForWorkspacePath(key)
	if err != nil {
		return
	}
	a.closeAppSession(session.ID)
	_ = a.store.UpdateChatSessionThread(session.ID, "")
}
