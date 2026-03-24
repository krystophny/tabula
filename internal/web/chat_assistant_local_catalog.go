package web

import (
	"fmt"
	"slices"
	"strings"
)

type localAssistantToolKind string

const (
	localAssistantToolKindShell                localAssistantToolKind = "shell"
	localAssistantToolKindSystemAction         localAssistantToolKind = "system_action"
	localAssistantToolKindMCP                  localAssistantToolKind = "mcp"
	localAssistantToolKindCanvasText           localAssistantToolKind = "canvas_text"
	localAssistantToolKindWebSearchUnavailable localAssistantToolKind = "web_search_unavailable"
)

type localAssistantExecutableTool struct {
	ModelName    string
	Kind         localAssistantToolKind
	InternalName string
	DefaultArgs  map[string]any
	Definition   map[string]any
}

type localAssistantToolCatalog struct {
	Definitions []map[string]any
	ToolsByName map[string]localAssistantExecutableTool
}

func (a *App) buildLocalAssistantToolCatalog(state localAssistantTurnState) (localAssistantToolCatalog, error) {
	out := localAssistantToolCatalog{
		Definitions: make([]map[string]any, 0, 8),
		ToolsByName: map[string]localAssistantExecutableTool{},
	}
	for _, tool := range append(
		[]localAssistantExecutableTool{
			localAssistantShellTool(),
			localAssistantCanvasShowTextTool(state),
			localAssistantWebSearchUnavailableTool(),
		},
		localAssistantSystemActionTools()...,
	) {
		out.add(tool)
	}
	if strings.TrimSpace(state.mcpURL) == "" {
		return out, nil
	}
	mcpTools, err := mcpToolsListURL(state.mcpURL)
	if err != nil {
		return localAssistantToolCatalog{}, err
	}
	for _, tool := range localAssistantMCPTools(state, mcpTools) {
		out.add(tool)
	}
	return out, nil
}

func localAssistantNeedsTools(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return false
	}
	keywords := []string{
		"tool",
		"canvas",
		"show ",
		"open ",
		"display ",
		"draw ",
		"render ",
		"file",
		"folder",
		"directory",
		"path",
		"workspace",
		"project",
		"artifact",
		"calendar",
		"event",
		"mail",
		"email",
		"inbox",
		"item",
		"task",
		"todo",
		"shell",
		"command",
		"terminal",
		"run ",
		"latest",
		"website",
		"web search",
		"news",
	}
	for _, keyword := range keywords {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}

func pruneLocalAssistantToolCatalog(catalog localAssistantToolCatalog, text string) localAssistantToolCatalog {
	if len(catalog.Definitions) == 0 {
		return catalog
	}
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return localAssistantToolCatalog{}
	}
	wantsCanvas := containsAnyLocalAssistantKeyword(lower, "canvas", "show ", "open ", "display ", "draw ", "render ", "image", "pdf", "document", "artifact")
	wantsWorkspace := containsAnyLocalAssistantKeyword(lower, "workspace", "project", "folder", "directory", "file", "path")
	wantsMail := containsAnyLocalAssistantKeyword(lower, "mail", "email", "inbox", "message")
	wantsCalendar := containsAnyLocalAssistantKeyword(lower, "calendar", "event", "meeting")
	wantsItems := containsAnyLocalAssistantKeyword(lower, "item", "task", "todo", "actor", "note", "idea")
	wantsShell := containsAnyLocalAssistantKeyword(lower, "shell", "command", "terminal", "run ", "list ", "find ", "grep", "rg ")
	wantsMCP := containsAnyLocalAssistantKeyword(lower, "mcp")
	wantsWeb := containsAnyLocalAssistantKeyword(lower, "website", "web search", "latest", "news")
	wantsStatus := containsAnyLocalAssistantKeyword(lower, "status", "silent", "dialogue", "meeting mode")

	out := localAssistantToolCatalog{
		Definitions: make([]map[string]any, 0, len(catalog.Definitions)),
		ToolsByName: map[string]localAssistantExecutableTool{},
	}
	for name, tool := range catalog.ToolsByName {
		if localAssistantIncludeTool(tool, wantsCanvas, wantsWorkspace, wantsMail, wantsCalendar, wantsItems, wantsShell, wantsMCP, wantsWeb, wantsStatus) {
			out.add(tool)
		}
		if strings.HasPrefix(name, "mcp__canvas_") && wantsCanvas {
			out.add(tool)
		}
	}
	return out
}

