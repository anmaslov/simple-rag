CREATE INDEX IF NOT EXISTS idx_chunks_content_fts_ru
ON page_chunks
USING GIN (to_tsvector('russian', coalesce(content, '')));

CREATE INDEX IF NOT EXISTS idx_pages_title_fts_ru
ON confluence_pages
USING GIN (to_tsvector('russian', coalesce(title, '')));
