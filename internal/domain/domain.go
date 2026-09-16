// Package domain содержит общие типы и ошибки, которые используются
// всеми слоями приложения: парсером, сервисом, провайдерами и транспортом.
package domain

import (
	"errors"
	"fmt"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Request struct {
	Model    string
	Messages []Message
}

type Response struct {
	Model    string
	Provider string
	Content  string
}

// Code — машиночитаемый код ошибки; попадает в API-ответ клиенту
// и определяет HTTP-статус.
type Code string

const (
	CodeEmptyText      Code = "EMPTY_TEXT"
	CodeModelMissing   Code = "MODEL_NOT_SPECIFIED"
	CodePromptMissing  Code = "PROMPT_NOT_SPECIFIED"
	CodeUnknownModel   Code = "UNKNOWN_MODEL"
	CodeInvalidJSON    Code = "INVALID_JSON"
	CodeProviderFailed Code = "PROVIDER_ERROR"
	CodeTimeout        Code = "TIMEOUT"
	CodeInternal       Code = "INTERNAL"
)

// Error — доменная ошибка с кодом; HTTP-слой по коду выбирает статус ответа.
type Error struct {
	Code    Code
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Err }

// NewError создаёт доменную ошибку без нижележащей причины.
func NewError(code Code, message string) *Error {
	return &Error{Code: code, Message: message}
}

// WrapError оборачивает нижележащую ошибку, добавляя код и контекст.
func WrapError(code Code, err error, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...), Err: err}
}

// ErrorCode возвращает код ошибки; для ошибок без кода считается внутренней.
func ErrorCode(err error) Code {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return CodeInternal
}