func containsAnyLocalAssistantKeyword(text string, keywords ...string) bool {
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func localAssistantIncludeTool(tool localAssistantExecutableTool, wantsCanvas, wantsWorkspace, wantsMail, wantsCalendar, wantsItems, wantsShell, wantsMCP, wantsWeb, wantsStatus bool) bool {
	if tool.Kind == localAssistantToolKindShell {
		return wantsShell || wantsCanvas || wantsWorkspace
	}
	if tool.Kind == localAssistantToolKindCanvasText {
		return wantsCanvas
	}
	if tool.Kind == localAssistantToolKindWebSearchUnavailable {
		return wantsWeb
	}
	if tool.Kind == localAssistantToolKindMCP && wantsMCP {
		return true
	}
	name := tool.InternalName
	switch {
	case strings.HasPrefix(name, "canvas_"), strings.HasPrefix(name, "temp_file_"):
		return wantsCanvas
	case strings.HasPrefix(name, "workspace_"):
		return wantsWorkspace
	case strings.HasPrefix(name, "artifact_"):
		return wantsWorkspace
	case strings.HasPrefix(name, "mail_"), strings.HasPrefix(name, "handoff."):
		return wantsMail
	case strings.HasPrefix(name, "calendar_"):
		return wantsCalendar
	case strings.HasPrefix(name, "item_"), strings.HasPrefix(name, "actor_"):
		return wantsItems
	}
	switch name {
	case "open_file_canvas", "navigate_canvas", "cursor_open_path":
		return wantsCanvas || wantsWorkspace
	case "show_status", "show_busy_state", "toggle_silent", "toggle_live_dialogue":
		return wantsStatus
	case "make_item", "delegate_item", "snooze_item", "split_items", "print_item":
		return wantsItems
	case "show_calendar", "create_calendar_event", "update_calendar_event", "delete_calendar_event":
		return wantsCalendar
	}
	return false
}

func (c *localAssistantToolCatalog) add(tool localAssistantExecutableTool) {
	if c == nil || strings.TrimSpace(tool.ModelName) == "" || tool.Definition == nil {
		return
	}
	c.Definitions = append(c.Definitions, tool.Definition)
	c.ToolsByName[tool.ModelName] = tool
}

func localAssistantShellTool() localAssistantExecutableTool {
	return localAssistantExecutableTool{
		ModelName:    "shell",
		Kind:         localAssistantToolKindShell,
		InternalName: "shell",
		Definition: map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "shell",
				"description": "Run a shell command inside the active workspace and inspect or modify files there.",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"command": map[string]any{
							"type":        "string",
							"description": "Shell command to execute.",
						},
						"cwd": map[string]any{
							"type":        "string",
							"description": "Optional relative or absolute directory inside the active workspace.",
						},
					},
					"required": []string{"command"},
				},
			},
		},
	}
}

func localAssistantCanvasShowTextTool(state localAssistantTurnState) localAssistantExecutableTool {
	if strings.TrimSpace(state.canvasID) == "" {
		return localAssistantExecutableTool{}
	}
	return localAssistantExecutableTool{
		ModelName:    "canvas_show_text",
		Kind:         localAssistantToolKindCanvasText,
		InternalName: "canvas_artifact_show",
		DefaultArgs: map[string]any{
			"session_id": state.canvasID,
			"kind":       "text",
		},
		Definition: map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "canvas_show_text",
				"description": "Show plain text directly on canvas. Use this for requests like show or draw this text on canvas. Do not search for an existing artifact first.",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"title": map[string]any{
							"type":        "string",
							"description": "Optional canvas title.",
						},
						"text": map[string]any{
							"type":        "string",
							"description": "The exact text to show on canvas.",
						},
					},
					"required": []string{"text"},
				},
			},
		},
	}
}

func localAssistantWebSearchUnavailableTool() localAssistantExecutableTool {
	return localAssistantExecutableTool{
		ModelName:    "web_search_unavailable",
		Kind:         localAssistantToolKindWebSearchUnavailable,
		InternalName: "web_search_unavailable",
		Definition: map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "web_search_unavailable",
				"description": "Call this when the user asks for websites, latest news, or web search. Local mode cannot browse websites yet.",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{
							"type":        "string",
							"description": "The requested website lookup or web search query.",
						},
					},
				},
			},
		},
	}
}

func localAssistantSystemActionTools() []localAssistantExecutableTool {
	names := append([]string(nil), supportedSystemActionNames...)
	slices.Sort(names)
	out := make([]localAssistantExecutableTool, 0, len(names)-1)
	for _, action := range names {
		if action == "shell" {
			continue
		}
		out = append(out, localAssistantExecutableTool{
			ModelName:    "action__" + action,
			Kind:         localAssistantToolKindSystemAction,
			InternalName: action,
			Definition: map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        "action__" + action,
					"description": localAssistantSystemActionDescription(action),
					"parameters":  localAssistantSystemActionSchema(action),
				},
			},
		})
	}
	return out
}

