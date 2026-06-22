package models

import (
	"encoding/json"
	"time"
)

const (
	SourceConfluence = "confluence"
	SourceGitLab     = "gitlab"
)

type Connection struct {
	ID            int64     `json:"id"`
	SourceType    string    `json:"source_type"`
	Name          string    `json:"name"`
	BaseURL       string    `json:"base_url"`
	AuthType      string    `json:"auth_type"`
	Username      string    `json:"username,omitempty"`
	HasToken      bool      `json:"has_token"`
	SkipTLSVerify bool      `json:"skip_tls_verify"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type ConnectionSecret struct {
	Connection
	Secret string `json:"-"`
}

type SourceScope struct {
	ID           int64           `json:"id"`
	ConnectionID int64           `json:"connection_id"`
	SourceType   string          `json:"source_type"`
	ScopeType    string          `json:"scope_type"`
	ExternalID   string          `json:"external_id"`
	Name         string          `json:"name"`
	Config       json.RawMessage `json:"config"`
	Enabled      bool            `json:"enabled"`
	LastSyncedAt *time.Time      `json:"last_synced_at,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type Document struct {
	ID              int64           `json:"id"`
	SourceType      string          `json:"source_type"`
	ConnectionID    int64           `json:"connection_id"`
	ScopeID         int64           `json:"scope_id"`
	ExternalID      string          `json:"external_id"`
	Title           string          `json:"title"`
	URL             string          `json:"url"`
	Content         string          `json:"content,omitempty"`
	ContentHash     string          `json:"content_hash"`
	SourceUpdatedAt *time.Time      `json:"source_updated_at,omitempty"`
	IndexedAt       *time.Time      `json:"indexed_at,omitempty"`
	Metadata        json.RawMessage `json:"metadata"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type Space struct {
	ID   int64  `json:"id"`
	Key  string `json:"space_key"`
	Name string `json:"name"`
}

// Page is retained for the compatibility /api/pages endpoint.
type Page struct {
	ID                  int64      `json:"id"`
	ConfluenceID        string     `json:"confluence_id"`
	SpaceKey            string     `json:"space_key"`
	Title               string     `json:"title"`
	URL                 string     `json:"url"`
	Version             int        `json:"version"`
	Status              string     `json:"status"`
	ContentHash         string     `json:"content_hash"`
	RawHTML             string     `json:"raw_html,omitempty"`
	PlainText           string     `json:"plain_text,omitempty"`
	AncestorsJSON       []byte     `json:"ancestors_json,omitempty"`
	ConfluenceUpdatedAt time.Time  `json:"confluence_updated_at"`
	IndexedAt           *time.Time `json:"indexed_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type SyncJob struct {
	ID               int64      `json:"id"`
	Status           string     `json:"status"`
	Mode             string     `json:"mode"`
	SourceType       string     `json:"source_type"`
	ConnectionID     *int64     `json:"connection_id,omitempty"`
	ScopeID          *int64     `json:"scope_id,omitempty"`
	ForceReindex     bool       `json:"force_reindex"`
	SpaceKey         string     `json:"space_key,omitempty"`
	CQL              string     `json:"cql,omitempty"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	FinishedAt       *time.Time `json:"finished_at,omitempty"`
	DocumentsFound   int        `json:"documents_found"`
	DocumentsIndexed int        `json:"documents_indexed"`
	DocumentsSkipped int        `json:"documents_skipped"`
	PagesFound       int        `json:"pages_found"`
	PagesIndexed     int        `json:"pages_indexed"`
	PagesSkipped     int        `json:"pages_skipped"`
	ErrorMessage     string     `json:"error_message,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type SearchScope struct {
	SourceTypes   []string `json:"source_types"`
	ConnectionIDs []int64  `json:"connection_ids"`
	ScopeIDs      []int64  `json:"scope_ids"`
}

type SearchResult struct {
	DocumentID   int64           `json:"document_id"`
	PageID       int64           `json:"page_id,omitempty"`
	ExternalID   string          `json:"external_id"`
	ConfluenceID string          `json:"confluence_id,omitempty"`
	SourceType   string          `json:"source_type"`
	ConnectionID int64           `json:"connection_id"`
	ScopeID      int64           `json:"scope_id"`
	SourceLabel  string          `json:"source_label"`
	Title        string          `json:"title"`
	URL          string          `json:"url"`
	SpaceKey     string          `json:"space_key,omitempty"`
	Repository   string          `json:"repository,omitempty"`
	Ref          string          `json:"ref,omitempty"`
	FilePath     string          `json:"file_path,omitempty"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
	ChunkID      int64           `json:"chunk_id"`
	Chunk        string          `json:"chunk"`
	Score        float64         `json:"score"`
}

type ChatSession struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ChatMessage struct {
	ID        int64          `json:"id"`
	SessionID string         `json:"session_id"`
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	Sources   []SearchResult `json:"sources"`
	CreatedAt time.Time      `json:"created_at"`
}
