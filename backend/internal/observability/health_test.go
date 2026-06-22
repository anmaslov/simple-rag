package observability

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthProbeSemantics(t *testing.T) {
	dependencyErr := errors.New("database unavailable")
	admin := NewAdminServer(":0", NewMetrics(), func(context.Context) error {
		return dependencyErr
	})

	assertProbeStatus(t, admin.server.Handler, "/livez", http.StatusOK)
	assertProbeStatus(t, admin.server.Handler, "/startupz", http.StatusServiceUnavailable)
	assertProbeStatus(t, admin.server.Handler, "/readyz", http.StatusServiceUnavailable)

	admin.SetStarted(true)
	admin.SetReady(true)
	assertProbeStatus(t, admin.server.Handler, "/startupz", http.StatusOK)
	assertProbeStatus(t, admin.server.Handler, "/livez", http.StatusOK)
	assertProbeStatus(t, admin.server.Handler, "/readyz", http.StatusServiceUnavailable)

	dependencyErr = nil
	assertProbeStatus(t, admin.server.Handler, "/readyz", http.StatusOK)
}

func assertProbeStatus(t *testing.T, handler http.Handler, path string, want int) {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	if recorder.Code != want {
		t.Fatalf("%s status = %d, want %d", path, recorder.Code, want)
	}
}
