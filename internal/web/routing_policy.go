package web

import (
	"regexp"
	"strings"

	"github.com/krystophny/tabura/internal/modelprofile"
)

type turnRoutingDirectives struct {
	ModelAlias       string
	ReasoningEffort  string
	SearchRequested  bool
	PromptText       string
	DirectiveApplied bool
}

type directivePattern struct {
	regex  *regexp.Regexp
	alias  string
	effort string
	search bool
}

var turnDirectivePatterns = []directivePattern{
	{
		regex:  regexp.MustCompile(`(?i)\b(?:search(?:\s+the\s+web)?|web\s+search|look\s+up|browse|google|find\s+online|search\s+online|suche(?:\s+im\s+web)?|such(?:\s+im\s+web)?|websuche|recherchier(?:e)?|im\s+web\s+suchen|online\s+suchen)\b`),
		search: true,
	},
	{
		regex:  regexp.MustCompile(`(?i)\bthink\s+quick(?:ly)?\b|\bdenk\s+kurz\b|\bdenke\s+kurz\b|\büberleg\s+kurz\b|\bueberleg\s+kurz\b`),
		effort: modelprofile.ReasoningLow,
	},
	{
		regex:  regexp.MustCompile(`(?i)\bthink\s+hard\b|\bthink\s+deep(?:ly)?\b|\bdenk\s+tief(?:\s+nach)?\b|\bdenke\s+tief(?:\s+nach)?\b|\büberleg\s+tief\b|\bueberleg\s+tief\b`),
		effort: modelprofile.ReasoningHigh,
	},
	{
		regex:  regexp.MustCompile(`(?i)\bthink(?:\s+a\s+bit)?\b|\bdenk\s+nach\b|\bdenke\b|\büberleg\b|\bueberleg\b`),
		effort: modelprofile.ReasoningMedium,
	},
	{
		regex: regexp.MustCompile(`(?i)\b(?:use|ask|let|have|run|solve|handle|do|switch(?:\s+to)?|delegate|frag|lass|benutz(?:e)?|verwende)\b(?:[\s,:-]+(?:the|das|den|die|dem|mal|bitte|doch|einfach|modell|model|to|mit))*[\s,:-]+(?:spark|codex)\b|\b(?:with|mit)\s+(?:spark|codex)\b`),
		alias: modelprofile.AliasSpark,
	},
	{
		regex: regexp.MustCompile(`(?i)\b(?:use|ask|let|have|run|solve|handle|do|switch(?:\s+to)?|delegate|frag|lass|benutz(?:e)?|verwende)\b(?:[\s,:-]+(?:the|das|den|die|dem|mal|bitte|doch|einfach|modell|model|to|mit))*[\s,:-]+gpt\b|\b(?:with|mit)\s+gpt\b`),
		alias: modelprofile.AliasGPT,
	},
	{
		regex: regexp.MustCompile(`(?i)\b(?:use|ask|let|have|run|solve|handle|do|switch(?:\s+to)?|delegate|frag|lass|benutz(?:e)?|verwende)\b(?:[\s,:-]+(?:the|das|den|die|dem|mal|bitte|doch|einfach|modell|model|to|mit))*[\s,:-]+mini\b|\b(?:with|mit)\s+mini\b`),
		alias: modelprofile.AliasMini,
	},
}

var currentInfoCuePattern = regexp.MustCompile(`(?i)\b(?:latest|current|today(?:'s)?|tomorrow|yesterday|news|weather|forecast|price|prices|stock|stocks|score|scores|standings|schedule|schedules)\b`)

func parseTurnRoutingDirectives(text string) turnRoutingDirectives {
	original := strings.TrimSpace(text)
	if original == "" {
		return turnRoutingDirectives{}
	}
	if strings.HasPrefix(original, "/") {
		return turnRoutingDirectives{PromptText: original}
	}
	working := original
	directives := turnRoutingDirectives{PromptText: original}
	for _, pattern := range turnDirectivePatterns {
		matches := pattern.regex.FindAllStringIndex(working, -1)
		if len(matches) == 0 {
			continue
		}
		directives.DirectiveApplied = true
		if pattern.search {
			directives.SearchRequested = true
		}
		if pattern.alias != "" {
			directives.ModelAlias = pattern.alias
		}
		if pattern.effort != "" {
			directives.ReasoningEffort = pattern.effort
		}
		working = pattern.regex.ReplaceAllString(working, " ")
	}
	if directives.ModelAlias == "" && directives.SearchRequested {
		directives.ModelAlias = modelprofile.AliasSpark
	}
	if directives.ModelAlias == "" && currentInfoCuePattern.MatchString(original) {
		directives.SearchRequested = true
		directives.ModelAlias = modelprofile.AliasSpark
	}
	cleaned := strings.Join(strings.Fields(working), " ")
	cleaned = strings.TrimSpace(strings.Trim(cleaned, ",:;-"))
	if cleaned == "" {
		cleaned = original
	}
	directives.PromptText = cleaned
	return directives
}

func routeProfileForRouting(requestedAlias string, fallback appServerModelProfile, sparkEffort string, reasoningOverride string) appServerModelProfile {
	alias := modelprofile.ResolveAlias(requestedAlias, fallback.Alias)
	if alias == "" {
		alias = modelprofile.AliasLocal
	}
	model := modelprofile.ModelForAlias(alias)
	if alias == modelprofile.AliasLocal {
		model = modelprofile.ModelLocal
	}
	if strings.TrimSpace(model) == "" {
		model = strings.TrimSpace(fallback.Model)
	}
	effortInput := strings.TrimSpace(reasoningOverride)
	if effortInput == "" && alias == modelprofile.AliasSpark {
		effortInput = strings.TrimSpace(sparkEffort)
	}
	effort := modelprofile.NormalizeReasoningEffort(alias, effortInput)
	if effort == "" {
		effort = strings.TrimSpace(modelprofile.MainThreadReasoningEffort(alias))
	}
	return appServerModelProfile{
		Alias:        alias,
		Model:        model,
		ThreadParams: fallback.ThreadParams,
		TurnParams:   modelprofile.MainThreadReasoningParamsForEffort(alias, effort),
	}
}

func enforceRoutingPolicy(userText string, actions []*SystemAction) []*SystemAction {
	if len(actions) == 0 {
		return nil
	}
	out := make([]*SystemAction, 0, len(actions))
	for _, action := range actions {
		if normalized := normalizeSystemActionForExecution(action, userText); normalized != nil {
			out = append(out, normalized)
		}
	}
	return out
}
