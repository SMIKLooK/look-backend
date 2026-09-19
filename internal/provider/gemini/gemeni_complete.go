package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"look-backend/internal/domain"
	"net/http"
	"net/url"
	"strings"
)

// Complete отправляет запрос в /models/{model}:generateContent
// и возвращает текст ответа модели.
func (c *Client) Complete(ctx context.Context, req domain.Request) (domain.Response, error) {
	if err := ctx.Err(); err != nil {
		return domain.Response{}, domain.WrapError(domain.CodeTimeout, err, "контекст отменён до запроса к gemini")
	}

	body, err := c.buildPayload(req)
	if err != nil {
		return domain.Response{}, err
	}

	httpResp, err := c.sendRequest(ctx, req.Model, body)
	if err != nil {
		return domain.Response{}, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, httpResp.Body)
		httpResp.Body.Close()
	}()

	text, err := c.parseResponse(httpResp)
	if err != nil {
		return domain.Response{}, err
	}

	return domain.Response{
		Model:    req.Model,
		Provider: c.Name(),
		Content:  text,
	}, nil
}

func (c *Client) buildPayload(req domain.Request) ([]byte, error) {
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
		return nil, domain.WrapError(domain.CodeInternal, err, "не удалось сериализовать запрос к gemini")
	}

	return body, nil
}

func (c *Client) sendRequest(
	ctx context.Context,
	model string,
	body []byte,
) (*http.Response, error) {
	endpoint := fmt.Sprintf("%s/models/%s:generateContent", c.baseURL, url.PathEscape(model))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, domain.WrapError(domain.CodeInternal, err, "не удалось сформировать запрос к gemini")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", c.apiKey)

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, domain.WrapError(domain.CodeTimeout, err, "таймаут запроса к gemini")
		}
		return nil, domain.WrapError(domain.CodeProviderFailed, err, "gemini недоступен")
	}

	return httpResp, nil
}

func (c *Client) parseResponse(httpResp *http.Response) (string, error) {
	raw, err := io.ReadAll(io.LimitReader(httpResp.Body, maxResponseBytes))
	if err != nil {
		return "", domain.WrapError(domain.CodeProviderFailed, err, "не удалось прочитать ответ gemini")
	}

	var parsed generateResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", domain.WrapError(domain.CodeProviderFailed, err,
			"gemini вернул некорректный ответ (HTTP %d): %s", httpResp.StatusCode, TruncateBody(raw))
	}

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		msg := fmt.Sprintf("HTTP %d", httpResp.StatusCode)
		if parsed.Error != nil && parsed.Error.Message != "" {
			msg += ": " + parsed.Error.Message
		}
		return "", domain.NewError(domain.CodeProviderFailed, "ошибка gemini: "+msg)
	}
	if parsed.PromptFeedback != nil && parsed.PromptFeedback.BlockReason != "" {
		return "", domain.NewError(domain.CodeProviderFailed,
			"gemini заблокировал запрос (blockReason: "+parsed.PromptFeedback.BlockReason+")")
	}

	for _, candidate := range parsed.Candidates {
		var b strings.Builder
		for _, p := range candidate.Content.Parts {
			b.WriteString(p.Text)
		}
		if text := strings.TrimSpace(b.String()); text != "" {
			return text, nil
		}
	}

	return "", domain.NewError(domain.CodeProviderFailed, "gemini не вернул текст в ответе")
}
