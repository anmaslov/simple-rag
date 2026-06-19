package rag

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"confluence-rag/backend/internal/llm"
	"confluence-rag/backend/internal/models"
)

func TestChatNormal(t *testing.T) {
	ctx := context.Background()
	results := []models.SearchResult{{
		DocumentID:  10,
		SourceType:  "confluence",
		SourceLabel: "Wiki / HR",
		Title:       "Vacation",
		URL:         "https://wiki/vacation",
		ChunkID:     20,
		Chunk:       "Заявку согласует руководитель.",
	}}
	repo := &chatRepositoryStub{
		history:  []models.ChatMessage{{Role: "user", Content: "Как оформить отпуск?"}},
		ensureID: "session-1",
	}
	searcher := &searcherStub{results: results}
	model := &llmStub{chatResponse: "Заявку согласует руководитель."}
	service := New(repo, searcher, model, 7)
	scope := models.SearchScope{SourceTypes: []string{"confluence"}}

	got, err := service.Chat(ctx, "existing-session", "А кто согласует?", scope, 0)
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}

	want := ChatResult{SessionID: "session-1", Answer: model.chatResponse, Sources: results}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Chat result mismatch:\nwant: %#v\ngot:  %#v", want, got)
	}
	if searcher.calls != 1 || searcher.topK != 7 || !reflect.DeepEqual(searcher.scope, scope) {
		t.Fatalf("unexpected search call: calls=%d topK=%d scope=%#v", searcher.calls, searcher.topK, searcher.scope)
	}
	for _, part := range []string{"Как оформить отпуск?", "А кто согласует?"} {
		if !strings.Contains(searcher.query, part) {
			t.Fatalf("search query must contain %q, got %q", part, searcher.query)
		}
	}
	if model.chatCalls != 1 || model.streamCalls != 0 {
		t.Fatalf("unexpected LLM calls: chat=%d stream=%d", model.chatCalls, model.streamCalls)
	}
	assertSavedConversation(t, repo.saves, "session-1", "А кто согласует?", model.chatResponse, results)
}

func TestChatEmptyContextDoesNotCallLLM(t *testing.T) {
	results := []models.SearchResult{}
	repo := &chatRepositoryStub{ensureID: "session-empty"}
	searcher := &searcherStub{results: results}
	model := &llmStub{chatErr: errors.New("must not be called"), streamErr: errors.New("must not be called")}
	service := New(repo, searcher, model, 5)

	got, err := service.Chat(context.Background(), "", "Неизвестный вопрос", models.SearchScope{}, 3)
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if got.Answer != emptyContextAnswer || got.SessionID != "session-empty" {
		t.Fatalf("unexpected result: %#v", got)
	}
	if !reflect.DeepEqual(got.Sources, results) {
		t.Fatalf("sources mismatch: want %#v, got %#v", results, got.Sources)
	}
	if model.chatCalls != 0 || model.streamCalls != 0 {
		t.Fatalf("LLM must not be called: chat=%d stream=%d", model.chatCalls, model.streamCalls)
	}
	assertSavedConversation(t, repo.saves, "session-empty", "Неизвестный вопрос", emptyContextAnswer, results)
}

