package web

import (
	"strings"
	"time"

	"github.com/krystophny/tabura/internal/modelprofile"
	"github.com/krystophny/tabura/internal/store"
)

const (
	assistantProviderLocal  = "local"
	assistantProviderOpenAI = "openai"
	assistantProviderSpark  = "spark"
	assistantProviderGPT    = "gpt"
	assistantProviderMini   = "mini"
)

type assistantResponseMetadata struct {
	Provider        string
	ProviderModel   string
	ProviderLatency int
}

func newAssistantResponseMetadata(provider, model string, latency time.Duration) assistantResponseMetadata {
	latencyMS := int(latency / time.Millisecond)
	if latencyMS < 0 {
		latencyMS = 0
	}
	return assistantResponseMetadata{
		Provider:        normalizeAssistantProvider(provider),
		ProviderModel:   strings.TrimSpace(model),
		ProviderLatency: latencyMS,
	}
}

func normalizeAssistantProvider(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case assistantProviderLocal:
		return assistantProviderLocal
	case assistantProviderOpenAI:
		return assistantProviderOpenAI
	case assistantProviderSpark:
		return assistantProviderSpark
	case assistantProviderGPT:
		return assistantProviderGPT
	case assistantProviderMini:
		return assistantProviderMini
	default:
		return ""
	}
}

func assistantProviderDisplayLabel(provider, model string) string {
	switch assistantProviderBadgeKey(provider, model) {
	case assistantProviderLocal:
		return "Local"
	case assistantProviderSpark:
		return "Spark"
	case assistantProviderGPT:
		return "GPT"
	case assistantProviderMini:
		return "Mini"
	case assistantProviderOpenAI:
		return "OpenAI"
	default:
		return "Local"
	}
}

func assistantProviderBadgeKey(provider, model string) string {
	normalizedProvider := normalizeAssistantProvider(provider)
	switch normalizedProvider {
	case assistantProviderSpark, assistantProviderGPT, assistantProviderMini:
		return normalizedProvider
	case assistantProviderLocal:
		return assistantProviderLocal
	case assistantProviderOpenAI:
		if alias := modelprofile.ResolveAlias(model, ""); alias != "" {
			return alias
		}
		return assistantProviderGPT
	case "":
		if alias := modelprofile.ResolveAlias(model, ""); alias != "" {
			return alias
		}
		return assistantProviderLocal
	default:
		return assistantProviderLocal
	}
}

func (m assistantResponseMetadata) storeOptions() []store.ChatMessageOption {
	return []store.ChatMessageOption{
		store.WithProviderMetadata(m.Provider, m.ProviderModel, m.ProviderLatency),
	}
}

func (m assistantResponseMetadata) applyToPayload(payload map[string]interface{}) {
	payload["provider"] = m.Provider
	payload["provider_label"] = assistantProviderDisplayLabel(m.Provider, m.ProviderModel)
	payload["provider_model"] = m.ProviderModel
	payload["provider_latency_ms"] = m.ProviderLatency
}

func providerForAppServerProfile(profile appServerModelProfile) string {
	switch modelprofile.ResolveAlias(profile.Alias, profile.Model) {
	case modelprofile.AliasMini:
		return assistantProviderMini
	case modelprofile.AliasGPT:
		return assistantProviderGPT
	case modelprofile.AliasSpark:
		return assistantProviderSpark
	default:
		return ""
	}
}

func (a *App) localAssistantProvider() string {
	return assistantProviderLocal
}

func (a *App) localAssistantModelLabel() string {
	if a == nil {
		return DefaultIntentLLMProfile
	}
	if model := strings.TrimSpace(a.assistantLLMModel); model != "" && !strings.EqualFold(model, DefaultIntentLLMModel) {
		return model
	}
	if profile := strings.TrimSpace(a.intentLLMProfile); profile != "" {
		return profile
	}
	if model := strings.TrimSpace(a.localIntentLLMModel()); model != "" {
		return model
	}
	return DefaultIntentLLMProfile
}
