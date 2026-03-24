package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAssistantBackendForTurnRoutesLocalByDefaultAndCodexOnlyForRemoteTurns(t *testing.T) {
	wsServer := setupMockAppServerStatusServer(t, "codex")
	defer wsServer.Close()
	wsURL := "ws" + strings.TrimPrefix(wsServer.URL, "http")

	app, err := New(t.TempDir(), "", "", wsURL, "", "", "", false)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	app.assistantLLMURL = "http://127.0.0.1:8081"
	t.Cleanup(func() {
		_ = app.Shutdown(context.Background())
	})

	localReq := &assistantTurnRequest{
		userText:    "Wann wurde Isaac Newton geboren?",
		baseProfile: appServerModelProfile{Alias: "local"},
	}
	if got := app.assistantBackendForTurn(localReq).mode(); got != assistantModeLocal {
		t.Fatalf("backend for local default turn = %q, want %q", got, assistantModeLocal)
	}

	explicitRemoteReq := &assistantTurnRequest{
		userText:    "let gpt answer this",
		turnModel:   "gpt",
		baseProfile: appServerModelProfile{Alias: "local"},
	}
	if got := app.assistantBackendForTurn(explicitRemoteReq).mode(); got != assistantModeCodex {
		t.Fatalf("backend for explicit remote turn = %q, want %q", got, assistantModeCodex)
	}

	searchReq := &assistantTurnRequest{
		userText:    "search the web for today's news",
		searchTurn:  true,
		baseProfile: appServerModelProfile{Alias: "local"},
	}
	if got := app.assistantBackendForTurn(searchReq).mode(); got != assistantModeLocal {
		t.Fatalf("backend for local search turn = %q, want %q", got, assistantModeLocal)
	}

	app.assistantLLMURL = ""
	app.intentLLMURL = ""
	if got := app.assistantBackendForTurn(localReq).mode(); got != assistantModeCodex {
		t.Fatalf("backend without local assistant config = %q, want %q", got, assistantModeCodex)
	}
}

func TestParseLocalAssistantDecisionParsesNativeToolCalls(t *testing.T) {
	decision, err := parseLocalAssistantDecision(localIntentLLMMessage{
		ToolCalls: []localAssistantLLMToolCall{{
			ID:   "call-shell",
			Type: "function",
			Function: localAssistantLLMFunctionCall{
				Name:      "shell",
				Arguments: `{"command":"printf 'hi'"}`,
			},
		}},
	})
	if err != nil {
		t.Fatalf("parseLocalAssistantDecision() error: %v", err)
	}
	if len(decision.ToolCalls) != 1 {
		t.Fatalf("tool call count = %d, want 1", len(decision.ToolCalls))
	}
	if got := decision.ToolCalls[0].Name; got != "shell" {
		t.Fatalf("tool call name = %q, want shell", got)
	}
	if got := strings.TrimSpace(decision.ToolCalls[0].Arguments["command"].(string)); got != "printf 'hi'" {
		t.Fatalf("tool call command = %q, want printf 'hi'", got)
	}
}

func TestParseLocalAssistantDecisionParsesJSONToolEnvelope(t *testing.T) {
	decision, err := parseLocalAssistantDecision(localIntentLLMMessage{
		Content: `{"tool_calls":[{"name":"mcp__canvas_artifact_show","arguments":{"kind":"text","title":"Tool Test","markdown_or_text":"Orbit Canvas"}}]}`,
	})
	if err != nil {
		t.Fatalf("parseLocalAssistantDecision() error: %v", err)
	}
	if len(decision.ToolCalls) != 1 {
		t.Fatalf("tool call count = %d, want 1", len(decision.ToolCalls))
	}
	call := decision.ToolCalls[0]
	if call.Name != "mcp__canvas_artifact_show" {
		t.Fatalf("tool call name = %q, want mcp__canvas_artifact_show", call.Name)
	}
	if got := strings.TrimSpace(strFromAny(call.Arguments["kind"])); got != "text" {
		t.Fatalf("tool kind = %q, want text", got)
	}
	if got := strings.TrimSpace(strFromAny(call.Arguments["title"])); got != "Tool Test" {
		t.Fatalf("tool title = %q, want Tool Test", got)
	}
	if got := strings.TrimSpace(strFromAny(call.Arguments["markdown_or_text"])); got != "Orbit Canvas" {
		t.Fatalf("tool body = %q, want Orbit Canvas", got)
	}
}

