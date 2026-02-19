package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/krystophny/tabula/internal/canvas"
)

const (
	ServerName            = "tabula"
	ServerVersion         = "0.3.0"
	LatestProtocolVersion = "2025-03-26"
)

var supportedProtocolVersions = map[string]struct{}{
	"2024-11-05": {},
	"2025-03-26": {},
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type Server struct {
	adapter *canvas.Adapter
}

func NewServer(adapter *canvas.Adapter) *Server {
	return &Server{adapter: adapter}
}

func (s *Server) DispatchMessage(message map[string]interface{}) map[string]interface{} {
	id, hasID := message["id"]
	method, _ := message["method"].(string)
	if strings.TrimSpace(method) == "" {
		if hasID {
			return rpcErr(id, -32600, "missing method")
		}
		return nil
	}
	if !hasID {
		return nil
	}
	params, _ := message["params"].(map[string]interface{})
	if params == nil {
		params = map[string]interface{}{}
	}

	result, rerr := s.dispatch(method, params)
	if rerr != nil {
		return map[string]interface{}{"jsonrpc": "2.0", "id": id, "error": rerr}
	}
	return map[string]interface{}{"jsonrpc": "2.0", "id": id, "result": result}
}

func rpcErr(id interface{}, code int, message string) map[string]interface{} {
	return map[string]interface{}{"jsonrpc": "2.0", "id": id, "error": RPCError{Code: code, Message: message}}
}

func (s *Server) dispatch(method string, params map[string]interface{}) (map[string]interface{}, *RPCError) {
	switch method {
	case "initialize":
		requested, _ := params["protocolVersion"].(string)
		v := LatestProtocolVersion
		if _, ok := supportedProtocolVersions[requested]; ok {
			v = requested
		}
		return map[string]interface{}{
			"protocolVersion": v,
			"capabilities": map[string]interface{}{
				"tools":     map[string]interface{}{"listChanged": false},
				"resources": map[string]interface{}{"subscribe": false},
			},
			"serverInfo": map[string]interface{}{"name": ServerName, "version": ServerVersion},
		}, nil
	case "ping":
		return map[string]interface{}{}, nil
	case "tools/list":
		return map[string]interface{}{"tools": toolDefinitions()}, nil
	case "resources/list":
		return map[string]interface{}{"resources": resourcesList(s.adapter)}, nil
	case "resources/templates/list":
		return map[string]interface{}{"resourceTemplates": resourceTemplates()}, nil
	case "resources/read":
		return s.dispatchResourceRead(params)
	case "tools/call":
		return s.dispatchToolCall(params)
	default:
		return nil, &RPCError{Code: -32601, Message: "method not found: " + method}
	}
}

func (s *Server) dispatchToolCall(params map[string]interface{}) (map[string]interface{}, *RPCError) {
	name, _ := params["name"].(string)
	if strings.TrimSpace(name) == "" {
		return nil, &RPCError{Code: -32602, Message: "tools/call requires non-empty name"}
	}
	args, _ := params["arguments"].(map[string]interface{})
	if args == nil {
		args = map[string]interface{}{}
	}
	structured, err := s.callTool(name, args)
	if err != nil {
		return map[string]interface{}{
			"content": []map[string]string{{"type": "text", "text": err.Error()}},
			"isError": true,
		}, nil
	}
	b, _ := json.Marshal(structured)
	return map[string]interface{}{
		"content":           []map[string]string{{"type": "text", "text": string(b)}},
		"structuredContent": structured,
		"isError":           false,
	}, nil
}

func (s *Server) callTool(name string, args map[string]interface{}) (map[string]interface{}, error) {
	sid := strArg(args, "session_id")
	switch name {
	case "canvas_session_open", "canvas_activate":
		return s.adapter.CanvasSessionOpen(sid, strArg(args, "mode_hint")), nil
	case "canvas_artifact_show":
		return s.adapter.CanvasArtifactShow(
			sid,
			strArg(args, "kind"),
			strArg(args, "title"),
			strArg(args, "markdown_or_text"),
			strArg(args, "path"),
			intArg(args, "page", 0),
			strArg(args, "reason"),
		)
	case "canvas_render_text":
		return s.adapter.CanvasArtifactShow(sid, "text", strArg(args, "title"), strArg(args, "markdown_or_text"), "", 0, "")
	case "canvas_render_image":
		return s.adapter.CanvasArtifactShow(sid, "image", strArg(args, "title"), "", strArg(args, "path"), 0, "")
	case "canvas_render_pdf":
		return s.adapter.CanvasArtifactShow(sid, "pdf", strArg(args, "title"), "", strArg(args, "path"), intArg(args, "page", 0), "")
	case "canvas_clear":
		return s.adapter.CanvasArtifactShow(sid, "clear", "", "", "", 0, strArg(args, "reason"))
	case "canvas_mark_set":
		target, _ := args["target"].(map[string]interface{})
		return s.adapter.CanvasMarkSet(
			sid,
			strArg(args, "mark_id"),
			strArg(args, "artifact_id"),
			canvas.MarkIntent(strArg(args, "intent")),
			canvas.MarkType(strArg(args, "type")),
			canvas.TargetKind(strArg(args, "target_kind")),
			target,
			strArg(args, "comment"),
			strArg(args, "author"),
		)
	case "canvas_mark_delete":
		return s.adapter.CanvasMarkDelete(sid, strArg(args, "mark_id"))
	case "canvas_marks_list":
		return s.adapter.CanvasMarksList(sid, strArg(args, "artifact_id"), canvas.MarkIntent(strArg(args, "intent")), intArg(args, "limit", 0)), nil
	case "canvas_mark_focus":
		return s.adapter.CanvasMarkFocus(sid, strArg(args, "mark_id"))
	case "canvas_commit":
		return s.adapter.CanvasCommit(sid, strArg(args, "artifact_id"), boolArg(args, "include_draft", true))
	case "canvas_status":
		return s.adapter.CanvasStatus(sid), nil
	case "canvas_history":
		return s.adapter.CanvasHistory(sid, intArg(args, "limit", 20)), nil
	case "canvas_selection":
		return s.adapter.CanvasSelection(sid), nil
	default:
		return nil, errors.New("unknown tool: " + name)
	}
}

func (s *Server) dispatchResourceRead(params map[string]interface{}) (map[string]interface{}, *RPCError) {
	uri, _ := params["uri"].(string)
	if strings.TrimSpace(uri) == "" {
		return nil, &RPCError{Code: -32602, Message: "resources/read requires uri"}
	}
	content, err := readResource(s.adapter, uri)
	if err != nil {
		return nil, &RPCError{Code: -32002, Message: err.Error()}
	}
	return map[string]interface{}{"contents": []map[string]interface{}{content}}, nil
}

func strArg(args map[string]interface{}, key string) string {
	v, _ := args[key].(string)
	return v
}

func intArg(args map[string]interface{}, key string, def int) int {
	v, ok := args[key]
	if !ok {
		return def
	}
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	default:
		return def
	}
}

