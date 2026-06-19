package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"confluence-rag/backend/internal/confluence"
	"confluence-rag/backend/internal/domain"
	"confluence-rag/backend/internal/gitlab"
	"confluence-rag/backend/internal/models"
)

type scopeRequest struct {
	ConnectionID    int64  `json:"connection_id"`
	SourceType      string `json:"source_type"`
	ScopeType       string `json:"scope_type"`
	Page            string `json:"page"`
	IncludeChildren bool   `json:"include_children"`
	SpaceKey        string `json:"space_key"`
	Project         string `json:"project"`
	ProjectID       int64  `json:"project_id"`
	Ref             string `json:"ref"`
	Name            string `json:"name"`
	Sync            bool   `json:"sync"`
}

func (s *Server) listScopes(w http.ResponseWriter, r *http.Request) {
	connectionID, _ := strconv.ParseInt(r.URL.Query().Get("connection_id"), 10, 64)
	items, err := s.repo.ListScopes(r.Context(), r.URL.Query().Get("source_type"), connectionID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"scopes": items})
}

func (s *Server) createScope(w http.ResponseWriter, r *http.Request) {
	var req scopeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	conn, err := s.repo.GetConnection(r.Context(), req.ConnectionID)
	if err != nil {
		s.repoError(w, r, err)
		return
	}

	in, err := s.buildScopeInput(r, conn, req)
	if err != nil {
		s.writeScopeInputError(w, r, err)
		return
	}
	item, err := s.repo.CreateScope(r.Context(), in)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if req.Sync {
		if _, err := s.repo.CreateSourceSyncJob(r.Context(), item.SourceType, item.ConnectionID, item.ID, modeForScope(item), false); err != nil {
			s.log.Error("initial scope job creation failed", "scope_id", item.ID, "error", err)
		}
	}
	writeJSON(w, http.StatusCreated, item)
}

type remoteScopeError struct {
	message string
	err     error
}

func (e *remoteScopeError) Error() string { return e.message }
func (e *remoteScopeError) Unwrap() error { return e.err }

func (s *Server) buildScopeInput(r *http.Request, conn models.ConnectionSecret, req scopeRequest) (domain.ScopeInput, error) {
	in := domain.ScopeInput{ConnectionID: conn.ID, SourceType: conn.SourceType}
	switch conn.SourceType {
	case models.SourceConfluence:
		return s.buildConfluenceScopeInput(r, conn, req, in)
	case models.SourceGitLab:
		return s.buildGitLabScopeInput(r, conn, req, in)
	default:
		return domain.ScopeInput{}, errors.New("unsupported source type")
	}
}

func (s *Server) buildConfluenceScopeInput(r *http.Request, conn models.ConnectionSecret, req scopeRequest, in domain.ScopeInput) (domain.ScopeInput, error) {
	if req.ScopeType == "space" {
		spaceKey := strings.TrimSpace(req.SpaceKey)
		if spaceKey == "" {
			return domain.ScopeInput{}, errors.New("space_key is required")
		}
		cfg, _ := json.Marshal(map[string]any{"space_key": spaceKey})
		in.ScopeType, in.ExternalID, in.Name, in.Config = "space", spaceKey, first(req.Name, spaceKey), cfg
		return in, nil
	}

	pageID, err := confluence.ParsePageID(req.Page)
	if err != nil {
		return domain.ScopeInput{}, err
	}
	page, err := confluence.New(s.confluenceConfig(conn), s.log).GetPage(r.Context(), pageID)
	if err != nil {
		return domain.ScopeInput{}, &remoteScopeError{message: "failed to load Confluence page", err: err}
	}
	raw, _ := json.Marshal(map[string]any{
		"page_id": pageID, "include_children": req.IncludeChildren, "space_key": page.SpaceKey,
	})
	in.ScopeType, in.ExternalID, in.Name, in.Config = "page", pageID, page.Title, raw
	return in, nil
}

func (s *Server) buildGitLabScopeInput(r *http.Request, conn models.ConnectionSecret, req scopeRequest, in domain.ScopeInput) (domain.ScopeInput, error) {
	projectKey := req.Project
	var err error
	if req.ProjectID > 0 {
		projectKey = strconv.FormatInt(req.ProjectID, 10)
	} else {
		projectKey, err = gitlab.ParseProject(req.Project, conn.BaseURL)
		if err != nil {
			return domain.ScopeInput{}, err
		}
	}
	project, err := s.newGitLabClient(conn).GetProject(r.Context(), projectKey)
	if err != nil {
		return domain.ScopeInput{}, &remoteScopeError{message: "failed to load GitLab project", err: err}
	}
	ref := req.Ref
	if ref == "" {
		ref = project.DefaultBranch
	}
	if ref == "" {
		return domain.ScopeInput{}, errors.New("ref is required because the project has no default branch")
	}
	raw, _ := json.Marshal(map[string]any{
		"project_id": project.ID, "project_path": project.PathWithNamespace, "ref": ref, "default_branch": project.DefaultBranch,
	})
	in.ScopeType = "repository"
	in.ExternalID = fmt.Sprintf("%d:%s", project.ID, ref)
	in.Name = project.PathWithNamespace + " @ " + ref
	in.Config = raw
	return in, nil
}

func (s *Server) writeScopeInputError(w http.ResponseWriter, r *http.Request, err error) {
	var remoteErr *remoteScopeError
	if errors.As(err, &remoteErr) {
		s.remoteError(w, r, remoteErr.message, remoteErr.err)
		return
	}
	badRequest(w, err)
}

func (s *Server) deleteScope(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := s.repo.DeleteScope(r.Context(), id); err != nil {
		s.repoError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) syncScope(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	scope, err := s.repo.GetScope(r.Context(), id)
	if err != nil {
		s.repoError(w, r, err)
		return
	}
	var req struct {
		Mode  string `json:"mode"`
		Force bool   `json:"force"`
	}
	if r.ContentLength > 0 && !decodeJSON(w, r, &req) {
		return
	}
	if req.Mode == "" {
		req.Mode = modeForScope(scope)
	}
	if !validSyncMode(req.Mode) {
		badRequest(w, errors.New("invalid sync mode"))
		return
	}
	job, err := s.repo.CreateSourceSyncJob(r.Context(), scope.SourceType, scope.ConnectionID, scope.ID, req.Mode, req.Force)
	if err != nil {
		if strings.Contains(err.Error(), "already active") {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "a sync job is already active for this scope"})
			return
		}
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func validSyncMode(mode string) bool {
	switch mode {
	case "incremental", "full", "page", "space", "repository":
		return true
	default:
		return false
	}
}

func modeForScope(scope models.SourceScope) string {
	switch scope.ScopeType {
	case "space":
		return "space"
	case "page":
		return "page"
	default:
		return "repository"
	}
}