func TestExecuteLocalAssistantShellToolTracksWorkingDirectory(t *testing.T) {
	workspaceDir := t.TempDir()
	subdir := workspaceDir + "/nested"
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	state := localAssistantTurnState{
		workspaceDir: workspaceDir,
		currentDir:   workspaceDir,
	}

	result := executeLocalAssistantShellTool(&state, localAssistantToolCall{
		ID:   "call-shell",
		Name: "shell",
		Arguments: map[string]any{
			"command": "cd nested && pwd",
		},
	})
	if result.IsError {
		t.Fatalf("shell tool returned error: %+v", result)
	}
	if got := strings.TrimSpace(result.Output); got != subdir {
		t.Fatalf("shell output = %q, want %q", got, subdir)
	}
	if got := state.currentDir; got != subdir {
		t.Fatalf("state currentDir = %q, want %q", got, subdir)
	}
}

func TestBuildLocalAssistantToolCatalogUsesExplicitMCPToolNamesAndBoundDefaults(t *testing.T) {
	var calls atomic.Int32
	mcp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode mcp payload: %v", err)
		}
		switch strings.TrimSpace(strFromAny(payload["method"])) {
		case "tools/list":
			calls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"tools": []map[string]any{{
						"name":        "echo_status",
						"description": "Echo a ready status.",
						"inputSchema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"status": map[string]any{"type": "string"},
							},
							"required": []string{"status"},
						},
					}, {
						"name":        "canvas_artifact_show",
						"description": "Show one artifact kind in canvas.",
						"inputSchema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"session_id": map[string]any{"type": "string"},
								"kind":       map[string]any{"type": "string"},
							},
							"required": []string{"session_id", "kind"},
						},
					}, {
						"name":        "temp_file_create",
						"description": "Create a temp file.",
						"inputSchema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"cwd":    map[string]any{"type": "string"},
								"prefix": map[string]any{"type": "string"},
							},
						},
					}},
				},
			})
		default:
			t.Fatalf("unexpected MCP method %q", payload["method"])
		}
	}))
	defer mcp.Close()

	app := newAuthedTestApp(t)
	state := localAssistantTurnState{
		sessionID:    "local-session",
		canvasID:     "canvas-session",
		workspaceDir: t.TempDir(),
		mcpURL:       mcp.URL,
	}
	catalog, err := app.buildLocalAssistantToolCatalog(state)
	if err != nil {
		t.Fatalf("buildLocalAssistantToolCatalog() error: %v", err)
	}
	echoTool, ok := catalog.ToolsByName["mcp__echo_status"]
	if !ok {
		t.Fatalf("missing explicit mcp__echo_status tool: %#v", catalog.ToolsByName)
	}
	if echoTool.InternalName != "echo_status" {
		t.Fatalf("echo tool internal name = %q, want echo_status", echoTool.InternalName)
	}
	canvasTool, ok := catalog.ToolsByName["mcp__canvas_artifact_show"]
	if !ok {
		t.Fatalf("missing explicit mcp__canvas_artifact_show tool")
	}
	if got := strings.TrimSpace(strFromAny(canvasTool.DefaultArgs["session_id"])); got != "canvas-session" {
		t.Fatalf("canvas session default = %q, want canvas-session", got)
	}
	tempTool, ok := catalog.ToolsByName["mcp__temp_file_create"]
	if !ok {
		t.Fatalf("missing explicit mcp__temp_file_create tool")
	}
	if got := strings.TrimSpace(strFromAny(tempTool.DefaultArgs["cwd"])); got != state.workspaceDir {
		t.Fatalf("temp file cwd default = %q, want %q", got, state.workspaceDir)
	}
	canvasTextTool, ok := catalog.ToolsByName["canvas_show_text"]
	if !ok {
		t.Fatalf("missing explicit canvas_show_text tool")
	}
	if got := strings.TrimSpace(strFromAny(canvasTextTool.DefaultArgs["session_id"])); got != "canvas-session" {
		t.Fatalf("canvas_show_text session default = %q, want canvas-session", got)
	}
	if calls.Load() != 1 {
		t.Fatalf("mcp list count = %d, want 1", calls.Load())
	}
}

