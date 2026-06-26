package domain

import (
	"context"
	"encoding/json"
	"time"

	"confluence-rag/backend/internal/models"
)

type ConnectionInput struct {
	SourceType    string
	Name          string
	BaseURL       string
	AuthType      string
	Username      string
	Secret        string
	SkipTLSVerify bool
}

type ScopeInput struct {
	ConnectionID int64
	SourceType   string
	ScopeType    string
	ExternalID   string
	Name         string
	Config       json.RawMessage
}

type DocumentInput struct {
	SourceType      string
	ConnectionID    int64
	ScopeID         int64
	ExternalID      string
	Title           string
	URL             string
	Content         string
	ContentHash     string
	SourceUpdatedAt time.Time
	Metadata        any
}

type ChunkInput struct {
	Index      int
	Content    string
	Hash       string
	TokenCount int
	Metadata   any
	Embedding  []float32
}

type ConnectionsRepository interface {
	CreateConnection(context.Context, ConnectionInput) (models.Connection, error)
	UpdateConnection(context.Context, int64, ConnectionInput) (models.Connection, error)
	ListConnections(context.Context, string) ([]models.Connection, error)
	GetConnection(context.Context, int64) (models.ConnectionSecret, error)
	DeleteConnection(context.Context, int64) error
}

type ScopesRepository interface {
	CreateScope(context.Context, ScopeInput) (models.SourceScope, error)
	ListScopes(context.Context, string, int64) ([]models.SourceScope, error)
	GetScope(context.Context, int64) (models.SourceScope, error)
	DeleteScope(context.Context, int64) error
	MarkScopeSynced(context.Context, int64) error
}

type DocumentsRepository interface {
	UpsertDocument(context.Context, DocumentInput) (models.Document, bool, error)
	DocumentHasChunks(context.Context, int64) (bool, error)
	ReplaceDocumentChunks(context.Context, int64, []ChunkInput) error
	DeleteDocumentsNotSeen(context.Context, int64, []string) (int64, error)
	ListDocuments(context.Context, string, int64, string) ([]models.Document, error)
}

type SpacesRepository interface {
	UpsertSpace(ctx context.Context, key, name string) error
	ListSpaces(ctx context.Context) ([]models.Space, error)
}

type PagesRepository interface {
	ListPages(ctx context.Context, space, q string) ([]models.Page, error)
	GetPage(ctx context.Context, id int64) (models.Page, error)
}

type SyncJobsRepository interface {
	CreateSourceSyncJob(context.Context, string, int64, int64, string, bool) (models.SyncJob, error)
	ClaimNextJob(ctx context.Context) (models.SyncJob, bool, error)
	CancelJob(ctx context.Context, id int64) (models.SyncJob, error)
	IsJobCancelled(ctx context.Context, id int64) (bool, error)
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
	VectorSearch(ctx context.Context, vector []float32, scope models.SearchScope, limit int) ([]models.SearchResult, error)
	KeywordSearch(ctx context.Context, query string, scope models.SearchScope, limit int) ([]models.SearchResult, error)
}

type Repository interface {
	ConnectionsRepository
	ScopesRepository
	DocumentsRepository
	SpacesRepository
	PagesRepository
	SyncJobsRepository
	ChatRepository
	SearchRepository
}
