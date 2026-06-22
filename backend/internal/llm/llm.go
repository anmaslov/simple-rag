package llm

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"confluence-rag/backend/internal/observability"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Messages    []Message
	Temperature float64
}

type ChatResponse struct {
	Content string
}

type LLM interface {
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
	ChatStream(ctx context.Context, req ChatRequest, onDelta func(string) error) error
}

type OpenAIClient struct {
	baseURL     string
	apiKey      string
	model       string
	defaultTemp float64
	client      *http.Client
}

func NewOpenAI(baseURL, apiKey, model string, temp float64, skipTLSVerify bool) *OpenAIClient {
	client := &http.Client{Timeout: 180 * time.Second}
	if skipTLSVerify {
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} //nolint:gosec // Explicit corporate self-signed opt-in.
	}
	client.Transport = observability.WrapTransport(client.Transport)
	return &OpenAIClient{baseURL: baseURL, apiKey: apiKey, model: model, defaultTemp: temp, client: client}
}

func (c *OpenAIClient) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	temp := req.Temperature
	if temp == 0 {
		temp = c.defaultTemp
	}
	payload, _ := json.Marshal(map[string]any{"model": c.model, "messages": req.Messages, "temperature": temp})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return ChatResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return ChatResponse{}, fmt.Errorf("llm status %d", resp.StatusCode)
	}
	var out struct {
		Choices []struct {
			Message Message `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ChatResponse{}, err
	}
	if len(out.Choices) == 0 {
		return ChatResponse{}, fmt.Errorf("llm returned no choices")
	}
	return ChatResponse{Content: out.Choices[0].Message.Content}, nil
}

func (c *OpenAIClient) ChatStream(ctx context.Context, req ChatRequest, onDelta func(string) error) error {
	temp := req.Temperature
	if temp == 0 {
		temp = c.defaultTemp
	}
	payload, _ := json.Marshal(map[string]any{
		"model":       c.model,
		"messages":    req.Messages,
		"temperature": temp,
		"stream":      true,
	})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		if len(body) > 0 {
			return fmt.Errorf("llm status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		return fmt.Errorf("llm status %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			return nil
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return err
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				if err := onDelta(choice.Delta.Content); err != nil {
					return err
				}
			}
			if choice.FinishReason != nil {
				return nil
			}
		}
	}
	return scanner.Err()
}
