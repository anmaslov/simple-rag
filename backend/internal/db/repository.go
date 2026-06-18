package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"confluence-rag/backend/internal/domain"
	"confluence-rag/backend/internal/models"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

const connectionColumns = "id,source_type,name,base_url,auth_type,username,(secret<>''),skip_tls_verify,created_at,updated_at"
const scopeColumns = "id,connection_id,source_type,scope_type,external_id,name,config,enabled,last_synced_at,created_at,updated_at"
const documentColumns = "id,source_type,connection_id,scope_id,external_id,title,url,content,content_hash,source_updated_at,indexed_at,metadata,created_at,updated_at"
const jobColumns = "id,status,mode,source_type,connection_id,scope_id,force_reindex,space_key,cql,started_at,finished_at,documents_found,documents_indexed,documents_skipped,pages_found,pages_indexed,pages_skipped,error_message,created_at,updated_at"

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

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

func scanConnection(row pgx.Row) (models.Connection, error) {
	var v models.Connection
	err := row.Scan(&v.ID, &v.SourceType, &v.Name, &v.BaseURL, &v.AuthType, &v.Username, &v.HasToken, &v.SkipTLSVerify, &v.CreatedAt, &v.UpdatedAt)
	return v, err
}

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

func scanScope(row pgx.Row) (models.SourceScope, error) {
	var v models.SourceScope
	err := row.Scan(&v.ID, &v.ConnectionID, &v.SourceType, &v.ScopeType, &v.ExternalID, &v.Name, &v.Config, &v.Enabled, &v.LastSyncedAt, &v.CreatedAt, &v.UpdatedAt)
	return v, err
}

func (r *Repository) DeleteScope(ctx context.Context, id int64) error {
	tag, err := r.pool.Exec(ctx, "DELETE FROM source_scopes WHERE id=$1", id)
	if err == nil && tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return err
}

func (r *Repository) MarkScopeSynced(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, "UPDATE source_scopes SET last_synced_at=now(),updated_at=now() WHERE id=$1", id)
	return err
}

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

func nullableTime(v interface{ IsZero() bool }) any {
	if v.IsZero() {
		return nil
	}
	return v
}

func scanDocument(row pgx.Row) (models.Document, error) {
	var v models.Document
	err := row.Scan(&v.ID, &v.SourceType, &v.ConnectionID, &v.ScopeID, &v.ExternalID, &v.Title, &v.URL, &v.Content, &v.ContentHash, &v.SourceUpdatedAt, &v.IndexedAt, &v.Metadata, &v.CreatedAt, &v.UpdatedAt)
	return v, err
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

type pgconnTag interface{ RowsAffected() int64 }

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

func (r *Repository) CreateSyncJob(ctx context.Context, mode, spaceKey, cql string) (models.SyncJob, error) {
	q, args, err := psql.Insert("sync_jobs").
		Columns("status", "mode", "source_type", "space_key", "cql", "force_reindex").
		Values("pending", mode, models.SourceConfluence, spaceKey, cql, mode == "full").
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

func scanJob(row pgx.Row) (models.SyncJob, error) {
	var v models.SyncJob
	err := row.Scan(&v.ID, &v.Status, &v.Mode, &v.SourceType, &v.ConnectionID, &v.ScopeID, &v.ForceReindex,
		&v.SpaceKey, &v.CQL, &v.StartedAt, &v.FinishedAt, &v.DocumentsFound, &v.DocumentsIndexed, &v.DocumentsSkipped,
		&v.PagesFound, &v.PagesIndexed, &v.PagesSkipped, &v.ErrorMessage, &v.CreatedAt, &v.UpdatedAt)
	return v, err
}

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

func scanCompatPage(row pgx.Row) (models.Page, error) {
	var v models.Page
	var updated *time.Time
	err := row.Scan(&v.ID, &v.ConfluenceID, &v.SpaceKey, &v.Title, &v.URL, &v.Version, &v.Status, &v.ContentHash,
		&v.PlainText, &updated, &v.IndexedAt, &v.CreatedAt, &v.UpdatedAt)
	if updated != nil {
		v.ConfluenceUpdatedAt = *updated
	}
	return v, err
}

func (r *Repository) EnsureChatSession(ctx context.Context, sessionID, title string) (string, error) {
	if sessionID != "" {
		return sessionID, nil
	}
	var id string
	err := r.pool.QueryRow(ctx, "INSERT INTO chat_sessions(title) VALUES($1) RETURNING id::text", truncate(title, 80)).Scan(&id)
	return id, err
}

func (r *Repository) SaveChatMessage(ctx context.Context, sessionID, role, content string, sources json.RawMessage) error {
	if sources == nil {
		sources = json.RawMessage("[]")
	}
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "INSERT INTO chat_messages(session_id,role,content,sources_json) VALUES($1,$2,$3,$4)", sessionID, role, content, sources); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "UPDATE chat_sessions SET updated_at=now() WHERE id=$1", sessionID)
		return err
	})
}

func (r *Repository) ListChatSessions(ctx context.Context) ([]models.ChatSession, error) {
	rows, err := r.pool.Query(ctx, "SELECT id::text,title,created_at,updated_at FROM chat_sessions ORDER BY updated_at DESC LIMIT 100")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ChatSession
	for rows.Next() {
		var v models.ChatSession
		if err := rows.Scan(&v.ID, &v.Title, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repository) ListChatMessages(ctx context.Context, sessionID string) ([]models.ChatMessage, error) {
	rows, err := r.pool.Query(ctx, "SELECT id,session_id::text,role,content,sources_json,created_at FROM chat_messages WHERE session_id=$1 ORDER BY created_at,id", sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ChatMessage
	for rows.Next() {
		var v models.ChatMessage
		var raw json.RawMessage
		if err := rows.Scan(&v.ID, &v.SessionID, &v.Role, &v.Content, &raw, &v.CreatedAt); err != nil {
			return nil, err
		}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &v.Sources); err != nil {
				return nil, err
			}
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repository) DeleteChatSession(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, "DELETE FROM chat_sessions WHERE id=$1", id)
	return err
}

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

func scanSearch(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]models.SearchResult, error) {
	var out []models.SearchResult
	for rows.Next() {
		var v models.SearchResult
		if err := rows.Scan(&v.DocumentID, &v.ExternalID, &v.SourceType, &v.ConnectionID, &v.ScopeID, &v.SourceLabel,
			&v.Title, &v.URL, &v.SpaceKey, &v.Repository, &v.Ref, &v.FilePath, &v.Metadata, &v.ChunkID, &v.Chunk, &v.Score); err != nil {
			return nil, err
		}
		if v.SourceType == models.SourceConfluence {
			v.PageID, v.ConfluenceID = v.DocumentID, v.ExternalID
		}
		out = append(out, v)
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
