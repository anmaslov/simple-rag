package db

import (
	"context"
	"errors"

	"confluence-rag/backend/internal/models"

	"github.com/jackc/pgx/v5"
)

const jobColumns = "id,status,mode,source_type,connection_id,scope_id,force_reindex,space_key,cql,started_at,finished_at,documents_found,documents_indexed,documents_skipped,pages_found,pages_indexed,pages_skipped,error_message,created_at,updated_at"

func (r *Repository) CreateSourceSyncJob(ctx context.Context, sourceType string, connectionID, scopeID int64, mode string, force bool) (models.SyncJob, error) {
	var active int
	err := r.pool.QueryRow(ctx, "SELECT count(*) FROM sync_jobs WHERE scope_id=$1 AND status IN ('pending','running')", scopeID).Scan(&active)
	if err != nil {
		return models.SyncJob{}, err
	}
	if active > 0 {
		return models.SyncJob{}, errors.New("a sync job is already active for this scope")
	}
	q, args, err := psql.Insert("sync_jobs").
		Columns("status", "mode", "source_type", "connection_id", "scope_id", "force_reindex").
		Values("pending", mode, sourceType, connectionID, scopeID, force).
		Suffix("RETURNING " + jobColumns).ToSql()
	if err != nil {
		return models.SyncJob{}, err
	}
	return scanJob(r.pool.QueryRow(ctx, q, args...))
}

func (r *Repository) ClaimNextJob(ctx context.Context) (models.SyncJob, bool, error) {
	q := `UPDATE sync_jobs SET status='running',started_at=now(),updated_at=now()
		WHERE id=(SELECT j.id FROM sync_jobs j
			WHERE j.status='pending'
			  AND (j.scope_id IS NULL OR NOT EXISTS (
			    SELECT 1 FROM sync_jobs r WHERE r.scope_id=j.scope_id AND r.status='running'
			  ))
			ORDER BY j.created_at LIMIT 1 FOR UPDATE SKIP LOCKED)
		RETURNING ` + jobColumns
	v, err := scanJob(r.pool.QueryRow(ctx, q))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.SyncJob{}, false, nil
	}
	return v, err == nil, err
}

func (r *Repository) FinishJob(ctx context.Context, id int64, status, msg string) error {
	_, err := r.pool.Exec(ctx, "UPDATE sync_jobs SET status=$2,error_message=$3,finished_at=now(),updated_at=now() WHERE id=$1", id, status, truncate(msg, 2000))
	return err
}

func (r *Repository) IncJob(ctx context.Context, id int64, found, indexed, skipped int) error {
	_, err := r.pool.Exec(ctx, `UPDATE sync_jobs SET
		documents_found=documents_found+$2,documents_indexed=documents_indexed+$3,documents_skipped=documents_skipped+$4,
		pages_found=pages_found+$2,pages_indexed=pages_indexed+$3,pages_skipped=pages_skipped+$4,updated_at=now()
		WHERE id=$1`, id, found, indexed, skipped)
	return err
}

func (r *Repository) ListJobs(ctx context.Context) ([]models.SyncJob, error) {
	rows, err := r.pool.Query(ctx, "SELECT "+jobColumns+" FROM sync_jobs ORDER BY created_at DESC LIMIT 100")
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
