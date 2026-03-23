package web

import (
	"net/http"
	"testing"
	"time"

	"github.com/krystophny/tabura/internal/store"
)

func TestLocalSystemActionTurnPublishesLocalProviderMetadata(t *testing.T) {
	app := newAuthedTestApp(t)
	app.intentLLMURL = ""

	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensureDefaultWorkspace: %v", err)
	}
	session, err := app.store.GetOrCreateChatSession(project.WorkspacePath)
	if err != nil {
		t.Fatalf("GetOrCreateChatSession: %v", err)
	}

	conn, clientConn, cleanup := newParticipantTestWSConn(t)
	defer cleanup()
	app.hub.registerChat(session.ID, conn)
	defer app.hub.unregisterChat(session.ID, conn)

	if handled := app.tryRunLocalSystemActionTurn(session.ID, session, "clear focus", nil, "", turnOutputModeVoice, false); !handled {
		t.Fatal("expected local system action turn to be handled")
	}

	payload := waitForWSJSONMessageType(t, clientConn, 2*time.Second, "assistant_output")
	if got := strFromAny(payload["provider"]); got != assistantProviderLocal {
		t.Fatalf("provider = %q, want %q", got, assistantProviderLocal)
	}
	if got := strFromAny(payload["provider_label"]); got != "Local" {
		t.Fatalf("provider_label = %q, want Local", got)
	}
	if got := strFromAny(payload["provider_model"]); got != app.localAssistantModelLabel() {
		t.Fatalf("provider_model = %q, want %q", got, app.localAssistantModelLabel())
	}
	if got := intFromAny(payload["provider_latency_ms"], -1); got < 0 {
		t.Fatalf("provider_latency_ms = %d, want >= 0", got)
	}

	messages, err := app.store.ListChatMessages(session.ID, 10)
	if err != nil {
		t.Fatalf("ListChatMessages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(messages))
	}
	if got := messages[0].Provider; got != assistantProviderLocal {
		t.Fatalf("stored provider = %q, want %q", got, assistantProviderLocal)
	}
	if got := messages[0].ProviderModel; got != app.localAssistantModelLabel() {
		t.Fatalf("stored provider_model = %q, want %q", got, app.localAssistantModelLabel())
	}
}

func TestFinalizeAssistantResponseWithMetadataPublishesProviderMetadata(t *testing.T) {
	app := newAuthedTestApp(t)

	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensureDefaultWorkspace: %v", err)
	}
	session, err := app.store.GetOrCreateChatSession(project.WorkspacePath)
	if err != nil {
		t.Fatalf("GetOrCreateChatSession: %v", err)
	}

	conn, clientConn, cleanup := newParticipantTestWSConn(t)
	defer cleanup()
	app.hub.registerChat(session.ID, conn)
	defer app.hub.unregisterChat(session.ID, conn)

	persistedAssistantID := int64(0)
	persistedAssistantText := ""
	metadata := assistantResponseMetadata{
		Provider:        assistantProviderOpenAI,
		ProviderModel:   "gpt-5.3-codex-spark",
		ProviderLatency: 321,
	}
	response := app.finalizeAssistantResponseWithMetadata(
		session.ID,
		project.WorkspacePath,
		"OpenAI reply.",
		&persistedAssistantID,
		&persistedAssistantText,
		"turn-openai",
		"",
		"thread-openai",
		turnOutputModeVoice,
		metadata,
	)
	if response != "OpenAI reply." {
		t.Fatalf("response = %q, want OpenAI reply.", response)
	}

	payload := waitForWSJSONMessageType(t, clientConn, 2*time.Second, "assistant_output")
	if got := strFromAny(payload["provider"]); got != assistantProviderOpenAI {
		t.Fatalf("provider = %q, want %q", got, assistantProviderOpenAI)
	}
	if got := strFromAny(payload["provider_label"]); got != "Spark" {
		t.Fatalf("provider_label = %q, want Spark", got)
	}
	if got := strFromAny(payload["provider_model"]); got != metadata.ProviderModel {
		t.Fatalf("provider_model = %q, want %q", got, metadata.ProviderModel)
	}
	if got := intFromAny(payload["provider_latency_ms"], -1); got != metadata.ProviderLatency {
		t.Fatalf("provider_latency_ms = %d, want %d", got, metadata.ProviderLatency)
	}

	messages, err := app.store.ListChatMessages(session.ID, 10)
	if err != nil {
		t.Fatalf("ListChatMessages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(messages))
	}
	if got := messages[0].Provider; got != assistantProviderOpenAI {
		t.Fatalf("stored provider = %q, want %q", got, assistantProviderOpenAI)
	}
	if got := messages[0].ProviderModel; got != metadata.ProviderModel {
		t.Fatalf("stored provider_model = %q, want %q", got, metadata.ProviderModel)
	}
	if got := messages[0].ProviderLatency; got != metadata.ProviderLatency {
		t.Fatalf("stored provider_latency = %d, want %d", got, metadata.ProviderLatency)
	}
}

