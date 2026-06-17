CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS confluence_spaces (
  id BIGSERIAL PRIMARY KEY,
  space_key TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS confluence_pages (
  id BIGSERIAL PRIMARY KEY,
  confluence_id TEXT NOT NULL UNIQUE,
  space_key TEXT NOT NULL,
  title TEXT NOT NULL,
  url TEXT NOT NULL,
  version INT NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'current',
  content_hash TEXT NOT NULL DEFAULT '',
  raw_html TEXT NOT NULL DEFAULT '',
  plain_text TEXT NOT NULL DEFAULT '',
  ancestors_json JSONB NOT NULL DEFAULT '[]',
  confluence_updated_at TIMESTAMPTZ,
  indexed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_pages_space_key ON confluence_pages(space_key);
CREATE INDEX IF NOT EXISTS idx_pages_updated_at ON confluence_pages(confluence_updated_at);
CREATE INDEX IF NOT EXISTS idx_pages_plain_text_fts ON confluence_pages USING GIN (to_tsvector('simple', plain_text));

CREATE TABLE IF NOT EXISTS page_chunks (
  id BIGSERIAL PRIMARY KEY,
  page_id BIGINT NOT NULL REFERENCES confluence_pages(id) ON DELETE CASCADE,
  chunk_index INT NOT NULL,
  content TEXT NOT NULL,
  content_hash TEXT NOT NULL,
  token_count INT NOT NULL DEFAULT 0,
  embedding vector(1024),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(page_id, chunk_index)
);
CREATE INDEX IF NOT EXISTS idx_chunks_page_id ON page_chunks(page_id);
CREATE INDEX IF NOT EXISTS idx_chunks_content_hash ON page_chunks(content_hash);
CREATE INDEX IF NOT EXISTS idx_chunks_content_fts ON page_chunks USING GIN (to_tsvector('simple', content));
CREATE INDEX IF NOT EXISTS idx_chunks_embedding ON page_chunks USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

CREATE TABLE IF NOT EXISTS sync_jobs (
  id BIGSERIAL PRIMARY KEY,
  status TEXT NOT NULL CHECK (status IN ('pending','running','success','failed')),
  mode TEXT NOT NULL CHECK (mode IN ('full','space','cql','incremental')),
  space_key TEXT NOT NULL DEFAULT '',
  cql TEXT NOT NULL DEFAULT '',
  started_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ,
  pages_found INT NOT NULL DEFAULT 0,
  pages_indexed INT NOT NULL DEFAULT 0,
  pages_skipped INT NOT NULL DEFAULT 0,
  error_message TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_sync_jobs_status_created ON sync_jobs(status, created_at);

CREATE TABLE IF NOT EXISTS app_settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS chat_sessions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  title TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS chat_messages (
  id BIGSERIAL PRIMARY KEY,
  session_id UUID NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
  role TEXT NOT NULL CHECK (role IN ('user','assistant','system')),
  content TEXT NOT NULL,
  sources_json JSONB NOT NULL DEFAULT '[]',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
