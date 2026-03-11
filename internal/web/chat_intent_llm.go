package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

var intentLLMSystemPrompt = buildIntentLLMSystemPrompt()

type localIntentLLMChatCompletionResponse struct {
	Choices []localIntentLLMChoice `json:"choices"`
}

type localIntentLLMChoice struct {
	Message localIntentLLMMessage `json:"message"`
}

type localIntentLLMMessage struct {
	Content string `json:"content"`
}

func parseIntentLLMProfileOptions(raw string) []string {
	clean := strings.TrimSpace(raw)
	if clean == "" {
		return nil
	}
	parts := strings.Split(clean, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		token := strings.ToLower(strings.TrimSpace(part))
		if token == "" {
			continue
		}
		if _, exists := seen[token]; exists {
			continue
		}
		seen[token] = struct{}{}
		out = append(out, token)
	}
	return out
}

func resolveIntentLLMProfile(raw string) string {
	clean := strings.ToLower(strings.TrimSpace(raw))
	if clean == "" {
		return DefaultIntentLLMProfile
	}
	return clean
}

func ensureIntentLLMProfileOption(options []string, profile string) []string {
	cleanProfile := strings.ToLower(strings.TrimSpace(profile))
	if cleanProfile == "" {
		cleanProfile = DefaultIntentLLMProfile
	}
	for _, option := range options {
		if strings.EqualFold(strings.TrimSpace(option), cleanProfile) {
			return options
		}
	}
	return append([]string{cleanProfile}, options...)
}

func (a *App) localIntentLLMModel() string {
	if a == nil {
		return DefaultIntentLLMModel
	}
	clean := strings.TrimSpace(a.intentLLMModel)
	if clean == "" {
		return DefaultIntentLLMModel
	}
	return clean
}

func addressedBoolPtr(value bool) *bool {
	v := value
	return &v
}

func parseOptionalBool(value interface{}) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true":
			return true, true
		case "false":
			return false, true
		default:
			return false, false
		}
	default:
		return false, false
	}
}

func parseIntentPlanClassification(raw string) (intentPlanClassification, error) {
	decoded, ok := decodeSystemActionJSON(raw)
	if !ok {
		return intentPlanClassification{}, nil
	}
	result := intentPlanClassification{
		Actions: collectSystemActionsFromDecoded(decoded),
	}
	if obj, ok := decoded.(map[string]interface{}); ok {
		if addressed, ok := parseOptionalBool(obj["addressed"]); ok {
			result.Addressed = addressedBoolPtr(addressed)
		}
	}
	return result, nil
}

