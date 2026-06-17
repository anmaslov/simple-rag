package chunker

import (
	"strings"
	"testing"
)

func TestCleanHTMLKeepsReadableTextAndTables(t *testing.T) {
	got := CleanHTML(`<h1>Title</h1><p>Hello <a href="/x">link</a></p><table><tr><td>A</td><td>B</td></tr></table><script>bad()</script>`)
	for _, want := range []string{"Title", "Hello", "link (/x)", "A", "B"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in %q", want, got)
		}
	}
	if strings.Contains(got, "bad") {
		t.Fatalf("script content leaked: %q", got)
	}
}

func TestChunkerSplit(t *testing.T) {
	chunks := Chunker{Size: 20, Overlap: 5}.Split("one two three four five six seven eight nine")
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	if chunks[0].Hash == "" || chunks[0].TokenCount == 0 {
		t.Fatalf("chunk metadata was not filled")
	}
}
