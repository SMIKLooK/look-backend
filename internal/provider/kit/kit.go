package kit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"look-backend/internal/domain"
)

const MaxResponseBytes = 8 << 20

func NewHTTPClient(timeout time.Duration, custom *http.Client) *http.Client {
	if custom != nil {
		return custom
	}
	c := &http.Client{}
	if timeout > 0 {
		c.Timeout = timeout
	}
	return c
}

type Result struct {
	StatusCode int
	Body       []byte
}

func Do(ctx context.Context, httpClient *http.Client, name string, req *http.Request) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, domain.WrapError(domain.CodeTimeout, err, "контекст отменён до запроса к "+name)
	}
	httpResp, err := httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Result{}, domain.WrapError(domain.CodeTimeout, err, "таймаут запроса к "+name)
		}
		return Result{}, domain.WrapError(domain.CodeProviderFailed, err, name+" недоступен")
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(httpResp.Body, MaxResponseBytes))
	if err != nil {
		return Result{}, domain.WrapError(domain.CodeProviderFailed, err, "не удалось прочитать ответ "+name)
	}
	return Result{StatusCode: httpResp.StatusCode, Body: body}, nil
}

// TruncateBody ограничивает текст тела для сообщений об ошибках.
func TruncateBody(b []byte) string {
	s := string(b)
	if len(s) > 512 {
		return s[:512] + "..."
	}
	return s
}

// ContainsFold сообщает, есть ли модель в списке (регистр не важен).
func ContainsFold(models []string, model string) bool {
	for _, known := range models {
		if strings.EqualFold(known, model) {
			return true
		}
	}
	return false
}

func ResolveFold(models []string, model string) string {
	for _, known := range models {
		if strings.EqualFold(known, model) {
			return known
		}
	}
	return model
}

// ChatMessage и ChatRequest — формат OpenAI /chat/completions: его
// придерживаются OpenRouter, GPTunneL и GigaChat.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model     string        `json:"model"`
	Messages  []ChatMessage `json:"messages"`
	MaxTokens int           `json:"max_tokens,omitempty"`
}

// MarshalChat собирает тело запроса /chat/completions из доменного запроса.
func MarshalChat(model string, messages []domain.Message, maxTokens int) ([]byte, error) {
	req := ChatRequest{Model: model, MaxTokens: maxTokens}
	for _, m := range messages {
		req.Messages = append(req.Messages, ChatMessage{Role: m.Role, Content: m.Content})
	}
	return json.Marshal(req)
}

type ChatReply struct {
	Content string
	ErrText string
	Choices int
}

func ParseChat(body []byte) (ChatReply, error) {
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ChatReply{}, err
	}
	reply := ChatReply{Choices: len(parsed.Choices)}
	if len(parsed.Error) > 0 {
		reply.ErrText = parseChatError(parsed.Error)
	}
	if reply.Choices > 0 {
		reply.Content = strings.TrimSpace(parsed.Choices[0].Message.Content)
	}
	return reply, nil
}

// parseChatError достаёт текст ошибки из поля error, каким бы оно ни было.
func parseChatError(raw json.RawMessage) string {
	var obj struct {
		Message     string `json:"message"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		if obj.Message != "" {
			return obj.Message
		}
		if obj.Description != "" {
			return obj.Description
		}
	}
	var plain string
	if err := json.Unmarshal(raw, &plain); err == nil {
		return strings.TrimSpace(plain)
	}
	return ""
}

func NewRequest(ctx context.Context, method, url string, body []byte) (*http.Request, error) {
	return http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
}
