package embeddings

import "fmt"

type ContractErrorKind string

const (
	ContractResponseCount   ContractErrorKind = "response_count"
	ContractMissingIndex    ContractErrorKind = "missing_index"
	ContractIndexOutOfRange ContractErrorKind = "index_out_of_range"
	ContractDuplicateIndex  ContractErrorKind = "duplicate_index"
	ContractEmptyVector     ContractErrorKind = "empty_vector"
	ContractWrongDimension  ContractErrorKind = "wrong_dimension"
)

// ContractError reports an invalid successful response from the embeddings API.
type ContractError struct {
	Kind     ContractErrorKind
	Index    int
	Expected int
	Actual   int
}

func (e *ContractError) Error() string {
	switch e.Kind {
	case ContractResponseCount:
		return fmt.Sprintf("embeddings response contract violation: expected %d items, got %d", e.Expected, e.Actual)
	case ContractMissingIndex:
		return "embeddings response contract violation: response item is missing index"
	case ContractIndexOutOfRange:
		return fmt.Sprintf("embeddings response contract violation: index %d is outside [0,%d)", e.Index, e.Expected)
	case ContractDuplicateIndex:
		return fmt.Sprintf("embeddings response contract violation: duplicate index %d", e.Index)
	case ContractEmptyVector:
		return fmt.Sprintf("embeddings response contract violation: empty vector at index %d", e.Index)
	case ContractWrongDimension:
		return fmt.Sprintf(
			"embeddings response contract violation: vector at index %d has dimension %d, expected %d",
			e.Index,
			e.Actual,
			e.Expected,
		)
	default:
		return "embeddings response contract violation"
	}
}

type openAIEmbeddingResponse struct {
	Data []openAIEmbedding `json:"data"`
}

type openAIEmbeddingRequest struct {
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Dimensions int      `json:"dimensions,omitempty"`
}

type openAIEmbedding struct {
	Embedding []float32 `json:"embedding"`
	Index     *int      `json:"index"`
}

func (e *OpenAIEmbedder) validateResponse(response openAIEmbeddingResponse, expectedCount int) ([][]float32, error) {
	if len(response.Data) != expectedCount {
		return nil, &ContractError{
			Kind:     ContractResponseCount,
			Expected: expectedCount,
			Actual:   len(response.Data),
		}
	}

	vecs := make([][]float32, expectedCount)
	seen := make([]bool, expectedCount)
	for _, item := range response.Data {
		if item.Index == nil {
			return nil, &ContractError{Kind: ContractMissingIndex}
		}
		index := *item.Index
		if index < 0 || index >= expectedCount {
			return nil, &ContractError{
				Kind:     ContractIndexOutOfRange,
				Index:    index,
				Expected: expectedCount,
			}
		}
		if seen[index] {
			return nil, &ContractError{Kind: ContractDuplicateIndex, Index: index}
		}
		if len(item.Embedding) == 0 {
			return nil, &ContractError{Kind: ContractEmptyVector, Index: index}
		}
		if e.expectedDimension > 0 && len(item.Embedding) != e.expectedDimension {
			return nil, &ContractError{
				Kind:     ContractWrongDimension,
				Index:    index,
				Expected: e.expectedDimension,
				Actual:   len(item.Embedding),
			}
		}

		seen[index] = true
		vecs[index] = item.Embedding
	}
	return vecs, nil
}
