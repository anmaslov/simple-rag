package main

import (
	"context"
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
	"confluence-rag/backend/internal/rag"
	"confluence-rag/backend/internal/search"
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
	searchSvc := search.New(repo, embedder, cfg.Search.VectorWeight, cfg.Search.KeywordWeight)
	llmClient := llm.NewOpenAI(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.Model, cfg.LLM.Temperature, cfg.LLM.SkipTLSVerify)
	ragSvc := rag.New(repo, searchSvc, llmClient, cfg.Search.TopK)
	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: httpapi.NewRouter(cfg, repo, searchSvc, ragSvc, log), ReadHeaderTimeout: 10 * time.Second}
	serverErr := make(chan error, 1)
	go func() {
		log.Info("api listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown HTTP server: %w", err)
	}
	return nil
}
