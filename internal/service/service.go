// Package service — бизнес-логика: разбирает текст на модель и запрос,
// находит подходящего провайдера и запрашивает у него ответ.
package service

import (
	"context"
	"time"

	"look-backend/internal/domain"
	"look-backend/internal/parser"
	"look-backend/internal/provider"
	"look-backend/internal/storage"
)

// Result — итог обработки текста; попадает в API-ответ.
type Result struct {
	Model     string `json:"model"`
	Provider  string `json:"provider"`
	Answer    string `json:"answer"`
	ElapsedMS int64  `json:"elapsed_ms"`
}

// ChatResult — итог диалогового запроса: то же + сессия и размер контекста.
type ChatResult struct {
	Result
	SessionID string `json:"session_id"`
	Messages  int    `json:"messages"` // сколько сообщений ушло модели вместе с историей
}

// Service связывает парсер текста, реестр провайдеров и хранилище истории.
type Service struct {
	parser     *parser.Parser
	registry   *provider.Registry
	store      storage.Store
	timeout    time.Duration // таймаут одного обращения к провайдеру; 0 — без таймаута
	maxHistory int           // сколько последних сообщений отправлять модели; 0 — без ограничения
}

// New собирает сервис; store == nil включает хранилище в памяти.
func New(p *parser.Parser, r *provider.Registry, store storage.Store, timeout time.Duration, maxHistory int) *Service {
	if store == nil {
		store = storage.NewMemoryStore(24*time.Hour, 10*time.Minute)
	}
	return &Service{
		parser:     p,
		registry:   r,
		store:      store,
		timeout:    timeout,
		maxHistory: maxHistory,
	}
}

// Keywords возвращает ключевые слова парсера — для служебных эндпоинтов.
func (s *Service) Keywords() []string { return s.parser.Keywords() }

// Providers возвращает список провайдеров — для служебных эндпоинтов.
func (s *Service) Providers() []provider.Info { return s.registry.List() }

// Aliases возвращает псевдонимы моделей — для служебных эндпоинтов.
func (s *Service) Aliases() map[string]string { return s.registry.Aliases() }

// Process разбирает текст, находит провайдера и возвращает ответ модели.
// Запрос одиночный, без истории диалога.
func (s *Service) Process(ctx context.Context, text string) (Result, error) {
	parsed, err := s.parser.Parse(text)
	if err != nil {
		return Result{}, err
	}

	prov, model, err := s.registry.Resolve(parsed.Model)
	if err != nil {
		return Result{}, err
	}

	messages := []domain.Message{{Role: "user", Content: parsed.Prompt}}
	resp, err := s.complete(ctx, prov, model, messages)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Model:     resp.Model,
		Provider:  resp.Provider,
		Answer:    resp.Content,
		ElapsedMS: resp.elapsedMS,
	}, nil
}

// Chat — запрос с историей диалога: модели уходят предыдущие сообщения
// сессии плюс новый запрос. После успешного ответа в историю добавляется
// пара user/assistant; при ошибке история не меняется.
func (s *Service) Chat(ctx context.Context, sessionID, text string) (ChatResult, error) {
	parsed, err := s.parser.Parse(text)
	if err != nil {
		return ChatResult{}, err
	}

	prov, model, err := s.registry.Resolve(parsed.Model)
	if err != nil {
		return ChatResult{}, err
	}

	history, err := s.store.History(sessionID)
	if err != nil {
		return ChatResult{}, err
	}
	messages := make([]domain.Message, 0, len(history)+1)
	messages = append(messages, history...)
	messages = append(messages, domain.Message{Role: "user", Content: parsed.Prompt})
	if s.maxHistory > 0 && len(messages) > s.maxHistory {
		messages = messages[len(messages)-s.maxHistory:]
	}

	resp, err := s.complete(ctx, prov, model, messages)
	if err != nil {
		return ChatResult{}, err
	}

	if err := s.store.Append(sessionID, domain.Message{Role: "user", Content: parsed.Prompt}); err != nil {
		return ChatResult{}, err
	}
	if err := s.store.Append(sessionID, domain.Message{Role: "assistant", Content: resp.Content}); err != nil {
		return ChatResult{}, err
	}
	return ChatResult{
		Result:    Result{Model: resp.Model, Provider: resp.Provider, Answer: resp.Content, ElapsedMS: resp.elapsedMS},
		SessionID: sessionID,
		Messages:  len(messages),
	}, nil
}

// History возвращает сохранённые сообщения сессии.
func (s *Service) History(sessionID string) ([]domain.Message, error) {
	return s.store.History(sessionID)
}

// Reset очищает историю сессии.
func (s *Service) Reset(sessionID string) error {
	return s.store.Reset(sessionID)
}

type completion struct {
	domain.Response
	elapsedMS int64
}

// complete вызывает провайдера с таймаутом сервиса.
func (s *Service) complete(ctx context.Context, prov provider.Provider, model string, messages []domain.Message) (completion, error) {
	callCtx, cancel := withTimeout(ctx, s.timeout)
	defer cancel()

	start := time.Now()
	resp, err := prov.Complete(callCtx, domain.Request{Model: model, Messages: messages})
	if err != nil {
		return completion{}, err
	}
	return completion{Response: resp, elapsedMS: time.Since(start).Milliseconds()}, nil
}

func withTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, d)
}