func TestExecuteLocalAssistantBoundMCPToolUsesCanvasTunnelForCanvasTools(t *testing.T) {
	var listCalls atomic.Int32
	var canvasCalls atomic.Int32
	canvasMCP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode canvas mcp payload: %v", err)
		}
		switch strings.TrimSpace(strFromAny(payload["method"])) {
		case "tools/call":
			canvasCalls.Add(1)
			params, _ := payload["params"].(map[string]any)
			if got := strings.TrimSpace(strFromAny(params["name"])); got != "canvas_artifact_show" {
				t.Fatalf("canvas tool name = %q, want canvas_artifact_show", got)
			}
			args, _ := params["arguments"].(map[string]any)
			if got := strings.TrimSpace(strFromAny(args["session_id"])); got != "canvas-session" {
				t.Fatalf("canvas session_id = %q, want canvas-session", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"structuredContent": map[string]any{
						"ok": true,
					},
				},
			})
		default:
			t.Fatalf("unexpected canvas MCP method %q", payload["method"])
		}
	}))
	defer canvasMCP.Close()

	generalMCP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode general mcp payload: %v", err)
		}
		switch strings.TrimSpace(strFromAny(payload["method"])) {
		case "tools/list":
			listCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"tools": []map[string]any{{
						"name":        "canvas_artifact_show",
						"description": "Show one artifact kind in canvas.",
						"inputSchema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"session_id": map[string]any{"type": "string"},
								"kind":       map[string]any{"type": "string"},
							},
							"required": []string{"session_id", "kind"},
						},
					}},
				},
			})
		case "tools/call":
			t.Fatalf("canvas tools should not call the general MCP endpoint")
		default:
			t.Fatalf("unexpected general MCP method %q", payload["method"])
		}
	}))
	defer generalMCP.Close()

	app := newAuthedTestApp(t)
	port, err := extractPort(canvasMCP.URL)
	if err != nil {
		t.Fatalf("extractPort(canvasMCP): %v", err)
	}
	app.tunnels.setPort("canvas-session", port)
	state := localAssistantTurnState{
		canvasID:     "canvas-session",
		workspaceDir: t.TempDir(),
		mcpURL:       generalMCP.URL,
	}
	catalog, err := app.buildLocalAssistantToolCatalog(state)
	if err != nil {
		t.Fatalf("buildLocalAssistantToolCatalog() error: %v", err)
	}
	result, err := app.executeLocalAssistantToolCall(context.Background(), &state, catalog, localAssistantToolCall{
		ID:   "call-canvas",
		Name: "mcp__canvas_artifact_show",
		Arguments: map[string]any{
			"kind":             "text",
			"title":            "Tool Test",
			"markdown_or_text": "Orbit Canvas",
		},
	})
	if err != nil {
		t.Fatalf("executeLocalAssistantToolCall() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("canvas tool returned error: %+v", result)
	}
	if listCalls.Load() != 1 {
		t.Fatalf("general MCP list count = %d, want 1", listCalls.Load())
	}
	if canvasCalls.Load() != 1 {
		t.Fatalf("canvas MCP call count = %d, want 1", canvasCalls.Load())
	}
}

func TestAssistantLLMRequestTimeoutUsesEnvOverride(t *testing.T) {
	t.Setenv("TABURA_ASSISTANT_LLM_TIMEOUT", "")
	if got := assistantLLMRequestTimeout(); got != defaultAssistantLLMTimeout {
		t.Fatalf("assistantLLMRequestTimeout() default = %s, want %s", got, defaultAssistantLLMTimeout)
	}

	t.Setenv("TABURA_ASSISTANT_LLM_TIMEOUT", "45s")
	if got := assistantLLMRequestTimeout(); got != 45*time.Second {
		t.Fatalf("assistantLLMRequestTimeout() override = %s, want %s", got, 45*time.Second)
	}

	t.Setenv("TABURA_ASSISTANT_LLM_TIMEOUT", "nope")
	if got := assistantLLMRequestTimeout(); got != defaultAssistantLLMTimeout {
		t.Fatalf("assistantLLMRequestTimeout() invalid = %s, want %s", got, defaultAssistantLLMTimeout)
	}
}

