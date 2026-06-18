package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"confluence-rag/backend/internal/chunker"
	"confluence-rag/backend/internal/config"
	"confluence-rag/backend/internal/confluence"
	"confluence-rag/backend/internal/domain"
	"confluence-rag/backend/internal/embeddings"
	"confluence-rag/backend/internal/gitlab"
	"confluence-rag/backend/internal/models"
)

type Worker struct {
	cfg      config.Config
	repo     domain.Repository
	embedder embeddings.Embedder
	log      *slog.Logger
}

func NewWorker(cfg config.Config, repo domain.Repository, embedder embeddings.Embedder, log *slog.Logger) *Worker {
	return &Worker{cfg: cfg, repo: repo, embedder: embedder, log: log}
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
	w.log.Info("claimed sync job", "job_id", job.ID, "source_type", job.SourceType, "scope_id", job.ScopeID, "mode", job.Mode)
	if err := w.runJob(ctx, job); err != nil {
		w.log.Error("sync job failed", "job_id", job.ID, "error", err)
		return w.repo.FinishJob(ctx, job.ID, "failed", safeJobError(err))
	}
	if job.ScopeID != nil {
		_ = w.repo.MarkScopeSynced(ctx, *job.ScopeID)
	}
	return w.repo.FinishJob(ctx, job.ID, "success", "")
}

func (w *Worker) runJob(ctx context.Context, job models.SyncJob) error {
	if job.ScopeID == nil {
		return w.runLegacyConfluenceJob(ctx, job)
	}
	scope, err := w.repo.GetScope(ctx, *job.ScopeID)
	if err != nil {
		return fmt.Errorf("load scope: %w", err)
	}
	conn, err := w.repo.GetConnection(ctx, scope.ConnectionID)
	if err != nil {
		return fmt.Errorf("load connection: %w", err)
	}
	switch scope.SourceType {
	case models.SourceConfluence:
		return w.syncConfluence(ctx, job, conn, scope)
	case models.SourceGitLab:
		return w.syncGitLab(ctx, job, conn, scope)
	default:
		return errors.New("unsupported source type")
	}
}

func (w *Worker) runLegacyConfluenceJob(ctx context.Context, job models.SyncJob) error {
	if w.cfg.Confluence.BaseURL == "" {
		return errors.New("legacy Confluence env connection is not configured")
	}
	client := confluence.New(w.cfg.Confluence, w.log)
	switch job.Mode {
	case "space":
		return w.syncLegacySpace(ctx, job.ID, client, job.SpaceKey, job.ForceReindex)
	case "cql":
		return w.syncLegacyCQL(ctx, job.ID, client, job.CQL, job.ForceReindex)
	case "incremental":
		cql := "type=page and lastmodified > now('-7d')"
		if len(w.cfg.Confluence.SpaceKeys) > 0 {
			cql += " and space in (" + joinQuoted(w.cfg.Confluence.SpaceKeys) + ")"
		}
		return w.syncLegacyCQL(ctx, job.ID, client, cql, false)
	default:
		if len(w.cfg.Confluence.RootPageIDs) == 0 {
			return errors.New("CONFLUENCE_ROOT_PAGE_IDS is required for legacy full sync")
		}
		for _, id := range w.cfg.Confluence.RootPageIDs {
			page, err := client.GetPage(ctx, id)
			if err != nil {
				w.skip(job.ID, id, err)
				continue
			}
			scope, err := w.ensureLegacyPageScope(ctx, page, true)
			if err != nil {
				return err
			}
			if err := w.syncPageTree(ctx, job.ID, client, scope, page, true, map[string]struct{}{}); err != nil {
				w.skip(job.ID, id, err)
			}
		}
		return nil
	}
}

func (w *Worker) ensureLegacyPageScope(ctx context.Context, page confluence.Page, children bool) (models.SourceScope, error) {
	connectionID, err := w.ensureEnvConnection(ctx)
	if err != nil {
		return models.SourceScope{}, err
	}
	cfg, _ := json.Marshal(map[string]any{"page_id": page.ID, "include_children": children, "space_key": page.SpaceKey, "from_env": true})
	return w.repo.CreateScope(ctx, domain.ScopeInput{ConnectionID: connectionID, SourceType: models.SourceConfluence, ScopeType: "page", ExternalID: page.ID, Name: page.Title, Config: cfg})
}

