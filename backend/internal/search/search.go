package search

import (
	"context"
	"sort"
	"strings"
	"unicode"

	"confluence-rag/backend/internal/domain"
	"confluence-rag/backend/internal/embeddings"
	"confluence-rag/backend/internal/models"
)

type Service struct {
	repo          domain.SearchRepository
	embedder      embeddings.Embedder
	vectorWeight  float64
	keywordWeight float64
}

func New(repo domain.SearchRepository, embedder embeddings.Embedder, vectorWeight, keywordWeight float64) *Service {
	return &Service{repo: repo, embedder: embedder, vectorWeight: vectorWeight, keywordWeight: keywordWeight}
}

func (s *Service) Search(ctx context.Context, query string, scope models.SearchScope, topK int) ([]models.SearchResult, error) {
	if topK <= 0 {
		topK = 10
	}
	vecs, err := s.embedder.Embed(ctx, []string{vectorQuery(query, scope)})
	if err != nil {
		return nil, err
	}
	var vector []float32
	if len(vecs) > 0 {
		vector = vecs[0]
	}
	candidateLimit := topK * 4
	vectorResults, err := s.repo.VectorSearch(ctx, vector, scope, candidateLimit)
	if err != nil {
		return nil, err
	}
	var keywordResults []models.SearchResult
	for _, keywordQuery := range keywordQueries(query) {
		items, err := s.repo.KeywordSearch(ctx, keywordQuery, scope, candidateLimit)
		if err != nil {
			return nil, err
		}
		keywordResults = append(keywordResults, items...)
	}
	return Merge(vectorResults, keywordResults, topK, s.vectorWeight, s.keywordWeight), nil
}

func Merge(vectorResults, keywordResults []models.SearchResult, topK int, vectorWeight, keywordWeight float64) []models.SearchResult {
	m := map[int64]*acc{}
	maxV, maxK := maxScore(vectorResults), maxScore(keywordResults)
	for _, r := range vectorResults {
		a := ensure(m, r)
		if maxV > 0 {
			a.vector = r.Score / maxV
		}
	}
	for _, r := range keywordResults {
		a := ensure(m, r)
		if maxK > 0 {
			a.keyword = r.Score / maxK
		}
	}
	out := make([]models.SearchResult, 0, len(m))
	for _, a := range m {
		a.item.Score = (a.vector*vectorWeight + a.keyword*keywordWeight) * sourceQuality(a.item)
		out = append(out, a.item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return diversifyDocuments(out, topK)
}

func ensure(m map[int64]*acc, r models.SearchResult) *acc {
	a, ok := m[r.ChunkID]
	if !ok {
		a = &acc{item: r}
		m[r.ChunkID] = a
	}
	return a
}

type acc struct {
	item            models.SearchResult
	vector, keyword float64
}

func maxScore(items []models.SearchResult) float64 {
	var max float64
	for _, r := range items {
		if r.Score > max {
			max = r.Score
		}
	}
	return max
}

func sourceQuality(item models.SearchResult) float64 {
	if item.SourceType != models.SourceGitLab {
		return 1
	}
	path := strings.ToLower(item.FilePath)
	if path == "" {
		path = strings.ToLower(item.Title)
	}
	switch {
	case strings.Contains(path, "/mock/"), strings.Contains(path, "/mocks/"),
		strings.HasPrefix(path, "mock/"), strings.Contains(path, "generated"):
		return 0.45
	case strings.HasSuffix(path, "_test.go"), strings.Contains(path, "/testdata/"):
		return 0.65
	default:
		return 1
	}
}

func diversifyDocuments(items []models.SearchResult, topK int) []models.SearchResult {
	if topK <= 0 || len(items) <= topK {
		return items
	}
	out := make([]models.SearchResult, 0, topK)
	deferred := make([]models.SearchResult, 0, len(items))
	seen := make(map[int64]struct{}, topK)
	for _, item := range items {
		key := item.DocumentID
		if key == 0 {
			key = -item.ChunkID
		}
		if _, ok := seen[key]; ok {
			deferred = append(deferred, item)
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
		if len(out) == topK {
			return out
		}
	}
	for _, item := range deferred {
		out = append(out, item)
		if len(out) == topK {
			break
		}
	}
	return out
}

func keywordQueries(query string) []string {
	queries := []string{query}
	var aliases []string
	for _, token := range strings.FieldsFunc(query, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		runes := []rune(token)
		if len(runes) < 2 || len(runes) > 10 || !isUpperToken(runes) {
			continue
		}
		if alias := transliterateCyrillic(runes); alias != token {
			aliases = append(aliases, alias)
		}
	}
	if len(aliases) > 0 {
		queries = append(queries, strings.Join(aliases, " "))
	}
	return queries
}

func vectorQuery(query string, scope models.SearchScope) string {
	if !isGitLabOnly(scope) || !asksForCodeFlow(query) {
		return query
	}
	aliases := keywordQueries(query)
	var technicalAliases string
	if len(aliases) > 1 {
		technicalAliases = aliases[len(aliases)-1] + " "
	}
	return query + "\nTechnical code search: " + technicalAliases +
		"implementation architecture call flow create start launch send recipients workflow service handler storage worker"
}

func isGitLabOnly(scope models.SearchScope) bool {
	return len(scope.SourceTypes) == 1 && scope.SourceTypes[0] == models.SourceGitLab
}

func asksForCodeFlow(query string) bool {
	q := strings.ToLower(query)
	for _, marker := range []string{
		"схем", "архитектур", "как работает", "как устроен", "как устроена",
		"workflow", "call flow", "architecture",
	} {
		if strings.Contains(q, marker) {
			return true
		}
	}
	return false
}

func isUpperToken(token []rune) bool {
	hasLetter := false
	for _, r := range token {
		if !unicode.IsLetter(r) {
			continue
		}
		hasLetter = true
		if unicode.IsLower(r) {
			return false
		}
	}
	return hasLetter
}

func transliterateCyrillic(token []rune) string {
	var b strings.Builder
	for _, r := range token {
		if replacement, ok := cyrillicTechnicalAlias[unicode.ToUpper(r)]; ok {
			b.WriteString(replacement)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var cyrillicTechnicalAlias = map[rune]string{
	'А': "A", 'Б': "B", 'В': "V", 'Г': "G", 'Д': "D", 'Е': "E", 'Ё': "E",
	'Ж': "ZH", 'З': "Z", 'И': "I", 'Й': "Y", 'К': "K", 'Л': "L", 'М': "M",
	'Н': "N", 'О': "O", 'П': "P", 'Р': "R", 'С': "S", 'Т': "T", 'У': "U",
	'Ф': "F", 'Х': "H", 'Ц': "TS", 'Ч': "CH", 'Ш': "SH", 'Щ': "SCH",
	'Ъ': "", 'Ы': "Y", 'Ь': "", 'Э': "E", 'Ю': "YU", 'Я': "YA",
}
