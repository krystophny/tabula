package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// cliTokenEnv lets CLI clients override the token file location. When unset,
// the runtime falls back to $XDG_RUNTIME_DIR/slopshell/cli-token, then to
// <dataDir>/cli-token.
const cliTokenEnv = "SLOPSHELL_CLI_TOKEN_FILE"

// resolveCLITokenPath picks the preferred on-disk location for the CLI token.
// Callers should be able to discover this path without talking to the server.
func resolveCLITokenPath(dataDir string) string {
	if override := strings.TrimSpace(os.Getenv(cliTokenEnv)); override != "" {
		return override
	}
	if runtimeDir := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR")); runtimeDir != "" {
		return filepath.Join(runtimeDir, "slopshell", "cli-token")
	}
	if strings.TrimSpace(dataDir) == "" {
		return ""
	}
	return filepath.Join(dataDir, "cli-token")
}

// initCLIToken generates a new random 32-byte hex token, writes it to disk
// with 0600 perms, and returns the chosen path and token. Regenerated each
// time the server starts so the token does not persist across restarts.
func initCLIToken(dataDir string) (string, string, error) {
	path := resolveCLITokenPath(dataDir)
	if path == "" {
		return "", "", errors.New("cli token path not resolvable")
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", "", fmt.Errorf("generate cli token: %w", err)
	}
	token := hex.EncodeToString(raw[:])
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", "", fmt.Errorf("mkdir cli token dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		return "", "", fmt.Errorf("write cli token: %w", err)
	}
	return path, token, nil
}

// isLoopbackRequest returns true only for connections whose remote peer is
// bound to a loopback address. It ignores X-Forwarded-For and any other
// forwarded headers deliberately: the CLI login endpoint is for same-host
// callers only, so trusting proxy headers would be unsafe.
func isLoopbackRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		host = strings.TrimSpace(r.RemoteAddr)
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return strings.EqualFold(host, "localhost")
}

// handleCLILogin accepts a CLI token previously written to the cli-token file
// and, on match, issues a standard auth-session cookie so subsequent requests
// and websocket upgrades authenticate normally. Loopback-only.
func (a *App) handleCLILogin(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRequest(r) {
		writeAPIError(w, http.StatusForbidden, "cli login is loopback-only")
		return
	}
	if strings.TrimSpace(a.cliToken) == "" {
		writeAPIError(w, http.StatusServiceUnavailable, "cli token not initialised")
		return
	}
	var req struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	provided := strings.TrimSpace(req.Token)
	if provided == "" {
		writeAPIError(w, http.StatusBadRequest, "token is required")
		return
	}
	if subtle.ConstantTimeCompare([]byte(provided), []byte(a.cliToken)) != 1 {
		writeAPIError(w, http.StatusUnauthorized, "invalid cli token")
		return
	}
	sessionToken := randomToken()
	if err := a.store.AddAuthSession(sessionToken); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	a.setAuthCookieForRequest(w, r, sessionToken)
	writeJSON(w, map[string]interface{}{
		"ok":         true,
		"token_path": a.cliTokenPath,
	})
}