func TestRunAssistantTurnFastLocalSkipsIntentEvalAndCapsOutput(t *testing.T) {
	var intentCalls atomic.Int32
	var llmCalls atomic.Int32

	intent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		intentCalls.Add(1)
		t.Fatalf("fast local turn should not call intent llm")
	}))
	defer intent.Close()

	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		llmCalls.Add(1)
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode llm payload: %v", err)
		}
		if got := intFromAny(payload["max_tokens"], -1); got != assistantLLMFastMaxTokens {
			t.Fatalf("fast local max_tokens = %d, want %d", got, assistantLLMFastMaxTokens)
		}
		messages, _ := payload["messages"].([]any)
		if len(messages) != 1 {
			t.Fatalf("fast local message count = %d, want 1", len(messages))
		}
		first, _ := messages[0].(map[string]any)
		if got := strings.TrimSpace(strFromAny(first["role"])); got != "user" {
			t.Fatalf("fast local first role = %q, want user", got)
		}
		gotPrompt := strings.TrimSpace(strFromAny(first["content"]))
		if !strings.Contains(gotPrompt, "User request:\nExplain me who you are") {
			t.Fatalf("fast local prompt = %q, want fast prompt wrapper with user request", gotPrompt)
		}
		if !strings.Contains(gotPrompt, "Answer in plain text only. Keep it brief: default to 1-3 short sentences.") {
			t.Fatalf("fast local prompt = %q, want brief fast guidance", gotPrompt)
		}
		templateKwargs, _ := payload["chat_template_kwargs"].(map[string]any)
		if got, ok := templateKwargs["enable_thinking"].(bool); !ok || got {
			t.Fatalf("fast local enable_thinking = %#v, want false", templateKwargs["enable_thinking"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"content": "Short direct reply.",
				},
			}},
		})
	}))
	defer llm.Close()

	app, err := New(t.TempDir(), "", "", "", "", "", "", false)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	app.assistantMode = assistantModeLocal
	app.assistantLLMURL = llm.URL
	app.intentLLMURL = intent.URL
	t.Cleanup(func() {
		_ = app.Shutdown(context.Background())
	})

	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensureDefaultWorkspace: %v", err)
	}
	session, err := app.chatSessionForWorkspace(project)
	if err != nil {
		t.Fatalf("chatSessionForWorkspace: %v", err)
	}
	if _, err := app.store.AddChatMessage(session.ID, "user", "Explain me who you are", "Explain me who you are", "text"); err != nil {
		t.Fatalf("AddChatMessage: %v", err)
	}

	app.runAssistantTurn(session.ID, dequeuedTurn{outputMode: turnOutputModeSilent, fastMode: true})

	if got := latestAssistantMessage(t, app, session.ID); got != "Short direct reply." {
		t.Fatalf("assistant message = %q, want direct fast reply", got)
	}
	if llmCalls.Load() != 1 {
		t.Fatalf("llm call count = %d, want 1", llmCalls.Load())
	}
	if intentCalls.Load() != 0 {
		t.Fatalf("intent llm call count = %d, want 0", intentCalls.Load())
	}
}

