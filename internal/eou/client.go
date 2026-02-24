package eou

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultURL         = "http://127.0.0.1:8425"
	DefaultTimeout     = 800 * time.Millisecond
	DefaultCommitScore = 0.60
)

type Client struct {
	baseURL string
	http    *http.Client
}

type PredictRequest struct {
	Text     string `json:"text"`
	LangHint string `json:"lang_hint,omitempty"`
}

type PredictResponse struct {
	PEnd   float64 `json:"p_end"`
	Label  string  `json:"label,omitempty"`
	Model  string  `json:"model,omitempty"`
	Reason string  `json:"reason,omitempty"`
}

func NewClient(baseURL string, timeout time.Duration) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = DefaultURL
	}
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: timeout},
	}
}

func (c *Client) Predict(ctx context.Context, req PredictRequest) (PredictResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return PredictResponse{}, fmt.Errorf("marshal predict request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/eou/predict", bytes.NewReader(body))
	if err != nil {
		return PredictResponse{}, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return PredictResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return PredictResponse{}, fmt.Errorf("eou HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out PredictResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return PredictResponse{}, fmt.Errorf("decode response: %w", err)
	}
	if out.PEnd < 0 || out.PEnd > 1 {
		return PredictResponse{}, fmt.Errorf("invalid p_end %.4f", out.PEnd)
	}
	return out, nil
}
