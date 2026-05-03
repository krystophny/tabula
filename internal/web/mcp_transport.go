package web

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sloppy-org/slopshell/internal/mcpclient"
)

// mcpEndpoint addresses a private Slopshell control or backend MCP endpoint. In
// production the listener binds a Unix domain socket (mode 0600) — TCP MCP listeners
// are not allowed because they leak to other UIDs on the host (cf.
// multi-user threat model). The httpURL field is reserved for httptest-style
// in-process test servers and must not be used outside tests.
type mcpEndpoint struct {
	socket  string
	httpURL string
}

type mcpListedTool = mcpclient.ListedTool

func (e mcpEndpoint) clientEndpoint() mcpclient.Endpoint {
	return mcpclient.Endpoint{SocketPath: e.socket, HTTPBaseURL: e.httpURL}
}

func (e mcpEndpoint) ok() bool {
	return e.clientEndpoint().OK()
}

// HTTPURL returns the absolute URL to POST against for the given route.
func (e mcpEndpoint) HTTPURL(route string) string {
	return e.clientEndpoint().HTTPURL(route)
}

// WSURL returns the websocket URL for the given route.
func (e mcpEndpoint) WSURL(route string) string {
	return e.clientEndpoint().WSURL(route)
}

// HTTPClient returns an *http.Client appropriate for the endpoint's transport.
func (e mcpEndpoint) HTTPClient(timeout time.Duration) *http.Client {
	return e.clientEndpoint().HTTPClient(timeout)
}

// WSDialer returns a websocket.Dialer appropriate for the endpoint's transport.
func (e mcpEndpoint) WSDialer() *websocket.Dialer {
	return e.clientEndpoint().WSDialer()
}

// parseEndpoint accepts either an empty string (returns zero-value endpoint),
// a unix:/path/to/sock URL, a bare absolute path, or an http(s):// URL. The
// http(s):// form is reserved for in-process httptest servers; production
// configuration must use a unix socket because plaintext loopback HTTP leaks
// to other UIDs on the host.
func parseEndpoint(raw string) (mcpEndpoint, error) {
	ep, err := mcpclient.ParseEndpoint(raw)
	if err != nil {
		return mcpEndpoint{}, err
	}
	return mcpEndpoint{socket: ep.SocketPath, httpURL: ep.HTTPBaseURL}, nil
}

// defaultLocalControlSocket returns the conventional per-user socket path for the
// Slopshell private runtime control socket. Linux:
// $XDG_RUNTIME_DIR/sloppy/control.sock. On
// macOS launchd does not export XDG_RUNTIME_DIR by default, so we fall back
// to $HOME/Library/Caches/sloppy/control.sock. The parent dir is created with
// 0700 by the StartUnix path; we just compute the location here.
func defaultLocalControlSocket() string {
	return mcpclient.DefaultSocketPath("SLOPSHELL_CONTROL_SOCKET", "control.sock")
}

func defaultHelpySocket() string {
	return mcpclient.DefaultSocketPath("SLOPSHELL_HELPY_SOCKET", "helpy.sock")
}

// workspaceSocketPath returns a unique per-session unix socket path for a
// private runtime control socket serving a workspace project.
func workspaceSocketPath(sessionID string) string {
	base := strings.TrimSpace(os.Getenv("SLOPSHELL_WORKSPACE_SOCKET_DIR"))
	if base == "" {
		base = filepath.Dir(mcpclient.DefaultSocketPath("SLOPSHELL_WORKSPACE_SOCKET_DIR", filepath.Join("workspaces", "session.sock")))
		if strings.HasSuffix(base, ".sock") {
			base = filepath.Dir(base)
		}
	}
	clean := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, sessionID)
	if clean == "" {
		clean = "session"
	}
	return filepath.Join(base, clean+".sock")
}

// waitForUnixMCPReady blocks until the socket exists and a /health probe
// returns 200, or the deadline elapses. errCh signals an early exit if the
// listener goroutine returned.
func waitForUnixMCPReady(ep mcpEndpoint, timeout time.Duration, errCh <-chan error) error {
	return mcpclient.WaitForReady(ep.clientEndpoint(), timeout, errCh)
}

// httpClientCache caches the per-socket *http.Client so we reuse the
// underlying *http.Transport (and its idle connections) across calls.
func cachedHTTPClientForEndpoint(ep mcpEndpoint, timeout time.Duration) *http.Client {
	return mcpclient.SharedHTTPClient(ep.clientEndpoint(), timeout)
}

// rejectURLForEndpoint returns an error if a caller still passes an http://…
// URL. Used by transitional shims while migrating call sites.
func rejectURLForEndpoint(raw string) error {
	return mcpclient.RejectPlainHTTP(raw)
}

// localControlEndpointURL returns the endpoint string stored in the workspace
// row for the Slopshell local control socket. Empty when no endpoint is
// configured.
func (a *App) localControlEndpointURL() string {
	if a == nil {
		return ""
	}
	if a.localControlEndpoint.socket != "" {
		return "unix:" + a.localControlEndpoint.socket
	}
	if a.localControlEndpoint.httpURL != "" {
		return a.localControlEndpoint.httpURL
	}
	return ""
}
