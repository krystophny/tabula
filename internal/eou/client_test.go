package eou

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientPredict(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/eou/predict" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"p_end": 0.72,
				"label": "end",
				"model": "test",
			})
		}))
		defer ts.Close()

		client := NewClient(ts.URL, 2*time.Second)
		resp, err := client.Predict(context.Background(), PredictRequest{Text: "done now"})
		if err != nil {
			t.Fatalf("predict failed: %v", err)
		}
		if resp.PEnd != 0.72 {
			t.Fatalf("unexpected p_end: %.2f", resp.PEnd)
		}
	})

	t.Run("invalid p_end rejected", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"p_end": 1.7})
		}))
		defer ts.Close()

		client := NewClient(ts.URL, 2*time.Second)
		_, err := client.Predict(context.Background(), PredictRequest{Text: "done now"})
		if err == nil {
			t.Fatal("expected error")
		}
	})
}
