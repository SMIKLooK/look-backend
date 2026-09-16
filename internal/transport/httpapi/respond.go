// Package httpapi — транспортный слой: HTTP-обработчики, JSON-ответы,
// middleware и сервер. Знает только о service и domain.
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"look-backend/internal/domain"
)

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorEnvelope struct {
	Status string    `json:"status"`
	Error  errorBody `json:"error"`
}

// okEnvelope — успешный ответ /api/v1/process.
type okEnvelope struct {
	Status    string `json:"status"`
	Model     string `json:"model"`
	Provider  string `json:"provider"`
	Answer    string `json:"answer"`
	ElapsedMS int64  `json:"elapsed_ms"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError превращает ошибку в JSON-ответ; HTTP-статус выбирается
// по коду доменной ошибки.
func writeError(w http.ResponseWriter, err error) {
	code := domain.ErrorCode(err)
	msg := err.Error()
	var domErr *domain.Error
	if errors.As(err, &domErr) {
		msg = domErr.Message
	}
	writeJSON(w, statusByCode(code), errorEnvelope{
		Status: "error",
		Error:  errorBody{Code: string(code), Message: msg},
	})
}

func statusByCode(code domain.Code) int {
	switch code {
	case domain.CodeInvalidJSON, domain.CodeEmptyText,
		domain.CodeModelMissing, domain.CodePromptMissing, domain.CodeUnknownModel:
		return http.StatusBadRequest
	case domain.CodeTimeout:
		return http.StatusGatewayTimeout
	case domain.CodeProviderFailed:
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}
