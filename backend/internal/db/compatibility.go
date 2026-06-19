package db

import (
	"context"

	"confluence-rag/backend/internal/models"

	sq "github.com/Masterminds/squirrel"
)

func (r *Repository) UpsertSpace(ctx context.Context, key, name string) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO confluence_spaces(space_key,name) VALUES($1,$2)
		ON CONFLICT(space_key) DO UPDATE SET name=excluded.name,updated_at=now()`, key, name)
	return err
}

func (r *Repository) ListSpaces(ctx context.Context) ([]models.Space, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,external_id,name FROM source_scopes WHERE source_type='confluence' AND scope_type='space' ORDER BY external_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Space
	for rows.Next() {
		var v models.Space
		if err := rows.Scan(&v.ID, &v.Key, &v.Name); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repository) ListPages(ctx context.Context, space, qText string) ([]models.Page, error) {
	b := psql.Select(`id,external_id,coalesce(metadata->>'space_key',''),title,url,
		coalesce((metadata->>'version')::int,0),coalesce(metadata->>'status','current'),content_hash,content,
		source_updated_at,indexed_at,created_at,updated_at`).From("documents").Where(sq.Eq{"source_type": models.SourceConfluence}).OrderBy("source_updated_at DESC NULLS LAST")
	if space != "" {
		b = b.Where("metadata->>'space_key'=?", space)
	}
	if qText != "" {
		b = b.Where("title ILIKE ?", "%"+qText+"%")
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
	var out []models.Page
	for rows.Next() {
		v, err := scanCompatPage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repository) GetPage(ctx context.Context, id int64) (models.Page, error) {
	return scanCompatPage(r.pool.QueryRow(ctx, `SELECT id,external_id,coalesce(metadata->>'space_key',''),title,url,
		coalesce((metadata->>'version')::int,0),coalesce(metadata->>'status','current'),content_hash,content,
		source_updated_at,indexed_at,created_at,updated_at FROM documents WHERE id=$1 AND source_type='confluence'`, id))
}
