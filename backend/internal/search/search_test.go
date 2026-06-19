package search

import (
	"strings"
	"testing"

	"confluence-rag/backend/internal/models"
)

func TestMergeNormalizesAndDeduplicates(t *testing.T) {
	vector := []models.SearchResult{{ChunkID: 1, Title: "A", Score: 0.9}, {ChunkID: 2, Title: "B", Score: 1.0}}
	keyword := []models.SearchResult{{ChunkID: 1, Title: "A", Score: 10}}
	got := Merge(vector, keyword, 10, 0.7, 0.3)
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got))
	}
	if got[0].ChunkID != 1 {
		t.Fatalf("expected merged keyword/vector result first, got %+v", got[0])
	}
}

func TestMergeDiversifiesDocumentsAndPenalizesMocks(t *testing.T) {
	vector := []models.SearchResult{
		{DocumentID: 1, ChunkID: 11, SourceType: "gitlab", FilePath: "internal/service/mailing.go", Score: 1.0},
		{DocumentID: 1, ChunkID: 12, SourceType: "gitlab", FilePath: "internal/service/mailing.go", Score: 0.99},
		{DocumentID: 2, ChunkID: 21, SourceType: "gitlab", FilePath: "internal/service/mock/mailing.go", Score: 0.98},
		{DocumentID: 3, ChunkID: 31, SourceType: "gitlab", FilePath: "internal/api/create_lna.go", Score: 0.8},
	}

	got := Merge(vector, nil, 3, 1, 0)
	if len(got) != 3 {
		t.Fatalf("expected 3 results, got %d", len(got))
	}
	if got[0].ChunkID != 11 || got[1].ChunkID != 31 || got[2].ChunkID != 21 {
		t.Fatalf("unexpected ranking: %+v", got)
	}
}

func TestKeywordQueriesAddsLatinAliasForCyrillicAcronym(t *testing.T) {
	got := keywordQueries("Опиши схему работы ЛНА или массовых рассылок")
	if len(got) != 2 {
		t.Fatalf("expected original query and acronym alias, got %#v", got)
	}
	if got[1] != "LNA" {
		t.Fatalf("expected LNA alias, got %q", got[1])
	}
}

func TestKeywordQueriesLeavesNormalWordsAlone(t *testing.T) {
	got := keywordQueries("Как работает рассылка")
	if len(got) != 1 {
		t.Fatalf("expected only original query, got %#v", got)
	}
}

func TestVectorQueryExpandsGitLabFlowQuestion(t *testing.T) {
	scope := models.SearchScope{SourceTypes: []string{models.SourceGitLab}}
	got := vectorQuery("Опиши схему работы ЛНА", scope)
	for _, want := range []string{"Опиши схему работы ЛНА", "LNA", "call flow", "send recipients"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected expanded vector query to contain %q, got %q", want, got)
		}
	}
}

func TestVectorQueryDoesNotExpandOrdinaryOrMixedSearch(t *testing.T) {
	query := "Где объявлен MailingID?"
	if got := vectorQuery(query, models.SearchScope{SourceTypes: []string{models.SourceGitLab}}); got != query {
		t.Fatalf("ordinary code lookup must stay unchanged, got %q", got)
	}
	flowQuery := "Опиши схему работы ЛНА"
	if got := vectorQuery(flowQuery, models.SearchScope{}); got != flowQuery {
		t.Fatalf("mixed-source search must stay unchanged, got %q", got)
	}
}
