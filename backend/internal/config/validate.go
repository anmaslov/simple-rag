package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

func (c Config) Validate() error {
	var errs []error

	required(&errs, "HTTP_ADDR", c.HTTPAddr)
	required(&errs, "DATABASE_URL", c.DatabaseURL)
	required(&errs, "EMBEDDINGS_BASE_URL", c.Embeddings.BaseURL)
	required(&errs, "EMBEDDINGS_MODEL", c.Embeddings.Model)
	required(&errs, "LLM_BASE_URL", c.LLM.BaseURL)
	required(&errs, "LLM_MODEL", c.LLM.Model)

	validateHTTPURL(&errs, "EMBEDDINGS_BASE_URL", c.Embeddings.BaseURL)
	validateHTTPURL(&errs, "LLM_BASE_URL", c.LLM.BaseURL)

	if c.Worker.PollInterval <= 0 {
		errs = append(errs, errors.New("WORKER_POLL_SECONDS must be greater than 0"))
	}
	if c.Chunk.Size <= 0 {
		errs = append(errs, errors.New("CHUNK_SIZE must be greater than 0"))
	}
	if c.Chunk.Overlap < 0 {
		errs = append(errs, errors.New("CHUNK_OVERLAP must be greater than or equal to 0"))
	} else if c.Chunk.Overlap >= c.Chunk.Size {
		errs = append(errs, errors.New("CHUNK_OVERLAP must be less than CHUNK_SIZE"))
	}
	if c.Search.TopK <= 0 {
		errs = append(errs, errors.New("SEARCH_TOP_K must be greater than 0"))
	}
	if !(c.Search.VectorWeight >= 0) {
		errs = append(errs, errors.New("SEARCH_VECTOR_WEIGHT must be greater than or equal to 0"))
	}
	if !(c.Search.KeywordWeight >= 0) {
		errs = append(errs, errors.New("SEARCH_KEYWORD_WEIGHT must be greater than or equal to 0"))
	}
	if !(c.Search.VectorWeight+c.Search.KeywordWeight > 0) {
		errs = append(errs, errors.New("search weights sum must be greater than 0"))
	}
	if c.Sources.PageLimit <= 0 {
		errs = append(errs, errors.New("SOURCE_PAGE_LIMIT must be greater than 0"))
	}
	if c.GitLab.MaxFileBytes <= 0 {
		errs = append(errs, errors.New("GITLAB_MAX_FILE_BYTES must be greater than 0"))
	}
	if c.GitLab.MaxPages <= 0 {
		errs = append(errs, errors.New("GITLAB_MAX_API_PAGES must be greater than 0"))
	}
	if c.Embeddings.Dim <= 0 {
		errs = append(errs, errors.New("EMBEDDINGS_DIM must be greater than 0"))
	}

	return errors.Join(errs...)
}

func required(errs *[]error, name, value string) {
	if strings.TrimSpace(value) == "" {
		*errs = append(*errs, fmt.Errorf("%s is required", name))
	}
}

func validateHTTPURL(errs *[]error, name, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed == nil {
		*errs = append(*errs, fmt.Errorf("%s must be a valid HTTP(S) URL", name))
		return
	}
	isHTTP := strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https")
	if !isHTTP || parsed.Host == "" {
		*errs = append(*errs, fmt.Errorf("%s must be a valid HTTP(S) URL", name))
	}
}
