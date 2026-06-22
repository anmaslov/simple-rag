package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"confluence-rag/backend/internal/models"
	"github.com/jackc/pgx/v5"
)

type searchRequest struct {
	Query     string             `json:"query"`
	Scope     models.SearchScope `json:"scope"`
	SpaceKeys []string           `json:"space_keys"`
	TopK      int                `json:"top_k"`
}

type searchScopeRepository interface {
	ListScopes(context.Context, string, int64) ([]models.SourceScope, error)
	GetConnection(context.Context, int64) (models.ConnectionSecret, error)
	GetScope(context.Context, int64) (models.SourceScope, error)
}

type invalidSearchScopeError struct {
	err error
}

func (e *invalidSearchScopeError) Error() string { return e.err.Error() }
func (e *invalidSearchScopeError) Unwrap() error { return e.err }

func invalidSearchScope(format string, args ...any) error {
	return &invalidSearchScopeError{err: fmt.Errorf(format, args...)}
}

func prepareSearchScope(
	ctx context.Context,
	repo searchScopeRepository,
	scope models.SearchScope,
	spaceKeys []string,
) (models.SearchScope, error) {
	resolved, err := resolveLegacySpaceKeys(ctx, repo, scope, spaceKeys)
	if err != nil {
		return models.SearchScope{}, err
	}
	if err := validateSearchScope(resolved); err != nil {
		return models.SearchScope{}, &invalidSearchScopeError{err: err}
	}
	if err := validateSearchScopeResources(ctx, repo, resolved); err != nil {
		return models.SearchScope{}, err
	}
	return resolved, nil
}

func resolveLegacySpaceKeys(
	ctx context.Context,
	repo searchScopeRepository,
	scope models.SearchScope,
	spaceKeys []string,
) (models.SearchScope, error) {
	if len(spaceKeys) == 0 || len(scope.ScopeIDs) > 0 {
		return scope, nil
	}
	scopes, err := repo.ListScopes(ctx, models.SourceConfluence, 0)
	if err != nil {
		return models.SearchScope{}, err
	}
	wanted := make(map[string]struct{}, len(spaceKeys))
	for _, key := range spaceKeys {
		wanted[key] = struct{}{}
	}
	scope.SourceTypes = []string{models.SourceConfluence}
	for _, item := range scopes {
		if _, ok := wanted[item.ExternalID]; item.ScopeType == "space" && ok {
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
	ids := make([]int64, 0, len(scope.ConnectionIDs)+len(scope.ScopeIDs))
	ids = append(ids, scope.ConnectionIDs...)
	ids = append(ids, scope.ScopeIDs...)
	for _, id := range ids {
		if id <= 0 {
			return errors.New("scope ids must be positive")
		}
	}
	return nil
}

func validateSearchScopeResources(ctx context.Context, repo searchScopeRepository, scope models.SearchScope) error {
	for _, id := range scope.ConnectionIDs {
		if _, err := repo.GetConnection(ctx, id); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return invalidSearchScope("connection_id %d does not exist", id)
			}
			return err
		}
	}
	for _, id := range scope.ScopeIDs {
		item, err := repo.GetScope(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return invalidSearchScope("scope_id %d does not exist", id)
			}
			return err
		}
		if len(scope.SourceTypes) > 0 && !contains(scope.SourceTypes, item.SourceType) {
			return invalidSearchScope("scope_id %d does not match source_types", id)
		}
		if len(scope.ConnectionIDs) > 0 && !containsID(scope.ConnectionIDs, item.ConnectionID) {
			return invalidSearchScope("scope_id %d does not match connection_ids", id)
		}
	}
	return nil
}

func (s *Server) requestSearchScope(
	w http.ResponseWriter,
	r *http.Request,
	scope models.SearchScope,
	spaceKeys []string,
) (models.SearchScope, bool) {
	resolved, err := prepareSearchScope(r.Context(), s.repo, scope, spaceKeys)
	if err == nil {
		return resolved, true
	}
	var invalid *invalidSearchScopeError
	if errors.As(err, &invalid) {
		badRequest(w, invalid)
	} else {
		s.internalError(w, r, err)
	}
	return models.SearchScope{}, false
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
	scope, ok := s.requestSearchScope(w, r, req.Scope, req.SpaceKeys)
	if !ok {
		return
	}
	results, err := s.search.Search(r.Context(), req.Query, scope, req.TopK)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}
