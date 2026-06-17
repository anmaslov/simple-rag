package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"confluence-rag/backend/internal/domain"
	"confluence-rag/backend/internal/models"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

const pageColumns = "id, confluence_id, space_key, title, url, version, status, content_hash, raw_html, plain_text, ancestors_json, confluence_updated_at, indexed_at, created_at, updated_at"
const jobColumns = "id,status,mode,space_key,cql,started_at,finished_at,pages_found,pages_indexed,pages_skipped,error_message,created_at,updated_at"

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) UpsertSpace(ctx context.Context, key, name string) error {
	query, args, err := psql.Insert("confluence_spaces").
		Columns("space_key", "name").
		Values(key, name).
		Suffix("ON CONFLICT(space_key) DO UPDATE SET name=excluded.name, updated_at=now()").
		ToSql()
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, query, args...)
	return err
}

func (r *Repository) ListSpaces(ctx context.Context) ([]models.Space, error) {
	query, args, err := psql.Select("id", "space_key", "name").
		From("confluence_spaces").
		OrderBy("space_key").
		ToSql()
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Space
	for rows.Next() {
		var s models.Space
		if err := rows.Scan(&s.ID, &s.Key, &s.Name); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repository) UpsertPage(ctx context.Context, in domain.UpsertPageInput) (models.Page, bool, error) {
	existingHash, err := r.pageContentHash(ctx, in.ConfluenceID)
	unchanged := err == nil && existingHash == in.ContentHash
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return models.Page{}, false, err
	}

	ancestors, err := json.Marshal(in.Ancestors)
	if err != nil {
		return models.Page{}, false, err
	}
	query, args, err := psql.Insert("confluence_pages").
		Columns("confluence_id", "space_key", "title", "url", "version", "status", "content_hash", "raw_html", "plain_text", "ancestors_json", "confluence_updated_at").
		Values(in.ConfluenceID, in.SpaceKey, in.Title, in.URL, in.Version, in.Status, in.ContentHash, in.RawHTML, in.PlainText, ancestors, in.ConfluenceUpdatedAt).
		Suffix(`ON CONFLICT(confluence_id) DO UPDATE SET
			space_key=excluded.space_key,
			title=excluded.title,
			url=excluded.url,
			version=excluded.version,
			status=excluded.status,
			content_hash=excluded.content_hash,
			raw_html=excluded.raw_html,
			plain_text=excluded.plain_text,
			ancestors_json=excluded.ancestors_json,
			confluence_updated_at=excluded.confluence_updated_at,
			updated_at=now()
			RETURNING ` + pageColumns).
		ToSql()
	if err != nil {
		return models.Page{}, false, err
	}

	page, err := scanPage(r.pool.QueryRow(ctx, query, args...))
	if err != nil {
		return models.Page{}, false, err
	}
	return page, unchanged, nil
}

func (r *Repository) pageContentHash(ctx context.Context, confluenceID string) (string, error) {
	query, args, err := psql.Select("content_hash").
		From("confluence_pages").
		Where(sq.Eq{"confluence_id": confluenceID}).
		ToSql()
	if err != nil {
		return "", err
	}
	var hash string
	err = r.pool.QueryRow(ctx, query, args...).Scan(&hash)
	return hash, err
}

func (r *Repository) PageHasChunks(ctx context.Context, pageID int64) (bool, error) {
	query, args, err := psql.Select("1").
		From("page_chunks").
		Where(sq.Eq{"page_id": pageID}).
		Limit(1).
		ToSql()
	if err != nil {
		return false, err
	}
	var one int
	err = r.pool.QueryRow(ctx, query, args...).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (r *Repository) ReplaceChunks(ctx context.Context, pageID int64, chunks []domain.ChunkInput) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		deleteSQL, deleteArgs, err := psql.Delete("page_chunks").
			Where(sq.Eq{"page_id": pageID}).
			ToSql()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, deleteSQL, deleteArgs...); err != nil {
			return err
		}

		for _, ch := range chunks {
			insertSQL, insertArgs, err := psql.Insert("page_chunks").
				Columns("page_id", "chunk_index", "content", "content_hash", "token_count", "embedding").
				Values(pageID, ch.Index, ch.Content, ch.Hash, ch.TokenCount, sq.Expr("?::vector", vectorLiteral(ch.Embedding))).
				ToSql()
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, insertSQL, insertArgs...); err != nil {
				return err
			}
		}

		updateSQL, updateArgs, err := psql.Update("confluence_pages").
			Set("indexed_at", sq.Expr("now()")).
			Set("updated_at", sq.Expr("now()")).
			Where(sq.Eq{"id": pageID}).
			ToSql()
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, updateSQL, updateArgs...)
		return err
	})
}

