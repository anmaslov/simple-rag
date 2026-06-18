package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr    string
	DatabaseURL string
	Confluence  ConfluenceConfig
	Embeddings  OpenAIConfig
	LLM         LLMConfig
	Chunk       ChunkConfig
	Search      SearchConfig
	Worker      WorkerConfig
	GitLab      GitLabConfig
}

type ConfluenceConfig struct {
	BaseURL       string
	Token         string
	AuthType      string
	Username      string
	RootPageIDs   []string
	SpaceKeys     []string
	SkipTLSVerify bool
	PageLimit     int
}

type OpenAIConfig struct {
	BaseURL       string
	APIKey        string
	Model         string
	Dim           int
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

func Load() Config {
	return Config{
		HTTPAddr:    env("HTTP_ADDR", ":8080"),
		DatabaseURL: env("DATABASE_URL", "postgres://rag:rag@localhost:5432/rag?sslmode=disable"),
		Confluence: ConfluenceConfig{
			BaseURL:       strings.TrimRight(env("CONFLUENCE_BASE_URL", ""), "/"),
			Token:         env("CONFLUENCE_TOKEN", ""),
			AuthType:      env("CONFLUENCE_AUTH_TYPE", "bearer"),
			Username:      env("CONFLUENCE_USERNAME", ""),
			RootPageIDs:   csv(env("CONFLUENCE_ROOT_PAGE_IDS", "")),
			SpaceKeys:     csv(env("CONFLUENCE_SPACE_KEYS", "")),
			SkipTLSVerify: envBool("CONFLUENCE_SKIP_TLS_VERIFY", false),
			PageLimit:     envInt("CONFLUENCE_PAGE_LIMIT", 50),
		},
		Embeddings: OpenAIConfig{
			BaseURL:       strings.TrimRight(env("EMBEDDINGS_BASE_URL", "http://localhost:11434/v1"), "/"),
			APIKey:        env("EMBEDDINGS_API_KEY", "ollama"),
			Model:         env("EMBEDDINGS_MODEL", "bge-m3"),
			Dim:           envInt("EMBEDDINGS_DIM", 1024),
			SkipTLSVerify: envBool("EMBEDDINGS_SKIP_TLS_VERIFY", false),
		},
		LLM: LLMConfig{
			BaseURL:       strings.TrimRight(env("LLM_BASE_URL", "http://localhost:11434/v1"), "/"),
			APIKey:        env("LLM_API_KEY", "ollama"),
			Model:         env("LLM_MODEL", "qwen2.5:14b"),
			Temperature:   envFloat("LLM_TEMPERATURE", 0.1),
			SkipTLSVerify: envBool("LLM_SKIP_TLS_VERIFY", false),
		},
		Chunk:  ChunkConfig{Size: envInt("CHUNK_SIZE", 900), Overlap: envInt("CHUNK_OVERLAP", 180)},
		Search: SearchConfig{TopK: envInt("SEARCH_TOP_K", 8), VectorWeight: envFloat("SEARCH_VECTOR_WEIGHT", 0.6), KeywordWeight: envFloat("SEARCH_KEYWORD_WEIGHT", 0.4)},
		Worker: WorkerConfig{PollInterval: time.Duration(envInt("WORKER_POLL_SECONDS", 5)) * time.Second},
		GitLab: GitLabConfig{
			MaxFileBytes:      int64(envInt("GITLAB_MAX_FILE_BYTES", 1048576)),
			MaxPages:          envInt("GITLAB_MAX_API_PAGES", 1000),
			ExcludedDirs:      csv(env("GITLAB_EXCLUDED_DIRS", ".git,vendor,node_modules,dist,build,target,.next,.nuxt,coverage")),
			ExcludedFiles:     csv(env("GITLAB_EXCLUDED_FILES", ".env,.env.*,id_rsa,id_ed25519,credentials.json,secrets.yml,*.pem,*.key,*.p12,*.jks")),
			AllowedExtensions: csv(env("GITLAB_TEXT_EXTENSIONS", ".go,.py,.js,.jsx,.ts,.tsx,.vue,.java,.kt,.kts,.rb,.php,.cs,.c,.h,.cpp,.hpp,.rs,.swift,.scala,.sh,.bash,.zsh,.sql,.md,.txt,.rst,.adoc,.yaml,.yml,.json,.toml,.xml,.html,.css,.scss,.less,.proto,.graphql,.tf,.hcl,.gradle,.properties,.ini,.conf,.dockerfile")),
		},
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v, err := strconv.Atoi(env(key, ""))
	if err != nil {
		return fallback
	}
	return v
}

func envFloat(key string, fallback float64) float64 {
	v, err := strconv.ParseFloat(env(key, ""), 64)
	if err != nil {
		return fallback
	}
	return v
}

func envBool(key string, fallback bool) bool {
	v, err := strconv.ParseBool(env(key, ""))
	if err != nil {
		return fallback
	}
	return v
}

func csv(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}