func localAssistantSystemActionDescription(action string) string {
	switch action {
	case "open_file_canvas":
		return "Open an existing workspace file on canvas."
	case "navigate_canvas":
		return "Move between visible canvas pages or artifacts."
	case "toggle_silent":
		return "Toggle silent mode."
	case "toggle_live_dialogue":
		return "Toggle dialogue or meeting live mode."
	case "show_status":
		return "Show the current runtime or workspace status."
	case "show_busy_state":
		return "Show the current busy state."
	case "clear_workspace":
		return "Clear the current workspace conversation state."
	case "cursor_open_path":
		return "Open a specific file path in the current cursor context."
	case "cursor_open_item":
		return "Open an item in the cursor context."
	case "cursor_triage_item":
		return "Triage an item in the cursor context."
	default:
		return "Execute native Tabura action " + action + "."
	}
}

func localAssistantSystemActionSchema(action string) map[string]any {
	switch action {
	case "toggle_silent", "toggle_live_dialogue", "cancel_work", "show_busy_state", "show_status":
		return map[string]any{"type": "object"}
	case "open_file_canvas", "cursor_open_path":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":   map[string]any{"type": "string", "description": "Relative or absolute path to open."},
				"file":   map[string]any{"type": "string", "description": "Alternate file path field."},
				"target": map[string]any{"type": "string", "description": "Alternate target path field."},
			},
		}
	case "navigate_canvas":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"direction": map[string]any{"type": "string", "description": "Navigation direction such as next or previous."},
				"scope":     map[string]any{"type": "string", "description": "Optional navigation scope."},
			},
			"required": []string{"direction"},
		}
	case "cursor_open_item", "cursor_triage_item", "delegate_item", "snooze_item", "print_item":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"item_id": map[string]any{"type": "integer", "description": "Target item id."},
				"title":   map[string]any{"type": "string", "description": "Target item title when id is unknown."},
			},
		}
	default:
		return map[string]any{"type": "object"}
	}
}

func localAssistantMCPTools(state localAssistantTurnState, tools []mcpListedTool) []localAssistantExecutableTool {
	out := make([]localAssistantExecutableTool, 0, len(tools))
	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) == "" {
			continue
		}
		defaultArgs := localAssistantMCPDefaultArgs(state, tool.Name)
		out = append(out, localAssistantExecutableTool{
			ModelName:    localAssistantMCPModelName(tool.Name),
			Kind:         localAssistantToolKindMCP,
			InternalName: tool.Name,
			DefaultArgs:  defaultArgs,
			Definition: map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        localAssistantMCPModelName(tool.Name),
					"description": localAssistantMCPDescription(tool.Name, tool.Description, defaultArgs),
					"parameters":  localAssistantVisibleSchema(tool.InputSchema, defaultArgs),
				},
			},
		})
	}
	return out
}

func localAssistantMCPDefaultArgs(state localAssistantTurnState, name string) map[string]any {
	defaults := map[string]any{}
	if strings.HasPrefix(name, "canvas_") && strings.TrimSpace(state.canvasID) != "" {
		defaults["session_id"] = state.canvasID
	}
	switch name {
	case "temp_file_create", "temp_file_remove":
		defaults["cwd"] = state.workspaceDir
	}
	return defaults
}

func localAssistantMCPModelName(name string) string {
	return "mcp__" + sanitizeLocalAssistantToolToken(name)
}

func sanitizeLocalAssistantToolToken(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastUnderscore = false
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
			lastUnderscore = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastUnderscore = false
		default:
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	return strings.Trim(b.String(), "_")
}

func localAssistantMCPDescription(name, description string, defaults map[string]any) string {
	desc := strings.TrimSpace(description)
	switch name {
	case "canvas_artifact_show":
		desc = "Show an artifact on canvas. Use this for canvas display."
	case "temp_file_create":
		desc = "Create a temporary file under .tabura/artifacts/tmp for file-backed canvas content."
	case "temp_file_remove":
		desc = "Remove a temporary canvas file."
	}
	if len(defaults) == 0 {
		return desc
	}
	bound := make([]string, 0, len(defaults))
	for key := range defaults {
		bound = append(bound, key)
	}
	slices.Sort(bound)
	return strings.TrimSpace(desc + " Runtime-bound arguments: " + strings.Join(bound, ", ") + ".")
}

