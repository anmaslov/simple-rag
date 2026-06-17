package rag

import (
	"strings"
	"testing"

	"confluence-rag/backend/internal/models"
)

func TestBuildSearchQueryUsesRecentUserHistory(t *testing.T) {
	history := []models.ChatMessage{
		{Role: "user", Content: "Как оформить отпуск?"},
		{Role: "assistant", Content: "Через портал отпусков."},
		{Role: "user", Content: "А кто согласует заявку?"},
	}

	got := buildSearchQuery(history, "А уведомления кому уходят?")

	for _, want := range []string{"Как оформить отпуск?", "А кто согласует заявку?", "А уведомления кому уходят?"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected query to contain %q, got %q", want, got)
		}
	}
}

func TestBuildPromptMessagesIncludesHistoryAndCurrentQuestion(t *testing.T) {
	history := []models.ChatMessage{
		{Role: "user", Content: "Расскажи про отпуск"},
		{Role: "assistant", Content: "Нашел процесс оформления."},
	}

	got := buildPromptMessages("Confluence chunk", history, "А как отменить?")
	if len(got) != 2 {
		t.Fatalf("expected system and user messages, got %d", len(got))
	}

	userPrompt := got[1].Content
	for _, want := range []string{"Confluence chunk", "История текущего диалога", "Пользователь: Расскажи про отпуск", "Ассистент: Нашел процесс оформления.", "Текущий вопрос: А как отменить?"} {
		if !strings.Contains(userPrompt, want) {
			t.Fatalf("expected prompt to contain %q, got %q", want, userPrompt)
		}
	}
}
