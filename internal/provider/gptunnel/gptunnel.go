package gptunnel

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"look-backend/internal/domain"
	"look-backend/internal/provider/kit"
)

const defaultBaseURL = "https://gptunnel.ru/v1"

// defaultModels — популярные текстовые модели GPTunneL.
// Полный каталог: https://gptunnel.ru/v1/models или docs.gptunnel.ru.
var defaultModels = []string{
	"gpt-4o",
	"gpt-4o-mini",
	"o3-mini",
	"claude-4.5-haiku", // самая дешёвая Claude в каталоге
	"claude-5-sonnet",
	"deepseek-v4-flash",
}

// Config — настройки клиента GPTunneL.
type Config struct {
	APIKey     string
	BaseURL    string        // по умолчанию https://gptunnel.ru/v1
	Models     []string      // поддерживаемые модели; пусто — defaultModels
	MaxTokens  int           // 0 — параметр max_tokens не отправляется
	Timeout    time.Duration // таймаут HTTP-запроса; 0 — без отдельного таймаута
	HTTPClient *http.Client  // опционально; удобно подменять в тестах
}

type Client struct {
	apiKey     string
	baseURL    string
	models     []string
	maxTokens  int
	httpClient *http.Client
}

func New(cfg Config) *Client {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	models := cfg.Models
	if len(models) == 0 {
		models = defaultModels
	}
	return &Client{
		apiKey:     cfg.APIKey,
		baseURL:    baseURL,
		models:     models,
		maxTokens:  cfg.MaxTokens,
		httpClient: kit.NewHTTPClient(cfg.Timeout, cfg.HTTPClient),
	}
}

func (c *Client) Name() string { return "gptunnel" }

func (c *Client) Models() []string { return append([]string(nil), c.models...) }

func (c *Client) Supports(model string) bool {
	return kit.ContainsFold(c.models, model)
}

func (c *Client) resolveModel(model string) string {
	return kit.ResolveFold(c.models, model)
}

// Complete отправляет запрос в /chat/completions и возвращает текст ответа.
func (c *Client) Complete(ctx context.Context, req domain.Request) (domain.Response, error) {
	model := c.resolveModel(req.Model)
	body, err := kit.MarshalChat(model, req.Messages, c.maxTokens)
	if err != nil {
		return domain.Response{}, domain.WrapError(domain.CodeInternal, err, "не удалось сериализовать запрос к gptunnel")
	}

	httpReq, err := kit.NewRequest(ctx, http.MethodPost, c.baseURL+"/chat/completions", body)
	if err != nil {
		return domain.Response{}, domain.WrapError(domain.CodeInternal, err, "не удалось сформировать запрос к gptunnel")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	result, err := kit.Do(ctx, c.httpClient, c.Name(), httpReq)
	if err != nil {
		return domain.Response{}, err
	}

	reply, parseErr := kit.ParseChat(result.Body)
	if parseErr != nil {
		return domain.Response{}, domain.WrapError(domain.CodeProviderFailed, parseErr,
			"gptunnel вернул некорректный ответ (HTTP %d): %s", result.StatusCode, kit.TruncateBody(result.Body))
	}
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		msg := fmt.Sprintf("HTTP %d", result.StatusCode)
		if reply.ErrText != "" {
			msg += ": " + reply.ErrText
		}
		return domain.Response{}, domain.NewError(domain.CodeProviderFailed, "ошибка gptunnel: "+msg)
	}
	if reply.Choices == 0 {
		return domain.Response{}, domain.NewError(domain.CodeProviderFailed, "gptunnel вернул пустой список choices")
	}
	return domain.Response{
		Model:    model,
		Provider: c.Name(),
		Content:  reply.Content,
	}, nil
}
