// Package echo — тестовый провайдер: возвращает запрос как есть.
// Нужен для локальной разработки и проверки API без реальных API-ключей.
package echo

import (
	"context"
	"strings"

	"look-backend/internal/domain"
)

type Echo struct{}

func New() *Echo { return &Echo{} }

func (e *Echo) Name() string { return "echo" }

func (e *Echo) Models() []string { return []string{"echo"} }

func (e *Echo) Supports(model string) bool {
	m := strings.ToLower(model)
	return m == "echo" || m == "test"
}

func (e *Echo) Complete(ctx context.Context, req domain.Request) (domain.Response, error) {
	if err := ctx.Err(); err != nil {
		return domain.Response{}, domain.WrapError(domain.CodeTimeout, err, "контекст отменён до ответа echo")
	}
	parts := make([]string, 0, len(req.Messages))
	for _, m := range req.Messages {
		parts = append(parts, m.Content)
	}
	return domain.Response{
		Model:    req.Model,
		Provider: e.Name(),
		Content:  "echo → " + strings.Join(parts, "\n"),
	}, nil
}
