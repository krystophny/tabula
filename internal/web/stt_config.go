package web

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/krystophny/tabura/internal/stt"
)

const sttConfigStateKey = "stt_config"

type sttConfigSnapshot struct {
	AllowedLanguages []string `json:"allowed_languages"`
	FallbackLanguage string   `json:"fallback_language"`
	InitialPrompt    string   `json:"initial_prompt"`
	PreVADEnabled    bool     `json:"pre_vad_enabled"`
	PreVADThreshold  float64  `json:"pre_vad_threshold_db"`
	PreVADMinSpeech  int      `json:"pre_vad_min_speech_ms"`
}

type sttConfigPatch struct {
	AllowedLanguages *[]string `json:"allowed_languages,omitempty"`
	FallbackLanguage *string   `json:"fallback_language,omitempty"`
	InitialPrompt    *string   `json:"initial_prompt,omitempty"`
	PreVADEnabled    *bool     `json:"pre_vad_enabled,omitempty"`
	PreVADThreshold  *float64  `json:"pre_vad_threshold_db,omitempty"`
	PreVADMinSpeech  *int      `json:"pre_vad_min_speech_ms,omitempty"`
}

func normalizeLanguageCodeEnv(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "" {
		return ""
	}
	if i := strings.Index(v, "_"); i >= 0 {
		v = strings.TrimSpace(v[:i])
	}
	if i := strings.Index(v, "-"); i >= 0 {
		v = strings.TrimSpace(v[:i])
	}
	if i := strings.Index(v, "."); i >= 0 {
		v = strings.TrimSpace(v[:i])
	}
	return v
}

func parseLanguageListEnv(raw string) []string {
	parts := strings.Split(strings.TrimSpace(raw), ",")
	seen := map[string]struct{}{}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		lang := normalizeLanguageCodeEnv(part)
		if lang == "" || lang == "auto" {
			continue
		}
		if _, ok := seen[lang]; ok {
			continue
		}
		seen[lang] = struct{}{}
		out = append(out, lang)
	}
	return out
}

func prependPreferredLanguage(languages []string, preferred string) []string {
	lang := normalizeLanguageCodeEnv(preferred)
	if lang == "" || lang == "auto" {
		return languages
	}
	out := make([]string, 0, len(languages)+1)
	out = append(out, lang)
	for _, existing := range languages {
		clean := normalizeLanguageCodeEnv(existing)
		if clean == "" || clean == "auto" || clean == lang {
			continue
		}
		out = append(out, clean)
	}
	return out
}

func parseEnvBoolDefault(key string, fallback bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func parseEnvFloatDefault(key string, fallback float64) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fallback
	}
	return value
}