func TestRunAssistantTurnNonFastLocalUsesSinglePromptWithoutToolsForDirectReply(t *testing.T) {
	var intentCalls atomic.Int32
	var llmCalls atomic.Int32
	var mcpListCalls atomic.Int32

	intent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		intentCalls.Add(1)
		t.Fatalf("non-fast local turn should not call intent llm")
	}))
	defer intent.Close()

	mcp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mcpListCalls.Add(1)
		t.Fatalf("direct non-fast local turn should not fetch MCP tools")
	}))
	defer mcp.Close()

	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		llmCalls.Add(1)
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode llm payload: %v", err)
		}
		if got := intFromAny(payload["max_tokens"], -1); got != assistantLLMDirectMaxTokens {
			t.Fatalf("non-fast local max_tokens = %d, want %d", got, assistantLLMDirectMaxTokens)
		}
		tools, _ := payload["tools"].([]any)
		if len(tools) != 0 {
			t.Fatalf("non-fast local direct request tools = %d, want 0", len(tools))
		}
		messages, _ := payload["messages"].([]any)
		if len(messages) != 2 {
			t.Fatalf("non-fast local message count = %d, want 2", len(messages))
		}
		first, _ := messages[0].(map[string]any)
		if got := strings.TrimSpace(strFromAny(first["role"])); got != "system" {
			t.Fatalf("non-fast local first role = %q, want system", got)
		}
		if got := strings.TrimSpace(strFromAny(first["content"])); !strings.Contains(got, "No tools are needed for this request. Answer directly.") {
			t.Fatalf("non-fast local system prompt = %q, want no-tools instruction", got)
		}
		second, _ := messages[1].(map[string]any)
		if got := strings.TrimSpace(strFromAny(second["role"])); got != "user" {
			t.Fatalf("non-fast local second role = %q, want user", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"content": "Direct non-fast reply.",
				},
			}},
		})
	}))
	defer llm.Close()

	app, err := New(t.TempDir(), "", mcp.URL, "", "", "", "", false)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	app.assistantMode = assistantModeLocal
	app.assistantLLMURL = llm.URL
	app.intentLLMURL = intent.URL
	t.Cleanup(func() {
		_ = app.Shutdown(context.Background())
	})

	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensureDefaultWorkspace: %v", err)
	}
	session, err := app.chatSessionForWorkspace(project)
	if err != nil {
		t.Fatalf("chatSessionForWorkspace: %v", err)
	}
	if _, err := app.store.AddChatMessage(session.ID, "user", "Explain me who you are", "Explain me who you are", "text"); err != nil {
		t.Fatalf("AddChatMessage: %v", err)
	}

	app.runAssistantTurn(session.ID, dequeuedTurn{outputMode: turnOutputModeSilent})

	if got := latestAssistantMessage(t, app, session.ID); got != "Direct non-fast reply." {
		t.Fatalf("assistant message = %q, want direct non-fast reply", got)
	}
	if llmCalls.Load() != 1 {
		t.Fatalf("llm call count = %d, want 1", llmCalls.Load())
	}
	if mcpListCalls.Load() != 0 {
		t.Fatalf("mcp list call count = %d, want 0", mcpListCalls.Load())
	}
	if intentCalls.Load() != 0 {
		t.Fatalf("intent llm call count = %d, want 0", intentCalls.Load())
	}
}

