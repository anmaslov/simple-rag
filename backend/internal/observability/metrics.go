package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	Registry *prometheus.Registry

	httpRequests *prometheus.CounterVec
	httpDuration *prometheus.HistogramVec
	httpInFlight prometheus.Gauge

	Worker *WorkerMetrics
}

type WorkerMetrics struct {
	ticks       *prometheus.CounterVec
	jobs        *prometheus.CounterVec
	jobDuration *prometheus.HistogramVec
	inFlight    prometheus.Gauge
}

func NewMetrics(pools ...*pgxpool.Pool) *Metrics {
	registry := newRegistry(pools...)
	m := &Metrics{
		Registry: registry,
		httpRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "simple_rag_http_requests_total",
			Help: "Total number of HTTP requests processed.",
		}, []string{"method", "route", "status_code"}),
		httpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "simple_rag_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route"}),
		httpInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "simple_rag_http_requests_in_flight",
			Help: "Current number of HTTP requests being processed.",
		}),
	}
	registry.MustRegister(m.httpRequests, m.httpDuration, m.httpInFlight)
	return m
}

func NewWorkerServiceMetrics(pools ...*pgxpool.Pool) *Metrics {
	registry := newRegistry(pools...)
	m := &Metrics{Registry: registry}
	m.Worker = &WorkerMetrics{
		ticks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "simple_rag_worker_ticks_total",
			Help: "Total number of worker polling iterations.",
		}, []string{"result"}),
		jobs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "simple_rag_worker_jobs_total",
			Help: "Total number of synchronization jobs processed.",
		}, []string{"source_type", "result"}),
		jobDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "simple_rag_worker_job_duration_seconds",
			Help:    "Synchronization job duration in seconds.",
			Buckets: []float64{1, 5, 15, 30, 60, 120, 300, 600, 1800, 3600},
		}, []string{"source_type"}),
		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "simple_rag_worker_jobs_in_flight",
			Help: "Current number of synchronization jobs being processed.",
		}),
	}
	registry.MustRegister(m.Worker.ticks, m.Worker.jobs, m.Worker.jobDuration, m.Worker.inFlight)
	return m
}

func newRegistry(pools ...*pgxpool.Pool) *prometheus.Registry {
	registry := prometheus.NewRegistry()
	registry.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	if len(pools) > 0 && pools[0] != nil {
		registerDatabaseMetrics(registry, pools[0])
	}
	return registry
}

func registerDatabaseMetrics(registry *prometheus.Registry, pool *pgxpool.Pool) {
	registry.MustRegister(
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "simple_rag_db_pool_connections_acquired",
			Help: "Number of database connections currently acquired by the application.",
		}, func() float64 { return float64(pool.Stat().AcquiredConns()) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "simple_rag_db_pool_connections_idle",
			Help: "Number of idle database connections.",
		}, func() float64 { return float64(pool.Stat().IdleConns()) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "simple_rag_db_pool_connections_total",
			Help: "Total number of database connections in the pool.",
		}, func() float64 { return float64(pool.Stat().TotalConns()) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "simple_rag_db_pool_connections_max",
			Help: "Maximum number of database connections allowed in the pool.",
		}, func() float64 { return float64(pool.Stat().MaxConns()) }),
	)
}

func (m *Metrics) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		m.httpInFlight.Inc()
		defer m.httpInFlight.Dec()

		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		route := "unmatched"
		if routeContext := chi.RouteContext(r.Context()); routeContext != nil {
			if pattern := routeContext.RoutePattern(); pattern != "" {
				route = pattern
			}
		}
		status := ww.Status()
		if status == 0 {
			status = http.StatusOK
		}
		m.httpRequests.WithLabelValues(r.Method, route, strconv.Itoa(status)).Inc()
		m.httpDuration.WithLabelValues(r.Method, route).Observe(time.Since(started).Seconds())
	})
}

func (m *WorkerMetrics) Tick(result string) {
	m.ticks.WithLabelValues(result).Inc()
}

func (m *WorkerMetrics) StartJob(sourceType string) func(result string) {
	started := time.Now()
	m.inFlight.Inc()
	return func(result string) {
		m.inFlight.Dec()
		m.jobs.WithLabelValues(sourceType, result).Inc()
		m.jobDuration.WithLabelValues(sourceType).Observe(time.Since(started).Seconds())
	}
}
