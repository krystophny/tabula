package pty

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type PtydTransport struct {
	conn   *websocket.Conn
	client *http.Client
	mu     sync.Mutex
	closed bool
}

func OpenPtyd(baseURL, sessionID, cwd string, cols, rows int) (*PtydTransport, error) {
	base := strings.TrimRight(baseURL, "/")
	client := &http.Client{Timeout: 10 * time.Second}
	openPayload := map[string]interface{}{"session_id": sessionID, "cwd": cwd, "cols": cols, "rows": rows}
	b, _ := json.Marshal(openPayload)
	resp, err := client.Post(base+"/api/pty/open", "application/json", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ptyd open failed: %s", strings.TrimSpace(string(body)))
	}
	wsURL := strings.TrimPrefix(base, "http://")
	wsURL = strings.TrimPrefix(wsURL, "https://")
	proto := "ws://"
	if strings.HasPrefix(base, "https://") {
		proto = "wss://"
	}
	conn, _, err := websocket.DefaultDialer.Dial(proto+wsURL+"/ws/pty/"+sessionID, nil)
	if err != nil {
		return nil, err
	}
	return &PtydTransport{conn: conn, client: client}, nil
}

func (t *PtydTransport) Write(data []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return io.ErrClosedPipe
	}
	return t.conn.WriteMessage(websocket.BinaryMessage, data)
}

func (t *PtydTransport) Resize(cols, rows int) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return io.ErrClosedPipe
	}
	payload := map[string]interface{}{"type": "resize", "cols": cols, "rows": rows}
	b, _ := json.Marshal(payload)
	return t.conn.WriteMessage(websocket.TextMessage, b)
}

func (t *PtydTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	return t.conn.Close()
}

func (t *PtydTransport) ReadLoop(onData func([]byte) error) error {
	for {
		mt, msg, err := t.conn.ReadMessage()
		if err != nil {
			return nil
		}
		switch mt {
		case websocket.BinaryMessage, websocket.TextMessage:
			if err := onData(msg); err != nil {
				return err
			}
		default:
			return errors.New("websocket closed")
		}
	}
}
