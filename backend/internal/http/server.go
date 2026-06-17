package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"confluence-rag/backend/internal/config"
	"confluence-rag/backend/internal/domain"
	"confluence-rag/backend/internal/rag"
	"confluence-rag/backend/internal/search"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	cfg    config.Config
	repo   domain.Repository
	search *search.Service
	rag    *rag.Service
}

func NewRouter(cfg config.Config, repo domain.Repository, searchSvc *search.Service, ragSvc *rag.Service) http.Handler {
	s := &Server{cfg: cfg, repo: repo, search: searchSvc, rag: ragSvc}
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer)
	r.Use(jsonContent)
	r.Get("/api/health", s.health)
	r.Get("/api/spaces", s.spaces)
	r.Post("/api/sync", s.createSync)
	r.Get("/api/sync/status", s.syncStatus)
	r.Get("/api/pages", s.pages)
	r.Get("/api/pages/{id}", s.page)
	r.Post("/api/search", s.searchHandler)
	r.Get("/api/chat/sessions", s.chatSessions)
	r.Get("/api/chat/sessions/{id}/messages", s.chatMessages)
	r.Delete("/api/chat/sessions/{id}", s.deleteChatSession)
	r.Post("/api/chat", s.chat)
	r.Post("/api/chat/stream", s.chatStream)
	r.Get("/api/settings", s.settings)
	r.Put("/api/settings", s.updateSettings)
	return r
}

func jsonContent(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) spaces(w http.ResponseWriter, r *http.Request) {
	spaces, err := s.repo.ListSpaces(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"spaces": spaces})
}

func (s *Server) createSync(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Mode     string `json:"mode"`
		SpaceKey string `json:"space_key"`
		CQL      string `json:"cql"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if req.Mode == "" {
		req.Mode = "full"
	}
	if err := validateSyncRequest(req.Mode, req.SpaceKey, req.CQL); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	job, err := s.repo.CreateSyncJob(r.Context(), req.Mode, req.SpaceKey, req.CQL)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func validateSyncRequest(mode, spaceKey, cql string) error {
	switch mode {
	case "full", "incremental":
		return nil
	case "space":
		if spaceKey == "" {
			return errors.New("space_key is required for space sync")
		}
		return nil
	case "cql":
		if cql == "" {
			return errors.New("cql is required for cql sync")
		}
		return nil
	default:
		return errors.New("mode must be one of full, space, cql, incremental")
	}
}

func (s *Server) syncStatus(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.repo.ListJobs(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
}

func (s *Server) pages(w http.ResponseWriter, r *http.Request) {
	pages, err := s.repo.ListPages(r.Context(), r.URL.Query().Get("space_key"), r.URL.Query().Get("q"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pages": pages})
}

func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	page, err := s.repo.GetPage(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) searchHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query     string   `json:"query"`
		SpaceKeys []string `json:"space_keys"`
		TopK      int      `json:"top_k"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Query == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query is required"})
		return
	}
	results, err := s.search.Search(r.Context(), req.Query, req.SpaceKeys, req.TopK)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (s *Server) chatSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.repo.ListChatSessions(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
}

func (s *Server) chatMessages(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	if sessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session id is required"})
		return
	}
	messages, err := s.repo.ListChatMessages(r.Context(), sessionID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": messages})
}

func (s *Server) deleteChatSession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	if sessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session id is required"})
		return
	}
	if err := s.repo.DeleteChatSession(r.Context(), sessionID); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID string   `json:"session_id"`
		Message   string   `json:"message"`
		SpaceKeys []string `json:"space_keys"`
		TopK      int      `json:"top_k"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is required"})
		return
	}
	resp, err := s.rag.Chat(r.Context(), req.SessionID, req.Message, req.SpaceKeys, req.TopK)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) chatStream(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID string   `json:"session_id"`
		Message   string   `json:"message"`
		SpaceKeys []string `json:"space_keys"`
		TopK      int      `json:"top_k"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is required"})
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming is not supported"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	emit := func(event rag.ChatStreamEvent) error {
		if err := writeSSE(w, event.Type, event); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}
	if err := s.rag.ChatStream(r.Context(), req.SessionID, req.Message, req.SpaceKeys, req.TopK, emit); err != nil {
		_ = emit(rag.ChatStreamEvent{Type: "error", Message: err.Error()})
	}
}

func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"confluence_base_url":      s.cfg.Confluence.BaseURL,
		"confluence_auth_type":     s.cfg.Confluence.AuthType,
		"confluence_root_page_ids": s.cfg.Confluence.RootPageIDs,
		"confluence_space_keys":    s.cfg.Confluence.SpaceKeys,
		"llm_base_url":             s.cfg.LLM.BaseURL,
		"llm_model":                s.cfg.LLM.Model,
		"embeddings_base_url":      s.cfg.Embeddings.BaseURL,
		"embeddings_model":         s.cfg.Embeddings.Model,
		"embeddings_dim":           s.cfg.Embeddings.Dim,
		"chunk_size":               s.cfg.Chunk.Size,
		"chunk_overlap":            s.cfg.Chunk.Overlap,
		"top_k":                    s.cfg.Search.TopK,
		"secrets":                  "configured via env; not returned",
	})
}

func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "settings are read-only in MVP; configure env and restart"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeSSE(w http.ResponseWriter, event string, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, body)
	return err
}

func writeError(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
}
