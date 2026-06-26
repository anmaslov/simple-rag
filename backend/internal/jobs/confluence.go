package jobs

import (
	"context"
	"encoding/json"
	"errors"

	"confluence-rag/backend/internal/chunker"
	"confluence-rag/backend/internal/confluence"
	"confluence-rag/backend/internal/domain"
	"confluence-rag/backend/internal/models"
)

func (w *Worker) syncConfluence(ctx context.Context, job models.SyncJob, conn models.ConnectionSecret, scope models.SourceScope) error {
	cfg := confluence.Config{
		BaseURL: conn.BaseURL, Token: conn.Secret, AuthType: conn.AuthType, Username: conn.Username,
		SkipTLSVerify: conn.SkipTLSVerify, PageLimit: w.cfg.Sources.PageLimit,
	}
	client := confluence.New(cfg, w.log)
	var sc struct {
		PageID          string `json:"page_id"`
		SpaceKey        string `json:"space_key"`
		IncludeChildren bool   `json:"include_children"`
	}
	if err := json.Unmarshal(scope.Config, &sc); err != nil {
		return errors.New("invalid Confluence scope configuration")
	}
	switch scope.ScopeType {
	case "space":
		if sc.SpaceKey == "" {
			sc.SpaceKey = scope.ExternalID
		}
		return w.syncSpace(ctx, job.ID, client, scope, sc.SpaceKey, job.ForceReindex)
	case "page":
		if sc.PageID == "" {
			sc.PageID = scope.ExternalID
		}
		page, err := client.GetPage(ctx, sc.PageID)
		if err != nil {
			return err
		}
		if sc.IncludeChildren {
			return w.syncPageTree(ctx, job.ID, client, scope, page, job.ForceReindex, map[string]struct{}{})
		}
		return w.countAndIndexPage(ctx, job.ID, scope, page, job.ForceReindex)
	default:
		return errors.New("unsupported Confluence scope type")
	}
}

func (w *Worker) syncSpace(ctx context.Context, jobID int64, client confluence.Client, scope models.SourceScope, space string, force bool) error {
	cur := confluence.Cursor{Limit: w.cfg.Sources.PageLimit}
	for pageNo := 0; pageNo < 10000; pageNo++ {
		if err := w.ensureJobActive(ctx, jobID); err != nil {
			return err
		}
		batch, err := client.ListPagesBySpace(ctx, space, cur)
		if err != nil {
			return err
		}
		for _, p := range batch.Pages {
			if err := w.ensureJobActive(ctx, jobID); err != nil {
				return err
			}
			if err := w.countAndIndexPage(ctx, jobID, scope, p, force); err != nil {
				if errors.Is(err, errJobCancelled) {
					return err
				}
				w.skip(jobID, p.ID, err)
			}
		}
		if !batch.HasNext {
			return nil
		}
		cur = batch.Next
	}
	return errors.New("Confluence pagination limit exceeded")
}

func (w *Worker) syncPageTree(ctx context.Context, jobID int64, client confluence.Client, scope models.SourceScope, page confluence.Page, force bool, visited map[string]struct{}) error {
	if err := w.ensureJobActive(ctx, jobID); err != nil {
		return err
	}
	if _, ok := visited[page.ID]; ok {
		return nil
	}
	visited[page.ID] = struct{}{}
	if err := w.countAndIndexPage(ctx, jobID, scope, page, force); err != nil {
		if errors.Is(err, errJobCancelled) {
			return err
		}
		w.skip(jobID, page.ID, err)
	}
	cur := confluence.Cursor{Limit: w.cfg.Sources.PageLimit}
	for pageNo := 0; pageNo < 10000; pageNo++ {
		batch, err := client.ListChildPages(ctx, page.ID, cur)
		if err != nil {
			return err
		}
		for _, child := range batch.Pages {
			if err := w.syncPageTree(ctx, jobID, client, scope, child, force, visited); err != nil {
				if errors.Is(err, errJobCancelled) {
					return err
				}
				w.skip(jobID, child.ID, err)
			}
		}
		if !batch.HasNext {
			return nil
		}
		cur = batch.Next
	}
	return errors.New("Confluence child pagination limit exceeded")
}

func (w *Worker) countAndIndexPage(ctx context.Context, jobID int64, scope models.SourceScope, p confluence.Page, force bool) error {
	_ = w.repo.IncJob(ctx, jobID, 1, 0, 0)
	plain := chunker.CleanHTML(p.RawHTML)
	meta := map[string]any{"space_key": p.SpaceKey, "version": p.Version, "status": p.Status, "ancestors": p.Ancestors}
	return w.indexDocument(ctx, jobID, domain.DocumentInput{
		SourceType: models.SourceConfluence, ConnectionID: scope.ConnectionID, ScopeID: scope.ID, ExternalID: p.ID,
		Title: p.Title, URL: p.URL, Content: plain, ContentHash: chunker.Hash(plain), SourceUpdatedAt: p.UpdatedAt, Metadata: meta,
	}, force, "")
}
