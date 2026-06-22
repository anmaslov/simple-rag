package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"confluence-rag/backend/internal/config"
	"confluence-rag/backend/internal/db"
	"confluence-rag/backend/internal/embeddings"
	"confluence-rag/backend/internal/jobs"
	"confluence-rag/backend/internal/observability"
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

	if err := run(ctx, cfg, log); err != nil && err != context.Canceled {
		log.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	shutdownTracing, err := observability.InitTracing(ctx, cfg.Observability, "simple-rag-worker")
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
	)
	metrics := observability.NewWorkerServiceMetrics(pool)
	worker := jobs.NewWorker(cfg, repo, embedder, log, metrics.Worker)
	admin := observability.NewAdminServer(cfg.Observability.Addr, metrics, pool.Ping)
	runErr := make(chan error, 2)
	go func() {
		if err := admin.ListenAndServe(log); err != nil {
			runErr <- fmt.Errorf("serve observability HTTP: %w", err)
		}
	}()
	go func() {
		if err := worker.Run(ctx); err != nil && err != context.Canceled {
			runErr <- fmt.Errorf("run worker: %w", err)
		}
	}()
	admin.SetStarted(true)
	admin.SetReady(true)

	select {
	case err := <-runErr:
		admin.SetReady(false)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = admin.Shutdown(shutdownCtx)
		return err
	case <-ctx.Done():
	}

	admin.SetReady(false)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := admin.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown observability HTTP server: %w", err)
	}
	return nil
}
