package jobs

import (
	"errors"
	"strings"
	"testing"
)

func TestEscapePathEscapesSegmentsWithoutEscapingSeparators(t *testing.T) {
	got := escapePath("docs/Go guide.md")
	want := "docs/Go%20guide.md"
	if got != want {
		t.Fatalf("escapePath() = %q, want %q", got, want)
	}
}

func TestSafeJobErrorRemovesNewlines(t *testing.T) {
	if got := safeJobError(nil); got != "" {
		t.Fatalf("safeJobError(nil) = %q, want empty string", got)
	}

	got := safeJobError(errors.New("first line\nsecond line"))
	want := "first line second line"
	if got != want {
		t.Fatalf("safeJobError() = %q, want %q", got, want)
	}
}

func TestValidateEmbeddingBatch(t *testing.T) {
	if err := validateEmbeddingBatch([][]float32{{1}, {2}}, 2); err != nil {
		t.Fatalf("valid batch rejected: %v", err)
	}

	if err := validateEmbeddingBatch([][]float32{{1}}, 2); err == nil || !strings.Contains(err.Error(), "count mismatch") {
		t.Fatalf("expected count mismatch, got %v", err)
	}

	if err := validateEmbeddingBatch([][]float32{{1}, nil}, 2); err == nil || !strings.Contains(err.Error(), "embedding 1 is empty") {
		t.Fatalf("expected empty vector error, got %v", err)
	}
}