func TestRunAssistantTurnNonFastLocalUsesPrunedExplicitToolPromptForCanvasRequest(t *testing.T) {
	var intentCalls atomic.Int32
	var llmCalls atomic.Int32
	var mcpListCalls atomic.Int32

	intent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		intentCalls.Add(1)
		t.Fatalf("non-fast local canvas turn should not call intent llm")
	}))
	defer intent.Close()

	mcp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode mcp payload: %v", err)
		}
		switch strings.TrimSpace(strFromAny(payload["method"])) {
		case "tools/list":
			mcpListCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"tools": []map[string]any{{
						"name":        "canvas_artifact_show",
						"description": "Show one artifact on canvas.",
						"inputSchema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"session_id": map[string]any{"type": "string"},
								"kind":       map[string]any{"type": "string"},
							},
						},
					}, {
						"name":        "temp_file_create",
						"description": "Create a temp file.",
						"inputSchema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"cwd":    map[string]any{"type": "string"},
								"prefix": map[string]any{"type": "string"},
							},
						},
					}, {
						"name":        "mail_message_list",
						"description": "List mail messages.",
						"inputSchema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"limit": map[string]any{"type": "integer"},
							},
						},
					}},
				},
			})
		default:
			t.Fatalf("unexpected MCP method %q", payload["method"])
		}
	}))
	defer mcp.Close()

	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		llmCalls.Add(1)
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode llm payload: %v", err)
		}
		if _, ok := payload["tools"]; ok {
			t.Fatal("canvas request should not send OpenAI tool definitions")
		}
		messages, _ := payload["messages"].([]any)
		system, _ := messages[0].(map[string]any)
		systemPrompt := strings.TrimSpace(strFromAny(system["content"]))
		for _, want := range []string{"Available tools in this turn:", "canvas_show_text", "mcp__canvas_artifact_show", "mcp__temp_file_create"} {
			if !strings.Contains(systemPrompt, want) {
				t.Fatalf("system prompt missing %q: %q", want, systemPrompt)
			}
		}
		if strings.Contains(systemPrompt, "mcp__mail_message_list") {
			t.Fatalf("system prompt should not include pruned mail tool: %q", systemPrompt)
		}
		if strings.Contains(systemPrompt, "action__toggle_silent") {
			t.Fatalf("system prompt should not include unrelated status tool: %q", systemPrompt)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"content": "Canvas tool prompt prepared.",
				},
			}},
		})
	}))
	defer llm.Close()

	app, err := New(t.TempDir(), "", mcp.URL, "", "", "", "", false)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	app.assistantMode = assistantModeLocal
	app.assistantLLMURL = llm.URL
	app.intentLLMURL = intent.URL
	t.Cleanup(func() {
		_ = app.Shutdown(context.Background())
	})

	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensureDefaultWorkspace: %v", err)
	}
	session, err := app.chatSessionForWorkspace(project)
	if err != nil {
		t.Fatalf("chatSessionForWorkspace: %v", err)
	}
	prompt := "Show a text artifact on canvas with the title Tool Test and body Orbit Canvas."
	if _, err := app.store.AddChatMessage(session.ID, "user", prompt, prompt, "text"); err != nil {
		t.Fatalf("AddChatMessage: %v", err)
	}

	app.runAssistantTurn(session.ID, dequeuedTurn{outputMode: turnOutputModeSilent})

	if got := latestAssistantMessage(t, app, session.ID); got != "Canvas tool prompt prepared." {
		t.Fatalf("assistant message = %q", got)
	}
	if llmCalls.Load() != 1 {
		t.Fatalf("llm call count = %d, want 1", llmCalls.Load())
	}
	if mcpListCalls.Load() != 1 {
		t.Fatalf("mcp list call count = %d, want 1", mcpListCalls.Load())
	}
	if intentCalls.Load() != 0 {
		t.Fatalf("intent llm call count = %d, want 0", intentCalls.Load())
	}
}

func TestRunAssistantTurnNonFastLocalRepairsPlanningTextIntoCanvasToolCall(t *testing.T) {
	var llmCalls atomic.Int32
	var mcpCalls atomic.Int32

	mcp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode mcp payload: %v", err)
		}
		switch strings.TrimSpace(strFromAny(payload["method"])) {
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"tools": []map[string]any{{
						"name":        "canvas_artifact_show",
						"description": "Show one artifact on canvas.",
						"inputSchema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"session_id":       map[string]any{"type": "string"},
								"kind":             map[string]any{"type": "string"},
								"title":            map[string]any{"type": "string"},
								"markdown_or_text": map[string]any{"type": "string"},
							},
						},
					}},
				},
			})
		case "tools/call":
			mcpCalls.Add(1)
			params, _ := payload["params"].(map[string]any)
			if got := strings.TrimSpace(strFromAny(params["name"])); got != "canvas_artifact_show" {
				t.Fatalf("tool name = %q, want canvas_artifact_show", got)
			}
			args, _ := params["arguments"].(map[string]any)
			if got := strings.TrimSpace(strFromAny(args["kind"])); got != "text" {
				t.Fatalf("tool kind = %q, want text", got)
			}
			if got := strings.TrimSpace(strFromAny(args["markdown_or_text"])); got != "Orbit Canvas" {
				t.Fatalf("tool body = %q, want Orbit Canvas", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"structuredContent": map[string]any{"ok": true},
				},
			})
		default:
			t.Fatalf("unexpected MCP method %q", payload["method"])
		}
	}))
	defer mcp.Close()

	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := llmCalls.Add(1)
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode llm payload: %v", err)
		}
		messages, _ := payload["messages"].([]any)
		switch call {
		case 1:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"message": map[string]any{
						"content": "I need to find a text artifact titled Tool Test and show it on canvas first.",
					},
				}},
			})
		case 2:
			last, _ := messages[len(messages)-1].(map[string]any)
			if got := strings.TrimSpace(strFromAny(last["content"])); !strings.Contains(got, "A tool is required for the user's request.") {
				t.Fatalf("repair prompt = %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"message": map[string]any{
						"tool_calls": []map[string]any{{
							"id":   "call-canvas",
							"type": "function",
							"function": map[string]any{
								"name":      "canvas_show_text",
								"arguments": `{"title":"Tool Test","text":"Orbit Canvas"}`,
							},
						}},
					},
				}},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"message": map[string]any{
						"content": "DONE",
					},
				}},
			})
		}
	}))
	defer llm.Close()

	app, err := New(t.TempDir(), "", mcp.URL, "", "", "", "", false)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	app.assistantMode = assistantModeLocal
	app.assistantLLMURL = llm.URL
	t.Cleanup(func() {
		_ = app.Shutdown(context.Background())
	})

	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensureDefaultWorkspace: %v", err)
	}
	session, err := app.chatSessionForWorkspace(project)
	if err != nil {
		t.Fatalf("chatSessionForWorkspace: %v", err)
	}
	prompt := "Render the exact text Orbit Canvas on the canvas with the title Tool Test. Use tools, then reply DONE."
	if _, err := app.store.AddChatMessage(session.ID, "user", prompt, prompt, "text"); err != nil {
		t.Fatalf("AddChatMessage: %v", err)
	}

	app.runAssistantTurn(session.ID, dequeuedTurn{outputMode: turnOutputModeSilent})

	if got := latestAssistantMessage(t, app, session.ID); got != "DONE" {
		t.Fatalf("assistant message = %q, want DONE", got)
	}
	if llmCalls.Load() != 3 {
		t.Fatalf("llm call count = %d, want 3", llmCalls.Load())
	}
	if mcpCalls.Load() != 1 {
		t.Fatalf("mcp call count = %d, want 1", mcpCalls.Load())
	}
}

