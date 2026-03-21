package web

import (
	"testing"

	"github.com/krystophny/tabura/internal/modelprofile"
	"github.com/krystophny/tabura/internal/store"
)

func TestEffectiveProjectChatModelFallsBackToSpark(t *testing.T) {
	app := newAuthedTestApp(t)
	app.appServerModel = modelprofile.ModelSpark

	project := store.Workspace{}
	if got := app.effectiveWorkspaceChatModelAlias(project); got != modelprofile.AliasSpark {
		t.Fatalf("effectiveWorkspaceChatModelAlias() = %q, want %q", got, modelprofile.AliasSpark)
	}
	if got := app.effectiveWorkspaceChatModelReasoningEffort(project); got != modelprofile.ReasoningLow {
		t.Fatalf("effectiveWorkspaceChatModelReasoningEffort() = %q, want %q", got, modelprofile.ReasoningLow)
	}
}

func TestEffectiveProjectChatModelDefaultsToSpark(t *testing.T) {
	app := newAuthedTestApp(t)
	app.appServerModel = ""

	project := store.Workspace{}
	if got := app.effectiveWorkspaceChatModelAlias(project); got != modelprofile.AliasSpark {
		t.Fatalf("effectiveWorkspaceChatModelAlias() = %q, want %q", got, modelprofile.AliasSpark)
	}
	if got := app.effectiveWorkspaceChatModelReasoningEffort(project); got != modelprofile.ReasoningLow {
		t.Fatalf("effectiveWorkspaceChatModelReasoningEffort() = %q, want %q", got, modelprofile.ReasoningLow)
	}
}

func TestAppServerModelProfileForProjectUsesStoredAliasAndNormalizesReasoning(t *testing.T) {
	app := newAuthedTestApp(t)

	profile := app.appServerModelProfileForWorkspace(store.Workspace{
		ChatModel:                modelprofile.AliasGPT,
		ChatModelReasoningEffort: "minimal",
	})
	if profile.Alias != modelprofile.AliasGPT {
		t.Fatalf("profile.Alias = %q, want %q", profile.Alias, modelprofile.AliasGPT)
	}
	if profile.Model != modelprofile.ModelGPT {
		t.Fatalf("profile.Model = %q, want %q", profile.Model, modelprofile.ModelGPT)
	}
	if profile.ThreadParams != nil {
		t.Fatalf("profile.ThreadParams = %#v, want nil", profile.ThreadParams)
	}
	if got := profile.TurnParams["effort"]; got != modelprofile.ReasoningHigh {
		t.Fatalf("profile.TurnParams[effort] = %#v, want %q", got, modelprofile.ReasoningHigh)
	}
}

func TestAppServerModelProfileForWorkspacePathFallsBackWhenProjectMissing(t *testing.T) {
	app := newAuthedTestApp(t)
	app.appServerModel = modelprofile.ModelSpark

	profile := app.appServerModelProfileForWorkspacePath("missing-project")
	if profile.Alias != modelprofile.AliasSpark {
		t.Fatalf("profile.Alias = %q, want %q", profile.Alias, modelprofile.AliasSpark)
	}
	if profile.Model != modelprofile.ModelSpark {
		t.Fatalf("profile.Model = %q, want %q", profile.Model, modelprofile.ModelSpark)
	}
	if got := profile.TurnParams["effort"]; got != modelprofile.ReasoningLow {
		t.Fatalf("profile.TurnParams[effort] = %#v, want %q", got, modelprofile.ReasoningLow)
	}
}

func TestAssistantTurnModeHonorsConfiguredRouting(t *testing.T) {
	app := newAuthedTestApp(t)

	app.assistantMode = assistantModeLocal
	if got := app.assistantTurnMode(false); got != assistantModeLocal {
		t.Fatalf("assistantTurnMode(local) = %q, want %q", got, assistantModeLocal)
	}

	app.assistantMode = assistantModeCodex
	if got := app.assistantTurnMode(false); got != assistantModeCodex {
		t.Fatalf("assistantTurnMode(codex) = %q, want %q", got, assistantModeCodex)
	}

	app.assistantMode = assistantModeAuto
	app.appServerClient = nil
	if got := app.assistantTurnMode(false); got != assistantModeLocal {
		t.Fatalf("assistantTurnMode(auto, no app-server) = %q, want %q", got, assistantModeLocal)
	}
	if got := app.assistantTurnMode(true); got != assistantModeLocal {
		t.Fatalf("assistantTurnMode(localOnly) = %q, want %q", got, assistantModeLocal)
	}
}
