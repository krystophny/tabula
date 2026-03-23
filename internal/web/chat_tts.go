package web

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
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

func (a *App) handleTTSSpeak(sessionID string, conn *chatWSConn, seq int64, text, lang string) {
	workspacePath := ""
	if session, err := a.store.GetChatSession(sessionID); err == nil {
		workspacePath = session.WorkspacePath
		if strings.TrimSpace(workspacePath) != "" {
			a.broadcastCompanionRuntimeState(workspacePath, companionRuntimeSnapshot{
				State:         companionRuntimeStateTalking,
				Reason:        "tts_started",
				WorkspacePath: workspacePath,
				OutputMode:    turnOutputModeVoice,
			})
		}
	}
	if conn == nil {
		return
	}
	conn.waitTTSStreamTurn(seq)
	defer conn.finishTTSStreamTurn(seq)

	delivered, clientErr := a.streamTTSAudio(sessionID, conn, seq, text, lang)
	if clientErr != "" {
		log.Printf("tts emit error: session=%s seq=%d err=%s", sessionID, seq, clientErr)
		if workspacePath != "" {
			a.broadcastCompanionRuntimeState(workspacePath, companionRuntimeSnapshot{
				State:         companionRuntimeStateError,
				Reason:        "tts_failed",
				Error:         clientErr,
				WorkspacePath: workspacePath,
				OutputMode:    turnOutputModeVoice,
			})
		}
		_ = conn.writeJSON(map[string]string{"type": "tts_error", "error": clientErr})
		return
	}
	_ = conn.writeJSON(map[string]any{"type": "tts_done", "seq": seq, "chunks": delivered})
	if workspacePath != "" {
		if project, err := a.store.GetWorkspaceByStoredPath(workspacePath); err == nil {
			a.settleCompanionRuntimeState(workspacePath, a.loadCompanionConfig(project), "tts_completed")
		}
	}
}

func (a *App) synthesizeTTSAudio(sessionID string, seq int64, text, lang string) ([]byte, string) {
	text = strings.TrimSpace(text)
	if text == "" {
		log.Printf("tts dropped: session=%s seq=%d reason=empty_text", sessionID, seq)
		return nil, "text is required"
	}
	if lang == "" {
		lang = "en"
	}

	ttsURL := strings.TrimSpace(a.ttsURL)
	if ttsURL == "" {
		log.Printf("tts dropped: session=%s seq=%d reason=service_not_configured", sessionID, seq)
		return nil, "TTS service not configured"
	}
	log.Printf("tts start: session=%s seq=%d chars=%d lang=%q", sessionID, seq, len([]rune(text)), strings.TrimSpace(lang))

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
		log.Printf("tts request build error: session=%s seq=%d err=%v", sessionID, seq, err)
		return nil, "failed to create TTS request"
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("tts upstream error: session=%s seq=%d err=%v", sessionID, seq, err)
		return nil, "TTS service unavailable"
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		log.Printf("tts upstream HTTP %d: session=%s seq=%d body=%s", resp.StatusCode, sessionID, seq, strings.TrimSpace(string(errBody)))
		return nil, fmt.Sprintf("TTS error: HTTP %d", resp.StatusCode)
	}

	wavData, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("tts read body error: session=%s seq=%d err=%v", sessionID, seq, err)
		return nil, "failed to read TTS response"
	}
	return wavData, ""
}

type streamingTTSChunk struct {
	Audio string `json:"audio"`
}

func (a *App) streamTTSAudio(sessionID string, conn *chatWSConn, seq int64, text, lang string) (int, string) {
	text = strings.TrimSpace(text)
	if text == "" {
		log.Printf("tts dropped: session=%s seq=%d reason=empty_text", sessionID, seq)
		return 0, "text is required"
	}
	if lang == "" {
		lang = "en"
	}

	ttsURL := strings.TrimSpace(a.ttsURL)
	if ttsURL == "" {
		log.Printf("tts dropped: session=%s seq=%d reason=service_not_configured", sessionID, seq)
		return 0, "TTS service not configured"
	}
	log.Printf("tts start: session=%s seq=%d chars=%d lang=%q stream=true", sessionID, seq, len([]rune(text)), strings.TrimSpace(lang))

	body, _ := json.Marshal(map[string]interface{}{
		"input":           text,
		"voice":           lang,
		"response_format": "wav",
		"stream":          true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), ttsRequestTimeout)
	defer cancel()

	upstream := fmt.Sprintf("%s/v1/audio/speech", strings.TrimRight(ttsURL, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstream, bytes.NewReader(body))
	if err != nil {
		log.Printf("tts request build error: session=%s seq=%d err=%v", sessionID, seq, err)
		return 0, "failed to create TTS request"
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("tts upstream error: session=%s seq=%d err=%v", sessionID, seq, err)
		return 0, "TTS service unavailable"
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		log.Printf("tts upstream HTTP %d: session=%s seq=%d body=%s", resp.StatusCode, sessionID, seq, strings.TrimSpace(string(errBody)))
		return 0, fmt.Sprintf("TTS error: HTTP %d", resp.StatusCode)
	}
	contentType := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if strings.HasPrefix(contentType, "audio/") {
		audio, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("tts raw read error: session=%s seq=%d err=%v", sessionID, seq, err)
			return 0, "failed to read TTS audio"
		}
		if len(audio) <= 44 {
			return 0, "TTS stream returned no audio"
		}
		if err := conn.writeBinary(audio); err != nil {
			log.Printf("tts websocket write error: session=%s seq=%d bytes=%d err=%v", sessionID, seq, len(audio), err)
			return 0, err.Error()
		}
		log.Printf("tts delivered: session=%s seq=%d chunk=%d bytes=%d raw=true", sessionID, seq, 1, len(audio))
		return 1, ""
	}

	scanner := bufio.NewScanner(resp.Body)
	const maxStreamingLine = 16 * 1024 * 1024
	scanner.Buffer(make([]byte, 0, 64*1024), maxStreamingLine)
	delivered := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var chunk streamingTTSChunk
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			log.Printf("tts stream decode error: session=%s seq=%d err=%v", sessionID, seq, err)
			return delivered, "failed to decode TTS stream"
		}
		audio, err := base64.StdEncoding.DecodeString(strings.TrimSpace(chunk.Audio))
		if err != nil {
			log.Printf("tts stream base64 error: session=%s seq=%d err=%v", sessionID, seq, err)
			return delivered, "failed to decode TTS chunk"
		}
		if len(audio) <= 44 {
			continue
		}
		if err := conn.writeBinary(audio); err != nil {
			log.Printf("tts websocket write error: session=%s seq=%d bytes=%d err=%v", sessionID, seq, len(audio), err)
			return delivered, err.Error()
		}
		delivered++
		log.Printf("tts delivered: session=%s seq=%d chunk=%d bytes=%d", sessionID, seq, delivered, len(audio))
	}
	if err := scanner.Err(); err != nil {
		log.Printf("tts stream read error: session=%s seq=%d err=%v", sessionID, seq, err)
		return delivered, "failed to read TTS stream"
	}
	if delivered == 0 {
		return 0, "TTS stream returned no audio"
	}
	return delivered, ""
}
