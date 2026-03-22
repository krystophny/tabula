package web

import (
	"strings"

	"github.com/krystophny/tabura/internal/appserver"
)

const maxChatVisualDataURLBytes = 2 << 20

type chatVisualAttachment struct {
	DataURL string
}

type chatPromptInputItem struct {
	Type     string
	Text     string
	ImageURL string
}

func normalizeChatVisualDataURL(raw string) string {
	dataURL := strings.TrimSpace(raw)
	if dataURL == "" || len(dataURL) > maxChatVisualDataURLBytes {
		return ""
	}
	lower := strings.ToLower(dataURL)
	if !strings.HasPrefix(lower, "data:image/") {
		return ""
	}
	if !strings.Contains(lower, ";base64,") {
		return ""
	}
	return dataURL
}

func latestCanvasPositionVisualAttachment(events []*chatCanvasPositionEvent) *chatVisualAttachment {
	for i := len(events) - 1; i >= 0; i-- {
		event := normalizeChatCanvasPositionEvent(events[i])
		if event == nil {
			continue
		}
		if dataURL := normalizeChatVisualDataURL(event.SnapshotDataURL); dataURL != "" {
			return &chatVisualAttachment{DataURL: dataURL}
		}
	}
	return nil
}

func buildChatPromptInput(prompt string, visual *chatVisualAttachment) []chatPromptInputItem {
	items := []chatPromptInputItem{{
		Type: "text",
		Text: strings.TrimSpace(prompt),
	}}
	if visual != nil && visual.DataURL != "" {
		items = append(items, chatPromptInputItem{
			Type:     "image_url",
			ImageURL: visual.DataURL,
		})
	}
	return items
}

func buildLocalAssistantUserContent(prompt string, visual *chatVisualAttachment) []map[string]any {
	parts := buildChatPromptInput(prompt, visual)
	items := make([]map[string]any, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case "image_url":
			if strings.TrimSpace(part.ImageURL) == "" {
				continue
			}
			items = append(items, map[string]any{
				"type":      "image_url",
				"image_url": map[string]any{"url": part.ImageURL},
			})
		default:
			if strings.TrimSpace(part.Text) == "" {
				continue
			}
			items = append(items, map[string]any{
				"type": "text",
				"text": part.Text,
			})
		}
	}
	return items
}

func buildAppServerTurnInput(prompt string, visual *chatVisualAttachment) []map[string]interface{} {
	parts := buildChatPromptInput(prompt, visual)
	items := make([]appserver.TurnInputItem, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case "image_url":
			items = append(items, appserver.TurnInputItem{
				Type:     "image_url",
				ImageURL: part.ImageURL,
			})
		default:
			items = append(items, appserver.TurnInputItem{
				Type:         "text",
				Text:         part.Text,
				TextElements: []interface{}{},
			})
		}
	}
	return appserver.BuildTurnInput(items)
}
