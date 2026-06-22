package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"

	"confluence-rag/backend/internal/chunker"
	"confluence-rag/backend/internal/domain"
	"confluence-rag/backend/internal/gitlab"
	"confluence-rag/backend/internal/models"
)

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
