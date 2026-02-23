package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultTTSURL     = "http://127.0.0.1:8424"
	ttsRequestTimeout = 30 * time.Second
)

func (a *App) handleTTSSpeak(conn *chatWSConn, text, lang string) {
	text = strings.TrimSpace(text)
	if text == "" {
		_ = conn.writeJSON(map[string]string{"type": "tts_error", "error": "text is required"})
		return
	}
	if lang == "" {
		lang = "en"
	}

	ttsURL := strings.TrimSpace(a.ttsURL)
	if ttsURL == "" {
		_ = conn.writeJSON(map[string]string{"type": "tts_error", "error": "TTS service not configured"})
		return
	}

	body, _ := json.Marshal(map[string]interface{}{
		"input":           text,
		"voice":           lang,
		"response_format": "wav",
	})

	ctx, cancel := context.WithTimeout(context.Background(), ttsRequestTimeout)
	defer cancel()

	upstream := fmt.Sprintf("%s/v1/audio/speech", strings.TrimRight(ttsURL, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstream, bytes.NewReader(body))
	if err != nil {
		_ = conn.writeJSON(map[string]string{"type": "tts_error", "error": "failed to create TTS request"})
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("TTS upstream error: %v", err)
		_ = conn.writeJSON(map[string]string{"type": "tts_error", "error": "TTS service unavailable"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		log.Printf("TTS upstream HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(errBody)))
		_ = conn.writeJSON(map[string]string{"type": "tts_error", "error": fmt.Sprintf("TTS error: HTTP %d", resp.StatusCode)})
		return
	}

	wavData, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("TTS read body error: %v", err)
		_ = conn.writeJSON(map[string]string{"type": "tts_error", "error": "failed to read TTS response"})
		return
	}

	_ = conn.writeBinary(wavData)
}
