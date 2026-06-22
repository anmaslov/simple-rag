package jobs

import "testing"

func TestUnchangedDocumentDoesNotReindex(t *testing.T) {
	if ShouldReindex(true, true, true, false) {
		t.Fatal("unchanged indexed document with chunks must not recreate embeddings")
	}
	if !ShouldReindex(true, true, true, true) {
		t.Fatal("forced sync must recreate embeddings")
	}
	if !ShouldReindex(false, true, true, false) {
		t.Fatal("changed document must recreate embeddings")
	}
}
