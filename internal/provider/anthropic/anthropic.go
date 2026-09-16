// Package anthropic — провайдер Anthropic (модели Claude).
package anthropic

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

const defaultBaseURL = "https://api.anthropic.com"

const apiVersion = "2023-06-01"

const defaultMaxTokens = 1024

var defaultModels = []string{"claude-sonnet-4-20250514", "claude-3-5-haiku-20241022"}

var defaultModelPrefixes = []string{"claude"}

// maxResponseBytes — ограничение на размер тела ответа провайдера.
const maxResponseBytes = 8 << 20

// Config — настройки клиента Anthropic.
type Config struct {
	APIKey        string
	BaseURL       string        // по умолчанию https://api.anthropic.com
	Models        []string      // поддерживаемые модели; пусто — defaultModels
	ModelPrefixes []string      // префиксы моделей; пусто — ["claude"]
	Timeout       time.Duration // таймаут HTTP-запроса; 0 — без отдельного таймаута
	MaxTokens     int           // для API Anthropic обязателен; 0 — defaultMaxTokens
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
	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
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
		maxTokens:  maxTokens,
		httpClient: httpClient,
	}
}

func (c *Client) Name() string { return "anthropic" }

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

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type messagesRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	Messages  []message `json:"messages"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type messagesResponse struct {
	Content []contentBlock `json:"content"`
	Error   *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// Complete отправляет запрос в /v1/messages и возвращает текст ответа.
func (c *Client) Complete(ctx context.Context, req domain.Request) (domain.Response, error) {
	if err := ctx.Err(); err != nil {
		return domain.Response{}, domain.WrapError(domain.CodeTimeout, err, "контекст отменён до запроса к anthropic")
	}

	payload := messagesRequest{Model: req.Model, MaxTokens: c.maxTokens}
	for _, m := range req.Messages {
		payload.Messages = append(payload.Messages, message{Role: m.Role, Content: m.Content})
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return domain.Response{}, domain.WrapError(domain.CodeInternal, err, "не удалось сериализовать запрос к anthropic")
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return domain.Response{}, domain.WrapError(domain.CodeInternal, err, "не удалось сформировать запрос к anthropic")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", apiVersion)

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return domain.Response{}, domain.WrapError(domain.CodeTimeout, err, "таймаут запроса к anthropic")
		}
		return domain.Response{}, domain.WrapError(domain.CodeProviderFailed, err, "anthropic недоступен")
	}
	defer httpResp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(httpResp.Body, maxResponseBytes))
	if err != nil {
		return domain.Response{}, domain.WrapError(domain.CodeProviderFailed, err, "не удалось прочитать ответ anthropic")
	}

	var parsed messagesResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return domain.Response{}, domain.WrapError(domain.CodeProviderFailed, err,
			"anthropic вернул некорректный ответ (HTTP %d): %s", httpResp.StatusCode, truncateBody(raw))
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		msg := fmt.Sprintf("HTTP %d", httpResp.StatusCode)
		if parsed.Error != nil && parsed.Error.Message != "" {
			msg += ": " + parsed.Error.Message
		}
		return domain.Response{}, domain.NewError(domain.CodeProviderFailed, "ошибка anthropic: "+msg)
	}
	for _, block := range parsed.Content {
		if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
			return domain.Response{
				Model:    req.Model,
				Provider: c.Name(),
				Content:  strings.TrimSpace(block.Text),
			}, nil
		}
	}
	return domain.Response{}, domain.NewError(domain.CodeProviderFailed, "anthropic не вернул текстовый блок в ответе")
}

func truncateBody(b []byte) string {
	s := string(b)
	if len(s) > 512 {
		return s[:512] + "..."
	}
	return s
}
