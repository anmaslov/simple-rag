package models

import "time"

type Space struct {
	ID   int64  `json:"id"`
	Key  string `json:"space_key"`
	Name string `json:"name"`
}

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

type Chunk struct {
	ID          int64  `json:"id"`
	PageID      int64  `json:"page_id"`
	ChunkIndex  int    `json:"chunk_index"`
	Content     string `json:"content"`
	ContentHash string `json:"content_hash"`
	TokenCount  int    `json:"token_count"`
}

type SyncJob struct {
	ID           int64      `json:"id"`
	Status       string     `json:"status"`
	Mode         string     `json:"mode"`
	SpaceKey     string     `json:"space_key,omitempty"`
	CQL          string     `json:"cql,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	PagesFound   int        `json:"pages_found"`
	PagesIndexed int        `json:"pages_indexed"`
	PagesSkipped int        `json:"pages_skipped"`
	ErrorMessage string     `json:"error_message,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type SearchResult struct {
	PageID       int64   `json:"page_id"`
	ConfluenceID string  `json:"confluence_id"`
	Title        string  `json:"title"`
	URL          string  `json:"url"`
	SpaceKey     string  `json:"space_key"`
	ChunkID      int64   `json:"chunk_id"`
	Chunk        string  `json:"chunk"`
	Score        float64 `json:"score"`
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
