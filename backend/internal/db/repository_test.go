package db

import (
	"reflect"
	"strings"
	"testing"

	"confluence-rag/backend/internal/models"
	sq "github.com/Masterminds/squirrel"
)

func TestAddScopeFilter(t *testing.T) {
	b := psql.Select("*").From("documents d")
	filtered := AddScopeFilter(b, models.SearchScope{
		SourceTypes:   []string{"gitlab"},
		ConnectionIDs: []int64{2},
		ScopeIDs:      []int64{7, 8},
	})
	sql, args, err := filtered.ToSql()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"d.source_type IN", "d.connection_id IN", "d.scope_id IN"} {
		if !strings.Contains(sql, want) {
			t.Fatalf("expected SQL to contain %q: %s", want, sql)
		}
	}
	if len(args) != 4 {
		t.Fatalf("expected 4 filter args, got %d", len(args))
	}
}

func TestAddScopeFilterEmptyMeansAllSources(t *testing.T) {
	base := sq.Select("*").From("documents d").PlaceholderFormat(sq.Dollar)
	sql, args, err := AddScopeFilter(base, models.SearchScope{}).ToSql()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sql, "WHERE") || len(args) != 0 {
		t.Fatalf("empty scope must not add filters: %s %#v", sql, args)
	}
}

func TestClaimNextJobQuery(t *testing.T) {
	sql, args, err := claimNextJobQuery()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"UPDATE sync_jobs SET status = $1",
		"NOT EXISTS (SELECT 1 FROM sync_jobs r",
		"FOR UPDATE SKIP LOCKED",
		"RETURNING " + jobColumns,
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("expected SQL to contain %q: %s", want, sql)
		}
	}
	if want := []any{"running", "pending", "running"}; !reflect.DeepEqual(args, want) {
		t.Fatalf("unexpected args: want %#v, got %#v", want, args)
	}
}
