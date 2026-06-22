package embeddings

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestOpenAIEmbedderEmbedSuccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		response string
		want     [][]float32
	}{
		{
			name: "ordered",
			response: `{"data":[
				{"index":0,"embedding":[1,2]},
				{"index":1,"embedding":[3,4]}
			]}`,
			want: [][]float32{{1, 2}, {3, 4}},
		},
		{
			name: "reordered",
			response: `{"data":[
				{"index":1,"embedding":[3,4]},
				{"index":0,"embedding":[1,2]}
			]}`,
			want: [][]float32{{1, 2}, {3, 4}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assertRequest(t, r, []string{"first", "second"})
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tt.response)
			}))
			defer server.Close()

			embedder := newTestEmbedder(server, WithExpectedDimension(2))
			got, err := embedder.Embed(context.Background(), []string{"first", "second"})
			if err != nil {
				t.Fatalf("Embed() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Embed() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestOpenAIEmbedderContractErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		response string
		wantKind ContractErrorKind
	}{
		{
			name:     "wrong response count",
			response: `{"data":[{"index":0,"embedding":[1,2]}]}`,
			wantKind: ContractResponseCount,
		},
		{
			name: "missing index",
			response: `{"data":[
				{"embedding":[1,2]},
				{"index":1,"embedding":[3,4]}
			]}`,
			wantKind: ContractMissingIndex,
		},
		{
			name: "duplicate index",
			response: `{"data":[
				{"index":0,"embedding":[1,2]},
				{"index":0,"embedding":[3,4]}
			]}`,
			wantKind: ContractDuplicateIndex,
		},
		{
			name: "out of range index",
			response: `{"data":[
				{"index":0,"embedding":[1,2]},
				{"index":2,"embedding":[3,4]}
			]}`,
			wantKind: ContractIndexOutOfRange,
		},
		{
			name: "wrong dimension",
			response: `{"data":[
				{"index":0,"embedding":[1,2]},
				{"index":1,"embedding":[3]}
			]}`,
			wantKind: ContractWrongDimension,
		},
		{
			name: "empty vector",
			response: `{"data":[
				{"index":0,"embedding":[1,2]},
				{"index":1,"embedding":[]}
			]}`,
			wantKind: ContractEmptyVector,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tt.response)
			}))
			defer server.Close()

			embedder := newTestEmbedder(server, WithExpectedDimension(2))
			_, err := embedder.Embed(context.Background(), []string{"first", "second"})
			var contractErr *ContractError
			if !errors.As(err, &contractErr) {
				t.Fatalf("Embed() error = %v, want *ContractError", err)
			}
			if contractErr.Kind != tt.wantKind {
				t.Fatalf("ContractError.Kind = %q, want %q", contractErr.Kind, tt.wantKind)
			}
		})
	}
}

func TestOpenAIEmbedderStatuses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		statuses       []int
		wantErr        bool
		wantRequests   int32
		wantSleepCalls int32
	}{
		{
			name:           "retryable eventually succeeds",
			statuses:       []int{http.StatusTooManyRequests, http.StatusBadGateway, http.StatusOK},
			wantRequests:   3,
			wantSleepCalls: 2,
		},
		{
			name:           "retryable exhausts attempts without final sleep",
			statuses:       []int{http.StatusServiceUnavailable},
			wantErr:        true,
			wantRequests:   defaultMaxAttempts,
			wantSleepCalls: defaultMaxAttempts - 1,
		},
		{
			name:         "non retryable",
			statuses:     []int{http.StatusBadRequest},
			wantErr:      true,
			wantRequests: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requestNumber := requests.Add(1)
				statusIndex := min(int(requestNumber)-1, len(tt.statuses)-1)
				status := tt.statuses[statusIndex]
				w.WriteHeader(status)
				if status == http.StatusOK {
					_, _ = io.WriteString(w, `{"data":[{"index":0,"embedding":[1,2]}]}`)
					return
				}
				_, _ = io.WriteString(w, `{"error":"temporary failure"}`)
			}))
			defer server.Close()

			embedder := newTestEmbedder(server, WithExpectedDimension(2))
			var sleepCalls atomic.Int32
			embedder.sleep = func(context.Context, time.Duration) error {
				sleepCalls.Add(1)
				return nil
			}

			_, err := embedder.Embed(context.Background(), []string{"text"})
			if (err != nil) != tt.wantErr {
				t.Fatalf("Embed() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got := requests.Load(); got != tt.wantRequests {
				t.Fatalf("request count = %d, want %d", got, tt.wantRequests)
			}
			if got := sleepCalls.Load(); got != tt.wantSleepCalls {
				t.Fatalf("sleep count = %d, want %d", got, tt.wantSleepCalls)
			}
		})
	}
}

func TestOpenAIEmbedderCancellation(t *testing.T) {
	t.Parallel()

	requestHandled := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		requestHandled <- struct{}{}
	}))
	defer server.Close()

	embedder := newTestEmbedder(server)
	embedder.retryBackoff = func(int) time.Duration { return time.Hour }

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := embedder.Embed(ctx, []string{"text"})
		errCh <- err
	}()

	select {
	case <-requestHandled:
		cancel()
	case <-time.After(2 * time.Second):
		t.Fatal("request was not handled")
	}

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Embed() error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Embed() did not return after cancellation")
	}
}

func newTestEmbedder(server *httptest.Server, options ...OpenAIOption) *OpenAIEmbedder {
	options = append([]OpenAIOption{WithHTTPDoer(server.Client())}, options...)
	return NewOpenAIWithOptions(
		server.URL,
		"secret",
		"test-model",
		false,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		options...,
	)
}

func assertRequest(t *testing.T, request *http.Request, wantInput []string) {
	t.Helper()

	if request.Method != http.MethodPost {
		t.Errorf("method = %q, want POST", request.Method)
	}
	if request.URL.Path != "/embeddings" {
		t.Errorf("path = %q, want /embeddings", request.URL.Path)
	}
	if got := request.Header.Get("Authorization"); got != "Bearer secret" {
		t.Errorf("Authorization = %q, want bearer token", got)
	}

	var body struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
	}
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if body.Model != "test-model" {
		t.Errorf("model = %q, want test-model", body.Model)
	}
	if !reflect.DeepEqual(body.Input, wantInput) {
		t.Errorf("input = %#v, want %#v", body.Input, wantInput)
	}
}