func TestChatStreamNormal(t *testing.T) {
	results := []models.SearchResult{{
		DocumentID:  1,
		SourceType:  "gitlab",
		SourceLabel: "GitLab / team/app",
		Title:       "README.md",
		URL:         "https://gitlab/team/app",
		ChunkID:     2,
		Chunk:       "Запуск выполняется командой make run.",
	}}
	repo := &chatRepositoryStub{ensureID: "stream-session"}
	searcher := &searcherStub{results: results}
	model := &llmStub{streamDeltas: []string{"Используйте ", "`make run`."}}
	service := New(repo, searcher, model, 10)
	var events []ChatStreamEvent

	err := service.ChatStream(
		context.Background(),
		"",
		"Как запустить?",
		models.SearchScope{},
		4,
		func(event ChatStreamEvent) error {
			events = append(events, event)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ChatStream returned error: %v", err)
	}

	assertEventTypes(t, events, "status", "sources", "session", "status", "delta", "delta", "done")
	if events[1].Message != "Нашел источников: 1" || !reflect.DeepEqual(events[1].Sources, results) {
		t.Fatalf("unexpected sources event: %#v", events[1])
	}
	if events[2].SessionID != "stream-session" {
		t.Fatalf("unexpected session event: %#v", events[2])
	}
	if model.chatCalls != 0 || model.streamCalls != 1 {
		t.Fatalf("unexpected LLM calls: chat=%d stream=%d", model.chatCalls, model.streamCalls)
	}
	assertSavedConversation(t, repo.saves, "stream-session", "Как запустить?", "Используйте `make run`.", results)
}

func TestChatStreamEmptyContextDoesNotCallLLM(t *testing.T) {
	results := []models.SearchResult{}
	repo := &chatRepositoryStub{ensureID: "stream-empty"}
	searcher := &searcherStub{results: results}
	model := &llmStub{chatErr: errors.New("must not be called"), streamErr: errors.New("must not be called")}
	service := New(repo, searcher, model, 10)
	var events []ChatStreamEvent

	err := service.ChatStream(
		context.Background(),
		"",
		"Нет ответа",
		models.SearchScope{},
		0,
		func(event ChatStreamEvent) error {
			events = append(events, event)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ChatStream returned error: %v", err)
	}

	assertEventTypes(t, events, "status", "sources", "session", "delta", "done")
	if events[3].Delta != emptyContextAnswer {
		t.Fatalf("unexpected empty-context delta: %#v", events[3])
	}
	if model.chatCalls != 0 || model.streamCalls != 0 {
		t.Fatalf("LLM must not be called: chat=%d stream=%d", model.chatCalls, model.streamCalls)
	}
	assertSavedConversation(t, repo.saves, "stream-empty", "Нет ответа", emptyContextAnswer, results)
}

func TestChatErrors(t *testing.T) {
	errHistory := errors.New("history failed")
	errSearch := errors.New("search failed")
	errSession := errors.New("session failed")
	errSaveUser := errors.New("save user failed")
	errLLM := errors.New("llm failed")
	errSaveAssistant := errors.New("save assistant failed")
	contextResult := []models.SearchResult{{ChunkID: 1, Chunk: "context"}}

	tests := []struct {
		name            string
		sessionID       string
		configure       func(*chatRepositoryStub, *searcherStub, *llmStub)
		wantErr         error
		wantSearchCalls int
		wantEnsureCalls int
		wantSaveRoles   []string
		wantLLMCalls    int
	}{
		{
			name:      "history",
			sessionID: "existing",
			configure: func(repo *chatRepositoryStub, _ *searcherStub, _ *llmStub) {
				repo.historyErr = errHistory
			},
			wantErr: errHistory,
		},
		{
			name: "search",
			configure: func(_ *chatRepositoryStub, searcher *searcherStub, _ *llmStub) {
				searcher.err = errSearch
			},
			wantErr:         errSearch,
			wantSearchCalls: 1,
		},
		{
			name: "session",
			configure: func(repo *chatRepositoryStub, _ *searcherStub, _ *llmStub) {
				repo.ensureErr = errSession
			},
			wantErr:         errSession,
			wantSearchCalls: 1,
			wantEnsureCalls: 1,
		},
		{
			name: "save user",
			configure: func(repo *chatRepositoryStub, _ *searcherStub, _ *llmStub) {
				repo.saveErrors = map[string]error{"user": errSaveUser}
			},
			wantErr:         errSaveUser,
			wantSearchCalls: 1,
			wantEnsureCalls: 1,
			wantSaveRoles:   []string{"user"},
		},
		{
			name: "llm",
			configure: func(_ *chatRepositoryStub, _ *searcherStub, model *llmStub) {
				model.chatErr = errLLM
			},
			wantErr:         errLLM,
			wantSearchCalls: 1,
			wantEnsureCalls: 1,
			wantSaveRoles:   []string{"user"},
			wantLLMCalls:    1,
		},
		{
			name: "save assistant",
			configure: func(repo *chatRepositoryStub, _ *searcherStub, _ *llmStub) {
				repo.saveErrors = map[string]error{"assistant": errSaveAssistant}
			},
			wantErr:         errSaveAssistant,
			wantSearchCalls: 1,
			wantEnsureCalls: 1,
			wantSaveRoles:   []string{"user", "assistant"},
			wantLLMCalls:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &chatRepositoryStub{ensureID: "session"}
			searcher := &searcherStub{results: contextResult}
			model := &llmStub{chatResponse: "answer"}
			tt.configure(repo, searcher, model)

			_, err := New(repo, searcher, model, 10).Chat(
				context.Background(),
				tt.sessionID,
				"question",
				models.SearchScope{},
				0,
			)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("want error %v, got %v", tt.wantErr, err)
			}
			if searcher.calls != tt.wantSearchCalls {
				t.Fatalf("search calls: want %d, got %d", tt.wantSearchCalls, searcher.calls)
			}
			if repo.ensureCalls != tt.wantEnsureCalls {
				t.Fatalf("ensure calls: want %d, got %d", tt.wantEnsureCalls, repo.ensureCalls)
			}
			if got := savedRoles(repo.saves); !reflect.DeepEqual(got, tt.wantSaveRoles) {
				t.Fatalf("save roles: want %#v, got %#v", tt.wantSaveRoles, got)
			}
			if model.chatCalls != tt.wantLLMCalls {
				t.Fatalf("LLM calls: want %d, got %d", tt.wantLLMCalls, model.chatCalls)
			}
		})
	}
}

