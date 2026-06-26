package httpapi

import (
	"log/slog"
	"net/http"

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
	log    *slog.Logger
}

func NewRouter(cfg config.Config, repo domain.Repository, searchSvc *search.Service, ragSvc *rag.Service, loggers ...*slog.Logger) http.Handler {
	log := slog.Default()
	if len(loggers) > 0 && loggers[0] != nil {
		log = loggers[0]
	}
	return NewRouterWithMiddleware(cfg, repo, searchSvc, ragSvc, log)
}

func NewRouterWithMiddleware(
	cfg config.Config,
	repo domain.Repository,
	searchSvc *search.Service,
	ragSvc *rag.Service,
	log *slog.Logger,
	middlewares ...func(http.Handler) http.Handler,
) http.Handler {
	if log == nil {
		log = slog.Default()
	}
	s := &Server{cfg: cfg, repo: repo, search: searchSvc, rag: ragSvc, log: log}
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP)
	r.Use(middlewares...)
	r.Use(middleware.Recoverer)
	r.Use(jsonContent)

	r.Get("/api/health", s.health)
	r.Route("/api/connections", s.connectionRoutes)
	r.Route("/api/scopes", s.scopeRoutes)
	r.Get("/api/documents", s.documents)
	r.Get("/api/jobs", s.syncStatus)
	r.Post("/api/jobs/{id}/cancel", s.cancelJob)

	// Compatibility endpoints.
	r.Get("/api/spaces", s.spaces)
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

func (s *Server) connectionRoutes(r chi.Router) {
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
}

func (s *Server) scopeRoutes(r chi.Router) {
	r.Get("/", s.listScopes)
	r.Post("/", s.createScope)
	r.Delete("/{id}", s.deleteScope)
	r.Post("/{id}/sync", s.syncScope)
}
