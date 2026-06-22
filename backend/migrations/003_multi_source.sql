CREATE TABLE IF NOT EXISTS source_connections (
  id BIGSERIAL PRIMARY KEY,
  source_type TEXT NOT NULL CHECK (source_type IN ('confluence', 'gitlab')),
  name TEXT NOT NULL,
  base_url TEXT NOT NULL,
  auth_type TEXT NOT NULL DEFAULT 'bearer' CHECK (auth_type IN ('bearer', 'basic', 'token')),
  username TEXT NOT NULL DEFAULT '',
  secret TEXT NOT NULL DEFAULT '',
  skip_tls_verify BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(source_type, name)
);

CREATE TABLE IF NOT EXISTS source_scopes (
  id BIGSERIAL PRIMARY KEY,
  connection_id BIGINT NOT NULL REFERENCES source_connections(id) ON DELETE CASCADE,
  source_type TEXT NOT NULL CHECK (source_type IN ('confluence', 'gitlab')),
  scope_type TEXT NOT NULL CHECK (scope_type IN ('space', 'page', 'repository')),
  external_id TEXT NOT NULL,
  name TEXT NOT NULL,
  config JSONB NOT NULL DEFAULT '{}',
  enabled BOOLEAN NOT NULL DEFAULT true,
  last_synced_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(connection_id, scope_type, external_id)
);
CREATE INDEX IF NOT EXISTS idx_source_scopes_connection ON source_scopes(connection_id);

CREATE TABLE IF NOT EXISTS documents (
  id BIGSERIAL PRIMARY KEY,
  source_type TEXT NOT NULL CHECK (source_type IN ('confluence', 'gitlab')),
  connection_id BIGINT NOT NULL REFERENCES source_connections(id) ON DELETE CASCADE,
  scope_id BIGINT NOT NULL REFERENCES source_scopes(id) ON DELETE CASCADE,
  external_id TEXT NOT NULL,
  title TEXT NOT NULL,
  url TEXT NOT NULL DEFAULT '',
  content TEXT NOT NULL DEFAULT '',
  content_hash TEXT NOT NULL DEFAULT '',
  source_updated_at TIMESTAMPTZ,
  indexed_at TIMESTAMPTZ,
  metadata JSONB NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(scope_id, external_id)
);
CREATE INDEX IF NOT EXISTS idx_documents_scope ON documents(scope_id);
CREATE INDEX IF NOT EXISTS idx_documents_connection ON documents(connection_id);
CREATE INDEX IF NOT EXISTS idx_documents_source_type ON documents(source_type);
CREATE INDEX IF NOT EXISTS idx_documents_content_hash ON documents(content_hash);
CREATE INDEX IF NOT EXISTS idx_documents_title_fts_ru
  ON documents USING GIN (to_tsvector('russian', coalesce(title, '')));
CREATE INDEX IF NOT EXISTS idx_documents_title_fts_simple
  ON documents USING GIN (to_tsvector('simple', coalesce(title, '')));

CREATE TABLE IF NOT EXISTS document_chunks (
  id BIGSERIAL PRIMARY KEY,
  document_id BIGINT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  chunk_index INT NOT NULL,
  content TEXT NOT NULL,
  content_hash TEXT NOT NULL,
  token_count INT NOT NULL DEFAULT 0,
  metadata JSONB NOT NULL DEFAULT '{}',
  embedding vector(1024),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(document_id, chunk_index)
);
CREATE INDEX IF NOT EXISTS idx_document_chunks_document ON document_chunks(document_id);
CREATE INDEX IF NOT EXISTS idx_document_chunks_hash ON document_chunks(content_hash);
CREATE INDEX IF NOT EXISTS idx_document_chunks_fts_ru
  ON document_chunks USING GIN (to_tsvector('russian', coalesce(content, '')));
CREATE INDEX IF NOT EXISTS idx_document_chunks_fts_simple
  ON document_chunks USING GIN (to_tsvector('simple', coalesce(content, '')));
