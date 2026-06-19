package httpapi

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"confluence-rag/backend/internal/models"
	"github.com/jackc/pgx/v5"
)

func TestPrepareSearchScopeResolvesLegacySpaceKeys(t *testing.T) {
	repo := &scopeRepositoryStub{
		scopes: []models.SourceScope{
			{ID: 10, ScopeType: "space", ExternalID: "ENG", SourceType: models.SourceConfluence},
			{ID: 11, ScopeType: "space", ExternalID: "OPS", SourceType: models.SourceConfluence},
			{ID: 12, ScopeType: "page", ExternalID: "ENG", SourceType: models.SourceConfluence},
		},
		scopeByID: map[int64]models.SourceScope{
			10: {ID: 10, ScopeType: "space", ExternalID: "ENG", SourceType: models.SourceConfluence},
		},
	}

	got, err := prepareSearchScope(context.Background(), repo, models.SearchScope{}, []string{"ENG", "missing"})
	if err != nil {
		t.Fatal(err)
	}
	want := models.SearchScope{
		SourceTypes: []string{models.SourceConfluence},
		ScopeIDs:    []int64{10},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected scope: got %+v, want %+v", got, want)
	}
	if repo.listScopesCalls != 1 {
		t.Fatalf("expected one legacy scope lookup, got %d", repo.listScopesCalls)
	}
}

func TestPrepareSearchScopePreservesExplicitScope(t *testing.T) {
	repo := &scopeRepositoryStub{
		scopeByID: map[int64]models.SourceScope{
			7: {ID: 7, ConnectionID: 3, SourceType: models.SourceGitLab},
		},
		connectionByID: map[int64]models.ConnectionSecret{
			3: {Connection: models.Connection{ID: 3, SourceType: models.SourceGitLab}},
		},
	}
	requested := models.SearchScope{
		SourceTypes:   []string{models.SourceGitLab},
		ConnectionIDs: []int64{3},
		ScopeIDs:      []int64{7},
	}

	got, err := prepareSearchScope(context.Background(), repo, requested, []string{"ignored"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, requested) {
		t.Fatalf("explicit scope changed: got %+v, want %+v", got, requested)
	}
	if repo.listScopesCalls != 0 {
		t.Fatalf("explicit scope must bypass legacy lookup, got %d calls", repo.listScopesCalls)
	}
}

func TestPrepareSearchScopeClassifiesClientErrors(t *testing.T) {
	tests := []struct {
		name  string
		repo  *scopeRepositoryStub
		scope models.SearchScope
		want  string
	}{
		{
			name:  "invalid source type",
			repo:  &scopeRepositoryStub{},
			scope: models.SearchScope{SourceTypes: []string{"jira"}},
			want:  "scope contains an invalid source_type",
		},
		{
			name: "missing connection",
			repo: &scopeRepositoryStub{connectionErr: pgx.ErrNoRows},
			scope: models.SearchScope{
				ConnectionIDs: []int64{42},
			},
			want: "connection_id 42 does not exist",
		},
		{
			name: "scope source mismatch",
			repo: &scopeRepositoryStub{
				scopeByID: map[int64]models.SourceScope{
					9: {ID: 9, SourceType: models.SourceConfluence},
				},
			},
			scope: models.SearchScope{
				SourceTypes: []string{models.SourceGitLab},
				ScopeIDs:    []int64{9},
			},
			want: "scope_id 9 does not match source_types",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := prepareSearchScope(context.Background(), tt.repo, tt.scope, nil)
			var invalid *invalidSearchScopeError
			if !errors.As(err, &invalid) {
				t.Fatalf("expected client scope error, got %v", err)
			}
			if err.Error() != tt.want {
				t.Fatalf("unexpected error: got %q, want %q", err, tt.want)
			}
		})
	}
}

func TestPrepareSearchScopePreservesRepositoryErrors(t *testing.T) {
	repoErr := errors.New("database unavailable")
	repo := &scopeRepositoryStub{connectionErr: repoErr}

	_, err := prepareSearchScope(
		context.Background(),
		repo,
		models.SearchScope{ConnectionIDs: []int64{1}},
		nil,
	)
	if !errors.Is(err, repoErr) {
		t.Fatalf("expected repository error, got %v", err)
	}
	var invalid *invalidSearchScopeError
	if errors.As(err, &invalid) {
		t.Fatalf("repository error must not be classified as a client error: %v", err)
	}
}

type scopeRepositoryStub struct {
	scopes             []models.SourceScope
	scopeByID          map[int64]models.SourceScope
	connectionByID     map[int64]models.ConnectionSecret
	listScopesErr      error
	scopeErr           error
	connectionErr      error
	listScopesCalls    int
	getScopeCalls      int
	getConnectionCalls int
}

func (r *scopeRepositoryStub) ListScopes(context.Context, string, int64) ([]models.SourceScope, error) {
	r.listScopesCalls++
	return r.scopes, r.listScopesErr
}

func (r *scopeRepositoryStub) GetConnection(_ context.Context, id int64) (models.ConnectionSecret, error) {
	r.getConnectionCalls++
	if r.connectionErr != nil {
		return models.ConnectionSecret{}, r.connectionErr
	}
	item, ok := r.connectionByID[id]
	if !ok {
		return models.ConnectionSecret{}, pgx.ErrNoRows
	}
	return item, nil
}

func (r *scopeRepositoryStub) GetScope(_ context.Context, id int64) (models.SourceScope, error) {
	r.getScopeCalls++
	if r.scopeErr != nil {
		return models.SourceScope{}, r.scopeErr
	}
	item, ok := r.scopeByID[id]
	if !ok {
		return models.SourceScope{}, pgx.ErrNoRows
	}
	return item, nil
}
