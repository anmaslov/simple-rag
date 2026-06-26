package config

import (
	"math"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLoadValidatedParsesExplicitEnvironment(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("HTTP_ADDR", "127.0.0.1:9090")
	t.Setenv("OBSERVABILITY_ADDR", "127.0.0.1:9091")
	t.Setenv("OTEL_SERVICE_NAME", "rag-test")
	t.Setenv("APP_VERSION", "1.2.3")
	t.Setenv("DEPLOYMENT_ENVIRONMENT", "test")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://jaeger:4317")
	t.Setenv("OTEL_TRACES_SAMPLER_ARG", "0.25")
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("EMBEDDINGS_BASE_URL", "https://embeddings.example/v1/")
	t.Setenv("EMBEDDINGS_MODEL", "embed-model")
	t.Setenv("EMBEDDINGS_DIM", "768")
	t.Setenv("EMBEDDINGS_SEND_DIMENSION", "false")
	t.Setenv("EMBEDDINGS_SKIP_TLS_VERIFY", "true")
	t.Setenv("LLM_BASE_URL", "http://llm.example/v1/")
	t.Setenv("LLM_MODEL", "llm-model")
	t.Setenv("LLM_TEMPERATURE", "0.25")
	t.Setenv("LLM_SKIP_TLS_VERIFY", "true")
	t.Setenv("CHUNK_SIZE", "500")
	t.Setenv("CHUNK_OVERLAP", "50")
	t.Setenv("SEARCH_TOP_K", "12")
	t.Setenv("SEARCH_VECTOR_WEIGHT", "0.7")
	t.Setenv("SEARCH_KEYWORD_WEIGHT", "0.3")
	t.Setenv("WORKER_POLL_SECONDS", "15")
	t.Setenv("SOURCE_PAGE_LIMIT", "75")
	t.Setenv("GITLAB_MAX_FILE_BYTES", "2147483648")
	t.Setenv("GITLAB_MAX_API_PAGES", "250")
	t.Setenv("GITLAB_EXCLUDED_DIRS", "vendor, node_modules")
	t.Setenv("GITLAB_EXCLUDED_FILES", ".env, secrets.yml")
	t.Setenv("GITLAB_TEXT_EXTENSIONS", ".go, .md")

	cfg, err := LoadValidated()
	if err != nil {
		t.Fatalf("LoadValidated() error = %v", err)
	}

	if cfg.HTTPAddr != "127.0.0.1:9090" {
		t.Errorf("HTTPAddr = %q", cfg.HTTPAddr)
	}
	if cfg.DatabaseURL != "postgres://example" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.Observability.Addr != "127.0.0.1:9091" ||
		cfg.Observability.ServiceName != "rag-test" ||
		cfg.Observability.ServiceVersion != "1.2.3" ||
		cfg.Observability.Environment != "test" ||
		cfg.Observability.OTLPEndpoint != "http://jaeger:4317" ||
		cfg.Observability.TraceSampleRate != 0.25 {
		t.Errorf("Observability = %+v", cfg.Observability)
	}
	if cfg.Embeddings.BaseURL != "https://embeddings.example/v1" {
		t.Errorf("Embeddings.BaseURL = %q", cfg.Embeddings.BaseURL)
	}
	if cfg.Embeddings.Model != "embed-model" ||
		cfg.Embeddings.Dim != 768 ||
		cfg.Embeddings.SendDimension ||
		!cfg.Embeddings.SkipTLSVerify {
		t.Errorf("Embeddings = %+v", cfg.Embeddings)
	}
	if cfg.LLM.BaseURL != "http://llm.example/v1" {
		t.Errorf("LLM.BaseURL = %q", cfg.LLM.BaseURL)
	}
	if cfg.LLM.Model != "llm-model" || cfg.LLM.Temperature != 0.25 || !cfg.LLM.SkipTLSVerify {
		t.Errorf("LLM = %+v", cfg.LLM)
	}
	if cfg.Chunk != (ChunkConfig{Size: 500, Overlap: 50}) {
		t.Errorf("Chunk = %+v", cfg.Chunk)
	}
	if cfg.Search != (SearchConfig{TopK: 12, VectorWeight: 0.7, KeywordWeight: 0.3}) {
		t.Errorf("Search = %+v", cfg.Search)
	}
	if cfg.Worker.PollInterval != 15*time.Second {
		t.Errorf("Worker.PollInterval = %s", cfg.Worker.PollInterval)
	}
	if cfg.Sources.PageLimit != 75 {
		t.Errorf("Sources.PageLimit = %d", cfg.Sources.PageLimit)
	}
	if cfg.GitLab.MaxFileBytes != 2147483648 || cfg.GitLab.MaxPages != 250 {
		t.Errorf("GitLab limits = %+v", cfg.GitLab)
	}
	assertStrings(t, "GitLab.ExcludedDirs", cfg.GitLab.ExcludedDirs, []string{"vendor", "node_modules"})
	assertStrings(t, "GitLab.ExcludedFiles", cfg.GitLab.ExcludedFiles, []string{".env", "secrets.yml"})
	assertStrings(t, "GitLab.AllowedExtensions", cfg.GitLab.AllowedExtensions, []string{".go", ".md"})
}

func TestLoadValidatedRejectsInvalidExplicitEnvironment(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "embeddings dimension", key: "EMBEDDINGS_DIM", value: "many"},
		{name: "embeddings send dimension flag", key: "EMBEDDINGS_SEND_DIMENSION", value: "sometimes"},
		{name: "embeddings TLS flag", key: "EMBEDDINGS_SKIP_TLS_VERIFY", value: "sometimes"},
		{name: "LLM temperature", key: "LLM_TEMPERATURE", value: "warm"},
		{name: "LLM TLS flag", key: "LLM_SKIP_TLS_VERIFY", value: "sometimes"},
		{name: "chunk size", key: "CHUNK_SIZE", value: "large"},
		{name: "chunk overlap", key: "CHUNK_OVERLAP", value: "some"},
		{name: "search top K", key: "SEARCH_TOP_K", value: "several"},
		{name: "vector weight", key: "SEARCH_VECTOR_WEIGHT", value: "high"},
		{name: "keyword weight", key: "SEARCH_KEYWORD_WEIGHT", value: "low"},
		{name: "worker poll", key: "WORKER_POLL_SECONDS", value: "soon"},
		{name: "source page limit", key: "SOURCE_PAGE_LIMIT", value: "many"},
		{name: "GitLab max file bytes", key: "GITLAB_MAX_FILE_BYTES", value: "huge"},
		{name: "GitLab max pages", key: "GITLAB_MAX_API_PAGES", value: "many"},
		{name: "trace sample rate", key: "OTEL_TRACES_SAMPLER_ARG", value: "many"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setValidEnvironment(t)
			t.Setenv(tt.key, tt.value)

			_, err := LoadValidated()
			if err == nil {
				t.Fatal("LoadValidated() error = nil")
			}
			if !strings.Contains(err.Error(), tt.key) {
				t.Fatalf("LoadValidated() error = %q, want env key %q", err, tt.key)
			}
		})
	}
}

