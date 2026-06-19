package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"confluence-rag/backend/internal/domain"
	"confluence-rag/backend/internal/llm"
	"confluence-rag/backend/internal/models"
)

const emptyContextAnswer = "Информация не найдена в выбранных проиндексированных источниках"

const maxHistoryMessages = 8

type Searcher interface {
	Search(ctx context.Context, query string, scope models.SearchScope, topK int) ([]models.SearchResult, error)
}

type Service struct {
	repo        domain.ChatRepository
	search      Searcher
	llm         llm.LLM
	defaultTopK int
}

func New(repo domain.ChatRepository, search Searcher, llm llm.LLM, defaultTopK int) *Service {
	return &Service{repo: repo, search: search, llm: llm, defaultTopK: defaultTopK}
}

type ChatResult struct {
	SessionID string                `json:"session_id"`
	Answer    string                `json:"answer"`
	Sources   []models.SearchResult `json:"sources"`
}

type ChatStreamEvent struct {
	Type      string                `json:"type"`
	Message   string                `json:"message,omitempty"`
	SessionID string                `json:"session_id,omitempty"`
	Delta     string                `json:"delta,omitempty"`
	Sources   []models.SearchResult `json:"sources,omitempty"`
}

func (s *Service) Chat(ctx context.Context, sessionID, message string, scope models.SearchScope, topK int) (ChatResult, error) {
	prepared, err := s.prepare(ctx, sessionID, message, scope, topK, preparationHooks{})
	if err != nil {
		return ChatResult{}, err
	}

	answer := emptyContextAnswer
	if prepared.hasContext() {
		resp, err := s.llm.Chat(ctx, prepared.request())
		if err != nil {
			return ChatResult{}, err
		}
		answer = resp.Content
	}
	if err := s.saveAssistant(ctx, prepared.sessionID, answer, prepared.results); err != nil {
		return ChatResult{}, err
	}
	return ChatResult{SessionID: prepared.sessionID, Answer: answer, Sources: prepared.results}, nil
}

func (s *Service) ChatStream(ctx context.Context, sessionID, message string, scope models.SearchScope, topK int, emit func(ChatStreamEvent) error) error {
	if err := emit(ChatStreamEvent{Type: "status", Message: "Ищу релевантные источники"}); err != nil {
		return err
	}

	prepared, err := s.prepare(ctx, sessionID, message, scope, topK, preparationHooks{
		onSources: func(results []models.SearchResult) error {
			return emit(ChatStreamEvent{
				Type:    "sources",
				Message: fmt.Sprintf("Нашел источников: %d", len(results)),
				Sources: results,
			})
		},
		onSession: func(sessionID string) error {
			return emit(ChatStreamEvent{Type: "session", SessionID: sessionID})
		},
	})
	if err != nil {
		return err
	}

	if !prepared.hasContext() {
		answer := emptyContextAnswer
		if err := emit(ChatStreamEvent{Type: "delta", Delta: answer}); err != nil {
			return err
		}
		if err := s.saveAssistant(ctx, prepared.sessionID, answer, prepared.results); err != nil {
			return err
		}
		return emit(ChatStreamEvent{Type: "done"})
	}

	if err := emit(ChatStreamEvent{Type: "status", Message: "Готовлю ответ по найденным материалам"}); err != nil {
		return err
	}
	var answer strings.Builder
	err = s.llm.ChatStream(ctx, prepared.request(), func(delta string) error {
		answer.WriteString(delta)
		return emit(ChatStreamEvent{Type: "delta", Delta: delta})
	})
	if err != nil {
		return err
	}
	if err := s.saveAssistant(ctx, prepared.sessionID, answer.String(), prepared.results); err != nil {
		return err
	}
	return emit(ChatStreamEvent{Type: "done"})
}

type preparedChat struct {
	sessionID   string
	message     string
	history     []models.ChatMessage
	results     []models.SearchResult
	contextText string
}

func (p preparedChat) hasContext() bool {
	return strings.TrimSpace(p.contextText) != ""
}

func (p preparedChat) request() llm.ChatRequest {
	return llm.ChatRequest{Messages: buildPromptMessages(p.contextText, p.history, p.message)}
}

type preparationHooks struct {
	onSources func([]models.SearchResult) error
	onSession func(string) error
}

func (s *Service) prepare(
	ctx context.Context,
	sessionID, message string,
	scope models.SearchScope,
	topK int,
	hooks preparationHooks,
) (preparedChat, error) {
	history, err := s.loadHistory(ctx, sessionID)
	if err != nil {
		return preparedChat{}, err
	}
	results, err := s.search.Search(ctx, buildSearchQuery(history, message), scope, s.resolveTopK(topK))
	if err != nil {
		return preparedChat{}, err
	}
	if hooks.onSources != nil {
		if err := hooks.onSources(results); err != nil {
			return preparedChat{}, err
		}
	}
	sessionID, err = s.repo.EnsureChatSession(ctx, sessionID, message)
	if err != nil {
		return preparedChat{}, err
	}
	if hooks.onSession != nil {
		if err := hooks.onSession(sessionID); err != nil {
			return preparedChat{}, err
		}
	}
	if err := s.repo.SaveChatMessage(ctx, sessionID, "user", message, nil); err != nil {
		return preparedChat{}, err
	}
	return preparedChat{
		sessionID:   sessionID,
		message:     message,
		history:     history,
		results:     results,
		contextText: buildContext(results),
	}, nil
}

func (s *Service) saveAssistant(ctx context.Context, sessionID, answer string, results []models.SearchResult) error {
	srcJSON, err := json.Marshal(results)
	if err != nil {
		return err
	}
	return s.repo.SaveChatMessage(ctx, sessionID, "assistant", answer, srcJSON)
}

func (s *Service) loadHistory(ctx context.Context, sessionID string) ([]models.ChatMessage, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, nil
	}
	messages, err := s.repo.ListChatMessages(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if len(messages) > maxHistoryMessages {
		messages = messages[len(messages)-maxHistoryMessages:]
	}
	return messages, nil
}

func (s *Service) resolveTopK(topK int) int {
	if topK > 0 {
		return topK
	}
	if s.defaultTopK > 0 {
		return s.defaultTopK
	}
	return 10
}
