package db

import (
	"context"
	"encoding/json"
	"errors"

	"confluence-rag/backend/internal/domain"
	"confluence-rag/backend/internal/models"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
)

const documentColumns = "id,source_type,connection_id,scope_id,external_id,title,url,content,content_hash,source_updated_at,indexed_at,metadata,created_at,updated_at"

func (r *Repository) UpsertDocument(ctx context.Context, in domain.DocumentInput) (models.Document, bool, error) {
	hashQuery, hashArgs, err := psql.Select("content_hash").
		From("documents").
		Where(sq.Eq{"scope_id": in.ScopeID, "external_id": in.ExternalID}).
		ToSql()
	if err != nil {
		return models.Document{}, false, err
	}
	var oldHash string
	err = r.pool.QueryRow(ctx, hashQuery, hashArgs...).Scan(&oldHash)
	unchanged := err == nil && oldHash == in.ContentHash
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return models.Document{}, false, err
	}
	meta, err := json.Marshal(in.Metadata)
	if err != nil {
		return models.Document{}, false, err
	}
	q, args, err := psql.Insert("documents").
		Columns("source_type", "connection_id", "scope_id", "external_id", "title", "url", "content", "content_hash", "source_updated_at", "metadata").
		Values(in.SourceType, in.ConnectionID, in.ScopeID, in.ExternalID, in.Title, in.URL, in.Content, in.ContentHash, nullableTime(in.SourceUpdatedAt), meta).
		Suffix(`ON CONFLICT(scope_id,external_id) DO UPDATE SET
			title=excluded.title,url=excluded.url,content=excluded.content,content_hash=excluded.content_hash,
			source_updated_at=excluded.source_updated_at,metadata=excluded.metadata,updated_at=now()
			RETURNING ` + documentColumns).ToSql()
	if err != nil {
		return models.Document{}, false, err
	}
	v, err := scanDocument(r.pool.QueryRow(ctx, q, args...))
	return v, unchanged, err
}

func (r *Repository) DocumentHasChunks(ctx context.Context, id int64) (bool, error) {
	exists := sq.Select("1").From("document_chunks").Where(sq.Eq{"document_id": id})
	q, args, err := psql.Select().Column("EXISTS (?)", exists).ToSql()
	if err != nil {
		return false, err
	}
	var ok bool
	err = r.pool.QueryRow(ctx, q, args...).Scan(&ok)
	return ok, err
}

func (r *Repository) ReplaceDocumentChunks(ctx context.Context, documentID int64, chunks []domain.ChunkInput) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q, args, err := psql.Delete("document_chunks").Where(sq.Eq{"document_id": documentID}).ToSql()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, q, args...); err != nil {
			return err
		}
		for _, ch := range chunks {
			meta, err := json.Marshal(ch.Metadata)
			if err != nil {
				return err
			}
			q, args, err = psql.Insert("document_chunks").
				Columns("document_id", "chunk_index", "content", "content_hash", "token_count", "metadata", "embedding").
				Values(documentID, ch.Index, ch.Content, ch.Hash, ch.TokenCount, meta, sq.Expr("?::vector", vectorLiteral(ch.Embedding))).
				ToSql()
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, q, args...)
			if err != nil {
				return err
			}
		}
		q, args, err = psql.Update("documents").
			Set("indexed_at", sq.Expr("now()")).
			Set("updated_at", sq.Expr("now()")).
			Where(sq.Eq{"id": documentID}).
			ToSql()
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, q, args...)
		return err
	})
}

func (r *Repository) DeleteDocumentsNotSeen(ctx context.Context, scopeID int64, ids []string) (int64, error) {
	b := psql.Delete("documents").Where(sq.Eq{"scope_id": scopeID})
	if len(ids) > 0 {
		b = b.Where("NOT (external_id=ANY(?))", ids)
	}
	q, args, err := b.ToSql()
	if err != nil {
		return 0, err
	}
	tag, err := r.pool.Exec(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (r *Repository) ListDocuments(ctx context.Context, sourceType string, scopeID int64, qText string) ([]models.Document, error) {
	b := psql.Select(documentColumns).From("documents").OrderBy("updated_at DESC").Limit(500)
	if sourceType != "" {
		b = b.Where(sq.Eq{"source_type": sourceType})
	}
	if scopeID > 0 {
		b = b.Where(sq.Eq{"scope_id": scopeID})
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
	var out []models.Document
	for rows.Next() {
		v, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

type pgconnTag interface {
	RowsAffected() int64
}
