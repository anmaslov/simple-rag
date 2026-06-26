package config

import "time"

type Config struct {
	HTTPAddr      string
	DatabaseURL   string
	Observability ObservabilityConfig
	Embeddings    OpenAIConfig
	LLM           LLMConfig
	Chunk         ChunkConfig
	Search        SearchConfig
	Worker        WorkerConfig
	Sources       SourcesConfig
	GitLab        GitLabConfig
}

type ObservabilityConfig struct {
	Addr            string
	ServiceName     string
	ServiceVersion  string
	Environment     string
	OTLPEndpoint    string
	TraceSampleRate float64
}

type SourcesConfig struct {
	PageLimit int
}

type OpenAIConfig struct {
	BaseURL       string
	APIKey        string
	Model         string
	Dim           int
	SendDimension bool
	SkipTLSVerify bool
}

type LLMConfig struct {
	BaseURL       string
	APIKey        string
	Model         string
	Temperature   float64
	SkipTLSVerify bool
}

type ChunkConfig struct {
	Size    int
	Overlap int
}

type SearchConfig struct {
	TopK          int
	VectorWeight  float64
	KeywordWeight float64
}

type WorkerConfig struct {
	PollInterval time.Duration
}

type GitLabConfig struct {
	MaxFileBytes      int64
	MaxPages          int
	ExcludedDirs      []string
	ExcludedFiles     []string
	AllowedExtensions []string
}
