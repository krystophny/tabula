package web

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
)

func firstNonEmptyLocalAssistantChunk(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func visibleLocalAssistantText(message localIntentLLMMessage, enableThinking bool) string {
	content := strings.TrimSpace(stripLocalAssistantThinkingPreamble(message.Content))
	if content != "" || enableThinking {
		return content
	}
	fallback := firstNonEmptyLocalAssistantChunk(
		message.ReasoningContent,
		message.Reasoning,
	)
	return strings.TrimSpace(stripLocalAssistantThinkingPreamble(fallback))
}

func annotateLocalAssistantSafetyStop(raw string) string {
	clean := strings.TrimSpace(raw)
	if clean == "" {
		return "[stopped at local safety limit]"
	}
	return clean + "\n\n[stopped at local safety limit]"
}

func localAssistantVisibleStreamDelta(delta localIntentLLMStreamDelta, enableThinking bool) string {
	content := stripLocalAssistantThinkingPreamble(delta.Content)
	if content != "" || enableThinking {
		return content
	}
	fallback := firstNonEmptyLocalAssistantChunk(
		delta.ReasoningContent,
		delta.Reasoning,
	)
	return stripLocalAssistantThinkingPreamble(fallback)
}

func accumulateLocalAssistantToolDelta(calls map[int]*localAssistantLLMToolCall, delta localAssistantLLMToolCallDelta) {
	index := delta.Index
	call := calls[index]
	if call == nil {
		call = &localAssistantLLMToolCall{}
		calls[index] = call
	}
	if id := strings.TrimSpace(delta.ID); id != "" {
		call.ID = id
	}
	if call.Type == "" {
		call.Type = strings.TrimSpace(delta.Type)
	}
	if name := strings.TrimSpace(delta.Function.Name); name != "" {
		call.Function.Name += name
	}
	if arguments := delta.Function.Arguments; arguments != "" {
		call.Function.Arguments += arguments
	}
}

func orderedLocalAssistantToolCalls(calls map[int]*localAssistantLLMToolCall) []localAssistantLLMToolCall {
	if len(calls) == 0 {
		return nil
	}
	indexes := make([]int, 0, len(calls))
	for index := range calls {
		indexes = append(indexes, index)
	}
	slices.Sort(indexes)
	out := make([]localAssistantLLMToolCall, 0, len(indexes))
	for _, index := range indexes {
		call := calls[index]
		if call == nil {
			continue
		}
		if strings.TrimSpace(call.ID) == "" {
			call.ID = randomToken()
		}
		out = append(out, *call)
	}
	return out
}

func decodeLocalAssistantCompletionPayload(body io.Reader, enableThinking bool) (localIntentLLMMessage, string, error) {
	var payload localIntentLLMChatCompletionResponse
	if err := json.NewDecoder(io.LimitReader(body, assistantLLMResponseLimit)).Decode(&payload); err != nil {
		return localIntentLLMMessage{}, "", err
	}
	if len(payload.Choices) == 0 {
		return localIntentLLMMessage{}, "", errors.New("assistant llm returned no choices")
	}
	choice := payload.Choices[0]
	message := choice.Message
	message.Content = visibleLocalAssistantText(message, enableThinking)
	if strings.EqualFold(strings.TrimSpace(choice.FinishReason), "length") {
		message.Content = annotateLocalAssistantSafetyStop(message.Content)
	}
	return message, strings.TrimSpace(choice.FinishReason), nil
}

func decodeLocalAssistantStreamingPayload(body io.Reader, enableThinking bool, onDelta func(fullText string, delta string)) (localIntentLLMMessage, string, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), assistantLLMResponseLimit)
	message := localIntentLLMMessage{}
	toolCalls := map[int]*localAssistantLLMToolCall{}
	finishReason := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			break
		}
		var chunk localIntentLLMStreamChatCompletionResponse
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return localIntentLLMMessage{}, finishReason, err
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]
		if choice.FinishReason != "" {
			finishReason = strings.TrimSpace(choice.FinishReason)
		}
		if delta := localAssistantVisibleStreamDelta(choice.Delta, enableThinking); delta != "" {
			message.Content += delta
			if onDelta != nil {
				onDelta(message.Content, delta)
			}
		}
		if choice.Delta.FunctionCall != nil && strings.TrimSpace(choice.Delta.FunctionCall.Name) != "" {
			message.FunctionCall = choice.Delta.FunctionCall
		}
		for _, toolCall := range choice.Delta.ToolCalls {
			accumulateLocalAssistantToolDelta(toolCalls, toolCall)
		}
	}
	if err := scanner.Err(); err != nil {
		return localIntentLLMMessage{}, finishReason, err
	}
	message.ToolCalls = orderedLocalAssistantToolCalls(toolCalls)
	if strings.EqualFold(finishReason, "length") {
		message.Content = annotateLocalAssistantSafetyStop(message.Content)
	}
	return message, finishReason, nil
}

func (a *App) requestLocalAssistantCompletionWithConfig(ctx context.Context, messages []map[string]any, tools []map[string]any, toolChoice string, enableThinking bool, maxTokens int, onDelta func(fullText string, delta string)) (localIntentLLMMessage, error) {
	baseURL := a.assistantLLMBaseURL()
	if baseURL == "" {
		return localIntentLLMMessage{}, errLocalAssistantNotConfigured
	}
	if maxTokens <= 0 {
		maxTokens = assistantLLMToolMaxTokens
	}
	request := map[string]any{
		"model":       a.localAssistantLLMModel(),
		"temperature": 0,
		"max_tokens":  maxTokens,
		"stream":      true,
		"chat_template_kwargs": map[string]any{
			"enable_thinking": enableThinking,
		},
		"messages": messages,
	}
	if len(tools) > 0 {
		request["tools"] = tools
		request["tool_choice"] = firstNonEmptyCursorText(strings.TrimSpace(toolChoice), "auto")
	}
	requestBody, _ := json.Marshal(request)
	requestCtx, cancel := context.WithTimeout(ctx, assistantLLMRequestTimeout())
	defer cancel()
	req, err := http.NewRequestWithContext(
		requestCtx,
		http.MethodPost,
		baseURL+"/v1/chat/completions",
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return localIntentLLMMessage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return localIntentLLMMessage{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, assistantLLMResponseLimit))
		return localIntentLLMMessage{}, fmt.Errorf("assistant llm HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	contentType := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if strings.HasPrefix(contentType, "text/event-stream") {
		message, _, err := decodeLocalAssistantStreamingPayload(resp.Body, enableThinking, onDelta)
		return message, err
	}
	message, _, err := decodeLocalAssistantCompletionPayload(resp.Body, enableThinking)
	if err != nil {
		return localIntentLLMMessage{}, err
	}
	if onDelta != nil && strings.TrimSpace(message.Content) != "" {
		onDelta(message.Content, message.Content)
	}
	return message, nil
}