func TestProviderForAppServerProfileMapsAliasesToNamedResponders(t *testing.T) {
	cases := []struct {
		name    string
		profile appServerModelProfile
		want    string
	}{
		{name: "spark alias", profile: appServerModelProfile{Alias: "spark"}, want: assistantProviderSpark},
		{name: "gpt alias", profile: appServerModelProfile{Alias: "gpt"}, want: assistantProviderGPT},
		{name: "spark model", profile: appServerModelProfile{Model: "gpt-5.3-codex-spark"}, want: assistantProviderSpark},
		{name: "gpt model", profile: appServerModelProfile{Model: "gpt-5.4"}, want: assistantProviderGPT},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := providerForAppServerProfile(tc.profile); got != tc.want {
				t.Fatalf("providerForAppServerProfile() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAssistantProviderDisplayLabelPrefersSpecificResponderName(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		model    string
		want     string
	}{
		{name: "local model", provider: assistantProviderLocal, model: "qwen3.5-9b", want: "Local"},
		{name: "spark from generic openai", provider: assistantProviderOpenAI, model: "gpt-5.3-codex-spark", want: "Spark"},
		{name: "gpt from generic openai", provider: assistantProviderOpenAI, model: "gpt-5.4", want: "GPT"},
		{name: "unknown provider defaults local", provider: "", model: "", want: "Local"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := assistantProviderDisplayLabel(tc.provider, tc.model); got != tc.want {
				t.Fatalf("assistantProviderDisplayLabel() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestChatSessionHistoryIncludesProviderMetadata(t *testing.T) {
	app := newAuthedTestApp(t)

	project, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensureDefaultWorkspace: %v", err)
	}
	session, err := app.store.GetOrCreateChatSession(project.WorkspacePath)
	if err != nil {
		t.Fatalf("GetOrCreateChatSession: %v", err)
	}
	if _, err := app.store.AddChatMessage(
		session.ID,
		"assistant",
		"History reply.",
		"History reply.",
		"markdown",
		store.WithProviderMetadata(assistantProviderOpenAI, "gpt-5.4", 123),
	); err != nil {
		t.Fatalf("AddChatMessage: %v", err)
	}

	rr := doAuthedRequest(t, app.Router(), http.MethodGet, "/api/chat/sessions/"+session.ID+"/history")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET history status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	payload := decodeJSONResponse(t, rr)
	messages, ok := payload["messages"].([]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("messages payload = %#v, want one message", payload["messages"])
	}
	msg, ok := messages[0].(map[string]any)
	if !ok {
		t.Fatalf("message payload = %#v", messages[0])
	}
	if got := strFromAny(msg["provider"]); got != assistantProviderOpenAI {
		t.Fatalf("history provider = %q, want %q", got, assistantProviderOpenAI)
	}
	if got := strFromAny(msg["provider_model"]); got != "gpt-5.4" {
		t.Fatalf("history provider_model = %q, want gpt-5.4", got)
	}
	if got := intFromAny(msg["provider_latency_ms"], -1); got != 123 {
		t.Fatalf("history provider_latency_ms = %d, want 123", got)
	}
}
