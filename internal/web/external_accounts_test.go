package web

import (
	"net/http"
	"testing"

	"github.com/krystophny/tabura/internal/store"
)

func TestExternalAccountCRUDAPI(t *testing.T) {
	app := newAuthedTestApp(t)

	disabled := false
	rrCreate := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/external-accounts", map[string]any{
		"sphere":   "work",
		"provider": "gmail",
		"label":    " Work Gmail ",
		"config": map[string]any{
			"username":   "alice@example.com",
			"token_path": "/tmp/tokens/work-gmail.json",
		},
		"enabled": disabled,
	})
	if rrCreate.Code != http.StatusCreated {
		t.Fatalf("create external account status = %d, want 201: %s", rrCreate.Code, rrCreate.Body.String())
	}
	createPayload := decodeJSONDataResponse(t, rrCreate)
	accountPayload, ok := createPayload["account"].(map[string]any)
	if !ok {
		t.Fatalf("create payload = %#v", createPayload)
	}
	accountID := int64(accountPayload["id"].(float64))
	if got := accountPayload["label"]; got != "Work Gmail" {
		t.Fatalf("created label = %#v, want %q", got, "Work Gmail")
	}
	if enabled, _ := accountPayload["enabled"].(bool); enabled {
		t.Fatalf("created account payload = %#v, want disabled account", accountPayload)
	}

	rrList := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/external-accounts?sphere=work", nil)
	if rrList.Code != http.StatusOK {
		t.Fatalf("list external accounts status = %d, want 200: %s", rrList.Code, rrList.Body.String())
	}
	listPayload := decodeJSONDataResponse(t, rrList)
	accounts, ok := listPayload["accounts"].([]any)
	if !ok || len(accounts) != 1 {
		t.Fatalf("list payload = %#v", listPayload)
	}

	enabled := true
	newLabel := "Work Gmail Primary"
	rrUpdate := doAuthedJSONRequest(t, app.Router(), http.MethodPut, "/api/external-accounts/"+itoa(accountID), map[string]any{
		"label":   newLabel,
		"enabled": enabled,
		"config": map[string]any{
			"username":   "alice@example.com",
			"token_file": "gmail-work.json",
		},
	})
	if rrUpdate.Code != http.StatusOK {
		t.Fatalf("update external account status = %d, want 200: %s", rrUpdate.Code, rrUpdate.Body.String())
	}
	updated, err := app.store.GetExternalAccount(accountID)
	if err != nil {
		t.Fatalf("GetExternalAccount(updated) error: %v", err)
	}
	if updated.Label != newLabel {
		t.Fatalf("updated label = %q, want %q", updated.Label, newLabel)
	}
	if !updated.Enabled {
		t.Fatal("expected updated external account to be enabled")
	}

	rrDelete := doAuthedJSONRequest(t, app.Router(), http.MethodDelete, "/api/external-accounts/"+itoa(accountID), nil)
	if rrDelete.Code != http.StatusNoContent {
		t.Fatalf("delete external account status = %d, want 204: %s", rrDelete.Code, rrDelete.Body.String())
	}
	if rrDelete.Body.Len() != 0 {
		t.Fatalf("delete external account body = %q, want empty", rrDelete.Body.String())
	}
	rrMissing := doAuthedJSONRequest(t, app.Router(), http.MethodDelete, "/api/external-accounts/"+itoa(accountID), nil)
	if rrMissing.Code != http.StatusNotFound {
		t.Fatalf("missing external account delete status = %d, want 404: %s", rrMissing.Code, rrMissing.Body.String())
	}
}

func TestExternalAccountAPIRejectsInvalidInput(t *testing.T) {
	app := newAuthedTestApp(t)

	rrBadCreate := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/external-accounts", map[string]any{
		"sphere":   "work",
		"provider": "gmail",
		"label":    "Bad config",
		"config": map[string]any{
			"password": "plaintext",
		},
	})
	if rrBadCreate.Code != http.StatusBadRequest {
		t.Fatalf("bad create status = %d, want 400: %s", rrBadCreate.Code, rrBadCreate.Body.String())
	}
	if got := decodeJSONResponse(t, rrBadCreate)["error"]; got == nil {
		t.Fatalf("bad create payload = %#v, want error", decodeJSONResponse(t, rrBadCreate))
	}

	rrBadList := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/external-accounts?sphere=office", nil)
	if rrBadList.Code != http.StatusBadRequest {
		t.Fatalf("bad list status = %d, want 400: %s", rrBadList.Code, rrBadList.Body.String())
	}

	rrMissingUpdate := doAuthedJSONRequest(t, app.Router(), http.MethodPut, "/api/external-accounts/9999", map[string]any{
		"label": "Missing",
	})
	if rrMissingUpdate.Code != http.StatusNotFound {
		t.Fatalf("missing update status = %d, want 404: %s", rrMissingUpdate.Code, rrMissingUpdate.Body.String())
	}
}

func TestSphereScopedExternalAccountAPI(t *testing.T) {
	app := newAuthedTestApp(t)

	rrCreate := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/spheres/work/accounts", map[string]any{
		"provider": "todoist",
		"label":    "Work Todoist",
		"config": map[string]any{
			"base_url": "https://todoist.example.test",
		},
	})
	if rrCreate.Code != http.StatusCreated {
		t.Fatalf("scoped create status = %d, want 201: %s", rrCreate.Code, rrCreate.Body.String())
	}
	createPayload := decodeJSONDataResponse(t, rrCreate)
	accountPayload, ok := createPayload["account"].(map[string]any)
	if !ok {
		t.Fatalf("scoped create payload = %#v", createPayload)
	}
	accountID := int64(accountPayload["id"].(float64))
	if got := strFromAny(accountPayload["sphere"]); got != store.SphereWork {
		t.Fatalf("created sphere = %q, want %q", got, store.SphereWork)
	}

	if _, err := app.store.CreateExternalAccount(store.SpherePrivate, store.ExternalProviderTodoist, "Private Todoist", map[string]any{
		"base_url": "https://todoist.example.test/private",
	}); err != nil {
		t.Fatalf("CreateExternalAccount(private) error: %v", err)
	}

	rrList := doAuthedJSONRequest(t, app.Router(), http.MethodGet, "/api/spheres/work/accounts", nil)
	if rrList.Code != http.StatusOK {
		t.Fatalf("scoped list status = %d, want 200: %s", rrList.Code, rrList.Body.String())
	}
	listPayload := decodeJSONDataResponse(t, rrList)
	accounts, ok := listPayload["accounts"].([]any)
	if !ok || len(accounts) != 1 {
		t.Fatalf("scoped list payload = %#v", listPayload)
	}
	if got := strFromAny(accounts[0].(map[string]any)["label"]); got != "Work Todoist" {
		t.Fatalf("listed label = %q, want Work Todoist", got)
	}

	rrDelete := doAuthedJSONRequest(t, app.Router(), http.MethodDelete, "/api/spheres/work/accounts/"+itoa(accountID), nil)
	if rrDelete.Code != http.StatusNoContent {
		t.Fatalf("scoped delete status = %d, want 204: %s", rrDelete.Code, rrDelete.Body.String())
	}

	rrBadCreate := doAuthedJSONRequest(t, app.Router(), http.MethodPost, "/api/spheres/work/accounts", map[string]any{
		"sphere":   "private",
		"provider": "todoist",
		"label":    "Wrong Sphere",
	})
	if rrBadCreate.Code != http.StatusBadRequest {
		t.Fatalf("scoped mismatched create status = %d, want 400: %s", rrBadCreate.Code, rrBadCreate.Body.String())
	}
}
