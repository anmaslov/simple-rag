package db

import (
	"context"

	"confluence-rag/backend/internal/models"

	sq "github.com/Masterminds/squirrel"
)

const searchSelect = `d.id,d.external_id,d.source_type,d.connection_id,d.scope_id,
	coalesce(cn.name,'') || ' / ' || coalesce(s.name,''),d.title,d.url,
	coalesce(d.metadata->>'space_key',''),coalesce(d.metadata->>'project_path',''),
	coalesce(d.metadata->>'ref',''),coalesce(d.metadata->>'file_path',''),d.metadata,ch.id,ch.content`

func (r *Repository) VectorSearch(ctx context.Context, vector []float32, scope models.SearchScope, limit int) ([]models.SearchResult, error) {
	b := psql.Select(searchSelect).Column("1-(ch.embedding <=> ?::vector)", vectorLiteral(vector)).
		From("document_chunks ch").Join("documents d ON d.id=ch.document_id").
		Join("source_connections cn ON cn.id=d.connection_id").Join("source_scopes s ON s.id=d.scope_id").
		Where("ch.embedding IS NOT NULL").OrderByClause("ch.embedding <=> ?::vector", vectorLiteral(vector)).Limit(uint64(limit))
	b = AddScopeFilter(b, scope)
	q, args, err := b.ToSql()
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSearch(rows)
}

func (r *Repository) KeywordSearch(ctx context.Context, text string, scope models.SearchScope, limit int) ([]models.SearchResult, error) {
	b := psql.Select(searchSelect).Column(`greatest(
		ts_rank_cd(to_tsvector('russian',coalesce(d.title,'')),websearch_to_tsquery('russian',?))+
		ts_rank_cd(to_tsvector('russian',coalesce(ch.content,'')),websearch_to_tsquery('russian',?)),
		ts_rank_cd(to_tsvector('simple',coalesce(d.title,'')||' '||coalesce(d.metadata->>'file_path','')||' '||coalesce(ch.content,'')),websearch_to_tsquery('simple',?))
	)`, text, text, text).
		From("document_chunks ch").Join("documents d ON d.id=ch.document_id").
		Join("source_connections cn ON cn.id=d.connection_id").Join("source_scopes s ON s.id=d.scope_id").
		Where(`to_tsvector('russian',coalesce(d.title,'')||' '||coalesce(ch.content,'')) @@ websearch_to_tsquery('russian',?)
			OR to_tsvector('simple',coalesce(d.title,'')||' '||coalesce(d.metadata->>'file_path','')||' '||coalesce(ch.content,'')) @@ websearch_to_tsquery('simple',?)`, text, text).
		OrderBy("16 DESC").Limit(uint64(limit))
	b = AddScopeFilter(b, scope)
	q, args, err := b.ToSql()
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSearch(rows)
}

func AddScopeFilter(b sq.SelectBuilder, scope models.SearchScope) sq.SelectBuilder {
	if len(scope.SourceTypes) > 0 {
		b = b.Where(sq.Eq{"d.source_type": scope.SourceTypes})
	}
	if len(scope.ConnectionIDs) > 0 {
		b = b.Where(sq.Eq{"d.connection_id": scope.ConnectionIDs})
	}
	if len(scope.ScopeIDs) > 0 {
		b = b.Where(sq.Eq{"d.scope_id": scope.ScopeIDs})
	}
	return b
}
