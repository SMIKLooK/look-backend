package httpapi

import (
	"encoding/json"
	"errors"

	"look-backend/internal/domain"
	"net/http"
)

// decodeBody читает и разбирает JSON-тело запроса; при ошибке сама
// формирует ответ и возвращает false.
func (h *Handler) DecodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge, errorEnvelope{
				Status: "error",
				Error:  errorBody{Code: "PAYLOAD_TOO_LARGE", Message: "тело запроса больше допустимого размера"},
			})
			return false
		}
		writeError(w, domain.NewError(domain.CodeInvalidJSON, "ожидается корректный JSON-тело запроса"))
		return false
	}
	return true
}
