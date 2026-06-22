package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	maxResponseBodyBytes    = 16 << 20
	maxErrorResponseBytes   = 8 << 10
	responseBodyReadOverage = 1
)

func (e *OpenAIEmbedder) doRequest(ctx context.Context, body []byte, expectedCount int) ([][]float32, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, false, fmt.Errorf("create embeddings request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if e.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, false, ctxErr
		}
		return nil, true, fmt.Errorf("send embeddings request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		message, readErr := readLimited(resp.Body, maxErrorResponseBytes)
		if readErr != nil && !errors.Is(readErr, errResponseTooLarge) {
			message = nil
		}
		statusErr := fmt.Errorf("embeddings request failed with status %d", resp.StatusCode)
		if detail := strings.TrimSpace(string(message)); detail != "" {
			statusErr = fmt.Errorf("%w: %s", statusErr, detail)
		}
		return nil, isRetryableStatus(resp.StatusCode), statusErr
	}

	responseBody, err := readLimited(resp.Body, maxResponseBodyBytes)
	if err != nil {
		return nil, false, fmt.Errorf("read embeddings response: %w", err)
	}
	var response openAIEmbeddingResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return nil, false, fmt.Errorf("decode embeddings response: %w", err)
	}

	vecs, err := e.validateResponse(response, expectedCount)
	if err != nil {
		return nil, false, err
	}
	return vecs, false, nil
}

var errResponseTooLarge = errors.New("response body exceeds limit")

func readLimited(reader io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, limit+responseBodyReadOverage))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errResponseTooLarge
	}
	return body, nil
}

func isRetryableStatus(statusCode int) bool {
	return statusCode == http.StatusRequestTimeout ||
		statusCode == http.StatusTooManyRequests ||
		statusCode >= http.StatusInternalServerError
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
