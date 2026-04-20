package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInitCLITokenWritesFileWithRestrictivePerms(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv(cliTokenEnv, "")
	path, token, err := initCLIToken(dir)
	if err != nil {
		t.Fatalf("initCLIToken: %v", err)
	}
	wantDir := filepath.Clean(dir)
	if filepath.Dir(path) != wantDir {
		t.Fatalf("token dir = %q, want %q", filepath.Dir(path), wantDir)
	}
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat token file: %v", err)
	}
	if runtime.GOOS != "windows" {
		if perm := stat.Mode().Perm(); perm != 0o600 {
			t.Fatalf("token perms = %o, want 0600", perm)
		}
	}
	if len(token) != 64 {
		t.Fatalf("token length = %d, want 64 hex chars", len(token))
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read token file: %v", err)
	}
	if got := strings.TrimSpace(string(body)); got != token {
		t.Fatalf("file contents = %q, want %q", got, token)
	}
}

func TestResolveCLITokenPathPrefersEnvThenXDGRuntimeDir(t *testing.T) {
	t.Setenv(cliTokenEnv, "/custom/path/cli-token")
	if got := resolveCLITokenPath("/data"); got != "/custom/path/cli-token" {
		t.Fatalf("env override path = %q, want /custom/path/cli-token", got)
	}
	t.Setenv(cliTokenEnv, "")
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	if got := resolveCLITokenPath("/data"); got != "/run/user/1000/slopshell/cli-token" {
		t.Fatalf("xdg path = %q, want /run/user/1000/slopshell/cli-token", got)
	}
	t.Setenv("XDG_RUNTIME_DIR", "")
	if got := resolveCLITokenPath("/data"); got != "/data/cli-token" {
		t.Fatalf("datadir fallback = %q, want /data/cli-token", got)
	}
}

func postCLILogin(t *testing.T, app *App, remoteAddr, token string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{"token": token})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/cli/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remoteAddr
	rr := httptest.NewRecorder()
	app.Router().ServeHTTP(rr, req)
	return rr
}

func TestCLILoginSucceedsOnLoopbackAndMintsCookie(t *testing.T) {
	app := newAuthedTestApp(t)
	app.cliToken = "secret-cli-token"
	app.cliTokenPath = "/tmp/slopshell/cli-token"

	rr := postCLILogin(t, app, "127.0.0.1:54321", "secret-cli-token")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var cookie *http.Cookie
	for _, c := range rr.Result().Cookies() {
		if c.Name == SessionCookie {
			cookie = c
			break
		}
	}
	if cookie == nil || strings.TrimSpace(cookie.Value) == "" {
		t.Fatalf("expected %s cookie, got %+v", SessionCookie, rr.Result().Cookies())
	}
	if !app.store.HasAuthSession(cookie.Value) {
		t.Fatalf("cookie token %q not registered as auth session", cookie.Value)
	}

	// Follow-up request using the cookie must succeed against the authed API.
	req := httptest.NewRequest(http.MethodGet, "/api/runtime", nil)
	req.AddCookie(cookie)
	r2 := httptest.NewRecorder()
	app.Router().ServeHTTP(r2, req)
	if r2.Code != http.StatusOK {
		t.Fatalf("authed runtime status = %d, body = %s", r2.Code, r2.Body.String())
	}
}

func TestCLILoginRejectsNonLoopback(t *testing.T) {
	app := newAuthedTestApp(t)
	app.cliToken = "secret-cli-token"

	rr := postCLILogin(t, app, "10.0.0.2:44444", "secret-cli-token")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", rr.Code, rr.Body.String())
	}
}

func TestCLILoginRejectsForwardedForSpoof(t *testing.T) {
	app := newAuthedTestApp(t)
	app.cliToken = "secret-cli-token"

	body, _ := json.Marshal(map[string]string{"token": "secret-cli-token"})
	req := httptest.NewRequest(http.MethodPost, "/api/cli/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "127.0.0.1")
	req.RemoteAddr = "203.0.113.5:44444"
	rr := httptest.NewRecorder()
	app.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("forwarded-for spoof not rejected: status = %d body = %s", rr.Code, rr.Body.String())
	}
}

func TestCLILoginRejectsWrongToken(t *testing.T) {
	app := newAuthedTestApp(t)
	app.cliToken = "secret-cli-token"

	rr := postCLILogin(t, app, "127.0.0.1:54321", "wrong-value")
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", rr.Code, rr.Body.String())
	}
}

func TestCLILoginWhenTokenNotInitialised(t *testing.T) {
	app := newAuthedTestApp(t)
	app.cliToken = ""

	rr := postCLILogin(t, app, "127.0.0.1:54321", "anything")
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503, body = %s", rr.Code, rr.Body.String())
	}
}
