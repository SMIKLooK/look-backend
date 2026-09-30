package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"look-backend/internal/domain"
	"look-backend/internal/provider/kit"
)

// Complete отправляет запрос в /models/{model}:generateContent
// и возвращает текст ответа модели.
func (c *Client) Complete(ctx context.Context, req domain.Request) (domain.Response, error) {
	body, err := c.buildPayload(req)
	if err != nil {
		return domain.Response{}, err
	}

	endpoint := fmt.Sprintf("%s/models/%s:generateContent", c.baseURL, url.PathEscape(req.Model))
	httpReq, err := kit.NewRequest(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return domain.Response{}, domain.WrapError(domain.CodeInternal, err, "не удалось сформировать запрос к gemini")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", c.apiKey)

	result, err := kit.Do(ctx, c.httpClient, c.Name(), httpReq)
	if err != nil {
		return domain.Response{}, err
	}

	text, err := c.parseResponse(result.StatusCode, result.Body)
	if err != nil {
		return domain.Response{}, err
	}
	return domain.Response{
		Model:    req.Model,
		Provider: c.Name(),
		Content:  text,
	}, nil
}

// buildPayload собирает тело generateContent; сообщения уже в доменном виде.
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

// parseResponse извлекает текст ответа: первый кандидат с непустыми частями.
func (c *Client) parseResponse(status int, raw []byte) (string, error) {
	var parsed generateResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", domain.WrapError(domain.CodeProviderFailed, err,
			"gemini вернул некорректный ответ (HTTP %d): %s", status, kit.TruncateBody(raw))
	}

	if status < 200 || status >= 300 {
		msg := fmt.Sprintf("HTTP %d", status)
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
