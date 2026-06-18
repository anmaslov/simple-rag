package httpapi

import (
	"testing"

	"confluence-rag/backend/internal/models"
)

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
