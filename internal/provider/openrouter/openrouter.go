// Package openrouter — провайдер агрегатора OpenRouter: один API-ключ
// даёт доступ к моделям десятков вендоров (openai/, google/, anthropic/,
// deepseek/ и т.д.). API совместим с OpenAI /chat/completions.
//
// Модели адресуются слагом "вендор/модель" (см. https://openrouter.ai/models),
// например: openai/gpt-5.6-terra, google/gemini-3.8-flash.
// Короткие имена без слэша разворачиваются в полный слаг по списку Models.
package openrouter

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"look-backend/internal/domain"
	"look-backend/internal/provider/kit"
)

const defaultBaseURL = "https://openrouter.ai/api/v1"

// appTitle — необязательный заголовок X-Title: имя приложения в статистике OpenRouter.
const appTitle = "look-backend"

// defaultModels — популярные модели OpenRouter (формат "vendor/model").
// Полный каталог: https://openrouter.ai/models
var defaultModels = []string{
	"openrouter/auto",             // роутер: сам выбирает модель под запрос
	"openrouter/free",             // роутер: случайная бесплатная модель, нулевая цена
	"google/gemini-3.8-flash",     // новейший Gemini
	"google/gemini-3.6-flash",     // Gemini, рекомендованный новым ключам
	"openai/gpt-5.6-terra",        // сбалансированный OpenAI
	"anthropic/claude-sonnet-5",   // новейший Claude Sonnet
	"deepseek/deepseek-v4-flash",  // дешёвая и сильная модель
	"z-ai/glm-5.3-flash",          // очень дешёвая
	"meta-llama/llama-4-maverick", // открытая Llama 4
}

// errWAFBlocked — часть текста, которым OpenRouter WAF отвечает на запросы
// из заблокированных регионов/сетей (HTTP 403, до проверки API-ключа).
const errWAFBlocked = "Access denied by security policy"

// Config — настройки клиента OpenRouter.
type Config struct {
	APIKey     string
	BaseURL    string        // по умолчанию https://openrouter.ai/api/v1
	Models     []string      // поддерживаемые модели; пусто — defaultModels
	Timeout    time.Duration // таймаут HTTP-запроса; 0 — без отдельного таймаута
	MaxTokens  int           // 0 — параметр max_tokens не отправляется
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

func (c *Client) Name() string { return "openrouter" }

func (c *Client) Models() []string { return append([]string(nil), c.models...) }

// Supports принимает слаги "vendor/model", точные имена из Models
// и короткие имена, совпадающие с частью после слэша в Models
// ("gpt-5.6-terra" → "openai/gpt-5.6-terra").
func (c *Client) Supports(model string) bool {
	m := strings.ToLower(model)
	if strings.Contains(m, "/") {
		return true
	}
	for _, known := range c.models {
		if strings.EqualFold(known, model) {
			return true
		}
		if slugSuffix(known) == m {
			return true
		}
	}
	return false
}

// resolveSlug приводит имя модели к слагу OpenRouter "vendor/model":
// точное совпадение или короткое имя из Models; иначе имя как есть —
// OpenRouter вернёт внятную ошибку для неизвестного слага.
func (c *Client) resolveSlug(model string) string {
	for _, known := range c.models {
		if strings.EqualFold(known, model) {
			return known
		}
	}
	m := strings.ToLower(model)
	for _, known := range c.models {
		if slugSuffix(known) == m {
			return known
		}
	}
	if strings.Contains(m, "/") {
		return strings.ToLower(model)
	}
	return model
}

func slugSuffix(slug string) string {
	if _, suffix, ok := strings.Cut(slug, "/"); ok {
		return strings.ToLower(suffix)
	}
	return ""
}

// Complete отправляет запрос в /chat/completions и возвращает текст ответа.
func (c *Client) Complete(ctx context.Context, req domain.Request) (domain.Response, error) {
	slug := c.resolveSlug(req.Model)
	body, err := kit.MarshalChat(slug, req.Messages, c.maxTokens)
	if err != nil {
		return domain.Response{}, domain.WrapError(domain.CodeInternal, err, "не удалось сериализовать запрос к openrouter")
	}

	httpReq, err := kit.NewRequest(ctx, http.MethodPost, c.baseURL+"/chat/completions", body)
	if err != nil {
		return domain.Response{}, domain.WrapError(domain.CodeInternal, err, "не удалось сформировать запрос к openrouter")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("X-Title", appTitle)

	result, err := kit.Do(ctx, c.httpClient, c.Name(), httpReq)
	if err != nil {
		return domain.Response{}, err
	}

	reply, parseErr := kit.ParseChat(result.Body)
	if parseErr != nil {
		return domain.Response{}, domain.WrapError(domain.CodeProviderFailed, parseErr,
			"openrouter вернул некорректный ответ (HTTP %d): %s", result.StatusCode, kit.TruncateBody(result.Body))
	}
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		if result.StatusCode == http.StatusForbidden && strings.Contains(reply.ErrText, errWAFBlocked) {
			return domain.Response{}, domain.NewError(domain.CodeProviderFailed,
				"openrouter блокирует запросы с этой сети (HTTP 403, WAF): регион/IP сервера не поддерживается. "+
					"Это не ошибка кода и не проблема ключа — запросы не доходят до API. "+
					"Решение: бесплатный прокси в поддерживаемом регионе (Cloudflare Worker, код в README) "+
					"и его адрес в keys.go → OpenRouterBaseURL")
		}
		msg := fmt.Sprintf("HTTP %d", result.StatusCode)
		if reply.ErrText != "" {
			msg += ": " + reply.ErrText
		}
		return domain.Response{}, domain.NewError(domain.CodeProviderFailed, "ошибка openrouter: "+msg)
	}
	if reply.Choices == 0 {
		return domain.Response{}, domain.NewError(domain.CodeProviderFailed, "openrouter вернул пустой список choices")
	}
	return domain.Response{
		Model:    slug,
		Provider: c.Name(),
		Content:  reply.Content,
	}, nil
}
