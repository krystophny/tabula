package web

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/sloppy-org/slopshell/internal/mcpclient"
	"github.com/sloppy-org/slopshell/internal/serve"
)

func (a *App) handleCanvasWS(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	sid := chi.URLParam(r, "session_id")
	ws, err := a.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	a.hub.registerCanvas(sid, ws)
	remote := a.tunnels.getRemoteCanvas(sid)

	defer func() {
		a.hub.unregisterCanvas(sid, ws)
		_ = ws.Close()
	}()

	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			return
		}
		if remote != nil {
			_ = remote.WriteMessage(websocket.TextMessage, msg)
		}
	}
}

func (a *App) handleCanvasSnapshot(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	sid := chi.URLParam(r, "session_id")
	ep, ok := a.tunnels.getEndpoint(sid)
	if !ok {
		http.Error(w, "no active tunnel for session", http.StatusNotFound)
		return
	}
	status, err := a.mcpToolsCall(ep, "canvas_status", map[string]interface{}{"session_id": sid})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	event, _ := status["active_artifact"].(map[string]interface{})
	writeJSON(w, map[string]interface{}{"status": status, "event": event})
}

// mcpToolsCall is the App-level helper used throughout the chat handlers. It
// targets a specific session's MCP listener (sloptools embedded over a unix
// socket).
func (a *App) mcpToolsCall(ep mcpEndpoint, name string, arguments map[string]interface{}) (map[string]interface{}, error) {
	return mcpToolsCallEndpoint(ep, name, arguments)
}

func mcpToolsListEndpoint(ep mcpEndpoint) ([]mcpListedTool, error) {
	client, err := mcpclient.New(ep.clientEndpoint(), nil, mcpToolsCallTimeout)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), mcpToolsCallTimeout)
	defer cancel()
	return client.ListTools(ctx)
}

func mcpToolsCallEndpoint(ep mcpEndpoint, name string, arguments map[string]interface{}) (map[string]interface{}, error) {
	client, err := mcpclient.New(ep.clientEndpoint(), nil, mcpToolsCallTimeout)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), mcpToolsCallTimeout)
	defer cancel()
	return client.CallTool(ctx, name, arguments)
}

func mcpResultErrorText(result map[string]interface{}) string {
	if result == nil {
		return "unknown error"
	}
	typed := make(map[string]any, len(result))
	for key, value := range result {
		typed[key] = value
	}
	return mcpclient.ResultErrorText(typed)
}

func checkWSOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if strings.TrimSpace(u.Hostname()) == "" {
		return false
	}
	requestScheme := "http"
	if isHTTPS(r) {
		requestScheme = "https"
	}
	if !strings.EqualFold(strings.TrimSpace(u.Scheme), requestScheme) {
		return false
	}
	originHost, originPort := hostPortForScheme(u.Host, u.Scheme)
	requestHost, requestPort := hostPortForScheme(r.Host, requestScheme)
	if originHost == "" || requestHost == "" {
		return false
	}
	return strings.EqualFold(originHost, requestHost) && originPort == requestPort
}

func hostPortForScheme(rawHost, scheme string) (string, string) {
	ref := strings.TrimSpace(rawHost)
	if ref == "" {
		return "", ""
	}
	parsed, err := url.Parse("//" + ref)
	if err != nil {
		return "", ""
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return "", ""
	}
	port := strings.TrimSpace(parsed.Port())
	if port == "" {
		if strings.EqualFold(strings.TrimSpace(scheme), "https") {
			port = "443"
		} else {
			port = "80"
		}
	}
	return host, port
}

