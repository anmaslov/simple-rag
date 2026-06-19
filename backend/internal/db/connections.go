package db

import (
	"context"

	"confluence-rag/backend/internal/domain"
	"confluence-rag/backend/internal/models"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
)

const connectionColumns = "id,source_type,name,base_url,auth_type,username,(secret<>''),skip_tls_verify,created_at,updated_at"

func (r *Repository) CreateConnection(ctx context.Context, in domain.ConnectionInput) (models.Connection, error) {
	q, args, err := psql.Insert("source_connections").
		Columns("source_type", "name", "base_url", "auth_type", "username", "secret", "skip_tls_verify").
		Values(in.SourceType, in.Name, in.BaseURL, in.AuthType, in.Username, in.Secret, in.SkipTLSVerify).
		Suffix("RETURNING " + connectionColumns).ToSql()
	if err != nil {
		return models.Connection{}, err
	}
	return scanConnection(r.pool.QueryRow(ctx, q, args...))
}

func (r *Repository) UpdateConnection(ctx context.Context, id int64, in domain.ConnectionInput) (models.Connection, error) {
	b := psql.Update("source_connections").
		Set("name", in.Name).Set("base_url", in.BaseURL).Set("auth_type", in.AuthType).
		Set("username", in.Username).Set("skip_tls_verify", in.SkipTLSVerify).Set("updated_at", sq.Expr("now()")).
		Where(sq.Eq{"id": id, "source_type": in.SourceType})
	if in.Secret != "" {
		b = b.Set("secret", in.Secret)
	}
	q, args, err := b.Suffix("RETURNING " + connectionColumns).ToSql()
	if err != nil {
		return models.Connection{}, err
	}
	return scanConnection(r.pool.QueryRow(ctx, q, args...))
}

func (r *Repository) ListConnections(ctx context.Context, sourceType string) ([]models.Connection, error) {
	b := psql.Select(connectionColumns).From("source_connections").OrderBy("source_type", "name")
	if sourceType != "" {
		b = b.Where(sq.Eq{"source_type": sourceType})
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
	var out []models.Connection
	for rows.Next() {
		v, err := scanConnection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repository) GetConnection(ctx context.Context, id int64) (models.ConnectionSecret, error) {
	var v models.ConnectionSecret
	err := r.pool.QueryRow(ctx, `SELECT id,source_type,name,base_url,auth_type,username,(secret<>''),skip_tls_verify,created_at,updated_at,secret
		FROM source_connections WHERE id=$1`, id).
		Scan(&v.ID, &v.SourceType, &v.Name, &v.BaseURL, &v.AuthType, &v.Username, &v.HasToken, &v.SkipTLSVerify, &v.CreatedAt, &v.UpdatedAt, &v.Secret)
	return v, err
}

func (r *Repository) DeleteConnection(ctx context.Context, id int64) error {
	tag, err := r.pool.Exec(ctx, "DELETE FROM source_connections WHERE id=$1", id)
	if err == nil && tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return err
}
