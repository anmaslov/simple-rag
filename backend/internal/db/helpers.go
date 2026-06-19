package db

import (
	"fmt"
	"strings"
	"time"

	"confluence-rag/backend/internal/models"

	"github.com/jackc/pgx/v5"
)

func scanConnection(row pgx.Row) (models.Connection, error) {
	var v models.Connection
	err := row.Scan(&v.ID, &v.SourceType, &v.Name, &v.BaseURL, &v.AuthType, &v.Username, &v.HasToken, &v.SkipTLSVerify, &v.CreatedAt, &v.UpdatedAt)
	return v, err
}

func scanScope(row pgx.Row) (models.SourceScope, error) {
	var v models.SourceScope
	err := row.Scan(&v.ID, &v.ConnectionID, &v.SourceType, &v.ScopeType, &v.ExternalID, &v.Name, &v.Config, &v.Enabled, &v.LastSyncedAt, &v.CreatedAt, &v.UpdatedAt)
	return v, err
}

func scanDocument(row pgx.Row) (models.Document, error) {
	var v models.Document
	err := row.Scan(&v.ID, &v.SourceType, &v.ConnectionID, &v.ScopeID, &v.ExternalID, &v.Title, &v.URL, &v.Content, &v.ContentHash, &v.SourceUpdatedAt, &v.IndexedAt, &v.Metadata, &v.CreatedAt, &v.UpdatedAt)
	return v, err
}

func scanJob(row pgx.Row) (models.SyncJob, error) {
	var v models.SyncJob
	err := row.Scan(&v.ID, &v.Status, &v.Mode, &v.SourceType, &v.ConnectionID, &v.ScopeID, &v.ForceReindex,
		&v.SpaceKey, &v.CQL, &v.StartedAt, &v.FinishedAt, &v.DocumentsFound, &v.DocumentsIndexed, &v.DocumentsSkipped,
		&v.PagesFound, &v.PagesIndexed, &v.PagesSkipped, &v.ErrorMessage, &v.CreatedAt, &v.UpdatedAt)
	return v, err
}

func scanCompatPage(row pgx.Row) (models.Page, error) {
	var v models.Page
	var updated *time.Time
	err := row.Scan(&v.ID, &v.ConfluenceID, &v.SpaceKey, &v.Title, &v.URL, &v.Version, &v.Status, &v.ContentHash,
		&v.PlainText, &updated, &v.IndexedAt, &v.CreatedAt, &v.UpdatedAt)
	if updated != nil {
		v.ConfluenceUpdatedAt = *updated
	}
	return v, err
}

func scanSearch(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]models.SearchResult, error) {
	var out []models.SearchResult
	for rows.Next() {
		var v models.SearchResult
		if err := rows.Scan(&v.DocumentID, &v.ExternalID, &v.SourceType, &v.ConnectionID, &v.ScopeID, &v.SourceLabel,
			&v.Title, &v.URL, &v.SpaceKey, &v.Repository, &v.Ref, &v.FilePath, &v.Metadata, &v.ChunkID, &v.Chunk, &v.Score); err != nil {
			return nil, err
		}
		if v.SourceType == models.SourceConfluence {
			v.PageID, v.ConfluenceID = v.DocumentID, v.ExternalID
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func nullableTime(v interface{ IsZero() bool }) any {
	if v.IsZero() {
		return nil
	}
	return v
}

func vectorLiteral(v []float32) any {
	if len(v) == 0 {
		return nil
	}
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = fmt.Sprintf("%g", f)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
