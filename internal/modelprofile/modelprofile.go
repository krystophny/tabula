package modelprofile

import "strings"

const (
	AliasSpark = "spark"
	AliasCodex = "codex"
	AliasGPT   = "gpt"

	ModelSpark = "gpt-5.3-codex-spark"
	ModelCodex = "gpt-5.3-codex"
	ModelGPT   = "gpt-5.2"

	ReasoningLow       = "low"
	ReasoningMedium    = "medium"
	ReasoningHigh      = "high"
	ReasoningExtraHigh = "extra_high"
)

var aliasToModel = map[string]string{
	AliasSpark: ModelSpark,
	AliasCodex: ModelCodex,
	AliasGPT:   ModelGPT,
}

var modelToAlias = map[string]string{
	strings.ToLower(ModelSpark): AliasSpark,
	strings.ToLower(ModelCodex): AliasCodex,
	strings.ToLower(ModelGPT):   AliasGPT,
}

var modelReasoningEfforts = map[string][]string{
	AliasSpark: {ReasoningLow, ReasoningMedium, ReasoningHigh, ReasoningExtraHigh},
	AliasCodex: {ReasoningLow, ReasoningMedium, ReasoningHigh, ReasoningExtraHigh},
	AliasGPT:   {ReasoningLow, ReasoningMedium, ReasoningHigh, ReasoningExtraHigh},
}

func SupportedAliases() []string {
	return []string{AliasCodex, AliasGPT, AliasSpark}
}

func SupportedModels() []string {
	return []string{ModelSpark, ModelCodex, ModelGPT}
}

func NormalizeAlias(raw string) string {
	key := strings.TrimSpace(strings.ToLower(raw))
	if _, ok := aliasToModel[key]; ok {
		return key
	}
	return ""
}

func AliasForModel(raw string) string {
	if alias := NormalizeAlias(raw); alias != "" {
		return alias
	}
	key := strings.TrimSpace(strings.ToLower(raw))
	if key == "" {
		return ""
	}
	if alias, ok := modelToAlias[key]; ok {
		return alias
	}
	return ""
}

func ResolveAlias(raw, fallback string) string {
	if alias := AliasForModel(raw); alias != "" {
		return alias
	}
	if alias := AliasForModel(fallback); alias != "" {
		return alias
	}
	return ""
}

func ModelForAlias(alias string) string {
	clean := NormalizeAlias(alias)
	if clean == "" {
		return ""
	}
	return aliasToModel[clean]
}

func ResolveModel(raw, fallbackAlias string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		if model := ModelForAlias(fallbackAlias); model != "" {
			return model
		}
		return ""
	}
	if model := ModelForAlias(trimmed); model != "" {
		return model
	}
	if alias := AliasForModel(trimmed); alias != "" {
		return aliasToModel[alias]
	}
	return trimmed
}

func MainThreadReasoningEffort(alias string) string {
	switch NormalizeAlias(alias) {
	case AliasSpark:
		return ReasoningLow
	case AliasCodex, AliasGPT:
		return ReasoningHigh
	default:
		return ReasoningLow
	}
}

func AvailableReasoningEffortsByAlias() map[string][]string {
	out := map[string][]string{}
	for alias, options := range modelReasoningEfforts {
		copied := append([]string(nil), options...)
		out[alias] = copied
	}
	return out
}

func ReasoningEffortsForAlias(alias string) []string {
	if options, ok := modelReasoningEfforts[NormalizeAlias(alias)]; ok {
		return append([]string(nil), options...)
	}
	return append([]string(nil), modelReasoningEfforts[AliasSpark]...)
}

func NormalizeReasoningEffort(alias, rawEffort string) string {
	effort := strings.TrimSpace(strings.ToLower(rawEffort))
	for _, candidate := range ReasoningEffortsForAlias(alias) {
		if candidate == effort {
			return effort
		}
	}
	defaultEffort := MainThreadReasoningEffort(alias)
	if defaultEffort == "" {
		return ""
	}
	return defaultEffort
}

func MainThreadReasoningParamsForEffort(alias, effort string) map[string]interface{} {
	effort = NormalizeReasoningEffort(alias, effort)
	return ReasoningParamsForEffort(effort)
}

func ReasoningParamsForEffort(effort string) map[string]interface{} {
	switch strings.TrimSpace(strings.ToLower(effort)) {
	case ReasoningLow, ReasoningMedium, ReasoningHigh, ReasoningExtraHigh:
		return map[string]interface{}{"model_reasoning_effort": strings.TrimSpace(strings.ToLower(effort))}
	default:
		return nil
	}
}

func MainThreadReasoningParams(alias string) map[string]interface{} {
	return MainThreadReasoningParamsForEffort(alias, "")
}

// ModelSystemHints returns model-specific system prompt additions.
// These are appended to the system prompt to compensate for known model
// weaknesses or to enforce delegation policies.
func ModelSystemHints(alias string) string {
	switch NormalizeAlias(alias) {
	case AliasSpark:
		return sparkSystemHints
	default:
		return ""
	}
}

const sparkSystemHints = `## Model-Specific Rules (spark)
You are running as the spark model. Spark is fast but unreliable at complex git operations.
- ALWAYS delegate to codex for: git rebase, git merge with conflicts, git cherry-pick with conflicts, interactive git operations, and any multi-step git recovery.
- If you detect merge conflicts (e.g. "CONFLICT" in command output, <<<<<<< markers in files), STOP immediately and delegate the resolution to codex.
- When delegating git tasks, include the full error output and repository state in the context.
- Do NOT attempt to resolve merge conflicts yourself. Do NOT edit files containing conflict markers.
`

func DelegateReasoningParams(model string) map[string]interface{} {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return nil
	}
	if alias := AliasForModel(trimmed); alias != "" {
		if alias == AliasSpark {
			return nil
		}
		return ReasoningParamsForEffort(ReasoningHigh)
	}
	if strings.Contains(strings.ToLower(trimmed), "spark") {
		return nil
	}
	return ReasoningParamsForEffort(ReasoningHigh)
}