func vectorLiteral(v []float32) any {
	if len(v) == 0 {
		return nil
	}
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = fmt.Sprintf("%g", f)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func (r *Repository) CreateSyncJob(ctx context.Context, mode, spaceKey, cql string) (models.SyncJob, error) {
	query, args, err := psql.Insert("sync_jobs").
		Columns("status", "mode", "space_key", "cql").
		Values("pending", mode, spaceKey, cql).
		Suffix("RETURNING " + jobColumns).
		ToSql()
	if err != nil {
		return models.SyncJob{}, err
	}
	return scanJob(r.pool.QueryRow(ctx, query, args...))
}

func (r *Repository) ClaimNextJob(ctx context.Context) (models.SyncJob, bool, error) {
	query, args, err := psql.Update("sync_jobs").
		Set("status", "running").
		Set("started_at", sq.Expr("now()")).
		Set("updated_at", sq.Expr("now()")).
		Where("id=(SELECT id FROM sync_jobs WHERE status='pending' ORDER BY created_at LIMIT 1 FOR UPDATE SKIP LOCKED)").
		Suffix("RETURNING " + jobColumns).
		ToSql()
	if err != nil {
		return models.SyncJob{}, false, err
	}
	job, err := scanJob(r.pool.QueryRow(ctx, query, args...))
	if err == pgx.ErrNoRows {
		return models.SyncJob{}, false, nil
	}
	if err != nil {
		return models.SyncJob{}, false, err
	}
	return job, true, nil
}

func (r *Repository) FinishJob(ctx context.Context, id int64, status, msg string) error {
	query, args, err := psql.Update("sync_jobs").
		Set("status", status).
		Set("error_message", msg).
		Set("finished_at", sq.Expr("now()")).
		Set("updated_at", sq.Expr("now()")).
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, query, args...)
	return err
}

func (r *Repository) IncJob(ctx context.Context, id int64, found, indexed, skipped int) error {
	query, args, err := psql.Update("sync_jobs").
		Set("pages_found", sq.Expr("pages_found + ?", found)).
		Set("pages_indexed", sq.Expr("pages_indexed + ?", indexed)).
		Set("pages_skipped", sq.Expr("pages_skipped + ?", skipped)).
		Set("updated_at", sq.Expr("now()")).
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, query, args...)
	return err
}

