// Package openai — провайдер для OpenAI-совместимых API:
// OpenAI, OpenRouter, vLLM, Ollama и любые другие /chat/completions.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"look-backend/internal/domain"
)

const defaultBaseURL = "https://api.openai.com/v1"

var defaultModels = []string{"gpt-4o", "gpt-4o-mini"}

// defaultModelPrefixes — префиксы имён моделей, по которым модель
// относится к этому провайдеру, даже если её нет в списке Models.
var defaultModelPrefixes = []string{"gpt", "o1", "o3", "o4"}

// maxResponseBytes — ограничение на размер тела ответа провайдера.
const maxResponseBytes = 8 << 20

// Config — настройки клиента OpenAI.
type Config struct {
	APIKey        string
	BaseURL       string        // по умолчанию https://api.openai.com/v1
	Models        []string      // поддерживаемые модели; пусто — defaultModels
	ModelPrefixes []string      // префиксы моделей; пусто — defaultModelPrefixes
	Timeout       time.Duration // таймаут HTTP-запроса; 0 — без отдельного таймаута
	MaxTokens     int           // 0 — параметр не отправляется
	HTTPClient    *http.Client  // опционально; удобно подменять в тестах
}

type Client struct {
	apiKey     string
	baseURL    string
	models     []string
	prefixes   []string
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
	prefixes := cfg.ModelPrefixes
	if len(prefixes) == 0 {
		prefixes = defaultModelPrefixes
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
		if cfg.Timeout > 0 {
			httpClient.Timeout = cfg.Timeout
		}
	}
	return &Client{
		apiKey:     cfg.APIKey,
		baseURL:    baseURL,
		models:     models,
		prefixes:   prefixes,
		maxTokens:  cfg.MaxTokens,
		httpClient: httpClient,
	}
}

func (c *Client) Name() string { return "openai" }

func (c *Client) Models() []string { return append([]string(nil), c.models...) }

func (c *Client) Supports(model string) bool {
	for _, known := range c.models {
		if strings.EqualFold(known, model) {
			return true
		}
	}
	m := strings.ToLower(model)
	for _, p := range c.prefixes {
		if strings.HasPrefix(m, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model     string        `json:"model"`
	Messages  []chatMessage `json:"messages"`
	MaxTokens int           `json:"max_tokens,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// Complete отправляет запрос в /chat/completions и возвращает текст ответа.
func (c *Client) Complete(ctx context.Context, req domain.Request) (domain.Response, error) {
	if err := ctx.Err(); err != nil {
		return domain.Response{}, domain.WrapError(domain.CodeTimeout, err, "контекст отменён до запроса к openai")
	}

	payload := chatRequest{Model: req.Model, MaxTokens: c.maxTokens}
	for _, m := range req.Messages {
		payload.Messages = append(payload.Messages, chatMessage{Role: m.Role, Content: m.Content})
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return domain.Response{}, domain.WrapError(domain.CodeInternal, err, "не удалось сериализовать запрос к openai")
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return domain.Response{}, domain.WrapError(domain.CodeInternal, err, "не удалось сформировать запрос к openai")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return domain.Response{}, domain.WrapError(domain.CodeTimeout, err, "таймаут запроса к openai")
		}
		return domain.Response{}, domain.WrapError(domain.CodeProviderFailed, err, "openai недоступен")
	}
	defer httpResp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(httpResp.Body, maxResponseBytes))
	if err != nil {
		return domain.Response{}, domain.WrapError(domain.CodeProviderFailed, err, "не удалось прочитать ответ openai")
	}

	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return domain.Response{}, domain.WrapError(domain.CodeProviderFailed, err,
			"openai вернул некорректный ответ (HTTP %d): %s", httpResp.StatusCode, truncateBody(raw))
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		msg := fmt.Sprintf("HTTP %d", httpResp.StatusCode)
		if parsed.Error != nil && parsed.Error.Message != "" {
			msg += ": " + parsed.Error.Message
		}
		return domain.Response{}, domain.NewError(domain.CodeProviderFailed, "ошибка openai: "+msg)
	}
	if len(parsed.Choices) == 0 {
		return domain.Response{}, domain.NewError(domain.CodeProviderFailed, "openai вернул пустой список choices")
	}
	return domain.Response{
		Model:    req.Model,
		Provider: c.Name(),
		Content:  strings.TrimSpace(parsed.Choices[0].Message.Content),
	}, nil
}

func truncateBody(b []byte) string {
	s := string(b)
	if len(s) > 512 {
		return s[:512] + "..."
	}
	return s
}
