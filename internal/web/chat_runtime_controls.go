package web

import (
	"strings"
)

func parseInlineRuntimeControlIntent(text string) *SystemAction {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	lower := strings.ToLower(trimmed)
	switch lower {
	case "be quiet", "quiet please", "go quiet", "toggle silent mode":
		return &SystemAction{Action: "toggle_silent", Params: map[string]interface{}{}}
	case "toggle live dialogue", "toggle dialogue mode", "toggle dialogue":
		return &SystemAction{Action: "toggle_live_dialogue", Params: map[string]interface{}{}}
	case "cancel work", "cancel current work", "stop work", "stop current work", "stop current task", "abort current task":
		return &SystemAction{Action: "cancel_work", Params: map[string]interface{}{}}
	case "status", "status?", "show status", "show me status", "what's your status", "what is your status":
		return &SystemAction{Action: "show_status", Params: map[string]interface{}{}}
	default:
		return nil
	}
}
