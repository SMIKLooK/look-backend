package service

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"look-backend/internal/domain"
	"look-backend/internal/parser"
	"look-backend/internal/provider"
	"look-backend/internal/storage"
)

// stubProvider — провайдер-заглушка для тестов сервиса.
type stubProvider struct {
	name    string
	models  []string
	content string
}

func (s *stubProvider) Name() string     { return s.name }
func (s *stubProvider) Models() []string { return s.models }

func (s *stubProvider) Supports(model string) bool {
	for _, m := range s.models {
		if strings.EqualFold(m, model) {
			return true
		}
	}
	return false
}

func (s *stubProvider) Complete(ctx context.Context, req domain.Request) (domain.Response, error) {
	if err := ctx.Err(); err != nil {
		return domain.Response{}, domain.WrapError(domain.CodeTimeout, err, "контекст отменён")
	}
	return domain.Response{
		Model:    req.Model,
		Provider: s.name,
		Content:  s.content + ": " + req.Messages[0].Content,
	}, nil
}

// turnProvider — заглушка, показывающая, сколько сообщений пришло модели
// и что было в последнем: по ней видно, доезжает ли история диалога.
type turnProvider struct{}

func (t *turnProvider) Name() string     { return "stub-turn" }
func (t *turnProvider) Models() []string { return []string{"stub-turn"} }

func (t *turnProvider) Supports(model string) bool {
	return strings.EqualFold(model, "stub-turn")
}

func (t *turnProvider) Complete(ctx context.Context, req domain.Request) (domain.Response, error) {
	last := req.Messages[len(req.Messages)-1]
	return domain.Response{
		Model:    req.Model,
		Provider: t.Name(),
		Content:  fmt.Sprintf("turns=%d last=%s", len(req.Messages), last.Content),
	}, nil
}

func TestService_Process(t *testing.T) {
	svc := New(parser.New(), provider.NewRegistry(
		&stubProvider{name: "stub", models: []string{"stub-1"}, content: "ответ"},
	), storage.NewMemoryStore(time.Hour, time.Minute), time.Second, 0)

	res, err := svc.Process(context.Background(), "stub-1 привет")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if res.Model != "stub-1" || res.Provider != "stub" || res.Answer != "ответ: привет" {
		t.Errorf("неожиданный результат: %+v", res)
	}
	if res.ElapsedMS < 0 {
		t.Errorf("elapsed_ms не может быть отрицательным: %d", res.ElapsedMS)
	}
}

func TestService_UnknownModel(t *testing.T) {
	svc := New(parser.New(), provider.NewRegistry(
		&stubProvider{name: "stub", models: []string{"stub-1"}},
	), storage.NewMemoryStore(time.Hour, time.Minute), 0, 0)

	_, err := svc.Process(context.Background(), "unknown-модель привет")
	if domain.ErrorCode(err) != domain.CodeUnknownModel {
		t.Fatalf("ожидался код %s, получен: %v", domain.CodeUnknownModel, err)
	}
}

func TestService_UnknownModelForArbitraryFirstWord(t *testing.T) {
	svc := New(parser.New(), provider.NewRegistry(
		&stubProvider{name: "stub", models: []string{"stub-1"}},
	), storage.NewMemoryStore(time.Hour, time.Minute), 0, 0)

	// первое слово трактуется как модель; если провайдер не найден —
	// клиент получает понятную ошибку
	_, err := svc.Process(context.Background(), "просто текст без модели")
	if domain.ErrorCode(err) != domain.CodeUnknownModel {
		t.Fatalf("ожидался код %s, получен: %v", domain.CodeUnknownModel, err)
	}
}

func TestService_ChatKeepsHistory(t *testing.T) {
	svc := New(parser.New(), provider.NewRegistry(&turnProvider{}),
		storage.NewMemoryStore(time.Hour, time.Minute), time.Second, 0)

	res1, err := svc.Chat(context.Background(), "s1", "stub-turn раз")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if res1.Messages != 1 || res1.Answer != "turns=1 last=раз" || res1.SessionID != "s1" {
		t.Fatalf("неожиданный первый ход: %+v", res1)
	}

	res2, err := svc.Chat(context.Background(), "s1", "stub-turn два")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	// второй ход видит предыдущую пару user/assistant + новый запрос
	if res2.Messages != 3 || res2.Answer != "turns=3 last=два" {
		t.Fatalf("история не доехала до модели: %+v", res2)
	}

	history, err := svc.History("s1")
	if err != nil {
		t.Fatalf("неожиданная ошибка History: %v", err)
	}
	if len(history) != 4 ||
		history[0].Role != "user" || history[1].Role != "assistant" ||
		history[2].Role != "user" || history[3].Role != "assistant" {
		t.Fatalf("неожиданная сохранённая история: %+v", history)
	}

	if err := svc.Reset("s1"); err != nil {
		t.Fatalf("неожиданная ошибка Reset: %v", err)
	}
	if history, _ = svc.History("s1"); len(history) != 0 {
		t.Fatalf("после Reset история должна быть пуста: %+v", history)
	}
}

func TestService_ChatTrimsHistory(t *testing.T) {
	svc := New(parser.New(), provider.NewRegistry(&turnProvider{}),
		storage.NewMemoryStore(time.Hour, time.Minute), time.Second, 2)

	if _, err := svc.Chat(context.Background(), "s1", "stub-turn раз"); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	res2, err := svc.Chat(context.Background(), "s1", "stub-turn два")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	// при лимите 2 модели уходят только два последних сообщения
	if res2.Messages != 2 || res2.Answer != "turns=2 last=два" {
		t.Fatalf("лимит истории не применился: %+v", res2)
	}
}

func TestService_ChatFailureKeepsHistoryClean(t *testing.T) {
	svc := New(parser.New(), provider.NewRegistry(
		&stubProvider{name: "stub", models: []string{"stub-1"}},
	), storage.NewMemoryStore(time.Hour, time.Minute), time.Second, 0)

	if _, err := svc.Chat(context.Background(), "s1", "unknown-model привет"); err != nil {
		if domain.ErrorCode(err) != domain.CodeUnknownModel {
			t.Fatalf("ожидался код %s, получен: %v", domain.CodeUnknownModel, err)
		}
	} else {
		t.Fatal("ожидалась ошибка UNKNOWN_MODEL")
	}

	history, _ := svc.History("s1")
	if len(history) != 0 {
		t.Fatalf("при неудачном запросе история не должна пополняться: %+v", history)
	}
}