func localAssistantVisibleSchema(schema map[string]any, defaults map[string]any) map[string]any {
	out := cloneLocalAssistantMap(schema)
	if out == nil {
		out = map[string]any{"type": "object"}
	}
	props, _ := out["properties"].(map[string]any)
	if props != nil {
		props = cloneLocalAssistantMap(props)
		for key := range defaults {
			delete(props, key)
		}
		if len(props) > 0 {
			out["properties"] = props
		} else {
			delete(out, "properties")
		}
	}
	if required, ok := out["required"].([]any); ok {
		filtered := make([]any, 0, len(required))
		for _, item := range required {
			key := strings.TrimSpace(fmt.Sprint(item))
			if key == "" {
				continue
			}
			if _, hidden := defaults[key]; hidden {
				continue
			}
			filtered = append(filtered, key)
		}
		if len(filtered) > 0 {
			out["required"] = filtered
		} else {
			delete(out, "required")
		}
	}
	if required, ok := out["required"].([]string); ok {
		filtered := make([]string, 0, len(required))
		for _, key := range required {
			if _, hidden := defaults[key]; hidden {
				continue
			}
			filtered = append(filtered, key)
		}
		if len(filtered) > 0 {
			out["required"] = filtered
		} else {
			delete(out, "required")
		}
	}
	out["type"] = "object"
	return out
}

func cloneLocalAssistantMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		switch typed := value.(type) {
		case map[string]any:
			out[key] = cloneLocalAssistantMap(typed)
		default:
			out[key] = value
		}
	}
	return out
}

func mergeLocalAssistantToolArguments(defaults, args map[string]any) map[string]any {
	out := make(map[string]any, len(defaults)+len(args))
	for key, value := range args {
		out[key] = value
	}
	for key, value := range defaults {
		out[key] = value
	}
	return out
}

func buildLocalAssistantToolPolicy(catalog localAssistantToolCatalog) string {
	if len(catalog.Definitions) == 0 {
		return "No tools are needed for this request. Answer directly."
	}
	lines := []string{
		"Tool policy:",
		"- If you need a tool, respond with JSON only: {\"tool_calls\":[{\"name\":\"tool_name\",\"arguments\":{...}}]}. Do not add prose before or after that JSON.",
		"- After tool results are returned, either respond with another tool_calls JSON object or with the final plain-text answer.",
		"- For files or code in the active workspace, use shell or the matching mcp__ tool instead of guessing.",
		"- For plain text that should appear on canvas, use canvas_show_text. Do not describe a plan and do not search for an existing artifact first.",
		"- For show, open, display, or draw requests on canvas, use tools. For generated canvas content, create a file with mcp__temp_file_create and show it with mcp__canvas_artifact_show.",
		"- To open an existing workspace file on canvas directly, use action__open_file_canvas.",
		"- Use mcp__... tools for canvas, artifacts, workspace, items, calendar, mail, actors, and other MCP-backed operations.",
		"- Use action__... tools for native Tabura runtime actions such as toggles, cursor actions, status, or opening an existing file on canvas.",
		"- If the user asks for websites, latest news, or web search, call web_search_unavailable and then explain the limitation briefly.",
		"- When a tool is clearly required, never reply with a plan such as I need to, let me, or first I will. Call the tool instead.",
		fmt.Sprintf("- Tool count: %d explicit tools are available in this turn.", len(catalog.Definitions)),
	}
	lines = append(lines, localAssistantToolCatalogLines(catalog)...)
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func localAssistantToolCatalogLines(catalog localAssistantToolCatalog) []string {
	if len(catalog.ToolsByName) == 0 {
		return nil
	}
	names := make([]string, 0, len(catalog.ToolsByName))
	for name := range catalog.ToolsByName {
		names = append(names, name)
	}
	slices.Sort(names)
	lines := []string{"Available tools in this turn:"}
	for _, name := range names {
		tool := catalog.ToolsByName[name]
		lines = append(lines, fmt.Sprintf("- %s: %s", name, localAssistantToolSummary(tool)))
	}
	return lines
}

func localAssistantToolSummary(tool localAssistantExecutableTool) string {
	function, _ := tool.Definition["function"].(map[string]any)
	desc := strings.TrimSpace(fmt.Sprint(function["description"]))
	desc = strings.ReplaceAll(desc, "\n", " ")
	desc = strings.Join(strings.Fields(desc), " ")
	if desc == "" || desc == "<nil>" {
		switch tool.Kind {
		case localAssistantToolKindShell:
			return "Run a shell command in the active workspace."
		case localAssistantToolKindSystemAction:
			return "Execute a native Tabura action."
		case localAssistantToolKindMCP:
			return "Call an MCP-backed tool."
		case localAssistantToolKindWebSearchUnavailable:
			return "Report that web search is unavailable in local mode."
		default:
			return "Execute this tool when needed."
		}
	}
	return desc
}
