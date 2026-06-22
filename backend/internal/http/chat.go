package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"confluence-rag/backend/internal/models"
	"confluence-rag/backend/internal/rag"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func (s *Server) chatSessions(w http.ResponseWriter, r *http.Request) {
	items, err := s.repo.ListChatSessions(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": items})
}

func (s *Server) chatMessages(w http.ResponseWriter, r *http.Request) {
	items, err := s.repo.ListChatMessages(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": items})
}

func (s *Server) deleteChatSession(w http.ResponseWriter, r *http.Request) {
	if err := s.repo.DeleteChatSession(r.Context(), chi.URLParam(r, "id")); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

type chatRequest struct {
	SessionID string             `json:"session_id"`
	Message   string             `json:"message"`
	Scope     models.SearchScope `json:"scope"`
	SpaceKeys []string           `json:"space_keys"`
	TopK      int                `json:"top_k"`
}

func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		badRequest(w, errors.New("message is required"))
		return
	}
	scope, ok := s.requestSearchScope(w, r, req.Scope, req.SpaceKeys)
	if !ok {
		return
	}
	resp, err := s.rag.Chat(r.Context(), req.SessionID, req.Message, scope, req.TopK)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) chatStream(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		badRequest(w, errors.New("message is required"))
		return
	}
	scope, ok := s.requestSearchScope(w, r, req.Scope, req.SpaceKeys)
	if !ok {
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
	if err := s.rag.ChatStream(r.Context(), req.SessionID, req.Message, scope, req.TopK, emit); err != nil {
		s.log.Error("streaming chat failed", "request_id", middleware.GetReqID(r.Context()), "error", err)
		_ = emit(rag.ChatStreamEvent{Type: "error", Message: "chat request failed"})
	}
}
