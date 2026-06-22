package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"confluence-rag/backend/internal/confluence"
	"confluence-rag/backend/internal/domain"
	"confluence-rag/backend/internal/gitlab"
	"confluence-rag/backend/internal/models"
)

type connectionRequest struct {
	SourceType    string `json:"source_type"`
	Name          string `json:"name"`
	BaseURL       string `json:"base_url"`
	AuthType      string `json:"auth_type"`
	Username      string `json:"username"`
	Token         string `json:"token"`
	SkipTLSVerify bool   `json:"skip_tls_verify"`
}

func (s *Server) listConnections(w http.ResponseWriter, r *http.Request) {
	items, err := s.repo.ListConnections(r.Context(), r.URL.Query().Get("source_type"))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connections": items})
}

func (s *Server) createConnection(w http.ResponseWriter, r *http.Request) {
	var req connectionRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	in, err := validateConnection(req, true)
	if err != nil {
		badRequest(w, err)
		return
	}
	item, err := s.repo.CreateConnection(r.Context(), in)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) updateConnection(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	current, err := s.repo.GetConnection(r.Context(), id)
	if err != nil {
		s.repoError(w, r, err)
		return
	}
	var req connectionRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.SourceType == "" {
		req.SourceType = current.SourceType
	}
	in, err := validateConnection(req, false)
	if err != nil {
		badRequest(w, err)
		return
	}
	item, err := s.repo.UpdateConnection(r.Context(), id, in)
	if err != nil {
		s.repoError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func validateConnection(req connectionRequest, create bool) (domain.ConnectionInput, error) {
	req.SourceType = strings.ToLower(strings.TrimSpace(req.SourceType))
	req.Name = strings.TrimSpace(req.Name)
	req.BaseURL = strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	req.AuthType = strings.ToLower(strings.TrimSpace(req.AuthType))
	if req.Name == "" || req.BaseURL == "" {
		return domain.ConnectionInput{}, errors.New("name and base_url are required")
	}
	u, err := url.ParseRequestURI(req.BaseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return domain.ConnectionInput{}, errors.New("base_url must be an http(s) URL without credentials")
	}
	switch req.SourceType {
	case models.SourceConfluence:
		if req.AuthType == "" {
			req.AuthType = "bearer"
		}
		if req.AuthType != "bearer" && req.AuthType != "basic" {
			return domain.ConnectionInput{}, errors.New("Confluence auth_type must be bearer or basic")
		}
		if req.AuthType == "basic" && strings.TrimSpace(req.Username) == "" {
			return domain.ConnectionInput{}, errors.New("username is required for basic auth")
		}
	case models.SourceGitLab:
		req.AuthType = "token"
	default:
		return domain.ConnectionInput{}, errors.New("source_type must be confluence or gitlab")
	}
	if create && strings.TrimSpace(req.Token) == "" {
		return domain.ConnectionInput{}, errors.New("token is required")
	}
	return domain.ConnectionInput{
		SourceType: req.SourceType, Name: req.Name, BaseURL: req.BaseURL, AuthType: req.AuthType,
		Username: strings.TrimSpace(req.Username), Secret: req.Token, SkipTLSVerify: req.SkipTLSVerify,
	}, nil
}

func (s *Server) deleteConnection(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := s.repo.DeleteConnection(r.Context(), id); err != nil {
		s.repoError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) testConnection(w http.ResponseWriter, r *http.Request) {
	conn, ok := s.connection(w, r)
	if !ok {
		return
	}
	var err error
	switch conn.SourceType {
	case models.SourceConfluence:
		err = confluence.New(s.confluenceConfig(conn), s.log).Test(r.Context())
	case models.SourceGitLab:
		err = s.newGitLabClient(conn).Test(r.Context())
	}
	if err != nil {
		s.log.Warn("connection test failed", "connection_id", conn.ID, "source_type", conn.SourceType, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "connection test failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) remoteConfluenceSpaces(w http.ResponseWriter, r *http.Request) {
	conn, ok := s.connection(w, r)
	if !ok {
		return
	}
	if conn.SourceType != models.SourceConfluence {
		badRequest(w, errors.New("connection is not Confluence"))
		return
	}
	items, err := confluence.New(s.confluenceConfig(conn), s.log).ListSpaces(r.Context())
	if err != nil {
		s.log.Warn("list Confluence spaces failed", "connection_id", conn.ID, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to load Confluence spaces"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"spaces": items})
}

func (s *Server) confluenceConfig(conn models.ConnectionSecret) confluence.Config {
	return confluence.Config{
		BaseURL: conn.BaseURL, Token: conn.Secret, AuthType: conn.AuthType, Username: conn.Username,
		SkipTLSVerify: conn.SkipTLSVerify, PageLimit: s.cfg.Sources.PageLimit,
	}
}

func (s *Server) gitlabProjects(w http.ResponseWriter, r *http.Request) {
	client, _, ok := s.gitlabClient(w, r)
	if !ok {
		return
	}
	items, err := client.SearchProjects(r.Context(), strings.TrimSpace(r.URL.Query().Get("q")))
	if err != nil {
		s.remoteError(w, r, "failed to load GitLab projects", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": items})
}

func (s *Server) gitlabBranches(w http.ResponseWriter, r *http.Request) {
	s.gitlabRefs(w, r, false)
}

func (s *Server) gitlabTags(w http.ResponseWriter, r *http.Request) {
	s.gitlabRefs(w, r, true)
}

func (s *Server) gitlabRefs(w http.ResponseWriter, r *http.Request, tags bool) {
	client, _, ok := s.gitlabClient(w, r)
	if !ok {
		return
	}
	projectID, err := strconv.ParseInt(r.URL.Query().Get("project_id"), 10, 64)
	if err != nil || projectID <= 0 {
		badRequest(w, errors.New("valid project_id is required"))
		return
	}
	var items []gitlab.Ref
	if tags {
		items, err = client.ListTags(r.Context(), projectID)
	} else {
		items, err = client.ListBranches(r.Context(), projectID)
	}
	if err != nil {
		s.remoteError(w, r, "failed to load GitLab refs", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"refs": items})
}

func (s *Server) gitlabClient(w http.ResponseWriter, r *http.Request) (*gitlab.RESTClient, models.ConnectionSecret, bool) {
	conn, ok := s.connection(w, r)
	if !ok {
		return nil, models.ConnectionSecret{}, false
	}
	if conn.SourceType != models.SourceGitLab {
		badRequest(w, errors.New("connection is not GitLab"))
		return nil, models.ConnectionSecret{}, false
	}
	return s.newGitLabClient(conn), conn, true
}

func (s *Server) newGitLabClient(conn models.ConnectionSecret) *gitlab.RESTClient {
	return gitlab.New(gitlab.Config{
		BaseURL: conn.BaseURL, Token: conn.Secret, SkipTLSVerify: conn.SkipTLSVerify, MaxPages: s.cfg.GitLab.MaxPages,
	}, s.log)
}

func (s *Server) connection(w http.ResponseWriter, r *http.Request) (models.ConnectionSecret, bool) {
	id, ok := parseID(w, r)
	if !ok {
		return models.ConnectionSecret{}, false
	}
	conn, err := s.repo.GetConnection(r.Context(), id)
	if err != nil {
		s.repoError(w, r, err)
		return models.ConnectionSecret{}, false
	}
	return conn, true
}
