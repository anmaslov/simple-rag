package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func Load() Config {
	cfg, _ := load(false)
	return cfg
}

func LoadValidated() (Config, error) {
	cfg, err := load(true)
	if err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func load(strict bool) (Config, error) {
	embeddingsDim, err := loadInt("EMBEDDINGS_DIM", 1024, strict)
	if err != nil {
		return Config{}, err
	}
	embeddingsSkipTLSVerify, err := loadBool("EMBEDDINGS_SKIP_TLS_VERIFY", false, strict)
	if err != nil {
		return Config{}, err
	}
	embeddingsSendDimension, err := loadBool("EMBEDDINGS_SEND_DIMENSION", true, strict)
	if err != nil {
		return Config{}, err
	}
	llmTemperature, err := loadFloat("LLM_TEMPERATURE", 0.1, strict)
	if err != nil {
		return Config{}, err
	}
	llmSkipTLSVerify, err := loadBool("LLM_SKIP_TLS_VERIFY", false, strict)
	if err != nil {
		return Config{}, err
	}
	chunkSize, err := loadInt("CHUNK_SIZE", 900, strict)
	if err != nil {
		return Config{}, err
	}
	chunkOverlap, err := loadInt("CHUNK_OVERLAP", 180, strict)
	if err != nil {
		return Config{}, err
	}
	searchTopK, err := loadInt("SEARCH_TOP_K", 8, strict)
	if err != nil {
		return Config{}, err
	}
	searchVectorWeight, err := loadFloat("SEARCH_VECTOR_WEIGHT", 0.6, strict)
	if err != nil {
		return Config{}, err
	}
	searchKeywordWeight, err := loadFloat("SEARCH_KEYWORD_WEIGHT", 0.4, strict)
	if err != nil {
		return Config{}, err
	}
	workerPollSeconds, err := loadInt("WORKER_POLL_SECONDS", 5, strict)
	if err != nil {
		return Config{}, err
	}
	sourcePageLimit, err := loadInt("SOURCE_PAGE_LIMIT", 50, strict)
	if err != nil {
		return Config{}, err
	}
	gitLabMaxFileBytes, err := loadInt64("GITLAB_MAX_FILE_BYTES", 1048576, strict)
	if err != nil {
		return Config{}, err
	}
	gitLabMaxPages, err := loadInt("GITLAB_MAX_API_PAGES", 1000, strict)
	if err != nil {
		return Config{}, err
	}
	traceSampleRate, err := loadFloat("OTEL_TRACES_SAMPLER_ARG", 0.1, strict)
	if err != nil {
		return Config{}, err
	}

	return Config{
		HTTPAddr:    loadString("HTTP_ADDR", ":8080", strict),
		DatabaseURL: loadString("DATABASE_URL", "postgres://rag:rag@localhost:5432/rag?sslmode=disable", strict),
		Observability: ObservabilityConfig{
			Addr:            loadString("OBSERVABILITY_ADDR", ":9090", strict),
			ServiceName:     loadString("OTEL_SERVICE_NAME", "", strict),
			ServiceVersion:  loadString("APP_VERSION", "dev", strict),
			Environment:     loadString("DEPLOYMENT_ENVIRONMENT", "development", strict),
			OTLPEndpoint:    firstEnv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "OTEL_EXPORTER_OTLP_ENDPOINT"),
			TraceSampleRate: traceSampleRate,
		},
		Embeddings: OpenAIConfig{
			BaseURL:       strings.TrimRight(loadString("EMBEDDINGS_BASE_URL", "http://localhost:11434/v1", strict), "/"),
			APIKey:        loadString("EMBEDDINGS_API_KEY", "ollama", strict),
			Model:         loadString("EMBEDDINGS_MODEL", "bge-m3", strict),
			Dim:           embeddingsDim,
			SendDimension: embeddingsSendDimension,
			SkipTLSVerify: embeddingsSkipTLSVerify,
		},
		LLM: LLMConfig{
			BaseURL:       strings.TrimRight(loadString("LLM_BASE_URL", "http://localhost:11434/v1", strict), "/"),
			APIKey:        loadString("LLM_API_KEY", "ollama", strict),
			Model:         loadString("LLM_MODEL", "qwen2.5:14b", strict),
			Temperature:   llmTemperature,
			SkipTLSVerify: llmSkipTLSVerify,
		},
		Chunk:   ChunkConfig{Size: chunkSize, Overlap: chunkOverlap},
		Search:  SearchConfig{TopK: searchTopK, VectorWeight: searchVectorWeight, KeywordWeight: searchKeywordWeight},
		Worker:  WorkerConfig{PollInterval: time.Duration(workerPollSeconds) * time.Second},
		Sources: SourcesConfig{PageLimit: sourcePageLimit},
		GitLab: GitLabConfig{
			MaxFileBytes:      gitLabMaxFileBytes,
			MaxPages:          gitLabMaxPages,
			ExcludedDirs:      csv(loadString("GITLAB_EXCLUDED_DIRS", ".git,vendor,node_modules,dist,build,target,.next,.nuxt,coverage", strict)),
			ExcludedFiles:     csv(loadString("GITLAB_EXCLUDED_FILES", ".env,.env.*,id_rsa,id_ed25519,credentials.json,secrets.yml,*.pem,*.key,*.p12,*.jks", strict)),
			AllowedExtensions: csv(loadString("GITLAB_TEXT_EXTENSIONS", ".go,.py,.js,.jsx,.ts,.tsx,.vue,.java,.kt,.kts,.rb,.php,.cs,.c,.h,.cpp,.hpp,.rs,.swift,.scala,.sh,.bash,.zsh,.sql,.md,.txt,.rst,.adoc,.yaml,.yml,.json,.toml,.xml,.html,.css,.scss,.less,.proto,.graphql,.tf,.hcl,.gradle,.properties,.ini,.conf,.dockerfile", strict)),
		},
	}, nil
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func loadString(key, fallback string, strict bool) string {
	if strict {
		if v, ok := os.LookupEnv(key); ok {
			return v
		}
		return fallback
	}
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func loadInt(key string, fallback int, strict bool) (int, error) {
	raw, ok := os.LookupEnv(key)
	if !ok || (!strict && raw == "") {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		if !strict {
			return fallback, nil
		}
		return 0, fmt.Errorf("%s: parse integer %q: %w", key, raw, err)
	}
	return value, nil
}

func loadInt64(key string, fallback int64, strict bool) (int64, error) {
	raw, ok := os.LookupEnv(key)
	if !ok || (!strict && raw == "") {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		if !strict {
			return fallback, nil
		}
		return 0, fmt.Errorf("%s: parse integer %q: %w", key, raw, err)
	}
	return value, nil
}

func loadFloat(key string, fallback float64, strict bool) (float64, error) {
	raw, ok := os.LookupEnv(key)
	if !ok || (!strict && raw == "") {
		return fallback, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		if !strict {
			return fallback, nil
		}
		return 0, fmt.Errorf("%s: parse float %q: %w", key, raw, err)
	}
	return value, nil
}

func loadBool(key string, fallback bool, strict bool) (bool, error) {
	raw, ok := os.LookupEnv(key)
	if !ok || (!strict && raw == "") {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		if !strict {
			return fallback, nil
		}
		return false, fmt.Errorf("%s: parse boolean %q: %w", key, raw, err)
	}
	return value, nil
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
