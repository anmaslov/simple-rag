package db

import (
	"context"
	"encoding/json"

	"confluence-rag/backend/internal/domain"
	"confluence-rag/backend/internal/models"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
)

const scopeColumns = "id,connection_id,source_type,scope_type,external_id,name,config,enabled,last_synced_at,created_at,updated_at"

func (r *Repository) CreateScope(ctx context.Context, in domain.ScopeInput) (models.SourceScope, error) {
	cfg := in.Config
	if len(cfg) == 0 {
		cfg = json.RawMessage("{}")
	}
	q, args, err := psql.Insert("source_scopes").
		Columns("connection_id", "source_type", "scope_type", "external_id", "name", "config").
		Values(in.ConnectionID, in.SourceType, in.ScopeType, in.ExternalID, in.Name, cfg).
		Suffix(`ON CONFLICT(connection_id,scope_type,external_id) DO UPDATE SET name=excluded.name,config=excluded.config,enabled=true,updated_at=now() RETURNING ` + scopeColumns).
		ToSql()
	if err != nil {
		return models.SourceScope{}, err
	}
	return scanScope(r.pool.QueryRow(ctx, q, args...))
}

func (r *Repository) ListScopes(ctx context.Context, sourceType string, connectionID int64) ([]models.SourceScope, error) {
	b := psql.Select(scopeColumns).From("source_scopes").OrderBy("source_type", "name")
	if sourceType != "" {
		b = b.Where(sq.Eq{"source_type": sourceType})
	}
	if connectionID > 0 {
		b = b.Where(sq.Eq{"connection_id": connectionID})
	}
	q, args, err := b.ToSql()
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.SourceScope
	for rows.Next() {
		v, err := scanScope(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repository) GetScope(ctx context.Context, id int64) (models.SourceScope, error) {
	q, args, err := psql.Select(scopeColumns).From("source_scopes").Where(sq.Eq{"id": id}).ToSql()
	if err != nil {
		return models.SourceScope{}, err
	}
	return scanScope(r.pool.QueryRow(ctx, q, args...))
}

func (r *Repository) DeleteScope(ctx context.Context, id int64) error {
	q, args, err := psql.Delete("source_scopes").Where(sq.Eq{"id": id}).ToSql()
	if err != nil {
		return err
	}
	tag, err := r.pool.Exec(ctx, q, args...)
	if err == nil && tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return err
}

func (r *Repository) MarkScopeSynced(ctx context.Context, id int64) error {
	q, args, err := psql.Update("source_scopes").
		Set("last_synced_at", sq.Expr("now()")).
		Set("updated_at", sq.Expr("now()")).
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, q, args...)
	return err
}