func TestRunAssistantTurnLocalAssistantCompletesMultiToolLoop(t *testing.T) {
	var intentCalls atomic.Int32
	var llmCalls atomic.Int32
	mcpCalls := atomic.Int32{}

	intent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		intentCalls.Add(1)
		t.Fatalf("non-fast local tool loop should not call intent llm")
	}))
	defer intent.Close()

	mcp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode mcp payload: %v", err)
		}
		switch strings.TrimSpace(strFromAny(payload["method"])) {
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"tools": []map[string]any{{
						"name":        "echo_status",
						"description": "Echo a ready status.",
						"inputSchema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"status": map[string]any{"type": "string"},
							},
							"required": []string{"status"},
						},
					}},
				},
			})
		case "tools/call":
			mcpCalls.Add(1)
			params, _ := payload["params"].(map[string]any)
			if got := strings.TrimSpace(strFromAny(params["name"])); got != "echo_status" {
				t.Fatalf("tool name = %q, want echo_status", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"structuredContent": map[string]any{
						"ok":     true,
						"status": "ready",
					},
				},
			})
		default:
			t.Fatalf("unexpected MCP method %q", payload["method"])
		}
	}))
	defer mcp.Close()

	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := llmCalls.Add(1)
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode llm payload: %v", err)
		}
		messages, _ := payload["messages"].([]any)
		switch call {
		case 1:
			if got := intFromAny(payload["max_tokens"], -1); got != assistantLLMDirectMaxTokens {
				t.Fatalf("initial tool-aware max_tokens = %d, want %d", got, assistantLLMDirectMaxTokens)
			}
			if _, ok := payload["tools"]; ok {
				t.Fatal("initial local request should not send OpenAI tool definitions")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"message": map[string]any{
						"content": `{"tool_calls":[{"name":"shell","arguments":{"command":"printf 'shell-step'"}}]}`,
					},
				}},
			})
		case 2:
			if got := intFromAny(payload["max_tokens"], -1); got != assistantLLMToolMaxTokens {
				t.Fatalf("follow-up tool-aware max_tokens = %d, want %d", got, assistantLLMToolMaxTokens)
			}
			last, _ := messages[len(messages)-1].(map[string]any)
			if got := strings.TrimSpace(strFromAny(last["content"])); !strings.Contains(got, "shell-step") {
				t.Fatalf("second llm call missing shell output: %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"message": map[string]any{
						"content": `{"tool_calls":[{"name":"mcp__echo_status","arguments":{"status":"ready"}}]}`,
					},
				}},
			})
		default:
			if got := intFromAny(payload["max_tokens"], -1); got != assistantLLMToolMaxTokens {
				t.Fatalf("final tool-aware max_tokens = %d, want %d", got, assistantLLMToolMaxTokens)
			}
			last, _ := messages[len(messages)-1].(map[string]any)
			if got := strings.TrimSpace(strFromAny(last["content"])); !strings.Contains(got, `"status":"ready"`) {
				t.Fatalf("final llm call missing mcp result: %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"message": map[string]any{
						"content": "Local backend completed shell and MCP steps.",
					},
				}},
			})
		}
	}))
	defer llm.Close()

	app, err := New(t.TempDir(), "", mcp.URL, "", "", "", "", false)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	app.assistantMode = assistantModeLocal
	app.assistantLLMURL = llm.URL
	app.intentLLMURL = intent.URL
	t.Cleanup(func() {
		_ = app.Shutdown(context.Background())
	})

	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensureDefaultWorkspace: %v", err)
	}
	session, err := app.chatSessionForWorkspace(project)
	if err != nil {
		t.Fatalf("chatSessionForWorkspace: %v", err)
	}
	if _, err := app.store.AddChatMessage(session.ID, "user", "inspect the workspace and use MCP", "inspect the workspace and use MCP", "text"); err != nil {
		t.Fatalf("AddChatMessage: %v", err)
	}

	app.runAssistantTurn(session.ID, dequeuedTurn{outputMode: turnOutputModeSilent})

	if got := latestAssistantMessage(t, app, session.ID); got != "Local backend completed shell and MCP steps." {
		t.Fatalf("assistant message = %q", got)
	}
	if llmCalls.Load() != 3 {
		t.Fatalf("llm call count = %d, want 3", llmCalls.Load())
	}
	if mcpCalls.Load() != 1 {
		t.Fatalf("mcp call count = %d, want 1", mcpCalls.Load())
	}
	if intentCalls.Load() != 0 {
		t.Fatalf("intent llm call count = %d, want 0", intentCalls.Load())
	}
}

