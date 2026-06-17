package embeddings

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

type OpenAIEmbedder struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
	log     *slog.Logger
}

func NewOpenAI(baseURL, apiKey, model string, skipTLSVerify bool, log *slog.Logger) *OpenAIEmbedder {
	client := &http.Client{Timeout: 120 * time.Second}
	if skipTLSVerify {
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} //nolint:gosec // Explicit corporate self-signed opt-in.
	}
	return &OpenAIEmbedder{baseURL: baseURL, apiKey: apiKey, model: model, client: client, log: log}
}

func (e *OpenAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	body, _ := json.Marshal(map[string]any{"model": e.model, "input": texts})
	var last error
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/embeddings", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		if e.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+e.apiKey)
		}
		resp, err := e.client.Do(req)
		if err == nil && resp.StatusCode < 300 {
			defer resp.Body.Close()
			var out struct {
				Data []struct {
					Embedding []float32 `json:"embedding"`
				} `json:"data"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
				return nil, err
			}
			vecs := make([][]float32, len(out.Data))
			for i, d := range out.Data {
				vecs[i] = d.Embedding
			}
			return vecs, nil
		}
		if resp != nil {
			resp.Body.Close()
			last = fmt.Errorf("embeddings status %d", resp.StatusCode)
		} else {
			last = err
		}
		e.log.Warn("embedding request failed", "attempt", attempt+1, "error", last)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * time.Second):
		}
	}
	return nil, last
}
