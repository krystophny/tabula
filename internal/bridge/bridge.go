package bridge

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func Run(mcpURL string) int {
	reader := bufio.NewReader(os.Stdin)
	client := &http.Client{}
	for {
		msg, framed, err := readMessage(reader)
		if err != nil {
			if err == io.EOF {
				return 0
			}
			_ = writeMessage(os.Stdout, map[string]interface{}{"jsonrpc": "2.0", "id": nil, "error": map[string]interface{}{"code": -32700, "message": err.Error()}}, framed)
			continue
		}
		b, _ := json.Marshal(msg)
		resp, err := client.Post(mcpURL, "application/json", bytes.NewReader(b))
		if err != nil {
			_ = writeMessage(os.Stdout, map[string]interface{}{"jsonrpc": "2.0", "id": msg["id"], "error": map[string]interface{}{"code": -32000, "message": err.Error()}}, framed)
			continue
		}
		if resp.StatusCode == http.StatusAccepted {
			_ = resp.Body.Close()
			continue
		}
		var out map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			_ = resp.Body.Close()
			continue
		}
		_ = resp.Body.Close()
		_ = writeMessage(os.Stdout, out, framed)
	}
}

func readMessage(r *bufio.Reader) (map[string]interface{}, bool, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		if err == io.EOF {
			return nil, true, io.EOF
		}
		return nil, true, err
	}
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 {
		return nil, true, io.EOF
	}
	if trimmed[0] == '{' {
		var m map[string]interface{}
		if err := json.Unmarshal(trimmed, &m); err != nil {
			return nil, false, err
		}
		return m, false, nil
	}
	headers := map[string]string{}
	for {
		t := strings.TrimSpace(string(line))
		if t == "" {
			break
		}
		parts := strings.SplitN(t, ":", 2)
		if len(parts) != 2 {
			return nil, true, fmt.Errorf("invalid header")
		}
		headers[strings.ToLower(strings.TrimSpace(parts[0]))] = strings.TrimSpace(parts[1])
		next, err := r.ReadBytes('\n')
		if err != nil {
			return nil, true, err
		}
		line = next
	}
	l, err := strconv.Atoi(headers["content-length"])
	if err != nil {
		return nil, true, err
	}
	body := make([]byte, l)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, true, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, true, err
	}
	return m, true, nil
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