func TestRunAssistantTurnLocalAssistantRecoversMalformedToolCall(t *testing.T) {
	var llmCalls atomic.Int32

	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := llmCalls.Add(1)
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode llm payload: %v", err)
		}
		messages, _ := payload["messages"].([]any)
		switch call {
		case 1:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"message": map[string]any{
						"content": `{"tool_calls":[{"name":"shll","arguments":{"command":"printf 'broken'"}}]}`,
					},
				}},
			})
		case 2:
			last, _ := messages[len(messages)-1].(map[string]any)
			if got := strings.TrimSpace(strFromAny(last["content"])); !strings.Contains(got, "unsupported local assistant tool") {
				t.Fatalf("tool error content = %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"message": map[string]any{
						"content": `{"tool_calls":[{"name":"shell","arguments":{"command":"printf 'recovered'"}}]}`,
					},
				}},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"message": map[string]any{
						"content": "Recovered after repairing the malformed tool call.",
					},
				}},
			})
		}
	}))
	defer llm.Close()

	app, err := New(t.TempDir(), "", "", "", "", "", "", false)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	app.assistantMode = assistantModeLocal
	app.assistantLLMURL = llm.URL
	app.intentLLMURL = ""
	t.Cleanup(func() {
		_ = app.Shutdown(context.Background())
	})

	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensureDefaultWorkspace: %v", err)
	}
	session, err := app.chatSessionForWorkspace(project)
	if err != nil {
		t.Fatalf("chatSessionForWorkspace: %v", err)
	}
	if _, err := app.store.AddChatMessage(session.ID, "user", "run the local tool", "run the local tool", "text"); err != nil {
		t.Fatalf("AddChatMessage: %v", err)
	}

	app.runAssistantTurn(session.ID, dequeuedTurn{outputMode: turnOutputModeSilent})

	if got := latestAssistantMessage(t, app, session.ID); got != "Recovered after repairing the malformed tool call." {
		t.Fatalf("assistant message = %q", got)
	}
	if llmCalls.Load() != 3 {
		t.Fatalf("llm call count = %d, want 3", llmCalls.Load())
	}
}