func boolArg(args map[string]interface{}, key string, def bool) bool {
	v, ok := args[key].(bool)
	if !ok {
		return def
	}
	return v
}

func toolDefinitions() []map[string]interface{} {
	return []map[string]interface{}{
		{"name": "canvas_session_open", "description": "Open canvas session and initialize runtime status.", "inputSchema": map[string]interface{}{"type": "object", "required": []string{"session_id"}}},
		{"name": "canvas_artifact_show", "description": "Show one artifact kind in canvas: text, image, pdf, or clear.", "inputSchema": map[string]interface{}{"type": "object", "required": []string{"session_id", "kind"}}},
		{"name": "canvas_mark_set", "description": "Create or update a mark (selection/annotation) on the active artifact.", "inputSchema": map[string]interface{}{"type": "object", "required": []string{"session_id", "intent", "type", "target_kind", "target"}}},
		{"name": "canvas_mark_delete", "description": "Delete a mark by id.", "inputSchema": map[string]interface{}{"type": "object", "required": []string{"session_id", "mark_id"}}},
		{"name": "canvas_marks_list", "description": "List marks for a session, optionally filtered by artifact/intent.", "inputSchema": map[string]interface{}{"type": "object", "required": []string{"session_id"}}},
		{"name": "canvas_mark_focus", "description": "Set or clear currently focused mark.", "inputSchema": map[string]interface{}{"type": "object", "required": []string{"session_id"}}},
		{"name": "canvas_commit", "description": "Commit draft marks to persistent annotations and write sidecar/PDF annotations.", "inputSchema": map[string]interface{}{"type": "object", "required": []string{"session_id"}}},
		{"name": "canvas_status", "description": "Get current session status and active artifact metadata.", "inputSchema": map[string]interface{}{"type": "object", "required": []string{"session_id"}}},
	}
}

