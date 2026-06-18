package main

import (
	"context"
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
	embedder := embeddings.NewOpenAI(cfg.Embeddings.BaseURL, cfg.Embeddings.APIKey, cfg.Embeddings.Model, cfg.Embeddings.SkipTLSVerify, log)
	searchSvc := search.New(repo, embedder, cfg.Search.VectorWeight, cfg.Search.KeywordWeight)
	llmClient := llm.NewOpenAI(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.Model, cfg.LLM.Temperature, cfg.LLM.SkipTLSVerify)
	ragSvc := rag.New(repo, searchSvc, llmClient, cfg.Search.TopK)
	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: httpapi.NewRouter(cfg, repo, searchSvc, ragSvc, log), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		log.Info("api listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("api failed", "error", err)
			stop()
		}
	}()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
