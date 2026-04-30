package web

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/sloppy-org/slopshell/internal/store"
)

func TestParseInlineItemIntentFilterCommands(t *testing.T) {
	now := time.Date(2026, time.March, 8, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		text         string
		wantAction   string
		wantSource   string
		wantNullWork bool
		wantClear    bool
		wantAll      bool
	}{
		{text: "show inbox", wantAction: "show_filtered_items", wantClear: true},
		{text: "show all inbox", wantAction: "show_filtered_items", wantClear: true, wantAll: true},
		{text: "show next actions", wantAction: "show_filtered_items", wantClear: true},
		{text: "show waiting", wantAction: "show_filtered_items", wantClear: true},
		{text: "show deferred", wantAction: "show_filtered_items", wantClear: true},
		{text: "show review queue", wantAction: "show_filtered_items", wantClear: true},
		{text: "daily review", wantAction: "show_filtered_items", wantClear: true},
		{text: "weekly review", wantAction: "show_filtered_items", wantClear: true},
		{text: "zeige posteingang", wantAction: "show_filtered_items", wantClear: true},
		{text: "show todoist tasks", wantAction: "show_filtered_items", wantSource: store.ExternalProviderTodoist},
		{text: "show all todoist tasks", wantAction: "show_filtered_items", wantSource: store.ExternalProviderTodoist, wantAll: true},
		{text: "show my todoist tasks", wantAction: "show_filtered_items", wantSource: store.ExternalProviderTodoist},
		{text: "zeige todoist aufgaben", wantAction: "show_filtered_items", wantSource: store.ExternalProviderTodoist},
		{text: "show evernote notes", wantAction: "show_filtered_items", wantSource: store.ExternalProviderEvernote},
		{text: "show unassigned items", wantAction: "show_filtered_items", wantNullWork: true},
		{text: "zeige nicht zugeordnete items", wantAction: "show_filtered_items", wantNullWork: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.text, func(t *testing.T) {
			action := parseInlineItemIntent(tc.text, now)
			if action == nil {
				t.Fatal("expected inline action")
			}
			if action.Action != tc.wantAction {
				t.Fatalf("action = %q, want %q", action.Action, tc.wantAction)
			}
			if tc.wantClear && !systemActionTruthyParam(action.Params, "clear_filters") {
				t.Fatalf("expected clear_filters in %#v", action.Params)
			}
			filters := systemActionNestedParams(action.Params, "filters")
			if tc.wantSource != "" && systemActionStringParam(filters, "source") != tc.wantSource {
				t.Fatalf("source = %q, want %q", systemActionStringParam(filters, "source"), tc.wantSource)
			}
			if tc.wantNullWork && systemActionStringParam(filters, "workspace_id") != "null" {
				t.Fatalf("workspace_id = %q, want null", systemActionStringParam(filters, "workspace_id"))
			}
			if got := systemActionAllSpheresParam(action.Params); got != tc.wantAll {
				t.Fatalf("all_spheres = %v, want %v", got, tc.wantAll)
			}
		})
	}
}

func TestClassifyAndExecuteSystemActionOpensReviewEntryPoint(t *testing.T) {
	app := newAuthedTestApp(t)
	app.intentLLMURL = ""

	workspace, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensure default workspace: %v", err)
	}
	if _, err := app.store.CreateItem("Review stalled outcome", store.ItemOptions{
		Kind:  store.ItemKindProject,
		State: store.ItemStateNext,
	}); err != nil {
		t.Fatalf("CreateItem(stalled project) error: %v", err)
	}
	session, err := app.store.GetOrCreateChatSession(workspace.WorkspacePath)
	if err != nil {
		t.Fatalf("chat session: %v", err)
	}

	message, payloads, handled := app.classifyAndExecuteSystemAction(context.Background(), session.ID, session, "daily review")
	if !handled {
		t.Fatal("expected daily review command to be handled")
	}
	if message != "Opened review with 1 item(s)." {
		t.Fatalf("message = %q", message)
	}
	if len(payloads) != 1 {
		t.Fatalf("payloads = %#v", payloads)
	}
	if got := strFromAny(payloads[0]["view"]); got != store.ItemStateReview {
		t.Fatalf("payload view = %q, want %q", got, store.ItemStateReview)
	}
	if !boolFromAny(payloads[0]["clear_filters"]) {
		t.Fatalf("payload clear_filters = %#v, want true", payloads[0])
	}
}

