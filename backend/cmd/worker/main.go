package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"confluence-rag/backend/internal/config"
	"confluence-rag/backend/internal/db"
	"confluence-rag/backend/internal/embeddings"
	"confluence-rag/backend/internal/jobs"
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
	worker := jobs.NewWorker(cfg, repo, embedder, log)
	return worker.Run(ctx)
}
