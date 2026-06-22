package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"confluence-rag/backend/internal/config"
	"confluence-rag/backend/internal/domain"
	"confluence-rag/backend/internal/embeddings"
	"confluence-rag/backend/internal/models"
	"confluence-rag/backend/internal/observability"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type Worker struct {
	cfg      config.Config
	repo     domain.Repository
	embedder embeddings.Embedder
	log      *slog.Logger
	metrics  *observability.WorkerMetrics
}

func NewWorker(cfg config.Config, repo domain.Repository, embedder embeddings.Embedder, log *slog.Logger, metrics ...*observability.WorkerMetrics) *Worker {
	var workerMetrics *observability.WorkerMetrics
	if len(metrics) > 0 {
		workerMetrics = metrics[0]
	}
	return &Worker{cfg: cfg, repo: repo, embedder: embedder, log: log, metrics: workerMetrics}
}

func (w *Worker) Run(ctx context.Context) error {
	t := time.NewTicker(w.cfg.Worker.PollInterval)
	defer t.Stop()
	for {
		if err := w.tick(ctx); err != nil {
			w.log.Error("worker tick failed", "error", err)
			if w.metrics != nil {
				w.metrics.Tick("error")
			}
		} else if w.metrics != nil {
			w.metrics.Tick("success")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

func (w *Worker) tick(ctx context.Context) error {
	job, ok, err := w.repo.ClaimNextJob(ctx)
	if err != nil || !ok {
		return err
	}
	ctx, span := otel.Tracer("simple-rag/worker").Start(ctx, "sync_job",
		trace.WithAttributes(
			attribute.Int64("job.id", job.ID),
			attribute.String("job.source_type", job.SourceType),
			attribute.String("job.mode", job.Mode),
		),
	)
	defer span.End()
	finishMetrics := func(string) {}
	if w.metrics != nil {
		finishMetrics = w.metrics.StartJob(job.SourceType)
	}
	w.log.Info("claimed sync job", "job_id", job.ID, "source_type", job.SourceType, "scope_id", job.ScopeID, "mode", job.Mode)
	if err := w.runJob(ctx, job); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "sync job failed")
		finishMetrics("failed")
		w.log.Error("sync job failed", "job_id", job.ID, "error", err)
		return w.repo.FinishJob(ctx, job.ID, "failed", safeJobError(err))
	}
	if job.ScopeID != nil {
		_ = w.repo.MarkScopeSynced(ctx, *job.ScopeID)
	}
	finishMetrics("success")
	return w.repo.FinishJob(ctx, job.ID, "success", "")
}

func (w *Worker) runJob(ctx context.Context, job models.SyncJob) error {
	if job.ScopeID == nil {
		return errors.New("sync job has no source scope")
	}
	scope, err := w.repo.GetScope(ctx, *job.ScopeID)
	if err != nil {
		return fmt.Errorf("load scope: %w", err)
	}
	conn, err := w.repo.GetConnection(ctx, scope.ConnectionID)
	if err != nil {
		return fmt.Errorf("load connection: %w", err)
	}
	switch scope.SourceType {
	case models.SourceConfluence:
		return w.syncConfluence(ctx, job, conn, scope)
	case models.SourceGitLab:
		return w.syncGitLab(ctx, job, conn, scope)
	default:
		return errors.New("unsupported source type")
	}
}