func TestClassifyAndExecuteSystemActionShowFilteredItems(t *testing.T) {
	app := newAuthedTestApp(t)
	app.intentLLMURL = ""

	workspace, err := app.ensureDefaultWorkspace()
	if err != nil {
		t.Fatalf("ensure default workspace: %v", err)
	}
	privateWorkspace, err := app.store.CreateWorkspace("Private", filepath.Join(t.TempDir(), "private"), store.SpherePrivate)
	if err != nil {
		t.Fatalf("CreateWorkspace(private) error: %v", err)
	}
	workWorkspace, err := app.store.CreateWorkspace("Work", filepath.Join(t.TempDir(), "work"), store.SphereWork)
	if err != nil {
		t.Fatalf("CreateWorkspace(work) error: %v", err)
	}
	if err := app.store.SetActiveSphere(store.SpherePrivate); err != nil {
		t.Fatalf("SetActiveSphere() error: %v", err)
	}
	past := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	sourceTodoist := store.ExternalProviderTodoist
	sourceExchange := store.ExternalProviderExchange
	if _, err := app.store.CreateItem("Todoist follow-up", store.ItemOptions{
		State:        store.ItemStateInbox,
		WorkspaceID:  &privateWorkspace.ID,
		VisibleAfter: &past,
		Source:       &sourceTodoist,
	}); err != nil {
		t.Fatalf("CreateItem(todoist) error: %v", err)
	}
	if _, err := app.store.CreateItem("Exchange follow-up", store.ItemOptions{
		State:        store.ItemStateInbox,
		WorkspaceID:  &workWorkspace.ID,
		VisibleAfter: &past,
		Source:       &sourceExchange,
	}); err != nil {
		t.Fatalf("CreateItem(exchange) error: %v", err)
	}
	session, err := app.store.GetOrCreateChatSession(workspace.WorkspacePath)
	if err != nil {
		t.Fatalf("chat session: %v", err)
	}

	message, payloads, handled := app.classifyAndExecuteSystemAction(context.Background(), session.ID, session, "show todoist tasks")
	if !handled {
		t.Fatal("expected filter command to be handled")
	}
	if message != "Opened inbox filtered to todoist with 1 item(s)." {
		t.Fatalf("message = %q", message)
	}
	if len(payloads) != 1 {
		t.Fatalf("payloads = %#v", payloads)
	}
	if got := strFromAny(payloads[0]["type"]); got != "show_item_sidebar_view" {
		t.Fatalf("payload type = %q, want show_item_sidebar_view", got)
	}
	filters, ok := payloads[0]["filters"].(map[string]interface{})
	if !ok {
		t.Fatalf("filters payload = %#v", payloads[0])
	}
	if got := strFromAny(filters["source"]); got != store.ExternalProviderTodoist {
		t.Fatalf("filters.source = %q, want %q", got, store.ExternalProviderTodoist)
	}

	message, payloads, handled = app.classifyAndExecuteSystemAction(context.Background(), session.ID, session, "show all inbox")
	if !handled {
		t.Fatal("expected all-spheres inbox command to be handled")
	}
	if message != "Opened inbox across all spheres with 2 item(s)." {
		t.Fatalf("message = %q", message)
	}
	if len(payloads) != 1 {
		t.Fatalf("payloads = %#v", payloads)
	}
	filters, ok = payloads[0]["filters"].(map[string]interface{})
	if !ok {
		t.Fatalf("filters payload = %#v", payloads[0])
	}
	if !boolFromAny(filters["all_spheres"]) {
		t.Fatalf("filters = %#v, want all_spheres=true", filters)
	}
}

func TestExecuteFilteredItemViewActionSupportsContextFilter(t *testing.T) {
	app := newAuthedTestApp(t)

	parent, err := app.store.CreateLabel("Work", nil)
	if err != nil {
		t.Fatalf("CreateLabel(parent) error: %v", err)
	}
	child, err := app.store.CreateLabel("W7x", &parent.ID)
	if err != nil {
		t.Fatalf("CreateLabel(child) error: %v", err)
	}
	other, err := app.store.CreateLabel("Private", nil)
	if err != nil {
		t.Fatalf("CreateLabel(other) error: %v", err)
	}

	workspace, err := app.store.CreateWorkspace("Workstream", filepath.Join(t.TempDir(), "workstream"), store.SpherePrivate)
	if err != nil {
		t.Fatalf("CreateWorkspace() error: %v", err)
	}
	if err := app.store.LinkLabelToWorkspace(child.ID, workspace.ID); err != nil {
		t.Fatalf("LinkLabelToWorkspace() error: %v", err)
	}
	past := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	if _, err := app.store.CreateItem("Review W7x backlog", store.ItemOptions{
		State:        store.ItemStateInbox,
		WorkspaceID:  &workspace.ID,
		VisibleAfter: &past,
	}); err != nil {
		t.Fatalf("CreateItem(work item) error: %v", err)
	}
	privateItem, err := app.store.CreateItem("Reply to family email", store.ItemOptions{
		State:        store.ItemStateInbox,
		VisibleAfter: &past,
	})
	if err != nil {
		t.Fatalf("CreateItem(private item) error: %v", err)
	}
	if err := app.store.LinkLabelToItem(other.ID, privateItem.ID); err != nil {
		t.Fatalf("LinkLabelToItem() error: %v", err)
	}

	message, payload, err := app.executeFilteredItemViewAction(&SystemAction{
		Action: "show_filtered_items",
		Params: map[string]interface{}{
			"view": store.ItemStateInbox,
			"filters": map[string]interface{}{
				"label_id": parent.ID,
			},
		},
	})
	if err != nil {
		t.Fatalf("executeFilteredItemViewAction() error: %v", err)
	}
	if message != "Opened inbox filtered to one label with 1 item(s)." {
		t.Fatalf("message = %q", message)
	}
	filters, ok := payload["filters"].(map[string]interface{})
	if !ok {
		t.Fatalf("filters payload = %#v", payload)
	}
	gotLabelID, ok := filters["label_id"].(int64)
	if !ok {
		t.Fatalf("filters.label_id type = %T", filters["label_id"])
	}
	if gotLabelID != parent.ID {
		t.Fatalf("filters.label_id = %d, want %d", gotLabelID, parent.ID)
	}
}
