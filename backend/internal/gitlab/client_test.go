package gitlab

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestParseProject(t *testing.T) {
	tests := map[string]string{
		"group/project":                                  "group/project",
		"https://gitlab.local/group/project":             "group/project",
		"https://gitlab.local/group/sub/project.git":     "group/sub/project",
		"https://gitlab.local/group/project/-/tree/main": "group/project",
	}
	for input, want := range tests {
		got, err := ParseProject(input, "https://gitlab.local")
		if err != nil || got != want {
			t.Fatalf("ParseProject(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	if _, err := ParseProject("https://other.local/group/project", "https://gitlab.local"); err == nil {
		t.Fatal("expected host mismatch error")
	}
}

func TestListTreePagination(t *testing.T) {
	var calls int
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if got := r.Header.Get("PRIVATE-TOKEN"); got != "secret" {
			t.Fatalf("missing private token header")
		}
		page := r.URL.Query().Get("page")
		header := make(http.Header)
		body := `[{"id":"2","type":"blob","path":"b.go"}]`
		if page == "1" {
			header.Set("X-Next-Page", "2")
			body = `[{"id":"1","type":"blob","path":"a.go"}]`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(body))}, nil
	})

	client := New(Config{BaseURL: "https://gitlab.local", Token: "secret", MaxPages: 5}, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	client.http.Transport = transport
	items, err := client.ListTree(context.Background(), 7, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || calls != 2 {
		t.Fatalf("expected 2 items in 2 requests, got %d items and %d requests", len(items), calls)
	}
}

func TestSelfSignedTLSRequiresOptIn(t *testing.T) {
	server := newTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	if _, err := NewHTTPClient(false).Do(req); err == nil {
		t.Fatal("expected certificate verification failure")
	}
	req, _ = http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := NewHTTPClient(true).Do(req)
	if err != nil {
		t.Fatalf("expected explicit TLS opt-in to work: %v", err)
	}
	resp.Body.Close()
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func newTLSServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Skipf("local listeners are unavailable in this environment: %v", recovered)
		}
	}()
	return httptest.NewTLSServer(handler)
}
