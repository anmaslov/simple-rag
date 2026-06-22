package db

import (
	"context"

	"confluence-rag/backend/internal/models"

	sq "github.com/Masterminds/squirrel"
)

func (r *Repository) UpsertSpace(ctx context.Context, key, name string) error {
	q, args, err := psql.Insert("confluence_spaces").
		Columns("space_key", "name").
		Values(key, name).
		Suffix("ON CONFLICT(space_key) DO UPDATE SET name=excluded.name,updated_at=now()").
		ToSql()
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, q, args...)
	return err
}

func (r *Repository) ListSpaces(ctx context.Context) ([]models.Space, error) {
	q, args, err := psql.Select("id", "external_id", "name").
		From("source_scopes").
		Where(sq.Eq{"source_type": models.SourceConfluence, "scope_type": "space"}).
		OrderBy("external_id").
		ToSql()
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, q, args...)
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
	q, args, err := psql.Select(`id,external_id,coalesce(metadata->>'space_key',''),title,url,
		coalesce((metadata->>'version')::int,0),coalesce(metadata->>'status','current'),content_hash,content,
		source_updated_at,indexed_at,created_at,updated_at`).
		From("documents").
		Where(sq.Eq{"id": id, "source_type": models.SourceConfluence}).
		ToSql()
	if err != nil {
		return models.Page{}, err
	}
	return scanCompatPage(r.pool.QueryRow(ctx, q, args...))
}
