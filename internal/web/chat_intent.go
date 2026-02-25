package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/krystophny/tabura/internal/store"
)

const (
	DefaultIntentClassifierURL     = "http://127.0.0.1:8425"
	intentClassifierMinConfidence  = 0.8
	intentClassifierRequestTimeout = 75 * time.Millisecond
	intentClassifierResponseLimit  = 64 * 1024
)

type localIntentClassifierResponse struct {
	Action     string                 `json:"action"`
	Intent     string                 `json:"intent"`
	Confidence float64                `json:"confidence"`
	Entities   map[string]interface{} `json:"entities"`
	Params     map[string]interface{} `json:"params"`
	Name       string                 `json:"name"`
	Alias      string                 `json:"alias"`
	Effort     string                 `json:"effort"`
}

type SystemAction struct {
	Action string                 `json:"action"`
	Params map[string]interface{} `json:"-"`
}

func parseSystemAction(raw string) (*SystemAction, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
		return nil, nil
	}
	action := strings.ToLower(strings.TrimSpace(fmt.Sprint(obj["action"])))
	if action == "" {
		return nil, nil
	}
	params := make(map[string]interface{}, len(obj))
	for key, value := range obj {
		if strings.EqualFold(strings.TrimSpace(key), "action") {
			continue
		}
		params[key] = value
	}
	return &SystemAction{Action: action, Params: params}, nil
}

func systemActionStringParam(params map[string]interface{}, key string) string {
	return strings.TrimSpace(fmt.Sprint(params[key]))
}

func normalizeSystemActionName(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "switch_project", "switch_model", "toggle_silent", "toggle_conversation", "cancel_work", "show_status":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
	}
}

func mergeSystemActionParams(target map[string]interface{}, source map[string]interface{}) {
	if source == nil {
		return
	}
	for key, value := range source {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		target[trimmed] = value
	}
}

func (a *App) classifyIntentLocally(ctx context.Context, text string) (*SystemAction, float64, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(a.intentClassifierURL), "/")
	if baseURL == "" {
		return nil, 0, nil
	}
	trimmedText := strings.TrimSpace(text)
	if trimmedText == "" {
		return nil, 0, nil
	}
	requestBody, _ := json.Marshal(map[string]string{"text": trimmedText})
	requestCtx, cancel := context.WithTimeout(ctx, intentClassifierRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(
		requestCtx,
		http.MethodPost,
		baseURL+"/classify",
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, intentClassifierResponseLimit))
		return nil, 0, fmt.Errorf("intent classifier HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload localIntentClassifierResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, intentClassifierResponseLimit)).Decode(&payload); err != nil {
		return nil, 0, err
	}
	actionName := normalizeSystemActionName(payload.Action)
	if actionName == "" {
		actionName = normalizeSystemActionName(payload.Intent)
	}
	if actionName == "" {
		return nil, payload.Confidence, nil
	}
	params := map[string]interface{}{}
	mergeSystemActionParams(params, payload.Entities)
	mergeSystemActionParams(params, payload.Params)
	if strings.TrimSpace(payload.Name) != "" {
		params["name"] = strings.TrimSpace(payload.Name)
	}
	if strings.TrimSpace(payload.Alias) != "" {
		params["alias"] = strings.TrimSpace(payload.Alias)
	}
	if strings.TrimSpace(payload.Effort) != "" {
		params["effort"] = strings.TrimSpace(payload.Effort)
	}
	return &SystemAction{Action: actionName, Params: params}, payload.Confidence, nil
}

func (a *App) executeSystemAction(sessionID string, session store.ChatSession, action *SystemAction) (string, map[string]interface{}, error) {
	if action == nil {
		return "", nil, errors.New("system action is required")
	}
	switch action.Action {
	case "switch_project":
		targetName := systemActionStringParam(action.Params, "name")
		project, err := a.hubFindProjectByName(targetName)
		if err != nil {
			return "", nil, err
		}
		activated, err := a.activateProject(project.ID)
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("Switched to %s.", activated.Name), map[string]interface{}{
			"type":       "switch_project",
			"project_id": activated.ID,
		}, nil
	case "switch_model":
		targetProject, err := a.hubPrimaryProject()
		if err != nil {
			return "", nil, err
		}
		updated, err := a.updateProjectChatModel(
			targetProject.ID,
			systemActionStringParam(action.Params, "alias"),
			systemActionStringParam(action.Params, "effort"),
		)
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf(
				"Model for %s set to %s (%s).",
				updated.Name,
				updated.ChatModel,
				updated.ChatModelReasoningEffort,
			), map[string]interface{}{
				"type":       "switch_model",
				"project_id": updated.ID,
				"alias":      updated.ChatModel,
				"effort":     updated.ChatModelReasoningEffort,
			}, nil
	case "toggle_silent":
		return "Toggled silent mode.", map[string]interface{}{"type": "toggle_silent"}, nil
	case "toggle_conversation":
		return "Toggled conversation mode.", map[string]interface{}{"type": "toggle_conversation"}, nil
	case "cancel_work":
		activeCanceled, queuedCanceled := a.cancelChatWork(sessionID)
		delegateCanceled := a.cancelDelegatedJobsForProject(session.ProjectKey)
		total := activeCanceled + queuedCanceled + delegateCanceled
		return fmt.Sprintf("Canceled %d running task(s).", total), nil, nil
	case "show_status":
		status, err := a.fetchCodexStatusMessage(session.ProjectKey)
		if err != nil {
			return "", nil, err
		}
		return status, nil, nil
	default:
		return "", nil, fmt.Errorf("unsupported action: %s", action.Action)
	}
}
