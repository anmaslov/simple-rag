package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"confluence-rag/backend/internal/config"
	"confluence-rag/backend/internal/db"
	"confluence-rag/backend/internal/embeddings"
	httpapi "confluence-rag/backend/internal/http"
	"confluence-rag/backend/internal/llm"
	"confluence-rag/backend/internal/observability"
	"confluence-rag/backend/internal/rag"
	"confluence-rag/backend/internal/search"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.LoadValidated()
	if err != nil {
		log.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, cfg, log); err != nil {
		log.Error("api stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	shutdownTracing, err := observability.InitTracing(ctx, cfg.Observability, "simple-rag-api")
	if err != nil {
		return fmt.Errorf("initialize tracing: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTracing(shutdownCtx); err != nil {
			log.Error("flush traces", "error", err)
		}
	}()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()
	repo := db.NewRepository(pool)
	embedder := embeddings.NewOpenAIWithOptions(
		cfg.Embeddings.BaseURL,
		cfg.Embeddings.APIKey,
		cfg.Embeddings.Model,
		cfg.Embeddings.SkipTLSVerify,
		log,
		embeddings.WithExpectedDimension(cfg.Embeddings.Dim),
		embeddings.WithRequestedDimension(requestedEmbeddingDimension(cfg)),
	)
	searchSvc := search.New(repo, embedder, cfg.Search.VectorWeight, cfg.Search.KeywordWeight)
	llmClient := llm.NewOpenAI(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.Model, cfg.LLM.Temperature, cfg.LLM.SkipTLSVerify)
	ragSvc := rag.New(repo, searchSvc, llmClient, cfg.Search.TopK)
	metrics := observability.NewMetrics(pool)
	router := httpapi.NewRouterWithMiddleware(cfg, repo, searchSvc, ragSvc, log, metrics.HTTPMiddleware)
	handler := otelhttp.NewHandler(router, "http.server")
	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	admin := observability.NewAdminServer(cfg.Observability.Addr, metrics, pool.Ping)
	serverErr := make(chan error, 1)
	go func() {
		log.Info("api listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- fmt.Errorf("serve API HTTP: %w", err)
		}
	}()
	go func() {
		if err := admin.ListenAndServe(log); err != nil {
			serverErr <- fmt.Errorf("serve observability HTTP: %w", err)
		}
	}()
	admin.SetStarted(true)
	admin.SetReady(true)

	var serveErr error
	select {
	case err := <-serverErr:
		log.Error("server stopped unexpectedly", "error", err)
		serveErr = err
	case <-ctx.Done():
	}

	admin.SetReady(false)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := errors.Join(srv.Shutdown(shutdownCtx), admin.Shutdown(shutdownCtx)); err != nil {
		return fmt.Errorf("shutdown HTTP servers: %w", err)
	}
	return serveErr
}

func requestedEmbeddingDimension(cfg config.Config) int {
	if !cfg.Embeddings.SendDimension {
		return 0
	}
	return cfg.Embeddings.Dim
}
