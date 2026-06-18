package search

import (
	"context"
	"sort"

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
	vecs, err := s.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, err
	}
	var vector []float32
	if len(vecs) > 0 {
		vector = vecs[0]
	}
	vectorResults, err := s.repo.VectorSearch(ctx, vector, scope, topK*2)
	if err != nil {
		return nil, err
	}
	keywordResults, err := s.repo.KeywordSearch(ctx, query, scope, topK*2)
	if err != nil {
		return nil, err
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
		a.item.Score = a.vector*vectorWeight + a.keyword*keywordWeight
		out = append(out, a.item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > topK {
		out = out[:topK]
	}
	return out
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