func resourceTemplates() []map[string]interface{} {
	return []map[string]interface{}{
		{"uriTemplate": "tabula://session/{session_id}", "name": "Canvas Session Status", "mimeType": "application/json", "description": "Current status for a canvas session."},
		{"uriTemplate": "tabula://session/{session_id}/marks", "name": "Canvas Session Marks", "mimeType": "application/json", "description": "Current marks for a canvas session."},
		{"uriTemplate": "tabula://session/{session_id}/history", "name": "Canvas Session History", "mimeType": "application/json", "description": "Recent event history for a canvas session."},
	}
}

func resourcesList(adapter *canvas.Adapter) []map[string]interface{} {
	out := []map[string]interface{}{}
	for _, sid := range adapter.ListSessions() {
		for _, uri := range []string{"tabula://session/" + sid, "tabula://session/" + sid + "/marks", "tabula://session/" + sid + "/history"} {
			out = append(out, map[string]interface{}{"uri": uri, "name": uri, "mimeType": "application/json"})
		}
	}
	return out
}

func readResource(adapter *canvas.Adapter, uri string) (map[string]interface{}, error) {
	if !strings.HasPrefix(uri, "tabula://session/") {
		return nil, fmt.Errorf("unsupported uri: %s", uri)
	}
	path := strings.TrimPrefix(uri, "tabula://session/")
	if path == "" {
		return nil, fmt.Errorf("missing session id")
	}
	parts := strings.Split(path, "/")
	sid := parts[0]
	var payload map[string]interface{}
	if len(parts) == 1 {
		payload = adapter.CanvasStatus(sid)
	} else {
		switch parts[1] {
		case "marks":
			payload = adapter.CanvasMarksList(sid, "", "", 0)
		case "history":
			payload = adapter.CanvasHistory(sid, 100)
		default:
			return nil, fmt.Errorf("unsupported session resource: %s", uri)
		}
	}
	b, _ := json.Marshal(payload)
	return map[string]interface{}{"uri": uri, "mimeType": "application/json", "text": string(b)}, nil
}

func RunStdio(adapter *canvas.Adapter) int {
	s := NewServer(adapter)
	reader := bufio.NewReader(os.Stdin)
	for {
		msg, framed, err := readMessage(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return 0
			}
			_ = writeMessage(os.Stdout, map[string]interface{}{"jsonrpc": "2.0", "id": nil, "error": RPCError{Code: -32700, Message: err.Error()}}, framed)
			continue
		}
		resp := s.DispatchMessage(msg)
		if resp == nil {
			continue
		}
		if err := writeMessage(os.Stdout, resp, framed); err != nil {
			return 1
		}
	}
}

func readMessage(r *bufio.Reader) (map[string]interface{}, bool, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		if errors.Is(err, io.EOF) && len(line) > 0 {
			// proceed
		} else {
			return nil, true, err
		}
	}
	if len(bytes.TrimSpace(line)) == 0 {
		return nil, true, io.EOF
	}
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var payload map[string]interface{}
		if err := json.Unmarshal(trimmed, &payload); err != nil {
			return nil, false, err
		}
		return payload, false, nil
	}

	headers := map[string]string{}
	for {
		t := strings.TrimSpace(string(line))
		if t == "" {
			break
		}
		parts := strings.SplitN(t, ":", 2)
		if len(parts) != 2 {
			return nil, true, fmt.Errorf("invalid header line")
		}
		headers[strings.ToLower(strings.TrimSpace(parts[0]))] = strings.TrimSpace(parts[1])
		next, err := r.ReadBytes('\n')
		if err != nil {
			return nil, true, err
		}
		line = next
	}
	lstr, ok := headers["content-length"]
	if !ok {
		return nil, true, fmt.Errorf("missing content-length header")
	}
	length, err := strconv.Atoi(lstr)
	if err != nil || length < 0 {
		return nil, true, fmt.Errorf("invalid content-length header")
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, true, err
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, true, err
	}
	return payload, true, nil
}

func writeMessage(w io.Writer, payload map[string]interface{}, framed bool) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if framed {
		if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(b)); err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	}
	_, err = w.Write(append(b, '\n'))
	return err
}
