package observability

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestHTTPMetricsUseRoutePattern(t *testing.T) {
	metrics := NewMetrics()
	router := chi.NewRouter()
	router.Use(metrics.HTTPMiddleware)
	router.Get("/items/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/items/123", nil))
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/items/456", nil))

	count := testutil.ToFloat64(metrics.httpRequests.WithLabelValues(http.MethodGet, "/items/{id}", "204"))
	if count != 2 {
		t.Fatalf("request count = %v, want 2", count)
	}
}

func TestHTTPMetricsCountRecoveredPanics(t *testing.T) {
	metrics := NewMetrics()
	router := chi.NewRouter()
	router.Use(metrics.HTTPMiddleware)
	router.Use(middleware.Recoverer)
	router.Get("/panic", func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/panic", nil))

	count := testutil.ToFloat64(metrics.httpRequests.WithLabelValues(http.MethodGet, "/panic", "500"))
	if count != 1 {
		t.Fatalf("request count = %v, want 1", count)
	}
}