func (r *Repository) ListJobs(ctx context.Context) ([]models.SyncJob, error) {
	query, args, err := psql.Select(jobColumns).
		From("sync_jobs").
		OrderBy("created_at DESC").
		Limit(50).
		ToSql()
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []models.SyncJob
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

func scanJob(row pgx.Row) (models.SyncJob, error) {
	var j models.SyncJob
	err := row.Scan(&j.ID, &j.Status, &j.Mode, &j.SpaceKey, &j.CQL, &j.StartedAt, &j.FinishedAt, &j.PagesFound, &j.PagesIndexed, &j.PagesSkipped, &j.ErrorMessage, &j.CreatedAt, &j.UpdatedAt)
	return j, err
}

func (r *Repository) ListPages(ctx context.Context, space, q string) ([]models.Page, error) {
	builder := psql.Select(pageColumns).
		From("confluence_pages").
		OrderBy("confluence_updated_at DESC NULLS LAST")
	if space != "" {
		builder = builder.Where(sq.Eq{"space_key": space})
	}
	if q != "" {
		builder = builder.Where("title ILIKE ?", "%"+q+"%")
	}
	query, args, err := builder.ToSql()
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Page
	for rows.Next() {
		p, err := scanPage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) GetPage(ctx context.Context, id int64) (models.Page, error) {
	query, args, err := psql.Select(pageColumns).
		From("confluence_pages").
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return models.Page{}, err
	}
	return scanPage(r.pool.QueryRow(ctx, query, args...))
}

func scanPage(row pgx.Row) (models.Page, error) {
	var p models.Page
	err := row.Scan(&p.ID, &p.ConfluenceID, &p.SpaceKey, &p.Title, &p.URL, &p.Version, &p.Status, &p.ContentHash, &p.RawHTML, &p.PlainText, &p.AncestorsJSON, &p.ConfluenceUpdatedAt, &p.IndexedAt, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

func (r *Repository) EnsureChatSession(ctx context.Context, sessionID, title string) (string, error) {
	if sessionID != "" {
		return sessionID, nil
	}
	query, args, err := psql.Insert("chat_sessions").
		Columns("title").
		Values(truncate(title, 80)).
		Suffix("RETURNING id::text").
		ToSql()
	if err != nil {
		return "", err
	}
	var out string
	err = r.pool.QueryRow(ctx, query, args...).Scan(&out)
	return out, err
}

func (r *Repository) SaveChatMessage(ctx context.Context, sessionID, role, content string, sources json.RawMessage) error {
	if sources == nil {
		sources = json.RawMessage("[]")
	}
	query, args, err := psql.Insert("chat_messages").
		Columns("session_id", "role", "content", "sources_json").
		Values(sessionID, role, content, sources).
		ToSql()
	if err != nil {
		return err
	}
	if _, err = r.pool.Exec(ctx, query, args...); err != nil {
		return err
	}
	updateSQL, updateArgs, err := psql.Update("chat_sessions").
		Set("updated_at", sq.Expr("now()")).
		Where(sq.Eq{"id": sessionID}).
		ToSql()
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, updateSQL, updateArgs...)
	return err
}

func (r *Repository) ListChatSessions(ctx context.Context) ([]models.ChatSession, error) {
	query, args, err := psql.Select("id::text", "title", "created_at", "updated_at").
		From("chat_sessions").
		OrderBy("updated_at DESC").
		Limit(100).
		ToSql()
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.ChatSession
	for rows.Next() {
		var s models.ChatSession
		if err := rows.Scan(&s.ID, &s.Title, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repository) ListChatMessages(ctx context.Context, sessionID string) ([]models.ChatMessage, error) {
	query, args, err := psql.Select("id", "session_id::text", "role", "content", "sources_json", "created_at").
		From("chat_messages").
		Where(sq.Eq{"session_id": sessionID}).
		OrderBy("created_at ASC", "id ASC").
		ToSql()
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.ChatMessage
	for rows.Next() {
		var m models.ChatMessage
		var sources json.RawMessage
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &sources, &m.CreatedAt); err != nil {
			return nil, err
		}
		if len(sources) > 0 {
			if err := json.Unmarshal(sources, &m.Sources); err != nil {
				return nil, err
			}
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *Repository) DeleteChatSession(ctx context.Context, sessionID string) error {
	query, args, err := psql.Delete("chat_sessions").
		Where(sq.Eq{"id": sessionID}).
		ToSql()
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, query, args...)
	return err
}

func (r *Repository) VectorSearch(ctx context.Context, vector []float32, spaces []string, limit int) ([]models.SearchResult, error) {
	builder := psql.Select("p.id", "p.confluence_id", "p.title", "p.url", "p.space_key", "c.id", "c.content").
		Column("1 - (c.embedding <=> ?::vector) AS score", vectorLiteral(vector)).
		From("page_chunks c").
		Join("confluence_pages p ON p.id=c.page_id").
		Where("c.embedding IS NOT NULL").
		OrderByClause("c.embedding <=> ?::vector", vectorLiteral(vector)).
		Limit(uint64(limit))
	builder = addSpaceFilter(builder, spaces)
	query, args, err := builder.ToSql()
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSearchResults(rows)
}

func (r *Repository) KeywordSearch(ctx context.Context, queryText string, spaces []string, limit int) ([]models.SearchResult, error) {
	builder := psql.Select("p.id", "p.confluence_id", "p.title", "p.url", "p.space_key", "c.id", "c.content").
		Column(`ts_rank_cd(
			setweight(to_tsvector('russian', coalesce(p.title, '')), 'A'),
			websearch_to_tsquery('russian', ?)
		) + ts_rank_cd(
			setweight(to_tsvector('russian', coalesce(c.content, '')), 'B'),
			websearch_to_tsquery('russian', ?)
		) AS score`, queryText, queryText).
		From("page_chunks c").
		Join("confluence_pages p ON p.id=c.page_id").
		Where(`(
			to_tsvector('russian', coalesce(p.title, '')) @@ websearch_to_tsquery('russian', ?)
			OR to_tsvector('russian', coalesce(c.content, '')) @@ websearch_to_tsquery('russian', ?)
		)`, queryText, queryText).
		OrderBy("score DESC").
		Limit(uint64(limit))
	builder = addSpaceFilter(builder, spaces)
	query, args, err := builder.ToSql()
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSearchResults(rows)
}

func addSpaceFilter(builder sq.SelectBuilder, spaces []string) sq.SelectBuilder {
	if len(spaces) == 0 {
		return builder
	}
	return builder.Where(sq.Eq{"p.space_key": spaces})
}

func scanSearchResults(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]models.SearchResult, error) {
	var out []models.SearchResult
	for rows.Next() {
		var r models.SearchResult
		if err := rows.Scan(&r.PageID, &r.ConfluenceID, &r.Title, &r.URL, &r.SpaceKey, &r.ChunkID, &r.Chunk, &r.Score); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

var _ domain.Repository = (*Repository)(nil)
