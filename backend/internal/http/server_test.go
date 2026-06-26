package httpapi

import (
	"net/http"
	"reflect"
	"sort"
	"testing"

	"confluence-rag/backend/internal/config"
	"confluence-rag/backend/internal/models"
	"github.com/go-chi/chi/v5"
)

func TestNewRouterRegistersPublicAPI(t *testing.T) {
	router, ok := NewRouter(config.Config{}, nil, nil, nil).(chi.Routes)
	if !ok {
		t.Fatal("router does not expose chi routes")
	}

	var got []string
	if err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		got = append(got, method+" "+route)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)

	want := []string{
		"DELETE /api/chat/sessions/{id}",
		"DELETE /api/connections/{id}/",
		"DELETE /api/scopes/{id}",
		"GET /api/chat/sessions",
		"GET /api/chat/sessions/{id}/messages",
		"GET /api/connections/",
		"GET /api/connections/{id}/confluence/spaces",
		"GET /api/connections/{id}/gitlab/branches",
		"GET /api/connections/{id}/gitlab/projects",
		"GET /api/connections/{id}/gitlab/tags",
		"GET /api/documents",
		"GET /api/health",
		"GET /api/jobs",
		"GET /api/pages",
		"GET /api/pages/{id}",
		"GET /api/scopes/",
		"GET /api/settings",
		"GET /api/spaces",
		"GET /api/sync/status",
		"POST /api/chat",
		"POST /api/chat/stream",
		"POST /api/connections/",
		"POST /api/connections/{id}/test",
		"POST /api/jobs/{id}/cancel",
		"POST /api/scopes/",
		"POST /api/scopes/{id}/sync",
		"POST /api/search",
		"PUT /api/connections/{id}/",
		"PUT /api/settings",
	}
	sort.Strings(want)

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected API routes:\ngot:  %v\nwant: %v", got, want)
	}
}

func TestValidateConnectionMasksSecretFromModel(t *testing.T) {
	in, err := validateConnection(connectionRequest{
		SourceType: "gitlab", Name: "Main", BaseURL: "https://gitlab.local", Token: "secret",
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if in.Secret != "secret" || in.AuthType != "token" {
		t.Fatalf("unexpected validated connection: %+v", in)
	}
}

func TestValidateConnection(t *testing.T) {
	cases := []connectionRequest{
		{},
		{SourceType: "unknown", Name: "x", BaseURL: "https://x", Token: "x"},
		{SourceType: "confluence", Name: "x", BaseURL: "file:///tmp/x", Token: "x"},
		{SourceType: "confluence", Name: "x", BaseURL: "https://user:pass@x", Token: "x"},
		{SourceType: "confluence", Name: "x", BaseURL: "https://x", AuthType: "basic", Token: "x"},
	}
	for i, req := range cases {
		if _, err := validateConnection(req, true); err == nil {
			t.Errorf("case %d expected validation error", i)
		}
	}
}

func TestValidateSearchScope(t *testing.T) {
	if err := validateSearchScope(models.SearchScope{}); err != nil {
		t.Fatalf("empty scope should be valid: %v", err)
	}
	if err := validateSearchScope(models.SearchScope{SourceTypes: []string{"jira"}}); err == nil {
		t.Fatal("expected invalid source type error")
	}
}