func TestChatStreamErrors(t *testing.T) {
	errSearch := errors.New("search failed")
	errSession := errors.New("session failed")
	errSave := errors.New("save failed")
	errLLM := errors.New("llm failed")
	contextResult := []models.SearchResult{{ChunkID: 1, Chunk: "context"}}

	tests := []struct {
		name          string
		configure     func(*chatRepositoryStub, *searcherStub, *llmStub)
		wantErr       error
		wantEvents    []string
		wantSaveRoles []string
		wantLLMCalls  int
	}{
		{
			name: "search",
			configure: func(_ *chatRepositoryStub, searcher *searcherStub, _ *llmStub) {
				searcher.err = errSearch
			},
			wantErr:    errSearch,
			wantEvents: []string{"status"},
		},
		{
			name: "session",
			configure: func(repo *chatRepositoryStub, _ *searcherStub, _ *llmStub) {
				repo.ensureErr = errSession
			},
			wantErr:    errSession,
			wantEvents: []string{"status", "sources"},
		},
		{
			name: "save user",
			configure: func(repo *chatRepositoryStub, _ *searcherStub, _ *llmStub) {
				repo.saveErrors = map[string]error{"user": errSave}
			},
			wantErr:       errSave,
			wantEvents:    []string{"status", "sources", "session"},
			wantSaveRoles: []string{"user"},
		},
		{
			name: "llm",
			configure: func(_ *chatRepositoryStub, _ *searcherStub, model *llmStub) {
				model.streamErr = errLLM
			},
			wantErr:       errLLM,
			wantEvents:    []string{"status", "sources", "session", "status"},
			wantSaveRoles: []string{"user"},
			wantLLMCalls:  1,
		},
		{
			name: "save assistant",
			configure: func(repo *chatRepositoryStub, _ *searcherStub, _ *llmStub) {
				repo.saveErrors = map[string]error{"assistant": errSave}
			},
			wantErr:       errSave,
			wantEvents:    []string{"status", "sources", "session", "status", "delta"},
			wantSaveRoles: []string{"user", "assistant"},
			wantLLMCalls:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &chatRepositoryStub{ensureID: "session"}
			searcher := &searcherStub{results: contextResult}
			model := &llmStub{streamDeltas: []string{"answer"}}
			tt.configure(repo, searcher, model)
			var events []ChatStreamEvent

			err := New(repo, searcher, model, 10).ChatStream(
				context.Background(),
				"",
				"question",
				models.SearchScope{},
				0,
				func(event ChatStreamEvent) error {
					events = append(events, event)
					return nil
				},
			)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("want error %v, got %v", tt.wantErr, err)
			}
			assertEventTypes(t, events, tt.wantEvents...)
			if got := savedRoles(repo.saves); !reflect.DeepEqual(got, tt.wantSaveRoles) {
				t.Fatalf("save roles: want %#v, got %#v", tt.wantSaveRoles, got)
			}
			if model.streamCalls != tt.wantLLMCalls {
				t.Fatalf("LLM calls: want %d, got %d", tt.wantLLMCalls, model.streamCalls)
			}
		})
	}
}

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

func TestBuildContextIncludesUniversalSourceLocation(t *testing.T) {
	got := buildContext([]models.SearchResult{
		{SourceType: "confluence", SourceLabel: "Wiki / HR", Title: "Vacation", SpaceKey: "HR", URL: "https://wiki/page", Chunk: "Policy"},
		{SourceType: "gitlab", SourceLabel: "GitLab / team/app", Title: "main.go", Repository: "team/app", Ref: "main", FilePath: "cmd/main.go", URL: "https://git/app", Chunk: "func main()"},
	})
	for _, want := range []string{"Source type: confluence", "Location: Wiki / HR", "Space: HR", "Source type: gitlab", "Repository: team/app", "Ref: main", "Path: cmd/main.go"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected context to contain %q, got %q", want, got)
		}
	}
}