func TestLoadValidatedUsesDefaultsForMissingEnvironment(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		check func(Config) bool
	}{
		{name: "string", key: "HTTP_ADDR", check: func(cfg Config) bool { return cfg.HTTPAddr == ":8080" }},
		{name: "integer", key: "CHUNK_SIZE", check: func(cfg Config) bool { return cfg.Chunk.Size == 900 }},
		{name: "float", key: "LLM_TEMPERATURE", check: func(cfg Config) bool { return cfg.LLM.Temperature == 0.1 }},
		{name: "boolean", key: "LLM_SKIP_TLS_VERIFY", check: func(cfg Config) bool { return !cfg.LLM.SkipTLSVerify }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setValidEnvironment(t)
			if err := os.Unsetenv(tt.key); err != nil {
				t.Fatalf("Unsetenv(%q): %v", tt.key, err)
			}

			cfg, err := LoadValidated()
			if err != nil {
				t.Fatalf("LoadValidated() error = %v", err)
			}
			if !tt.check(cfg) {
				t.Fatalf("LoadValidated() did not use default for missing %s: %+v", tt.key, cfg)
			}
		})
	}
}

func TestLoadKeepsFallbackCompatibility(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		check func(Config) bool
	}{
		{name: "integer", key: "CHUNK_SIZE", value: "invalid", check: func(cfg Config) bool { return cfg.Chunk.Size == 900 }},
		{name: "float", key: "LLM_TEMPERATURE", value: "invalid", check: func(cfg Config) bool { return cfg.LLM.Temperature == 0.1 }},
		{name: "boolean", key: "LLM_SKIP_TLS_VERIFY", value: "invalid", check: func(cfg Config) bool { return !cfg.LLM.SkipTLSVerify }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setValidEnvironment(t)
			t.Setenv(tt.key, tt.value)
			if cfg := Load(); !tt.check(cfg) {
				t.Fatalf("Load() did not use fallback for %s: %+v", tt.key, cfg)
			}
		})
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{name: "worker poll positive", mutate: func(c *Config) { c.Worker.PollInterval = 0 }, wantErr: "WORKER_POLL_SECONDS"},
		{name: "chunk size positive", mutate: func(c *Config) { c.Chunk.Size = 0 }, wantErr: "CHUNK_SIZE"},
		{name: "chunk overlap non-negative", mutate: func(c *Config) { c.Chunk.Overlap = -1 }, wantErr: "CHUNK_OVERLAP"},
		{name: "chunk overlap below size", mutate: func(c *Config) { c.Chunk.Overlap = c.Chunk.Size }, wantErr: "CHUNK_OVERLAP"},
		{name: "search top K positive", mutate: func(c *Config) { c.Search.TopK = 0 }, wantErr: "SEARCH_TOP_K"},
		{name: "vector weight non-negative", mutate: func(c *Config) { c.Search.VectorWeight = -0.1 }, wantErr: "SEARCH_VECTOR_WEIGHT"},
		{name: "vector weight is a number", mutate: func(c *Config) { c.Search.VectorWeight = math.NaN() }, wantErr: "SEARCH_VECTOR_WEIGHT"},
		{name: "keyword weight non-negative", mutate: func(c *Config) { c.Search.KeywordWeight = -0.1 }, wantErr: "SEARCH_KEYWORD_WEIGHT"},
		{name: "keyword weight is a number", mutate: func(c *Config) { c.Search.KeywordWeight = math.NaN() }, wantErr: "SEARCH_KEYWORD_WEIGHT"},
		{name: "weight sum positive", mutate: func(c *Config) { c.Search.VectorWeight, c.Search.KeywordWeight = 0, 0 }, wantErr: "weights sum"},
		{name: "source page limit positive", mutate: func(c *Config) { c.Sources.PageLimit = 0 }, wantErr: "SOURCE_PAGE_LIMIT"},
		{name: "GitLab max file bytes positive", mutate: func(c *Config) { c.GitLab.MaxFileBytes = 0 }, wantErr: "GITLAB_MAX_FILE_BYTES"},
		{name: "GitLab max pages positive", mutate: func(c *Config) { c.GitLab.MaxPages = 0 }, wantErr: "GITLAB_MAX_API_PAGES"},
		{name: "embeddings dimension positive", mutate: func(c *Config) { c.Embeddings.Dim = 0 }, wantErr: "EMBEDDINGS_DIM"},
		{name: "observability address required", mutate: func(c *Config) { c.Observability.Addr = "" }, wantErr: "OBSERVABILITY_ADDR"},
		{name: "trace sample rate non-negative", mutate: func(c *Config) { c.Observability.TraceSampleRate = -0.1 }, wantErr: "OTEL_TRACES_SAMPLER_ARG"},
		{name: "trace sample rate at most one", mutate: func(c *Config) { c.Observability.TraceSampleRate = 1.1 }, wantErr: "OTEL_TRACES_SAMPLER_ARG"},
		{name: "trace sample rate is a number", mutate: func(c *Config) { c.Observability.TraceSampleRate = math.NaN() }, wantErr: "OTEL_TRACES_SAMPLER_ARG"},
		{name: "HTTP address required", mutate: func(c *Config) { c.HTTPAddr = " " }, wantErr: "HTTP_ADDR"},
		{name: "database URL required", mutate: func(c *Config) { c.DatabaseURL = "" }, wantErr: "DATABASE_URL"},
		{name: "embeddings base URL required", mutate: func(c *Config) { c.Embeddings.BaseURL = "" }, wantErr: "EMBEDDINGS_BASE_URL"},
		{name: "embeddings model required", mutate: func(c *Config) { c.Embeddings.Model = "" }, wantErr: "EMBEDDINGS_MODEL"},
		{name: "LLM base URL required", mutate: func(c *Config) { c.LLM.BaseURL = "" }, wantErr: "LLM_BASE_URL"},
		{name: "LLM model required", mutate: func(c *Config) { c.LLM.Model = "" }, wantErr: "LLM_MODEL"},
		{name: "embeddings URL scheme", mutate: func(c *Config) { c.Embeddings.BaseURL = "ftp://example.com" }, wantErr: "EMBEDDINGS_BASE_URL"},
		{name: "embeddings URL host", mutate: func(c *Config) { c.Embeddings.BaseURL = "http:///v1" }, wantErr: "EMBEDDINGS_BASE_URL"},
		{name: "LLM URL scheme", mutate: func(c *Config) { c.LLM.BaseURL = "localhost:11434/v1" }, wantErr: "LLM_BASE_URL"},
		{name: "LLM URL host", mutate: func(c *Config) { c.LLM.BaseURL = "https:///v1" }, wantErr: "LLM_BASE_URL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.mutate(&cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatal("Validate() error = nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %q, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestConfigValidateAcceptsValidConfig(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestLoadValidatedRejectsExplicitEmptyRequiredValues(t *testing.T) {
	tests := []string{
		"HTTP_ADDR",
		"OBSERVABILITY_ADDR",
		"DATABASE_URL",
		"EMBEDDINGS_BASE_URL",
		"EMBEDDINGS_MODEL",
		"LLM_BASE_URL",
		"LLM_MODEL",
	}

	for _, key := range tests {
		t.Run(key, func(t *testing.T) {
			setValidEnvironment(t)
			t.Setenv(key, "")

			_, err := LoadValidated()
			if err == nil {
				t.Fatal("LoadValidated() error = nil")
			}
			if !strings.Contains(err.Error(), key) {
				t.Fatalf("LoadValidated() error = %q, want %q", err, key)
			}
		})
	}
}

func setValidEnvironment(t *testing.T) {
	t.Helper()
	values := map[string]string{
		"HTTP_ADDR":                  ":8080",
		"OBSERVABILITY_ADDR":         ":9090",
		"DATABASE_URL":               "postgres://rag:rag@localhost:5432/rag?sslmode=disable",
		"EMBEDDINGS_BASE_URL":        "http://localhost:11434/v1",
		"EMBEDDINGS_API_KEY":         "ollama",
		"EMBEDDINGS_MODEL":           "bge-m3",
		"EMBEDDINGS_DIM":             "1024",
		"EMBEDDINGS_SEND_DIMENSION":  "true",
		"EMBEDDINGS_SKIP_TLS_VERIFY": "false",
		"LLM_BASE_URL":               "http://localhost:11434/v1",
		"LLM_API_KEY":                "ollama",
		"LLM_MODEL":                  "qwen2.5:14b",
		"LLM_TEMPERATURE":            "0.1",
		"LLM_SKIP_TLS_VERIFY":        "false",
		"CHUNK_SIZE":                 "900",
		"CHUNK_OVERLAP":              "180",
		"SEARCH_TOP_K":               "8",
		"SEARCH_VECTOR_WEIGHT":       "0.6",
		"SEARCH_KEYWORD_WEIGHT":      "0.4",
		"WORKER_POLL_SECONDS":        "5",
		"SOURCE_PAGE_LIMIT":          "50",
		"GITLAB_MAX_FILE_BYTES":      "1048576",
		"GITLAB_MAX_API_PAGES":       "1000",
		"GITLAB_EXCLUDED_DIRS":       ".git,vendor,node_modules",
		"GITLAB_EXCLUDED_FILES":      ".env,secrets.yml",
		"GITLAB_TEXT_EXTENSIONS":     ".go,.md",
		"OTEL_TRACES_SAMPLER_ARG":    "0.1",
	}
	for key, value := range values {
		t.Setenv(key, value)
	}
}

func validConfig() Config {
	return Config{
		HTTPAddr:    ":8080",
		DatabaseURL: "postgres://rag:rag@localhost:5432/rag?sslmode=disable",
		Observability: ObservabilityConfig{
			Addr:            ":9090",
			ServiceVersion:  "test",
			Environment:     "test",
			TraceSampleRate: 0.1,
		},
		Embeddings: OpenAIConfig{
			BaseURL: "https://embeddings.example/v1",
			Model:   "embed-model",
			Dim:     1024,
		},
		LLM: LLMConfig{
			BaseURL: "http://llm.example/v1",
			Model:   "llm-model",
		},
		Chunk:   ChunkConfig{Size: 900, Overlap: 180},
		Search:  SearchConfig{TopK: 8, VectorWeight: 0.6, KeywordWeight: 0.4},
		Worker:  WorkerConfig{PollInterval: 5 * time.Second},
		Sources: SourcesConfig{PageLimit: 50},
		GitLab:  GitLabConfig{MaxFileBytes: 1048576, MaxPages: 1000},
	}
}

func assertStrings(t *testing.T, name string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %#v, want %#v", name, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s = %#v, want %#v", name, got, want)
		}
	}
}
