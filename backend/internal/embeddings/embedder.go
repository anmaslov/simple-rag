package embeddings

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"confluence-rag/backend/internal/observability"
)

const (
	defaultHTTPTimeout = 120 * time.Second
	defaultMaxAttempts = 4
)

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// HTTPDoer is implemented by *http.Client and can be replaced in tests.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type OpenAIOption func(*OpenAIEmbedder)

// WithHTTPDoer replaces the default HTTP client.
func WithHTTPDoer(doer HTTPDoer) OpenAIOption {
	return func(embedder *OpenAIEmbedder) {
		if doer != nil {
			embedder.client = doer
		}
	}
}

// WithExpectedDimension enables validation of embedding vector dimensions.
// A non-positive value disables dimension validation.
func WithExpectedDimension(dimension int) OpenAIOption {
	return func(embedder *OpenAIEmbedder) {
		embedder.expectedDimension = dimension
	}
}

type OpenAIEmbedder struct {
	baseURL           string
	apiKey            string
	model             string
	client            HTTPDoer
	log               *slog.Logger
	expectedDimension int
	maxAttempts       int
	retryBackoff      func(attempt int) time.Duration
	sleep             func(context.Context, time.Duration) error
}

func NewOpenAI(baseURL, apiKey, model string, skipTLSVerify bool, log *slog.Logger) *OpenAIEmbedder {
	return NewOpenAIWithOptions(baseURL, apiKey, model, skipTLSVerify, log)
}

func NewOpenAIWithOptions(
	baseURL, apiKey, model string,
	skipTLSVerify bool,
	log *slog.Logger,
	options ...OpenAIOption,
) *OpenAIEmbedder {
	client := &http.Client{Timeout: defaultHTTPTimeout}
	if skipTLSVerify {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // Explicit corporate self-signed opt-in.
		client.Transport = transport
	}
	client.Transport = observability.WrapTransport(client.Transport)
	if log == nil {
		log = slog.Default()
	}

	embedder := &OpenAIEmbedder{
		baseURL:     strings.TrimRight(baseURL, "/"),
		apiKey:      apiKey,
		model:       model,
		client:      client,
		log:         log,
		maxAttempts: defaultMaxAttempts,
		retryBackoff: func(attempt int) time.Duration {
			return time.Duration(attempt) * time.Second
		},
		sleep: sleepContext,
	}
	for _, option := range options {
		if option != nil {
			option(embedder)
		}
	}
	return embedder
}

func (e *OpenAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	body, err := json.Marshal(struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
	}{
		Model: e.model,
		Input: texts,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal embeddings request: %w", err)
	}

	var lastErr error
	for attempt := 1; attempt <= e.maxAttempts; attempt++ {
		vecs, retry, err := e.doRequest(ctx, body, len(texts))
		if err == nil {
			return vecs, nil
		}
		if !retry {
			return nil, err
		}
		lastErr = err
		e.log.Warn("embedding request failed", "attempt", attempt, "error", err)

		if attempt == e.maxAttempts {
			break
		}
		if err := e.sleep(ctx, e.retryBackoff(attempt)); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
}