func (a *App) classifyIntentPlanWithLLMResult(ctx context.Context, text string) (intentPlanClassification, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(a.intentLLMURL), "/")
	if baseURL == "" {
		return intentPlanClassification{}, nil
	}
	trimmedText := strings.TrimSpace(text)
	if trimmedText == "" {
		return intentPlanClassification{}, nil
	}
	requiresOpenCanvas := requestRequiresOpenCanvasAction(trimmedText)
	policy := LivePolicyDialogue
	if a != nil {
		policy = a.LivePolicy()
	}
	requestPlan := func(systemPrompt string, userPrompt string) (intentPlanClassification, error) {
		requestBody, _ := json.Marshal(map[string]interface{}{
			"model":       a.localIntentLLMModel(),
			"temperature": 0,
			"max_tokens":  256,
			"response_format": map[string]interface{}{
				"type": "json_object",
			},
			"chat_template_kwargs": map[string]interface{}{
				"enable_thinking": false,
			},
			"messages": []map[string]string{
				{"role": "system", "content": systemPrompt},
				{"role": "user", "content": userPrompt},
			},
		})
		requestCtx, cancel := context.WithTimeout(ctx, intentLLMRequestTimeout)
		defer cancel()
		req, err := http.NewRequestWithContext(
			requestCtx,
			http.MethodPost,
			baseURL+"/v1/chat/completions",
			bytes.NewReader(requestBody),
		)
		if err != nil {
			return intentPlanClassification{}, err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return intentPlanClassification{}, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, intentLLMResponseLimit))
			return intentPlanClassification{}, fmt.Errorf("intent llm HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		var payload localIntentLLMChatCompletionResponse
		if err := json.NewDecoder(io.LimitReader(resp.Body, intentLLMResponseLimit)).Decode(&payload); err != nil {
			return intentPlanClassification{}, err
		}
		if len(payload.Choices) == 0 {
			return intentPlanClassification{}, nil
		}
		content := strings.TrimSpace(payload.Choices[0].Message.Content)
		if content == "" {
			return intentPlanClassification{}, nil
		}
		classification, parseErr := parseIntentPlanClassification(stripCodeFence(content))
		if parseErr != nil {
			return intentPlanClassification{}, parseErr
		}
		normalized := make([]*SystemAction, 0, len(classification.Actions))
		for _, action := range classification.Actions {
			if normalizedAction := normalizeSystemActionForExecution(action, trimmedText); normalizedAction != nil {
				normalized = append(normalized, normalizedAction)
			}
		}
		classification.Actions = normalized
		return classification, nil
	}

	initialSystemPrompt := buildIntentLLMSystemPromptForPolicy(policy)
	if requiresOpenCanvas {
		initialSystemPrompt += "\n\nConstraint: for explicit open/show/display file requests you MUST return an actions array whose final step is open_file_canvas. If path is uncertain, include a shell search step first and then use path=\"$last_shell_path\"."
	}
	classification, err := requestPlan(initialSystemPrompt, trimmedText)
	if err != nil {
		return intentPlanClassification{}, err
	}
	if requiresOpenCanvas && !planContainsAction(classification.Actions, "open_file_canvas") {
		previousPlanJSON := "null"
		if len(classification.Actions) > 0 {
			if encoded, marshalErr := json.Marshal(classification.Actions); marshalErr == nil {
				previousPlanJSON = string(encoded)
			}
		}
		hints := extractOpenRequestHints(trimmedText)
		hintText := "(none)"
		if len(hints) > 0 {
			hintText = strings.Join(hints, ", ")
		}
		retrySystemPrompt := buildIntentLLMSystemPromptForPolicy(policy) + "\n\nConstraint: for explicit open/show/display file requests you MUST return an actions array whose final step is open_file_canvas. If path is uncertain, include a shell search step first and then use path=\"$last_shell_path\"."
		retryUserPrompt := "User request:\n" + trimmedText + "\n\nExtracted filename hints:\n" + hintText + "\n\nPrevious invalid plan (missing open_file_canvas or empty):\n" + previousPlanJSON
		if repaired, repairErr := requestPlan(retrySystemPrompt, retryUserPrompt); repairErr == nil && len(repaired.Actions) > 0 {
			classification = repaired
		}
		if !planContainsAction(classification.Actions, "open_file_canvas") {
			classification.Actions = ensureOpenCanvasTerminalAction(classification.Actions)
		}
		if !planContainsAction(classification.Actions, "open_file_canvas") {
			return intentPlanClassification{}, nil
		}
	}
	return classification, nil
}

func (a *App) classifyIntentPlanWithLLM(ctx context.Context, text string) ([]*SystemAction, error) {
	classification, err := a.classifyIntentPlanWithLLMResult(ctx, text)
	if err != nil {
		return nil, err
	}
	if len(classification.Actions) == 0 {
		return nil, nil
	}
	return classification.Actions, nil
}

func (a *App) classifyIntentWithLLM(ctx context.Context, text string) (*SystemAction, error) {
	actions, err := a.classifyIntentPlanWithLLM(ctx, text)
	if err != nil {
		return nil, err
	}
	if len(actions) == 0 {
		return nil, nil
	}
	return actions[0], nil
}
