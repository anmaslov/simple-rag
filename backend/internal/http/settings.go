package httpapi

import "net/http"

func (s *Server) settings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ai": map[string]any{
			"llm_base_url": s.cfg.LLM.BaseURL, "llm_model": s.cfg.LLM.Model,
			"embeddings_base_url": s.cfg.Embeddings.BaseURL, "embeddings_model": s.cfg.Embeddings.Model,
			"embeddings_dim": s.cfg.Embeddings.Dim,
		},
		"indexing": map[string]any{
			"chunk_size": s.cfg.Chunk.Size, "chunk_overlap": s.cfg.Chunk.Overlap,
			"source_page_limit": s.cfg.Sources.PageLimit, "gitlab_max_file_bytes": s.cfg.GitLab.MaxFileBytes,
		},
		"search": map[string]any{
			"top_k": s.cfg.Search.TopK, "vector_weight": s.cfg.Search.VectorWeight,
			"keyword_weight": s.cfg.Search.KeywordWeight,
		},
		"security": map[string]any{
			"source_credentials": "stored in database and never returned by API",
			"tls_verification":   "enabled by default; configured per connection",
		},
	})
}

func (s *Server) updateSettings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "runtime settings are read-only; source connections are managed on the Sources page"})
}
