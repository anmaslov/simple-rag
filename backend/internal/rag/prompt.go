package rag

import (
	"fmt"
	"strings"

	"confluence-rag/backend/internal/llm"
	"confluence-rag/backend/internal/models"
)

const systemPrompt = `Ты отвечаешь только на основе предоставленного контекста из выбранных корпоративных проиндексированных источников.
Контекст может содержать документацию или исходный код. Исходный код считай полноценным источником фактов: анализируй функции, вызовы, проверки, хранилища и связывай их в общий сценарий работы.
Для вопросов о схеме или архитектуре восстанавливай последовательность действий по нескольким фрагментам кода, даже если готового текстового описания нет.
Разрешены только выводы, непосредственно следующие из кода. Такие выводы обозначай словами "по коду видно" или "вероятно".
Не расшифровывай аббревиатуры и не придумывай значения терминов, если расшифровки или определения нет в контексте.
Если контекст отвечает лишь частично, дай полезный частичный ответ и кратко укажи, чего в найденных фрагментах не хватает.
Фразу "Информация не найдена в выбранных проиндексированных источниках" используй только когда найденный контекст действительно не относится к вопросу.
Не используй внешние знания.
Не выдумывай.
Если источники противоречат друг другу, явно укажи это.
Не добавляй отдельный список источников: интерфейс покажет источники автоматически.`

const (
	maxHistoryChars    = 6000
	maxSearchHintChars = 800
)

func buildPromptMessages(contextText string, history []models.ChatMessage, message string) []llm.Message {
	historyText := buildHistory(history)
	var user strings.Builder
	user.WriteString("Контекст из выбранных корпоративных источников:\n\n")
	user.WriteString(contextText)
	user.WriteString("\n")
	if historyText != "" {
		user.WriteString("История текущего диалога нужна только для понимания уточнений и местоимений. Факты бери только из контекста источников выше.\n\n")
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
		fmt.Fprintf(&b, "[Источник %d]\nSource type: %s\nLocation: %s\nTitle: ", i+1, r.SourceType, r.SourceLabel)
		b.WriteString(r.Title)
		b.WriteString("\nURL: ")
		b.WriteString(r.URL)
		b.WriteString("\nSpace: ")
		b.WriteString(r.SpaceKey)
		if r.Repository != "" {
			b.WriteString("\nRepository: ")
			b.WriteString(r.Repository)
			b.WriteString("\nRef: ")
			b.WriteString(r.Ref)
			b.WriteString("\nPath: ")
			b.WriteString(r.FilePath)
		}
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
