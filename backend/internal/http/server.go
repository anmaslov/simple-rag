package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"confluence-rag/backend/internal/config"
	"confluence-rag/backend/internal/confluence"
	"confluence-rag/backend/internal/domain"
	"confluence-rag/backend/internal/gitlab"
	"confluence-rag/backend/internal/models"
	"confluence-rag/backend/internal/rag"
	"confluence-rag/backend/internal/search"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
)

type Server struct {
	cfg    config.Config
	repo   domain.Repository
	search *search.Service
	rag    *rag.Service
	log    *slog.Logger
}

func NewRouter(cfg config.Config, repo domain.Repository, searchSvc *search.Service, ragSvc *rag.Service, loggers ...*slog.Logger) http.Handler {
	log := slog.Default()
	if len(loggers) > 0 && loggers[0] != nil {
		log = loggers[0]
	}
	s := &Server{cfg: cfg, repo: repo, search: searchSvc, rag: ragSvc, log: log}
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer)
	r.Use(jsonContent)
	r.Get("/api/health", s.health)

	r.Route("/api/connections", func(r chi.Router) {
		r.Get("/", s.listConnections)
		r.Post("/", s.createConnection)
		r.Route("/{id}", func(r chi.Router) {
			r.Put("/", s.updateConnection)
			r.Delete("/", s.deleteConnection)
			r.Post("/test", s.testConnection)
			r.Get("/confluence/spaces", s.remoteConfluenceSpaces)
			r.Get("/gitlab/projects", s.gitlabProjects)
			r.Get("/gitlab/branches", s.gitlabBranches)
			r.Get("/gitlab/tags", s.gitlabTags)
		})
	})
	r.Route("/api/scopes", func(r chi.Router) {
		r.Get("/", s.listScopes)
		r.Post("/", s.createScope)
		r.Delete("/{id}", s.deleteScope)
		r.Post("/{id}/sync", s.syncScope)
	})
	r.Get("/api/documents", s.documents)

	// Compatibility endpoints.
	r.Get("/api/spaces", s.spaces)
	r.Post("/api/sync", s.createLegacySync)
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

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

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
	return domain.ConnectionInput{SourceType: req.SourceType, Name: req.Name, BaseURL: req.BaseURL, AuthType: req.AuthType, Username: strings.TrimSpace(req.Username), Secret: req.Token, SkipTLSVerify: req.SkipTLSVerify}, nil
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
		cfg := s.cfg.Confluence
		cfg.BaseURL, cfg.Token, cfg.AuthType, cfg.Username, cfg.SkipTLSVerify = conn.BaseURL, conn.Secret, conn.AuthType, conn.Username, conn.SkipTLSVerify
		err = confluence.New(cfg, s.log).Test(r.Context())
	case models.SourceGitLab:
		err = gitlab.New(gitlab.Config{BaseURL: conn.BaseURL, Token: conn.Secret, SkipTLSVerify: conn.SkipTLSVerify, MaxPages: s.cfg.GitLab.MaxPages}, s.log).Test(r.Context())
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
	cfg := s.cfg.Confluence
	cfg.BaseURL, cfg.Token, cfg.AuthType, cfg.Username, cfg.SkipTLSVerify = conn.BaseURL, conn.Secret, conn.AuthType, conn.Username, conn.SkipTLSVerify
	items, err := confluence.New(cfg, s.log).ListSpaces(r.Context())
	if err != nil {
		s.log.Warn("list Confluence spaces failed", "connection_id", conn.ID, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to load Confluence spaces"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"spaces": items})
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
	return gitlab.New(gitlab.Config{BaseURL: conn.BaseURL, Token: conn.Secret, SkipTLSVerify: conn.SkipTLSVerify, MaxPages: s.cfg.GitLab.MaxPages}, s.log), conn, true
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
	req.SourceType = conn.SourceType
	var in domain.ScopeInput
	in.ConnectionID, in.SourceType = conn.ID, conn.SourceType
	switch conn.SourceType {
	case models.SourceConfluence:
		if req.ScopeType == "space" {
			req.SpaceKey = strings.TrimSpace(req.SpaceKey)
			if req.SpaceKey == "" {
				badRequest(w, errors.New("space_key is required"))
				return
			}
			cfg, _ := json.Marshal(map[string]any{"space_key": req.SpaceKey})
			in.ScopeType, in.ExternalID, in.Name, in.Config = "space", req.SpaceKey, first(req.Name, req.SpaceKey), cfg
		} else {
			pageID, err := confluence.ParsePageID(req.Page)
			if err != nil {
				badRequest(w, err)
				return
			}
			cfg := s.cfg.Confluence
			cfg.BaseURL, cfg.Token, cfg.AuthType, cfg.Username, cfg.SkipTLSVerify = conn.BaseURL, conn.Secret, conn.AuthType, conn.Username, conn.SkipTLSVerify
			page, err := confluence.New(cfg, s.log).GetPage(r.Context(), pageID)
			if err != nil {
				s.remoteError(w, r, "failed to load Confluence page", err)
				return
			}
			raw, _ := json.Marshal(map[string]any{"page_id": pageID, "include_children": req.IncludeChildren, "space_key": page.SpaceKey})
			in.ScopeType, in.ExternalID, in.Name, in.Config = "page", pageID, page.Title, raw
		}
	case models.SourceGitLab:
		client := gitlab.New(gitlab.Config{BaseURL: conn.BaseURL, Token: conn.Secret, SkipTLSVerify: conn.SkipTLSVerify, MaxPages: s.cfg.GitLab.MaxPages}, s.log)
		projectKey := req.Project
		if req.ProjectID > 0 {
			projectKey = strconv.FormatInt(req.ProjectID, 10)
		} else {
			projectKey, err = gitlab.ParseProject(req.Project, conn.BaseURL)
			if err != nil {
				badRequest(w, err)
				return
			}
		}
		project, err := client.GetProject(r.Context(), projectKey)
		if err != nil {
			s.remoteError(w, r, "failed to load GitLab project", err)
			return
		}
		if req.Ref == "" {
			req.Ref = project.DefaultBranch
		}
		if req.Ref == "" {
			badRequest(w, errors.New("ref is required because the project has no default branch"))
			return
		}
		raw, _ := json.Marshal(map[string]any{"project_id": project.ID, "project_path": project.PathWithNamespace, "ref": req.Ref, "default_branch": project.DefaultBranch})
		in.ScopeType, in.ExternalID, in.Name, in.Config = "repository", fmt.Sprintf("%d:%s", project.ID, req.Ref), project.PathWithNamespace+" @ "+req.Ref, raw
	default:
		badRequest(w, errors.New("unsupported source type"))
		return
	}
	item, err := s.repo.CreateScope(r.Context(), in)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if req.Sync {
		_, err = s.repo.CreateSourceSyncJob(r.Context(), item.SourceType, item.ConnectionID, item.ID, modeForScope(item), false)
		if err != nil {
			s.log.Error("initial scope job creation failed", "scope_id", item.ID, "error", err)
		}
	}
	writeJSON(w, http.StatusCreated, item)
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
	if req.Mode != "incremental" && req.Mode != "full" && req.Mode != "page" && req.Mode != "space" && req.Mode != "repository" {
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

func (s *Server) createLegacySync(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Mode     string `json:"mode"`
		SpaceKey string `json:"space_key"`
		CQL      string `json:"cql"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Mode == "" {
		req.Mode = "full"
	}
	if err := validateSyncRequest(req.Mode, req.SpaceKey, req.CQL); err != nil {
		badRequest(w, err)
		return
	}
	job, err := s.repo.CreateSyncJob(r.Context(), req.Mode, req.SpaceKey, req.CQL)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func validateSyncRequest(mode, spaceKey, cql string) error {
	switch mode {
	case "full", "incremental":
		return nil
	case "space":
		if strings.TrimSpace(spaceKey) == "" {
			return errors.New("space_key is required for space sync")
		}
	case "cql":
		if strings.TrimSpace(cql) == "" {
			return errors.New("cql is required for cql sync")
		}
	default:
		return errors.New("mode must be one of full, space, cql, incremental")
	}
	return nil
}

func (s *Server) syncStatus(w http.ResponseWriter, r *http.Request) {
	items, err := s.repo.ListJobs(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": items})
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

type searchRequest struct {
	Query     string             `json:"query"`
	Scope     models.SearchScope `json:"scope"`
	SpaceKeys []string           `json:"space_keys"`
	TopK      int                `json:"top_k"`
}

func (s *Server) resolveScope(ctx context.Context, req searchRequest) (models.SearchScope, error) {
	scope := req.Scope
	if len(req.SpaceKeys) == 0 || len(scope.ScopeIDs) > 0 {
		return scope, nil
	}
	scopes, err := s.repo.ListScopes(ctx, models.SourceConfluence, 0)
	if err != nil {
		return scope, err
	}
	wanted := map[string]bool{}
	for _, key := range req.SpaceKeys {
		wanted[key] = true
	}
	scope.SourceTypes = []string{models.SourceConfluence}
	for _, item := range scopes {
		if item.ScopeType == "space" && wanted[item.ExternalID] {
			scope.ScopeIDs = append(scope.ScopeIDs, item.ID)
		}
	}
	return scope, nil
}

func validateSearchScope(scope models.SearchScope) error {
	for _, source := range scope.SourceTypes {
		if source != models.SourceConfluence && source != models.SourceGitLab {
			return errors.New("scope contains an invalid source_type")
		}
	}
	for _, id := range append(append([]int64{}, scope.ConnectionIDs...), scope.ScopeIDs...) {
		if id <= 0 {
			return errors.New("scope ids must be positive")
		}
	}
	return nil
}

func (s *Server) validateSearchScopeResources(ctx context.Context, scope models.SearchScope) error {
	for _, id := range scope.ConnectionIDs {
		if _, err := s.repo.GetConnection(ctx, id); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("connection_id %d does not exist", id)
			}
			return err
		}
	}
	for _, id := range scope.ScopeIDs {
		item, err := s.repo.GetScope(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("scope_id %d does not exist", id)
			}
			return err
		}
		if len(scope.SourceTypes) > 0 && !contains(scope.SourceTypes, item.SourceType) {
			return fmt.Errorf("scope_id %d does not match source_types", id)
		}
		if len(scope.ConnectionIDs) > 0 && !containsID(scope.ConnectionIDs, item.ConnectionID) {
			return fmt.Errorf("scope_id %d does not match connection_ids", id)
		}
	}
	return nil
}

func (s *Server) searchHandler(w http.ResponseWriter, r *http.Request) {
	var req searchRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Query = strings.TrimSpace(req.Query)
	if req.Query == "" {
		badRequest(w, errors.New("query is required"))
		return
	}
	scope, err := s.resolveScope(r.Context(), req)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := validateSearchScope(scope); err != nil {
		badRequest(w, err)
		return
	}
	if err := s.validateSearchScopeResources(r.Context(), scope); err != nil {
		if strings.Contains(err.Error(), "does not") {
			badRequest(w, err)
		} else {
			s.internalError(w, r, err)
		}
		return
	}
	results, err := s.search.Search(r.Context(), req.Query, scope, req.TopK)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

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
	scope, err := s.resolveScope(r.Context(), searchRequest{Scope: req.Scope, SpaceKeys: req.SpaceKeys})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := validateSearchScope(scope); err != nil {
		badRequest(w, err)
		return
	}
	if err := s.validateSearchScopeResources(r.Context(), scope); err != nil {
		if strings.Contains(err.Error(), "does not") {
			badRequest(w, err)
		} else {
			s.internalError(w, r, err)
		}
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
	scope, err := s.resolveScope(r.Context(), searchRequest{Scope: req.Scope, SpaceKeys: req.SpaceKeys})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := validateSearchScope(scope); err != nil {
		badRequest(w, err)
		return
	}
	if err := s.validateSearchScopeResources(r.Context(), scope); err != nil {
		if strings.Contains(err.Error(), "does not") {
			badRequest(w, err)
		} else {
			s.internalError(w, r, err)
		}
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

func (s *Server) settings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"confluence_base_url": s.cfg.Confluence.BaseURL, "confluence_auth_type": s.cfg.Confluence.AuthType,
		"confluence_root_page_ids": s.cfg.Confluence.RootPageIDs, "confluence_space_keys": s.cfg.Confluence.SpaceKeys,
		"llm_base_url": s.cfg.LLM.BaseURL, "llm_model": s.cfg.LLM.Model,
		"embeddings_base_url": s.cfg.Embeddings.BaseURL, "embeddings_model": s.cfg.Embeddings.Model,
		"embeddings_dim": s.cfg.Embeddings.Dim, "chunk_size": s.cfg.Chunk.Size, "chunk_overlap": s.cfg.Chunk.Overlap,
		"top_k": s.cfg.Search.TopK, "gitlab_max_file_bytes": s.cfg.GitLab.MaxFileBytes,
		"secrets": "configured via environment or write-only connection API; never returned",
	})
}

func (s *Server) updateSettings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "runtime settings are read-only; source connections are managed on the Sources page"})
}

func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		badRequest(w, errors.New("invalid id"))
		return 0, false
	}
	return id, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON request"})
		return false
	}
	return true
}

func first(items ...string) string {
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			return strings.TrimSpace(item)
		}
	}
	return ""
}

func contains(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func containsID(items []int64, value int64) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
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

func badRequest(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
}

func (s *Server) repoError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "resource not found"})
		return
	}
	s.internalError(w, r, err)
}

func (s *Server) internalError(w http.ResponseWriter, r *http.Request, err error) {
	s.log.Error("API request failed", "request_id", middleware.GetReqID(r.Context()), "path", r.URL.Path, "error", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
}

func (s *Server) remoteError(w http.ResponseWriter, r *http.Request, message string, err error) {
	s.log.Warn(message, "request_id", middleware.GetReqID(r.Context()), "error", err)
	writeJSON(w, http.StatusBadGateway, map[string]string{"error": message})
}
