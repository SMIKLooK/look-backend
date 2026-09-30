package domain

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestNewError_Error(t *testing.T) {
	e := NewError(CodeUnknownModel, "неизвестная модель: xyz")
	if got := e.Error(); got != "неизвестная модель: xyz" {
		t.Errorf("Error() = %q, ожидалось сообщение без причины", got)
	}
	if e.Unwrap() != nil {
		t.Error("у ошибки без причины Unwrap должен возвращать nil")
	}
}

func TestWrapError_ErrorAndUnwrap(t *testing.T) {
	cause := errors.New("connection refused")
	e := WrapError(CodeProviderFailed, cause, "%s недоступен", "gemini")

	if !strings.Contains(e.Error(), "gemini недоступен") || !strings.Contains(e.Error(), "connection refused") {
		t.Errorf("Error() должен содержать контекст и причину, получено %q", e.Error())
	}
	if !errors.Is(e, cause) {
		t.Error("Unwrap должен возвращать исходную ошибку (errors.Is не сработал)")
	}
}

func TestErrorCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want Code
	}{
		{
			name: "доменная ошибка",
			err:  NewError(CodeEmptyText, "пусто"),
			want: CodeEmptyText,
		},
		{
			name: "обёрнутая доменная ошибка",
			err:  fmt.Errorf("контекст: %w", NewError(CodeTimeout, "таймаут")),
			want: CodeTimeout,
		},
		{
			name: "посторонняя ошибка считается внутренней",
			err:  errors.New("что-то сломалось"),
			want: CodeInternal,
		},
		{
			name: "nil считается внутренней",
			err:  nil,
			want: CodeInternal,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ErrorCode(tt.err); got != tt.want {
				t.Errorf("ErrorCode = %s, ожидалось %s", got, tt.want)
			}
		})
	}
}
