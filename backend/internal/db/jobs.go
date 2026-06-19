package db

import (
	"context"
	"errors"

	"confluence-rag/backend/internal/models"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
)

const jobColumns = "id,status,mode,source_type,connection_id,scope_id,force_reindex,space_key,cql,started_at,finished_at,documents_found,documents_indexed,documents_skipped,pages_found,pages_indexed,pages_skipped,error_message,created_at,updated_at"

func (r *Repository) CreateSourceSyncJob(ctx context.Context, sourceType string, connectionID, scopeID int64, mode string, force bool) (models.SyncJob, error) {
	q, args, err := psql.Select("count(*)").
		From("sync_jobs").
		Where(sq.Eq{"scope_id": scopeID, "status": []string{"pending", "running"}}).
		ToSql()
	if err != nil {
		return models.SyncJob{}, err
	}
	var active int
	err = r.pool.QueryRow(ctx, q, args...).Scan(&active)
	if err != nil {
		return models.SyncJob{}, err
	}
	if active > 0 {
		return models.SyncJob{}, errors.New("a sync job is already active for this scope")
	}
	q, args, err = psql.Insert("sync_jobs").
		Columns("status", "mode", "source_type", "connection_id", "scope_id", "force_reindex").
		Values("pending", mode, sourceType, connectionID, scopeID, force).
		Suffix("RETURNING " + jobColumns).ToSql()
	if err != nil {
		return models.SyncJob{}, err
	}
	return scanJob(r.pool.QueryRow(ctx, q, args...))
}

func (r *Repository) ClaimNextJob(ctx context.Context) (models.SyncJob, bool, error) {
	q, args, err := claimNextJobQuery()
	if err != nil {
		return models.SyncJob{}, false, err
	}
	v, err := scanJob(r.pool.QueryRow(ctx, q, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.SyncJob{}, false, nil
	}
	return v, err == nil, err
}

func claimNextJobQuery() (string, []any, error) {
	runningForScope := sq.Select("1").
		From("sync_jobs r").
		Where("r.scope_id=j.scope_id").
		Where(sq.Eq{"r.status": "running"})
	candidate := sq.Select("j.id").
		From("sync_jobs j").
		Where(sq.Eq{"j.status": "pending"}).
		Where(sq.Or{
			sq.Expr("j.scope_id IS NULL"),
			sq.Expr("NOT EXISTS (?)", runningForScope),
		}).
		OrderBy("j.created_at").
		Limit(1).
		Suffix("FOR UPDATE SKIP LOCKED")
	q, args, err := psql.Update("sync_jobs").
		Set("status", "running").
		Set("started_at", sq.Expr("now()")).
		Set("updated_at", sq.Expr("now()")).
		Where(sq.Expr("id=(?)", candidate)).
		Suffix("RETURNING " + jobColumns).
		ToSql()
	return q, args, err
}

func (r *Repository) FinishJob(ctx context.Context, id int64, status, msg string) error {
	q, args, err := psql.Update("sync_jobs").
		Set("status", status).
		Set("error_message", truncate(msg, 2000)).
		Set("finished_at", sq.Expr("now()")).
		Set("updated_at", sq.Expr("now()")).
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, q, args...)
	return err
}

func (r *Repository) IncJob(ctx context.Context, id int64, found, indexed, skipped int) error {
	q, args, err := psql.Update("sync_jobs").
		Set("documents_found", sq.Expr("documents_found+?", found)).
		Set("documents_indexed", sq.Expr("documents_indexed+?", indexed)).
		Set("documents_skipped", sq.Expr("documents_skipped+?", skipped)).
		Set("pages_found", sq.Expr("pages_found+?", found)).
		Set("pages_indexed", sq.Expr("pages_indexed+?", indexed)).
		Set("pages_skipped", sq.Expr("pages_skipped+?", skipped)).
		Set("updated_at", sq.Expr("now()")).
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, q, args...)
	return err
}

func (r *Repository) ListJobs(ctx context.Context) ([]models.SyncJob, error) {
	q, args, err := psql.Select(jobColumns).
		From("sync_jobs").
		OrderBy("created_at DESC").
		Limit(100).
		ToSql()
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.SyncJob
	for rows.Next() {
		v, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
