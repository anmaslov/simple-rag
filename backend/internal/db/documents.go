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
	var oldHash string
	err := r.pool.QueryRow(ctx, "SELECT content_hash FROM documents WHERE scope_id=$1 AND external_id=$2", in.ScopeID, in.ExternalID).Scan(&oldHash)
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
	var ok bool
	err := r.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM document_chunks WHERE document_id=$1)", id).Scan(&ok)
	return ok, err
}

func (r *Repository) ReplaceDocumentChunks(ctx context.Context, documentID int64, chunks []domain.ChunkInput) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "DELETE FROM document_chunks WHERE document_id=$1", documentID); err != nil {
			return err
		}
		for _, ch := range chunks {
			meta, err := json.Marshal(ch.Metadata)
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `INSERT INTO document_chunks(document_id,chunk_index,content,content_hash,token_count,metadata,embedding)
				VALUES($1,$2,$3,$4,$5,$6,$7::vector)`, documentID, ch.Index, ch.Content, ch.Hash, ch.TokenCount, meta, vectorLiteral(ch.Embedding))
			if err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, "UPDATE documents SET indexed_at=now(),updated_at=now() WHERE id=$1", documentID)
		return err
	})
}

func (r *Repository) DeleteDocumentsNotSeen(ctx context.Context, scopeID int64, ids []string) (int64, error) {
	var tag pgconnTag
	var err error
	if len(ids) == 0 {
		tag, err = r.pool.Exec(ctx, "DELETE FROM documents WHERE scope_id=$1", scopeID)
	} else {
		tag, err = r.pool.Exec(ctx, "DELETE FROM documents WHERE scope_id=$1 AND NOT (external_id=ANY($2))", scopeID, ids)
	}
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