func parseEnvIntDefault(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func parseEnvDurationDefault(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func (a *App) defaultSTTConfig() sttConfigSnapshot {
	langs := make([]string, 0, len(a.sttAllowedLanguagesDefault))
	for _, lang := range a.sttAllowedLanguagesDefault {
		normalized := normalizeLanguageCodeEnv(lang)
		if normalized == "" || normalized == "auto" {
			continue
		}
		langs = append(langs, normalized)
	}
	if len(langs) == 0 {
		langs = parseLanguageListEnv(DefaultSTTAllowedLanguages)
	}

	fallback := normalizeLanguageCodeEnv(a.sttFallbackLanguageDefault)
	if fallback == "" {
		if len(langs) > 0 {
			fallback = langs[0]
		} else {
			fallback = DefaultSTTFallbackLanguage
		}
	}

	return sttConfigSnapshot{
		AllowedLanguages: langs,
		FallbackLanguage: fallback,
		InitialPrompt:    strings.TrimSpace(a.sttInitialPromptDefault),
		PreVADEnabled:    a.sttPreVADEnabledDefault,
		PreVADThreshold:  a.sttPreVADThresholdDBDefault,
		PreVADMinSpeech:  a.sttPreVADMinSpeechMSDefault,
	}
}

func normalizeSTTConfigSnapshot(cfg sttConfigSnapshot) sttConfigSnapshot {
	langs := parseLanguageListEnv(strings.Join(cfg.AllowedLanguages, ","))
	fallback := normalizeLanguageCodeEnv(cfg.FallbackLanguage)
	if fallback == "" {
		if len(langs) > 0 {
			fallback = langs[0]
		} else {
			fallback = DefaultSTTFallbackLanguage
		}
	}
	foundFallback := false
	for _, lang := range langs {
		if lang == fallback {
			foundFallback = true
			break
		}
	}
	if !foundFallback && fallback != "" {
		langs = append(langs, fallback)
	}
	if len(langs) == 0 {
		langs = parseLanguageListEnv(DefaultSTTAllowedLanguages)
		if len(langs) == 0 {
			langs = []string{DefaultSTTFallbackLanguage}
		}
	}

	threshold := cfg.PreVADThreshold
	if threshold > -10 || threshold < -100 {
		threshold = DefaultSTTPreVADThresholdDB
	}
	minSpeech := cfg.PreVADMinSpeech
	if minSpeech <= 0 {
		minSpeech = DefaultSTTPreVADMinSpeechMS
	}

	sort.Strings(langs)
	return sttConfigSnapshot{
		AllowedLanguages: langs,
		FallbackLanguage: fallback,
		InitialPrompt:    strings.TrimSpace(cfg.InitialPrompt),
		PreVADEnabled:    cfg.PreVADEnabled,
		PreVADThreshold:  threshold,
		PreVADMinSpeech:  minSpeech,
	}
}

func (a *App) loadSTTConfig() sttConfigSnapshot {
	cfg := a.defaultSTTConfig()
	raw, err := a.store.AppState(sttConfigStateKey)
	if err != nil || strings.TrimSpace(raw) == "" {
		return normalizeSTTConfigSnapshot(cfg)
	}
	var persisted sttConfigSnapshot
	if err := json.Unmarshal([]byte(raw), &persisted); err != nil {
		return normalizeSTTConfigSnapshot(cfg)
	}
	if len(persisted.AllowedLanguages) > 0 {
		cfg.AllowedLanguages = persisted.AllowedLanguages
	}
	if strings.TrimSpace(persisted.FallbackLanguage) != "" {
		cfg.FallbackLanguage = persisted.FallbackLanguage
	}
	if strings.TrimSpace(persisted.InitialPrompt) != "" {
		cfg.InitialPrompt = persisted.InitialPrompt
	}
	cfg.PreVADEnabled = persisted.PreVADEnabled
	if persisted.PreVADThreshold != 0 {
		cfg.PreVADThreshold = persisted.PreVADThreshold
	}
	if persisted.PreVADMinSpeech > 0 {
		cfg.PreVADMinSpeech = persisted.PreVADMinSpeech
	}
	return normalizeSTTConfigSnapshot(cfg)
}

func (a *App) saveSTTConfig(cfg sttConfigSnapshot) error {
	cfg = normalizeSTTConfigSnapshot(cfg)
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return a.store.SetAppState(sttConfigStateKey, string(data))
}

func (a *App) sttTranscribeOptions() stt.TranscribeOptions {
	cfg := a.loadSTTConfig()
	return stt.TranscribeOptions{
		AllowedLanguages: cfg.AllowedLanguages,
		FallbackLanguage: cfg.FallbackLanguage,
		InitialPrompt:    cfg.InitialPrompt,
		PreVAD: stt.PreVADConfig{
			Enabled:     cfg.PreVADEnabled,
			ThresholdDB: cfg.PreVADThreshold,
			MinSpeechMS: cfg.PreVADMinSpeech,
			FrameMS:     20,
		},
	}
}

func (a *App) handleSTTConfigGet(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	writeJSON(w, a.loadSTTConfig())
}

func (a *App) handleSTTConfigPut(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1024*1024))
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var patch sttConfigPatch
	if err := json.Unmarshal(body, &patch); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	cfg := a.loadSTTConfig()
	if patch.AllowedLanguages != nil {
		cfg.AllowedLanguages = append([]string{}, (*patch.AllowedLanguages)...)
	}
	if patch.FallbackLanguage != nil {
		cfg.FallbackLanguage = *patch.FallbackLanguage
	}
	if patch.InitialPrompt != nil {
		cfg.InitialPrompt = *patch.InitialPrompt
	}
	if patch.PreVADEnabled != nil {
		cfg.PreVADEnabled = *patch.PreVADEnabled
	}
	if patch.PreVADThreshold != nil {
		cfg.PreVADThreshold = *patch.PreVADThreshold
	}
	if patch.PreVADMinSpeech != nil {
		cfg.PreVADMinSpeech = *patch.PreVADMinSpeech
	}

	cfg = normalizeSTTConfigSnapshot(cfg)
	if err := a.saveSTTConfig(cfg); err != nil {
		http.Error(w, "failed to save stt config", http.StatusInternalServerError)
		return
	}
	writeJSON(w, cfg)
}
