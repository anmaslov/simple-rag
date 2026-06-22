package observability

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type ReadinessCheck func(context.Context) error

type AdminServer struct {
	server    *http.Server
	readiness ReadinessCheck
	started   atomic.Bool
	ready     atomic.Bool
}

func NewAdminServer(addr string, metrics *Metrics, readiness ReadinessCheck) *AdminServer {
	s := &AdminServer{readiness: readiness}
	mux := http.NewServeMux()
	mux.HandleFunc("/livez", s.liveness)
	mux.HandleFunc("/readyz", s.readinessHandler)
	mux.HandleFunc("/startupz", s.startup)
	mux.Handle("/metrics", promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{}))
	s.server = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

func (s *AdminServer) SetStarted(value bool) {
	s.started.Store(value)
}

func (s *AdminServer) SetReady(value bool) {
	s.ready.Store(value)
}

func (s *AdminServer) ListenAndServe(log *slog.Logger) error {
	log.Info("observability server listening", "addr", s.server.Addr)
	err := s.server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *AdminServer) Shutdown(ctx context.Context) error {
	s.ready.Store(false)
	return s.server.Shutdown(ctx)
}

func (s *AdminServer) liveness(w http.ResponseWriter, _ *http.Request) {
	writeHealth(w, http.StatusOK, "alive")
}

func (s *AdminServer) startup(w http.ResponseWriter, _ *http.Request) {
	if !s.started.Load() {
		writeHealth(w, http.StatusServiceUnavailable, "starting")
		return
	}
	writeHealth(w, http.StatusOK, "started")
}

func (s *AdminServer) readinessHandler(w http.ResponseWriter, r *http.Request) {
	if !s.ready.Load() {
		writeHealth(w, http.StatusServiceUnavailable, "not_ready")
		return
	}
	if s.readiness != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := s.readiness(ctx); err != nil {
			writeHealth(w, http.StatusServiceUnavailable, "dependency_unavailable")
			return
		}
	}
	writeHealth(w, http.StatusOK, "ready")
}

func writeHealth(w http.ResponseWriter, status int, state string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": state})
}
