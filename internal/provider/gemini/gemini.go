// Package gemini — провайдер Google Gemini (Generative Language API).
package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"look-backend/internal/domain"
)

const defaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// defaultModels — модели по умолчанию для списка GET /api/v1/models.
// Поддерживается любая модель с префиксом "gemini" (см. defaultModelPrefixes).
var defaultModels = []string{"gemini-3.8-flash", "gemini-3.6-flash", "gemini-2.5-pro"}

var defaultModelPrefixes = []string{"gemini"}

// maxResponseBytes — ограничение на размер тела ответа провайдера.
const maxResponseBytes = 8 << 20

// Config — настройки клиента Gemini.
type Config struct {
	APIKey        string
	BaseURL       string        // по умолчанию https://generativelanguage.googleapis.com/v1beta
	Models        []string      // поддерживаемые модели; пусто — defaultModels
	ModelPrefixes []string      // префиксы моделей; пусто — ["gemini"]
	Timeout       time.Duration // таймаут HTTP-запроса; 0 — без отдельного таймаута
	MaxTokens     int           // 0 — параметр maxOutputTokens не отправляется
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

func (c *Client) Name() string { return "gemini" }

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

type part struct {
	Text string `json:"text"`
}

type content struct {
	Role  string `json:"role"`
	Parts []part `json:"parts"`
}

type generationConfig struct {
	MaxOutputTokens int `json:"maxOutputTokens,omitempty"`
}

type generateRequest struct {
	Contents         []content         `json:"contents"`
	GenerationConfig *generationConfig `json:"generationConfig,omitempty"`
}

type generateResponse struct {
	Candidates []struct {
		Content struct {
			Parts []part `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback *struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

// Complete отправляет запрос в /models/{model}:generateContent
// и возвращает текст ответа модели.
func (c *Client) Complete(ctx context.Context, req domain.Request) (domain.Response, error) {
	if err := ctx.Err(); err != nil {
		return domain.Response{}, domain.WrapError(domain.CodeTimeout, err, "контекст отменён до запроса к gemini")
	}

	payload := generateRequest{}
	for _, m := range req.Messages {
		// Gemini называет роль ассистента "model"
		role := m.Role
		if strings.EqualFold(role, "assistant") {
			role = "model"
		}
		payload.Contents = append(payload.Contents, content{
			Role:  role,
			Parts: []part{{Text: m.Content}},
		})
	}
	if c.maxTokens > 0 {
		payload.GenerationConfig = &generationConfig{MaxOutputTokens: c.maxTokens}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return domain.Response{}, domain.WrapError(domain.CodeInternal, err, "не удалось сериализовать запрос к gemini")
	}

	endpoint := fmt.Sprintf("%s/models/%s:generateContent", c.baseURL, url.PathEscape(req.Model))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return domain.Response{}, domain.WrapError(domain.CodeInternal, err, "не удалось сформировать запрос к gemini")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", c.apiKey)

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return domain.Response{}, domain.WrapError(domain.CodeTimeout, err, "таймаут запроса к gemini")
		}
		return domain.Response{}, domain.WrapError(domain.CodeProviderFailed, err, "gemini недоступен")
	}
	defer httpResp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(httpResp.Body, maxResponseBytes))
	if err != nil {
		return domain.Response{}, domain.WrapError(domain.CodeProviderFailed, err, "не удалось прочитать ответ gemini")
	}

	var parsed generateResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return domain.Response{}, domain.WrapError(domain.CodeProviderFailed, err,
			"gemini вернул некорректный ответ (HTTP %d): %s", httpResp.StatusCode, truncateBody(raw))
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		msg := fmt.Sprintf("HTTP %d", httpResp.StatusCode)
		if parsed.Error != nil && parsed.Error.Message != "" {
			msg += ": " + parsed.Error.Message
		}
		return domain.Response{}, domain.NewError(domain.CodeProviderFailed, "ошибка gemini: "+msg)
	}
	if parsed.PromptFeedback != nil && parsed.PromptFeedback.BlockReason != "" {
		return domain.Response{}, domain.NewError(domain.CodeProviderFailed,
			"gemini заблокировал запрос (blockReason: "+parsed.PromptFeedback.BlockReason+")")
	}
	for _, candidate := range parsed.Candidates {
		var b strings.Builder
		for _, p := range candidate.Content.Parts {
			b.WriteString(p.Text)
		}
		if text := strings.TrimSpace(b.String()); text != "" {
			return domain.Response{
				Model:    req.Model,
				Provider: c.Name(),
				Content:  text,
			}, nil
		}
	}
	return domain.Response{}, domain.NewError(domain.CodeProviderFailed, "gemini не вернул текст в ответе")
}

func truncateBody(b []byte) string {
	s := string(b)
	if len(s) > 512 {
		return s[:512] + "..."
	}
	return s
}
