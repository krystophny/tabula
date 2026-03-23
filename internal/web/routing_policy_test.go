package web

import (
	"strings"
	"testing"

	"github.com/krystophny/tabura/internal/modelprofile"
)

func TestParseTurnRoutingDirectivesDefaultsToLocal(t *testing.T) {
	directives := parseTurnRoutingDirectives("summarize this note")
	if directives.ModelAlias != "" {
		t.Fatalf("ModelAlias = %q, want empty", directives.ModelAlias)
	}
	if directives.ReasoningEffort != "" {
		t.Fatalf("ReasoningEffort = %q, want empty", directives.ReasoningEffort)
	}
	if directives.SearchRequested {
		t.Fatal("SearchRequested = true, want false")
	}
	if directives.PromptText != "summarize this note" {
		t.Fatalf("PromptText = %q, want original", directives.PromptText)
	}
}

func TestParseTurnRoutingDirectivesSupportsSparkCodexGPTMiniAndReasoning(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		wantAlias  string
		wantEffort string
	}{
		{name: "spark", text: "Use Spark for this and think hard: analyze the latest error.", wantAlias: modelprofile.AliasSpark, wantEffort: modelprofile.ReasoningHigh},
		{name: "codex alias", text: "Lass Codex denk kurz den Buildfehler ansehen.", wantAlias: modelprofile.AliasSpark, wantEffort: modelprofile.ReasoningLow},
		{name: "gpt", text: "Please use GPT and think quickly about this timeout.", wantAlias: modelprofile.AliasGPT, wantEffort: modelprofile.ReasoningLow},
		{name: "mini", text: "Let mini think a bit about this API diff.", wantAlias: modelprofile.AliasMini, wantEffort: modelprofile.ReasoningMedium},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			directives := parseTurnRoutingDirectives(tc.text)
			if directives.ModelAlias != tc.wantAlias {
				t.Fatalf("ModelAlias = %q, want %q", directives.ModelAlias, tc.wantAlias)
			}
			if directives.ReasoningEffort != tc.wantEffort {
				t.Fatalf("ReasoningEffort = %q, want %q", directives.ReasoningEffort, tc.wantEffort)
			}
			if strings.TrimSpace(directives.PromptText) == "" {
				t.Fatal("PromptText is empty")
			}
		})
	}
}

func TestParseTurnRoutingDirectivesRoutesSearchesToSpark(t *testing.T) {
	directives := parseTurnRoutingDirectives("Search the web for today's AMD news.")
	if !directives.SearchRequested {
		t.Fatal("SearchRequested = false, want true")
	}
	if directives.ModelAlias != modelprofile.AliasSpark {
		t.Fatalf("ModelAlias = %q, want %q", directives.ModelAlias, modelprofile.AliasSpark)
	}
}

func TestRouteProfileForRoutingUsesMiniHighAndLocalFallback(t *testing.T) {
	base := appServerModelProfile{
		Alias: modelprofile.AliasLocal,
		Model: modelprofile.ModelLocal,
	}
	miniProfile := routeProfileForRouting(modelprofile.AliasMini, base, modelprofile.ReasoningLow, "")
	if miniProfile.Alias != modelprofile.AliasMini {
		t.Fatalf("mini alias = %q, want %q", miniProfile.Alias, modelprofile.AliasMini)
	}
	if miniProfile.Model != modelprofile.ModelMini {
		t.Fatalf("mini model = %q, want %q", miniProfile.Model, modelprofile.ModelMini)
	}
	if got := strings.TrimSpace(strFromAny(miniProfile.TurnParams["effort"])); got != modelprofile.ReasoningHigh {
		t.Fatalf("mini effort = %q, want %q", got, modelprofile.ReasoningHigh)
	}
	localProfile := routeProfileForRouting("", base, modelprofile.ReasoningMedium, "")
	if localProfile.Alias != modelprofile.AliasLocal {
		t.Fatalf("local alias = %q, want %q", localProfile.Alias, modelprofile.AliasLocal)
	}
	if got := strings.TrimSpace(strFromAny(localProfile.TurnParams["effort"])); got != modelprofile.ReasoningNone {
		t.Fatalf("local effort = %q, want %q", got, modelprofile.ReasoningNone)
	}
}

func TestEnforceRoutingPolicyNormalizesButDoesNotDropActions(t *testing.T) {
	actions := []*SystemAction{
		{Action: "toggle_silent", Params: map[string]interface{}{}},
		{Action: "show_status", Params: map[string]interface{}{}},
	}
	enforced := enforceRoutingPolicy("use spark for this", actions)
	if len(enforced) != 2 {
		t.Fatalf("enforced length = %d, want 2", len(enforced))
	}
}
