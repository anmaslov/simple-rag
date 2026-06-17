package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"confluence-rag/backend/internal/domain"
	"confluence-rag/backend/internal/llm"
	"confluence-rag/backend/internal/models"
	"confluence-rag/backend/internal/search"
)

const systemPrompt = `Ты отвечаешь только на основе предоставленного контекста из корпоративного Confluence.
Если в контексте нет ответа, скажи: "В проиндексированных материалах Confluence я не нашёл ответа".
Не используй внешние знания.
Не выдумывай.
Если источники противоречат друг другу, явно укажи это.
Не добавляй отдельный список источников: интерфейс покажет источники автоматически.`

const (
	maxHistoryMessages = 8
	maxHistoryChars    = 6000
	maxSearchHintChars = 800
)

type Service struct {
	repo        domain.ChatRepository
	search      *search.Service
	llm         llm.LLM
	defaultTopK int
}

func New(repo domain.ChatRepository, search *search.Service, llm llm.LLM, defaultTopK int) *Service {
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

func (s *Service) Chat(ctx context.Context, sessionID, message string, spaces []string, topK int) (ChatResult, error) {
	history, err := s.loadHistory(ctx, sessionID)
	if err != nil {
		return ChatResult{}, err
	}
	results, err := s.search.Search(ctx, buildSearchQuery(history, message), spaces, s.resolveTopK(topK))
	if err != nil {
		return ChatResult{}, err
	}
	sessionID, err = s.repo.EnsureChatSession(ctx, sessionID, message)
	if err != nil {
		return ChatResult{}, err
	}
	if err := s.repo.SaveChatMessage(ctx, sessionID, "user", message, nil); err != nil {
		return ChatResult{}, err
	}
	contextText := buildContext(results)
	resp, err := s.llm.Chat(ctx, llm.ChatRequest{Messages: buildPromptMessages(contextText, history, message)})
	if err != nil {
		return ChatResult{}, err
	}
	if strings.TrimSpace(contextText) == "" {
		resp.Content = "В проиндексированных материалах Confluence я не нашёл ответа"
	}
	srcJSON, err := json.Marshal(results)
	if err != nil {
		return ChatResult{}, err
	}
	if err := s.repo.SaveChatMessage(ctx, sessionID, "assistant", resp.Content, srcJSON); err != nil {
		return ChatResult{}, err
	}
	return ChatResult{SessionID: sessionID, Answer: resp.Content, Sources: results}, nil
}

func (s *Service) ChatStream(ctx context.Context, sessionID, message string, spaces []string, topK int, emit func(ChatStreamEvent) error) error {
	if err := emit(ChatStreamEvent{Type: "status", Message: "Ищу релевантные источники"}); err != nil {
		return err
	}
	history, err := s.loadHistory(ctx, sessionID)
	if err != nil {
		return err
	}
	results, err := s.search.Search(ctx, buildSearchQuery(history, message), spaces, s.resolveTopK(topK))
	if err != nil {
		return err
	}
	if err := emit(ChatStreamEvent{Type: "sources", Message: fmt.Sprintf("Нашел источников: %d", len(results)), Sources: results}); err != nil {
		return err
	}
	sessionID, err = s.repo.EnsureChatSession(ctx, sessionID, message)
	if err != nil {
		return err
	}
	if err := emit(ChatStreamEvent{Type: "session", SessionID: sessionID}); err != nil {
		return err
	}
	if err := s.repo.SaveChatMessage(ctx, sessionID, "user", message, nil); err != nil {
		return err
	}

	contextText := buildContext(results)
	if strings.TrimSpace(contextText) == "" {
		answer := "В проиндексированных материалах Confluence я не нашёл ответа"
		if err := emit(ChatStreamEvent{Type: "delta", Delta: answer}); err != nil {
			return err
		}
		if err := s.repo.SaveChatMessage(ctx, sessionID, "assistant", answer, nil); err != nil {
			return err
		}
		return emit(ChatStreamEvent{Type: "done"})
	}

	if err := emit(ChatStreamEvent{Type: "status", Message: "Готовлю ответ по найденным материалам"}); err != nil {
		return err
	}
	var answer strings.Builder
	err = s.llm.ChatStream(ctx, llm.ChatRequest{Messages: buildPromptMessages(contextText, history, message)}, func(delta string) error {
		answer.WriteString(delta)
		return emit(ChatStreamEvent{Type: "delta", Delta: delta})
	})
	if err != nil {
		return err
	}
	srcJSON, err := json.Marshal(results)
	if err != nil {
		return err
	}
	if err := s.repo.SaveChatMessage(ctx, sessionID, "assistant", answer.String(), srcJSON); err != nil {
		return err
	}
	return emit(ChatStreamEvent{Type: "done"})
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

func buildPromptMessages(contextText string, history []models.ChatMessage, message string) []llm.Message {
	historyText := buildHistory(history)
	var user strings.Builder
	user.WriteString("Контекст Confluence:\n\n")
	user.WriteString(contextText)
	user.WriteString("\n")
	if historyText != "" {
		user.WriteString("История текущего диалога нужна только для понимания уточнений и местоимений. Факты бери из контекста Confluence выше.\n\n")
		user.WriteString(historyText)
		user.WriteString("\n\n")
	}
	user.WriteString("Текущий вопрос: ")
	user.WriteString(message)

	return []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: user.String()},
	}
}

func buildSearchQuery(history []models.ChatMessage, message string) string {
	hint := buildRecentUserHint(history)
	if hint == "" {
		return message
	}
	return truncateRunes(hint+"\n"+message, maxSearchHintChars)
}

func buildRecentUserHint(history []models.ChatMessage) string {
	var items []string
	for i := len(history) - 1; i >= 0 && len(items) < 3; i-- {
		m := history[i]
		if m.Role != "user" {
			continue
		}
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		items = append(items, content)
	}
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
	return strings.Join(items, "\n")
}

func buildHistory(history []models.ChatMessage) string {
	var b strings.Builder
	chars := 0
	for _, m := range history {
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		role := "Пользователь"
		if m.Role == "assistant" {
			role = "Ассистент"
		}
		next := role + ": " + content + "\n"
		nextChars := len([]rune(next))
		if chars+nextChars > maxHistoryChars {
			break
		}
		b.WriteString(next)
		chars += nextChars
	}
	return strings.TrimSpace(b.String())
}

func buildContext(results []models.SearchResult) string {
	var b strings.Builder
	for i, r := range results {
		fmt.Fprintf(&b, "[Источник %d]\nTitle: ", i+1)
		b.WriteString(r.Title)
		b.WriteString("\nURL: ")
		b.WriteString(r.URL)
		b.WriteString("\nSpace: ")
		b.WriteString(r.SpaceKey)
		b.WriteString("\nContent:\n")
		b.WriteString(r.Chunk)
		b.WriteString("\n\n")
	}
	return b.String()
}

func truncateRunes(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[len(r)-limit:])
}
