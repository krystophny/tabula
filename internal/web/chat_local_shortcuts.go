package web

import "strings"

type localAssistantToolFamily string

const (
	localAssistantToolFamilyNone      localAssistantToolFamily = ""
	localAssistantToolFamilyCanvas    localAssistantToolFamily = "canvas"
	localAssistantToolFamilyWorkspace localAssistantToolFamily = "workspace"
	localAssistantToolFamilyShell     localAssistantToolFamily = "shell"
	localAssistantToolFamilyMail      localAssistantToolFamily = "mail"
	localAssistantToolFamilyCalendar  localAssistantToolFamily = "calendar"
	localAssistantToolFamilyItems     localAssistantToolFamily = "items"
	localAssistantToolFamilyRuntime   localAssistantToolFamily = "runtime"
	localAssistantToolFamilyWeb       localAssistantToolFamily = "web"
)

func normalizeLocalAssistantAddress(text string) string {
	clean := strings.TrimSpace(text)
	if clean == "" {
		return ""
	}
	lower := strings.ToLower(clean)
	for _, name := range []string{"tabura", "sloppy", "computer"} {
		for _, prefix := range []string{name + " ", name + ",", name + ":", name + ";"} {
			if strings.HasPrefix(lower, prefix) {
				return strings.TrimSpace(clean[len(prefix):])
			}
		}
		if lower == name {
			return ""
		}
	}
	return clean
}

func selectLocalAssistantToolFamily(text string) localAssistantToolFamily {
	lower := strings.ToLower(normalizeLocalAssistantAddress(text))
	if lower == "" {
		return localAssistantToolFamilyNone
	}
	switch {
	case containsAnyLocalAssistantKeyword(lower, "website", "web search", "browse ", "latest ", "news", "search the web"):
		return localAssistantToolFamilyWeb
	case containsAnyLocalAssistantKeyword(lower, "go silent", "be silent", "stop talking", "status", "busy", "dialogue mode", "meeting mode", "live dialogue", "cancel work"):
		return localAssistantToolFamilyRuntime
	case containsAnyLocalAssistantKeyword(lower, "mail", "email", "inbox", "message", "archive", "unread", "label", "folder"):
		return localAssistantToolFamilyMail
	case containsAnyLocalAssistantKeyword(lower, "calendar", "event", "invite", "schedule"):
		return localAssistantToolFamilyCalendar
	case containsAnyLocalAssistantKeyword(lower, "item", "task", "todo", "actor", "delegate", "snooze", "waiting", "someday"):
		return localAssistantToolFamilyItems
	case containsAnyLocalAssistantKeyword(lower, "shell", "terminal", "command", "bash", "zsh", "rg ", "grep ", "find ", "ls ", "cat ", "pwd", "git "):
		return localAssistantToolFamilyShell
	case containsAnyLocalAssistantKeyword(lower, "canvas", "draw ", "render ", "display ", "show ", "open ") &&
		containsAnyLocalAssistantKeyword(lower, "canvas", "artifact", "diagram", "flowchart", "schematic", "readme", "file", "document", "pdf", "image", "text"):
		return localAssistantToolFamilyCanvas
	case containsAnyLocalAssistantKeyword(lower, "directory", "folder", "workspace", "project", "path", "read file", "list files", "what files", "readme", "file"):
		return localAssistantToolFamilyWorkspace
	default:
		return localAssistantToolFamilyNone
	}
}

func containsAnyLocalAssistantKeyword(text string, keywords ...string) bool {
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}
