package jobs

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"confluence-rag/backend/internal/chunker"
	"confluence-rag/backend/internal/config"
	"confluence-rag/backend/internal/confluence"
	"confluence-rag/backend/internal/domain"
	"confluence-rag/backend/internal/embeddings"
)

type Worker struct {
	cfg      config.Config
	repo     domain.Repository
	cf       confluence.Client
	embedder embeddings.Embedder
	log      *slog.Logger
}

func NewWorker(cfg config.Config, repo domain.Repository, cf confluence.Client, embedder embeddings.Embedder, log *slog.Logger) *Worker {
	return &Worker{cfg: cfg, repo: repo, cf: cf, embedder: embedder, log: log}
}

func (w *Worker) Run(ctx context.Context) error {
	t := time.NewTicker(w.cfg.Worker.PollInterval)
	defer t.Stop()
	for {
		if err := w.tick(ctx); err != nil {
			w.log.Error("worker tick failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

func (w *Worker) tick(ctx context.Context) error {
	job, ok, err := w.repo.ClaimNextJob(ctx)
	if err != nil || !ok {
		return err
	}
	w.log.Info("claimed sync job", "job_id", job.ID, "mode", job.Mode, "space", job.SpaceKey)
	if err := w.runJob(ctx, job.ID, job.Mode, job.SpaceKey, job.CQL); err != nil {
		w.log.Error("sync job failed", "job_id", job.ID, "error", err)
		return w.repo.FinishJob(ctx, job.ID, "failed", err.Error())
	}
	return w.repo.FinishJob(ctx, job.ID, "success", "")
}

func (w *Worker) runJob(ctx context.Context, jobID int64, mode, spaceKey, cql string) error {
	switch mode {
	case "space":
		return w.syncSpace(ctx, jobID, spaceKey)
	case "cql":
		return w.syncCQL(ctx, jobID, cql)
	case "incremental":
		return w.syncIncremental(ctx, jobID)
	default:
		return w.syncConfiguredRoots(ctx, jobID, true)
	}
}

func (w *Worker) syncConfiguredRoots(ctx context.Context, jobID int64, forceReindex bool) error {
	if len(w.cfg.Confluence.RootPageIDs) == 0 {
		return errors.New("CONFLUENCE_ROOT_PAGE_IDS is required for full sync")
	}

	visited := make(map[string]struct{}, len(w.cfg.Confluence.RootPageIDs))
	for _, pageID := range w.cfg.Confluence.RootPageIDs {
		root, err := w.cf.GetPage(ctx, pageID)
		if err != nil {
			w.log.Error("root page load failed", "page_id", pageID, "error", err)
			continue
		}
		if err := w.syncPageSubtree(ctx, jobID, root, visited, forceReindex); err != nil {
			w.log.Error("root page sync failed", "page_id", pageID, "error", err)
		}
	}
	return nil
}

func (w *Worker) syncPageSubtree(ctx context.Context, jobID int64, page confluence.Page, visited map[string]struct{}, forceReindex bool) error {
	if _, ok := visited[page.ID]; ok {
		return nil
	}
	visited[page.ID] = struct{}{}
	w.countAndIndexPage(ctx, jobID, page, forceReindex)

	cur := confluence.Cursor{Limit: w.cfg.Confluence.PageLimit}
	for {
		batch, err := w.cf.ListChildPages(ctx, page.ID, cur)
		if err != nil {
			return err
		}
		for _, child := range batch.Pages {
			if err := w.syncPageSubtree(ctx, jobID, child, visited, forceReindex); err != nil {
				w.log.Error("child page sync failed", "page_id", child.ID, "space", child.SpaceKey, "error", err)
			}
		}
		if !batch.HasNext {
			return nil
		}
		cur = batch.Next
	}
}

func (w *Worker) syncIncremental(ctx context.Context, jobID int64) error {
	cql := "type=page and lastmodified > now('-7d')"
	if len(w.cfg.Confluence.RootPageIDs) > 0 {
		ids := joinCSV(w.cfg.Confluence.RootPageIDs)
		cql += " and (id in (" + ids + ") or ancestor in (" + ids + "))"
		return w.syncCQL(ctx, jobID, cql)
	}
	if len(w.cfg.Confluence.SpaceKeys) > 0 {
		cql += " and space in (" + joinQuoted(w.cfg.Confluence.SpaceKeys) + ")"
	}
	return w.syncCQL(ctx, jobID, cql)
}

func (w *Worker) syncSpace(ctx context.Context, jobID int64, spaceKey string) error {
	cur := confluence.Cursor{Limit: w.cfg.Confluence.PageLimit}
	for {
		batch, err := w.cf.ListPagesBySpace(ctx, spaceKey, cur)
		if err != nil {
			return err
		}
		for _, p := range batch.Pages {
			w.countAndIndexPage(ctx, jobID, p, false)
		}
		if !batch.HasNext {
			return nil
		}
		cur = batch.Next
	}
}

func (w *Worker) syncCQL(ctx context.Context, jobID int64, cql string) error {
	cur := confluence.Cursor{Limit: w.cfg.Confluence.PageLimit}
	for {
		batch, err := w.cf.SearchPagesByCQL(ctx, cql, cur)
		if err != nil {
			return err
		}
		for _, p := range batch.Pages {
			w.countAndIndexPage(ctx, jobID, p, false)
		}
		if !batch.HasNext {
			return nil
		}
		cur = batch.Next
	}
}

func (w *Worker) countAndIndexPage(ctx context.Context, jobID int64, p confluence.Page, forceReindex bool) {
	if err := w.repo.IncJob(ctx, jobID, 1, 0, 0); err != nil {
		w.log.Warn("sync job counter update failed", "job_id", jobID, "error", err)
	}
	if err := w.indexPage(ctx, jobID, p, forceReindex); err != nil {
		w.log.Error("page indexing failed", "page_id", p.ID, "space", p.SpaceKey, "error", err)
		if err := w.repo.IncJob(ctx, jobID, 0, 0, 1); err != nil {
			w.log.Warn("sync job counter update failed", "job_id", jobID, "error", err)
		}
	}
}

func (w *Worker) indexPage(ctx context.Context, jobID int64, p confluence.Page, forceReindex bool) error {
	if p.SpaceKey != "" {
		if err := w.repo.UpsertSpace(ctx, p.SpaceKey, p.SpaceKey); err != nil {
			w.log.Warn("space upsert failed", "space", p.SpaceKey, "error", err)
		}
	}
	plain := chunker.CleanHTML(p.RawHTML)
	hash := chunker.Hash(plain)
	page, unchanged, err := w.repo.UpsertPage(ctx, domain.UpsertPageInput{
		ConfluenceID: p.ID, SpaceKey: p.SpaceKey, Title: p.Title, URL: p.URL, Version: p.Version, Status: p.Status,
		ContentHash: hash, RawHTML: p.RawHTML, PlainText: plain, Ancestors: p.Ancestors, ConfluenceUpdatedAt: p.UpdatedAt,
	})
	if err != nil {
		return err
	}
	chunks := chunker.Chunker{Size: w.cfg.Chunk.Size, Overlap: w.cfg.Chunk.Overlap}.Split(plain)
	if len(chunks) == 0 {
		return w.repo.IncJob(ctx, jobID, 0, 0, 1)
	}
	if unchanged && page.IndexedAt != nil && !forceReindex {
		hasChunks, err := w.repo.PageHasChunks(ctx, page.ID)
		if err != nil {
			return err
		}
		if hasChunks {
			return w.repo.IncJob(ctx, jobID, 0, 0, 1)
		}
	}
	texts := make([]string, len(chunks))
	for i, ch := range chunks {
		texts[i] = ch.Content
	}
	vecs, err := w.embedder.Embed(ctx, texts)
	if err != nil {
		return err
	}
	inputs := make([]domain.ChunkInput, 0, len(chunks))
	for i, ch := range chunks {
		var vec []float32
		if i < len(vecs) {
			vec = vecs[i]
		}
		inputs = append(inputs, domain.ChunkInput{Index: ch.Index, Content: ch.Content, Hash: ch.Hash, TokenCount: ch.TokenCount, Embedding: vec})
	}
	if err := w.repo.ReplaceChunks(ctx, page.ID, inputs); err != nil {
		return err
	}
	return w.repo.IncJob(ctx, jobID, 0, 1, 0)
}

func joinQuoted(items []string) string {
	out := ""
	for i, v := range items {
		if i > 0 {
			out += ","
		}
		out += "'" + strings.ReplaceAll(v, "'", "''") + "'"
	}
	return out
}

func joinCSV(items []string) string {
	out := ""
	for i, v := range items {
		if i > 0 {
			out += ","
		}
		out += strings.TrimSpace(v)
	}
	return out
}
