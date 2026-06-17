package search

import (
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