func (w *Worker) ensureEnvConnection(ctx context.Context) (int64, error) {
	conns, err := w.repo.ListConnections(ctx, models.SourceConfluence)
	if err != nil {
		return 0, err
	}
	var connectionID int64
	for _, c := range conns {
		if c.Name == "Environment Confluence" {
			connectionID = c.ID
			break
		}
	}
	if connectionID == 0 {
		conn, err := w.repo.CreateConnection(ctx, domain.ConnectionInput{
			SourceType: models.SourceConfluence, Name: "Environment Confluence", BaseURL: w.cfg.Confluence.BaseURL,
			AuthType: w.cfg.Confluence.AuthType, Username: w.cfg.Confluence.Username, Secret: w.cfg.Confluence.Token,
			SkipTLSVerify: w.cfg.Confluence.SkipTLSVerify,
		})
		if err != nil {
			return 0, err
		}
		connectionID = conn.ID
	}
	return connectionID, nil
}

func (w *Worker) syncConfluence(ctx context.Context, job models.SyncJob, conn models.ConnectionSecret, scope models.SourceScope) error {
	cfg := w.cfg.Confluence
	cfg.BaseURL, cfg.Token, cfg.AuthType, cfg.Username, cfg.SkipTLSVerify = conn.BaseURL, conn.Secret, conn.AuthType, conn.Username, conn.SkipTLSVerify
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
	cur := confluence.Cursor{Limit: w.cfg.Confluence.PageLimit}
	for pageNo := 0; pageNo < 10000; pageNo++ {
		batch, err := client.ListPagesBySpace(ctx, space, cur)
		if err != nil {
			return err
		}
		for _, p := range batch.Pages {
			if err := w.countAndIndexPage(ctx, jobID, scope, p, force); err != nil {
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
	if _, ok := visited[page.ID]; ok {
		return nil
	}
	visited[page.ID] = struct{}{}
	if err := w.countAndIndexPage(ctx, jobID, scope, page, force); err != nil {
		w.skip(jobID, page.ID, err)
	}
	cur := confluence.Cursor{Limit: w.cfg.Confluence.PageLimit}
	for pageNo := 0; pageNo < 10000; pageNo++ {
		batch, err := client.ListChildPages(ctx, page.ID, cur)
		if err != nil {
			return err
		}
		for _, child := range batch.Pages {
			if err := w.syncPageTree(ctx, jobID, client, scope, child, force, visited); err != nil {
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

func (w *Worker) syncGitLab(ctx context.Context, job models.SyncJob, conn models.ConnectionSecret, scope models.SourceScope) error {
	var sc struct {
		ProjectID   int64  `json:"project_id"`
		ProjectPath string `json:"project_path"`
		Ref         string `json:"ref"`
	}
	if err := json.Unmarshal(scope.Config, &sc); err != nil || sc.ProjectID == 0 {
		return errors.New("invalid GitLab scope configuration")
	}
	client := gitlab.New(gitlab.Config{BaseURL: conn.BaseURL, Token: conn.Secret, SkipTLSVerify: conn.SkipTLSVerify, MaxPages: w.cfg.GitLab.MaxPages}, w.log)
	project, err := client.GetProject(ctx, strconv.FormatInt(sc.ProjectID, 10))
	if err != nil {
		return err
	}
	if sc.ProjectPath == "" {
		sc.ProjectPath = project.PathWithNamespace
	}
	if sc.Ref == "" {
		sc.Ref = project.DefaultBranch
	}
	tree, err := client.ListTree(ctx, sc.ProjectID, sc.Ref)
	if err != nil {
		return err
	}
	policy := gitlab.FilePolicy{MaxBytes: w.cfg.GitLab.MaxFileBytes, ExcludedDirs: w.cfg.GitLab.ExcludedDirs, ExcludedFiles: w.cfg.GitLab.ExcludedFiles, AllowedExtensions: w.cfg.GitLab.AllowedExtensions}
	seen := make([]string, 0, len(tree))
	for _, item := range tree {
		if item.Type != "blob" || !policy.Allow(item.Path, 0, nil) {
			continue
		}
		_ = w.repo.IncJob(ctx, job.ID, 1, 0, 0)
		file, err := client.GetRawFile(ctx, sc.ProjectID, item.Path, sc.Ref)
		if err != nil {
			w.skip(job.ID, item.Path, err)
			continue
		}
		if !policy.Allow(item.Path, int64(len(file.Content)), file.Content) {
			_ = w.repo.IncJob(ctx, job.ID, 0, 0, 1)
			continue
		}
		externalID := sc.Ref + ":" + item.Path
		seen = append(seen, externalID)
		webURL := strings.TrimRight(conn.BaseURL, "/") + "/" + sc.ProjectPath + "/-/blob/" + url.PathEscape(sc.Ref) + "/" + escapePath(item.Path)
		content := string(file.Content)
		meta := map[string]any{
			"project_id": sc.ProjectID, "project_path": sc.ProjectPath, "namespace": path.Dir(sc.ProjectPath),
			"project_name": path.Base(sc.ProjectPath), "ref": sc.Ref, "commit_sha": file.LastCommit,
			"file_path": item.Path, "extension": strings.TrimPrefix(path.Ext(item.Path), "."),
		}
		prefix := fmt.Sprintf("Repository: %s\nRef: %s\nPath: %s\n\n", sc.ProjectPath, sc.Ref, item.Path)
		err = w.indexDocument(ctx, job.ID, domain.DocumentInput{
			SourceType: models.SourceGitLab, ConnectionID: scope.ConnectionID, ScopeID: scope.ID, ExternalID: externalID,
			Title: item.Path, URL: webURL, Content: content, ContentHash: chunker.Hash(content), Metadata: meta,
		}, job.ForceReindex, prefix)
		if err != nil {
			w.skip(job.ID, item.Path, err)
		}
	}
	if _, err := w.repo.DeleteDocumentsNotSeen(ctx, scope.ID, seen); err != nil {
		return err
	}
	return nil
}

func (w *Worker) indexDocument(ctx context.Context, jobID int64, in domain.DocumentInput, force bool, chunkPrefix string) error {
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
	inputs := make([]domain.ChunkInput, 0, len(chunks))
	for i, ch := range chunks {
		var vec []float32
		if i < len(vecs) {
			vec = vecs[i]
		}
		content := chunkPrefix + ch.Content
		inputs = append(inputs, domain.ChunkInput{Index: ch.Index, Content: content, Hash: chunker.Hash(content), TokenCount: ch.TokenCount, Metadata: in.Metadata, Embedding: vec})
	}
	if err := w.repo.ReplaceDocumentChunks(ctx, doc.ID, inputs); err != nil {
		return err
	}
	return w.repo.IncJob(ctx, jobID, 0, 1, 0)
}

func ShouldReindex(unchanged, indexed, hasChunks, force bool) bool {
	return force || !unchanged || !indexed || !hasChunks
}

func (w *Worker) syncLegacySpace(ctx context.Context, jobID int64, client confluence.Client, space string, force bool) error {
	scope, err := w.ensureLegacySpaceScope(ctx, space)
	if err != nil {
		return err
	}
	return w.syncSpace(ctx, jobID, client, scope, space, force)
}

func (w *Worker) syncLegacyCQL(ctx context.Context, jobID int64, client confluence.Client, cql string, force bool) error {
	cur := confluence.Cursor{Limit: w.cfg.Confluence.PageLimit}
	for pageNo := 0; pageNo < 10000; pageNo++ {
		batch, err := client.SearchPagesByCQL(ctx, cql, cur)
		if err != nil {
			return err
		}
		for _, p := range batch.Pages {
			scope, err := w.ensureLegacySpaceScope(ctx, p.SpaceKey)
			if err != nil {
				w.skip(jobID, p.ID, err)
				continue
			}
			if err := w.countAndIndexPage(ctx, jobID, scope, p, force); err != nil {
				w.skip(jobID, p.ID, err)
			}
		}
		if !batch.HasNext {
			return nil
		}
		cur = batch.Next
	}
	return errors.New("Confluence CQL pagination limit exceeded")
}

func (w *Worker) ensureLegacySpaceScope(ctx context.Context, space string) (models.SourceScope, error) {
	connectionID, err := w.ensureEnvConnection(ctx)
	if err != nil {
		return models.SourceScope{}, err
	}
	cfg, _ := json.Marshal(map[string]any{"space_key": space, "from_env": true})
	return w.repo.CreateScope(ctx, domain.ScopeInput{ConnectionID: connectionID, SourceType: models.SourceConfluence, ScopeType: "space", ExternalID: space, Name: space, Config: cfg})
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

func joinQuoted(items []string) string {
	out := make([]string, 0, len(items))
	for _, v := range items {
		out = append(out, "'"+strings.ReplaceAll(v, "'", "''")+"'")
	}
	return strings.Join(out, ",")
}