func (a *App) handleFilesProxy(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	// Files are rendered inside same-origin canvas panes (image/PDF), so this
	// route must allow same-origin embedding instead of the global DENY policy.
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	w.Header().Set("Content-Security-Policy",
		"default-src 'self'; "+
			"script-src 'self' 'wasm-unsafe-eval'; "+
			"style-src 'self' 'unsafe-inline'; "+
			"img-src 'self' data:; "+
			"connect-src 'self' ws: wss:; "+
			"frame-ancestors 'self'; "+
			"base-uri 'none'; "+
			"form-action 'self'")
	sid := chi.URLParam(r, "session_id")
	rawPath := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	filePath, err := url.PathUnescape(rawPath)
	if err != nil {
		http.Error(w, "invalid path encoding", http.StatusBadRequest)
		return
	}
	filePath = strings.TrimPrefix(filePath, "/")
	if filePath == "" {
		http.Error(w, "missing path", http.StatusBadRequest)
		return
	}
	if strings.Contains(filePath, "..") || strings.ContainsRune(filePath, '\x00') {
		http.Error(w, "invalid path", http.StatusForbidden)
		return
	}
	ep, ok := a.tunnels.getEndpoint(sid)
	if !ok {
		http.Error(w, "no active tunnel for session", http.StatusNotFound)
		return
	}
	upstreamURL := ep.HTTPURL("/files/" + filePath)
	resp, err := cachedHTTPClientForEndpoint(ep, 30*time.Second).Get(upstreamURL)
	if err != nil {
		http.Error(w, "file fetch failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, vv := range resp.Header {
		if strings.EqualFold(k, "Content-Type") {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (a *App) startCanvasRelay(sessionID string, ep mcpEndpoint) {
	if !ep.ok() {
		return
	}
	ctx := a.tunnels.replaceRelayCancel(sessionID)

	go func() {
		defer func() {
			a.tunnels.deleteRelayCancel(sessionID)
			a.tunnels.deleteRemoteCanvas(sessionID)
		}()

		conn, _, err := ep.WSDialer().Dial(ep.WSURL("/ws/canvas"), nil)
		if err != nil {
			errMsg := []byte(`{"type":"relay_error","message":"canvas backend unavailable"}`)
			for _, ws := range a.hub.canvasClients(sessionID) {
				_ = ws.WriteMessage(websocket.TextMessage, errMsg)
			}
			return
		}
		a.tunnels.setRemoteCanvas(sessionID, conn)

		for {
			select {
			case <-ctx.Done():
				_ = conn.Close()
				return
			default:
			}
			mt, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if mt != websocket.TextMessage {
				continue
			}
			for _, ws := range a.hub.canvasClients(sessionID) {
				_ = ws.WriteMessage(websocket.TextMessage, msg)
			}
		}
	}()
}

func (a *App) startLocalServe() error {
	if a.tunnels.hasEndpoint(LocalSessionID) {
		return nil
	}
	if a.localProjectDir == "" {
		return nil
	}
	// httpURL endpoints are reserved for in-process tests — they refer to an
	// already-running httptest server, not a socket we should bind.
	if a.localControlEndpoint.httpURL != "" {
		a.tunnels.setEndpoint(LocalSessionID, a.localControlEndpoint)
		a.startCanvasRelay(LocalSessionID, a.localControlEndpoint)
		return nil
	}
	socket := strings.TrimSpace(a.localControlSocket)
	if socket == "" {
		socket = defaultLocalControlSocket()
	}
	if socket == "" {
		return errors.New("no control socket path: set SLOPSHELL_CONTROL_SOCKET or pass --control-socket")
	}
	app := serve.NewApp(a.localProjectDir, a.dataDir)
	_, cancel := context.WithCancel(context.Background())
	a.tunnels.setLocalServe(app, cancel)
	errCh := make(chan error, 1)
	go func() {
		errCh <- app.StartUnix(socket)
	}()
	ep := mcpEndpoint{socket: socket}
	if err := waitForUnixMCPReady(ep, 10*time.Second, errCh); err != nil {
		cancel()
		return err
	}
	a.tunnels.setEndpoint(LocalSessionID, ep)
	a.startCanvasRelay(LocalSessionID, ep)
	return nil
}
