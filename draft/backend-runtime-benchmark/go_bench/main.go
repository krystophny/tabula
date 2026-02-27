package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	datasetSize   = 512
	throughputOps = 300000
	latencyOps    = 50000
)

type requestPayload struct {
	TurnID      string            `json:"turn_id"`
	ProjectKey  string            `json:"project_key"`
	Message     string            `json:"message"`
	ChatMode    string            `json:"chat_mode"`
	RecentFiles []string          `json:"recent_files"`
	Flags       map[string]bool   `json:"flags"`
	Meta        map[string]string `json:"meta"`
	Timestamp   int64             `json:"timestamp"`
}

type responsePayload struct {
	OK             bool   `json:"ok"`
	TurnID         string `json:"turn_id"`
	Intent         string `json:"intent"`
	TokenCount     int    `json:"token_count"`
	RenderOnCanvas bool   `json:"render_on_canvas"`
	HashPrefix     string `json:"hash_prefix"`
}

type benchmarkResult struct {
	Runtime             string             `json:"runtime"`
	DatasetSize         int                `json:"dataset_size"`
	ThroughputOps       int                `json:"throughput_ops"`
	LatencySamples      int                `json:"latency_samples"`
	ThroughputOpsPerSec float64            `json:"throughput_ops_per_sec"`
	LatencyUS           map[string]float64 `json:"latency_us"`
	Checksum            uint64             `json:"checksum"`
}

func makePayloads() []string {
	actions := []string{"review this patch", "switch project alpha", "cancel active turn", "open pr review", "summarize the latest logs"}
	payloads := make([]string, 0, datasetSize)
	for i := 0; i < datasetSize; i++ {
		action := actions[i%len(actions)]
		repeat := 4 + (i % 9)
		parts := make([]string, 0, repeat)
		for j := 0; j < repeat; j++ {
			parts = append(parts, fmt.Sprintf("token%d_%d", i%17, j))
		}
		req := requestPayload{
			TurnID:      fmt.Sprintf("turn-%06d", i),
			ProjectKey:  fmt.Sprintf("/workspace/proj-%02d", i%11),
			Message:     fmt.Sprintf("Please %s while handling backend request %d. %s", action, i%97, strings.Join(parts, " ")),
			ChatMode:    "chat",
			RecentFiles: []string{"internal/web/chat.go", "internal/web/server.go", "internal/web/static/app.js"},
			Flags: map[string]bool{
				"silent":       i%3 == 0,
				"conversation": i%2 == 0,
			},
			Meta: map[string]string{
				"branch": "fix/tap-stop-working",
				"model":  "spark",
			},
			Timestamp: 1700000000 + int64(i),
		}
		raw, err := json.Marshal(req)
		if err != nil {
			panic(err)
		}
		payloads = append(payloads, string(raw))
	}
	return payloads
}

func detectIntent(text string) string {
	switch {
	case strings.Contains(text, "open pr") || strings.Contains(text, "review"):
		return "open_pr_review"
	case strings.Contains(text, "switch project"):
		return "switch_project"
	case strings.Contains(text, "cancel"):
		return "cancel_turn"
	default:
		return "chat"
	}
}

func handle(raw string) (int, byte) {
	var req requestPayload
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		panic(err)
	}
	normalized := strings.ToLower(strings.TrimSpace(req.Message))
	tokenCount := len(strings.Fields(normalized))
	sum := sha256.Sum256([]byte(req.ProjectKey + "|" + req.TurnID + "|" + normalized))
	response := responsePayload{
		OK:             true,
		TurnID:         req.TurnID,
		Intent:         detectIntent(normalized),
		TokenCount:     tokenCount,
		RenderOnCanvas: tokenCount > 30 || strings.Contains(normalized, "diff"),
		HashPrefix:     hex.EncodeToString(sum[:8]),
	}
	out, err := json.Marshal(response)
	if err != nil {
		panic(err)
	}
	first := byte(0)
	if len(out) > 0 {
		first = out[0]
	}
	return len(out), first
}

func percentile(samples []float64, p float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	if p <= 0 {
		return samples[0]
	}
	if p >= 1 {
		return samples[len(samples)-1]
	}
	index := int(p * float64(len(samples)-1))
	return samples[index]
}

func main() {
	payloads := makePayloads()
	var checksum uint64

	startThroughput := time.Now()
	for i := 0; i < throughputOps; i++ {
		n, b := handle(payloads[i%datasetSize])
		checksum += uint64(n) + uint64(b)
	}
	elapsedThroughput := time.Since(startThroughput).Seconds()

	samples := make([]float64, 0, latencyOps)
	for i := 0; i < latencyOps; i++ {
		raw := payloads[(i*7)%datasetSize]
		start := time.Now()
		n, b := handle(raw)
		dtUS := float64(time.Since(start).Nanoseconds()) / 1000.0
		samples = append(samples, dtUS)
		checksum += uint64(n) + uint64(b)
	}
	sort.Float64s(samples)

	result := benchmarkResult{
		Runtime:             "go",
		DatasetSize:         datasetSize,
		ThroughputOps:       throughputOps,
		LatencySamples:      latencyOps,
		ThroughputOpsPerSec: float64(throughputOps) / elapsedThroughput,
		LatencyUS: map[string]float64{
			"p50": percentile(samples, 0.50),
			"p95": percentile(samples, 0.95),
			"p99": percentile(samples, 0.99),
		},
		Checksum: checksum,
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(encoded))
}