CREATE INDEX IF NOT EXISTS idx_document_chunks_embedding
  ON document_chunks USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

ALTER TABLE sync_jobs
  ADD COLUMN IF NOT EXISTS source_type TEXT NOT NULL DEFAULT 'confluence',
  ADD COLUMN IF NOT EXISTS connection_id BIGINT REFERENCES source_connections(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS scope_id BIGINT REFERENCES source_scopes(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS force_reindex BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS documents_found INT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS documents_indexed INT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS documents_skipped INT NOT NULL DEFAULT 0;

ALTER TABLE sync_jobs DROP CONSTRAINT IF EXISTS sync_jobs_mode_check;
ALTER TABLE sync_jobs ADD CONSTRAINT sync_jobs_mode_check
  CHECK (mode IN ('full','space','page','cql','incremental','repository'));
ALTER TABLE sync_jobs DROP CONSTRAINT IF EXISTS sync_jobs_source_type_check;
ALTER TABLE sync_jobs ADD CONSTRAINT sync_jobs_source_type_check
  CHECK (source_type IN ('confluence','gitlab'));
CREATE INDEX IF NOT EXISTS idx_sync_jobs_scope_active
  ON sync_jobs(scope_id, status) WHERE status IN ('pending','running');

-- Preserve existing Confluence data without requiring a PostgreSQL volume reset.
INSERT INTO source_connections(source_type, name, base_url, auth_type)
VALUES ('confluence', 'Migrated Confluence', '', 'bearer')
ON CONFLICT(source_type, name) DO NOTHING;

INSERT INTO source_scopes(connection_id, source_type, scope_type, external_id, name, config)
SELECT c.id, 'confluence', 'space', s.space_key, s.name, jsonb_build_object('space_key', s.space_key, 'migrated', true)
FROM confluence_spaces s
JOIN source_connections c ON c.source_type='confluence' AND c.name='Migrated Confluence'
ON CONFLICT(connection_id, scope_type, external_id) DO NOTHING;

INSERT INTO source_scopes(connection_id, source_type, scope_type, external_id, name, config)
SELECT c.id, 'confluence', 'space', p.space_key, p.space_key, jsonb_build_object('space_key', p.space_key, 'migrated', true)
FROM (SELECT DISTINCT space_key FROM confluence_pages) p
JOIN source_connections c ON c.source_type='confluence' AND c.name='Migrated Confluence'
ON CONFLICT(connection_id, scope_type, external_id) DO NOTHING;

INSERT INTO documents(source_type, connection_id, scope_id, external_id, title, url, content, content_hash, source_updated_at, indexed_at, metadata, created_at, updated_at)
SELECT 'confluence', c.id, s.id, p.confluence_id, p.title, p.url, p.plain_text, p.content_hash,
       p.confluence_updated_at, p.indexed_at,
       jsonb_build_object('space_key', p.space_key, 'version', p.version, 'status', p.status, 'ancestors', p.ancestors_json),
       p.created_at, p.updated_at
FROM confluence_pages p
JOIN source_connections c ON c.source_type='confluence' AND c.name='Migrated Confluence'
JOIN source_scopes s ON s.connection_id=c.id AND s.scope_type='space' AND s.external_id=p.space_key
ON CONFLICT(scope_id, external_id) DO NOTHING;

INSERT INTO document_chunks(document_id, chunk_index, content, content_hash, token_count, embedding, created_at, updated_at)
SELECT d.id, pc.chunk_index, pc.content, pc.content_hash, pc.token_count, pc.embedding, pc.created_at, pc.updated_at
FROM page_chunks pc
JOIN confluence_pages p ON p.id=pc.page_id
JOIN source_connections c ON c.source_type='confluence' AND c.name='Migrated Confluence'
JOIN source_scopes s ON s.connection_id=c.id AND s.scope_type='space' AND s.external_id=p.space_key
JOIN documents d ON d.scope_id=s.id AND d.external_id=p.confluence_id
ON CONFLICT(document_id, chunk_index) DO NOTHING;