type savedMessage struct {
	sessionID string
	role      string
	content   string
	sources   json.RawMessage
}

type chatRepositoryStub struct {
	history     []models.ChatMessage
	historyErr  error
	ensureID    string
	ensureErr   error
	saveErrors  map[string]error
	ensureCalls int
	saves       []savedMessage
}

func (r *chatRepositoryStub) EnsureChatSession(_ context.Context, _, _ string) (string, error) {
	r.ensureCalls++
	if r.ensureErr != nil {
		return "", r.ensureErr
	}
	return r.ensureID, nil
}

func (r *chatRepositoryStub) SaveChatMessage(_ context.Context, sessionID, role, content string, sources json.RawMessage) error {
	r.saves = append(r.saves, savedMessage{
		sessionID: sessionID,
		role:      role,
		content:   content,
		sources:   append(json.RawMessage(nil), sources...),
	})
	return r.saveErrors[role]
}

func (r *chatRepositoryStub) ListChatSessions(context.Context) ([]models.ChatSession, error) {
	return nil, nil
}

func (r *chatRepositoryStub) ListChatMessages(context.Context, string) ([]models.ChatMessage, error) {
	if r.historyErr != nil {
		return nil, r.historyErr
	}
	return r.history, nil
}

func (r *chatRepositoryStub) DeleteChatSession(context.Context, string) error {
	return nil
}

type searcherStub struct {
	results []models.SearchResult
	err     error
	calls   int
	query   string
	scope   models.SearchScope
	topK    int
}

func (s *searcherStub) Search(_ context.Context, query string, scope models.SearchScope, topK int) ([]models.SearchResult, error) {
	s.calls++
	s.query = query
	s.scope = scope
	s.topK = topK
	return s.results, s.err
}

type llmStub struct {
	chatResponse string
	chatErr      error
	streamDeltas []string
	streamErr    error
	chatCalls    int
	streamCalls  int
	requests     []llm.ChatRequest
}

func (l *llmStub) Chat(_ context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	l.chatCalls++
	l.requests = append(l.requests, req)
	return llm.ChatResponse{Content: l.chatResponse}, l.chatErr
}

func (l *llmStub) ChatStream(_ context.Context, req llm.ChatRequest, onDelta func(string) error) error {
	l.streamCalls++
	l.requests = append(l.requests, req)
	if l.streamErr != nil {
		return l.streamErr
	}
	for _, delta := range l.streamDeltas {
		if err := onDelta(delta); err != nil {
			return err
		}
	}
	return nil
}

func assertSavedConversation(
	t *testing.T,
	saves []savedMessage,
	sessionID, question, answer string,
	results []models.SearchResult,
) {
	t.Helper()
	if len(saves) != 2 {
		t.Fatalf("expected two saved messages, got %#v", saves)
	}
	if saves[0].sessionID != sessionID || saves[0].role != "user" || saves[0].content != question || saves[0].sources != nil {
		t.Fatalf("unexpected saved user message: %#v", saves[0])
	}
	if saves[1].sessionID != sessionID || saves[1].role != "assistant" || saves[1].content != answer {
		t.Fatalf("unexpected saved assistant message: %#v", saves[1])
	}
	wantSources, err := json.Marshal(results)
	if err != nil {
		t.Fatalf("marshal expected sources: %v", err)
	}
	if string(saves[1].sources) != string(wantSources) {
		t.Fatalf("saved sources mismatch: want %s, got %s", wantSources, saves[1].sources)
	}
}

func assertEventTypes(t *testing.T, events []ChatStreamEvent, want ...string) {
	t.Helper()
	got := make([]string, len(events))
	for i, event := range events {
		got[i] = event.Type
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("event types mismatch: want %#v, got %#v", want, got)
	}
}

func savedRoles(saves []savedMessage) []string {
	if len(saves) == 0 {
		return nil
	}
	roles := make([]string, len(saves))
	for i, save := range saves {
		roles[i] = save.role
	}
	return roles
}

var _ Searcher = (*searcherStub)(nil)
