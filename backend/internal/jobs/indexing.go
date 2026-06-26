package jobs

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"confluence-rag/backend/internal/chunker"
	"confluence-rag/backend/internal/domain"
)

func (w *Worker) indexDocument(ctx context.Context, jobID int64, in domain.DocumentInput, force bool, chunkPrefix string) error {
	if err := w.ensureJobActive(ctx, jobID); err != nil {
		return err
	}
	doc, unchanged, err := w.repo.UpsertDocument(ctx, in)
	if err != nil {
		return err
	}
	if unchanged && doc.IndexedAt != nil && !force {
		has, err := w.repo.DocumentHasChunks(ctx, doc.ID)
		if err != nil {
			return err
		}
		if !ShouldReindex(unchanged, doc.IndexedAt != nil, has, force) {
			return w.repo.IncJob(ctx, jobID, 0, 0, 1)
		}
	}
	chunks := chunker.Chunker{Size: w.cfg.Chunk.Size, Overlap: w.cfg.Chunk.Overlap}.Split(in.Content)
	if len(chunks) == 0 {
		return w.repo.IncJob(ctx, jobID, 0, 0, 1)
	}
	texts := make([]string, len(chunks))
	for i, ch := range chunks {
		texts[i] = chunkPrefix + ch.Content
	}
	vecs, err := w.embedder.Embed(ctx, texts)
	if err != nil {
		return err
	}
	if err := validateEmbeddingBatch(vecs, len(chunks)); err != nil {
		return err
	}
	if err := w.ensureJobActive(ctx, jobID); err != nil {
		return err
	}
	inputs := make([]domain.ChunkInput, 0, len(chunks))
	for i, ch := range chunks {
		content := chunkPrefix + ch.Content
		inputs = append(inputs, domain.ChunkInput{Index: ch.Index, Content: content, Hash: chunker.Hash(content), TokenCount: ch.TokenCount, Metadata: in.Metadata, Embedding: vecs[i]})
	}
	if err := w.repo.ReplaceDocumentChunks(ctx, doc.ID, inputs); err != nil {
		return err
	}
	return w.repo.IncJob(ctx, jobID, 0, 1, 0)
}

func validateEmbeddingBatch(vecs [][]float32, expected int) error {
	if len(vecs) != expected {
		return fmt.Errorf("embedding count mismatch: got %d vectors for %d chunks", len(vecs), expected)
	}
	for i, vec := range vecs {
		if len(vec) == 0 {
			return fmt.Errorf("embedding %d is empty", i)
		}
	}
	return nil
}

func ShouldReindex(unchanged, indexed, hasChunks, force bool) bool {
	return force || !unchanged || !indexed || !hasChunks
}

func (w *Worker) skip(jobID int64, document string, err error) {
	w.log.Error("document indexing failed", "job_id", jobID, "document", document, "error", err)
	_ = w.repo.IncJob(context.Background(), jobID, 0, 0, 1)
}

func escapePath(v string) string {
	parts := strings.Split(v, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}

func safeJobError(err error) string {
	if err == nil {
		return ""
	}
	return strings.ReplaceAll(err.Error(), "\n", " ")
}
