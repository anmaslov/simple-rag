package httpapi

import (
	"net/http"
	"strconv"
)

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) documents(w http.ResponseWriter, r *http.Request) {
	scopeID, _ := strconv.ParseInt(r.URL.Query().Get("scope_id"), 10, 64)
	items, err := s.repo.ListDocuments(r.Context(), r.URL.Query().Get("source_type"), scopeID, r.URL.Query().Get("q"))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"documents": items})
}

func (s *Server) spaces(w http.ResponseWriter, r *http.Request) {
	items, err := s.repo.ListSpaces(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"spaces": items})
}

func (s *Server) syncStatus(w http.ResponseWriter, r *http.Request) {
	items, err := s.repo.ListJobs(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": items})
}

func (s *Server) cancelJob(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	item, err := s.repo.CancelJob(r.Context(), id)
	if err != nil {
		s.repoError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) pages(w http.ResponseWriter, r *http.Request) {
	items, err := s.repo.ListPages(r.Context(), r.URL.Query().Get("space_key"), r.URL.Query().Get("q"))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pages": items})
}

func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	item, err := s.repo.GetPage(r.Context(), id)
	if err != nil {
		s.repoError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}
