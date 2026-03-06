package stt

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const ffmpegNormalizeTimeout = 25 * time.Second

// NormalizeForWhisper converts any incoming audio payload to a deterministic
// STT-service-friendly format: mono 16k WAV.
func NormalizeForWhisper(mimeType string, data []byte) (string, []byte, error) {
	if NormalizeMimeType(mimeType) == "audio/wav" && isStrictPCM16Mono16kWAV(data) {
		out := make([]byte, len(data))
		copy(out, data)
		return "audio/wav", out, nil
	}
	wav, err := transcodeToMono16kWAV(data)
	if err != nil {
		return "", nil, err
	}
	return "audio/wav", wav, nil
}

func isStrictPCM16Mono16kWAV(data []byte) bool {
	format, ok := parseWAVFormat(data)
	if !ok {
		return false
	}
	return format.audioFormat == 1 &&
		format.channels == 1 &&
		format.sampleRate == 16000 &&
		format.bitsPerSample == 16 &&
		format.dataLen > 0 &&
		format.dataLen%2 == 0
}

type wavFormat struct {
	audioFormat   uint16
	channels      uint16
	sampleRate    uint32
	bitsPerSample uint16
	dataLen       int
}

func parseWAVFormat(data []byte) (wavFormat, bool) {
	if len(data) < 44 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return wavFormat{}, false
	}

	var format wavFormat
	offset := 12
	foundFmt := false
	foundData := false
	for offset+8 <= len(data) {
		chunkID := string(data[offset : offset+4])
		chunkSize := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		offset += 8
		if chunkSize < 0 || offset > len(data) || offset+chunkSize > len(data) {
			return wavFormat{}, false
		}

		switch chunkID {
		case "fmt ":
			if chunkSize < 16 {
				return wavFormat{}, false
			}
			format.audioFormat = binary.LittleEndian.Uint16(data[offset : offset+2])
			format.channels = binary.LittleEndian.Uint16(data[offset+2 : offset+4])
			format.sampleRate = binary.LittleEndian.Uint32(data[offset+4 : offset+8])
			format.bitsPerSample = binary.LittleEndian.Uint16(data[offset+14 : offset+16])
			foundFmt = true
		case "data":
			format.dataLen = chunkSize
			foundData = true
		}

		offset += chunkSize
		if chunkSize%2 == 1 {
			offset++
		}
	}

	if !foundFmt || !foundData {
		return wavFormat{}, false
	}
	return format, true
}

func transcodeToMono16kWAV(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("audio payload is empty")
	}
	ctx, cancel := context.WithTimeout(context.Background(), ffmpegNormalizeTimeout)
	defer cancel()

	cmd := exec.CommandContext(
		ctx,
		"ffmpeg",
		"-hide_banner",
		"-loglevel", "error",
		"-nostdin",
		"-i", "pipe:0",
		"-ac", "1",
		"-ar", "16000",
		"-acodec", "pcm_s16le",
		"-f", "s16le",
		"pipe:1",
	)
	cmd.Stdin = bytes.NewReader(data)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(err.Error())
		}
		if msg == "" {
			msg = "unknown ffmpeg failure"
		}
		return nil, fmt.Errorf("ffmpeg normalize failed: %s", msg)
	}
	out := stdout.Bytes()
	if len(out) == 0 {
		return nil, fmt.Errorf("ffmpeg produced empty PCM output")
	}
	if len(out)%2 != 0 {
		return nil, fmt.Errorf("ffmpeg produced misaligned PCM output (%d bytes)", len(out))
	}
	return wrapPCM16Mono16kWAV(out), nil
}

func wrapPCM16Mono16kWAV(pcm []byte) []byte {
	dataLen := len(pcm)
	out := make([]byte, 44+dataLen)
	copy(out[0:4], "RIFF")
	binary.LittleEndian.PutUint32(out[4:8], uint32(36+dataLen))
	copy(out[8:12], "WAVE")
	copy(out[12:16], "fmt ")
	binary.LittleEndian.PutUint32(out[16:20], 16)
	binary.LittleEndian.PutUint16(out[20:22], 1) // PCM
	binary.LittleEndian.PutUint16(out[22:24], 1) // mono
	binary.LittleEndian.PutUint32(out[24:28], 16000)
	binary.LittleEndian.PutUint32(out[28:32], 32000) // sampleRate * channels * bytesPerSample
	binary.LittleEndian.PutUint16(out[32:34], 2)     // channels * bytesPerSample
	binary.LittleEndian.PutUint16(out[34:36], 16)    // bits per sample
	copy(out[36:40], "data")
	binary.LittleEndian.PutUint32(out[40:44], uint32(dataLen))
	copy(out[44:], pcm)
	return out
}
