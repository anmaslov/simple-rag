package domain

import (
	"context"
	"encoding/json"
	"time"

	"confluence-rag/backend/internal/models"
)

type UpsertPageInput struct {
	ConfluenceID        string
	SpaceKey            string
	Title               string
	URL                 string
	Status              string
	ContentHash         string
	RawHTML             string
	PlainText           string
	Version             int
	Ancestors           any
	ConfluenceUpdatedAt time.Time
}

type ChunkInput struct {
	Index      int
	Content    string
	Hash       string
	TokenCount int
	Embedding  []float32
}

type SpacesRepository interface {
	UpsertSpace(ctx context.Context, key, name string) error
	ListSpaces(ctx context.Context) ([]models.Space, error)
}

type PagesRepository interface {
	UpsertPage(ctx context.Context, in UpsertPageInput) (models.Page, bool, error)
	PageHasChunks(ctx context.Context, pageID int64) (bool, error)
	ReplaceChunks(ctx context.Context, pageID int64, chunks []ChunkInput) error
	ListPages(ctx context.Context, space, q string) ([]models.Page, error)
	GetPage(ctx context.Context, id int64) (models.Page, error)
}

type SyncJobsRepository interface {
	CreateSyncJob(ctx context.Context, mode, spaceKey, cql string) (models.SyncJob, error)
	ClaimNextJob(ctx context.Context) (models.SyncJob, bool, error)
	FinishJob(ctx context.Context, id int64, status, msg string) error
	IncJob(ctx context.Context, id int64, found, indexed, skipped int) error
	ListJobs(ctx context.Context) ([]models.SyncJob, error)
}

type ChatRepository interface {
	EnsureChatSession(ctx context.Context, sessionID, title string) (string, error)
	SaveChatMessage(ctx context.Context, sessionID, role, content string, sources json.RawMessage) error
	ListChatSessions(ctx context.Context) ([]models.ChatSession, error)
	ListChatMessages(ctx context.Context, sessionID string) ([]models.ChatMessage, error)
	DeleteChatSession(ctx context.Context, sessionID string) error
}

type SearchRepository interface {
	VectorSearch(ctx context.Context, vector []float32, spaces []string, limit int) ([]models.SearchResult, error)
	KeywordSearch(ctx context.Context, query string, spaces []string, limit int) ([]models.SearchResult, error)
}

type Repository interface {
	SpacesRepository
	PagesRepository
	SyncJobsRepository
	ChatRepository
	SearchRepository
}
