package service

import (
	"context"
	"time"

	"look-backend/internal/domain"
	"look-backend/internal/parser"
	"look-backend/internal/provider"
)

type Result struct {
	Model     string `json:"model"`
	Provider  string `json:"provider"`
	Answer    string `json:"answer"`
	ElapsedMS int64  `json:"elapsed_ms"`
}

type ChatResult struct {
	Result
	SessionID string `json:"session_id"`
}

type Service struct {
	parser   *parser.Parser
	registry *provider.Registry
	timeout  time.Duration // таймаут одного обращения к провайдеру; 0 — без таймаута
}

func New(
	p *parser.Parser,
	r *provider.Registry,
	timeout time.Duration,
) *Service {
	return &Service{
		parser:   p,
		registry: r,
		timeout:  timeout,
	}
}

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

	reqMessages := []domain.Message{
		{
			Role:    "user",
			Content: parsed.Prompt,
		},
	}

	resp, err := s.complete(ctx, prov, model, reqMessages)
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

type completion struct {
	domain.Response
	elapsedMS int64
}

// complete вызывает провайдера с таймаутом сервиса.
func (s *Service) complete(
	ctx context.Context,
	prov provider.Provider,
	model string,
	messages []domain.Message,
) (completion, error) {
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
