package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"confluence-rag/backend/internal/config"
	"confluence-rag/backend/internal/confluence"
	"confluence-rag/backend/internal/db"
	"confluence-rag/backend/internal/embeddings"
	"confluence-rag/backend/internal/jobs"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("db connect failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	repo := db.NewRepository(pool)
	cf := confluence.New(cfg.Confluence, log)
	embedder := embeddings.NewOpenAI(cfg.Embeddings.BaseURL, cfg.Embeddings.APIKey, cfg.Embeddings.Model, cfg.Embeddings.SkipTLSVerify, log)
	worker := jobs.NewWorker(cfg, repo, cf, embedder, log)
	if err := worker.Run(ctx); err != nil && err != context.Canceled {
		log.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}
