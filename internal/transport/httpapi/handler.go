package httpapi

import (
	"log/slog"
	"net/http"

	"look-backend/internal/service"
)

type Handler struct {
	service      *service.Service
	maxBodyBytes int64
	log          *slog.Logger
}

// NewHandler создаёт обработчики; maxBodyBytes ограничивает размер тела запроса.
func NewHandler(service *service.Service, maxBodyBytes int64, log *slog.Logger) *Handler {
	return &Handler{service: service, maxBodyBytes: maxBodyBytes, log: log}
}

// RegisterRoutes регистрирует маршруты API в mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/process", h.handleProcess)
	mux.HandleFunc("GET /api/v1/models", h.HandleModels)
	mux.HandleFunc("GET /healthz", h.HandleHealth)
}

// handleProcess принимает {"text": "..."} и возвращает ответ модели.
// Одиночный запрос без истории диалога.
func (h *Handler) handleProcess(w http.ResponseWriter, r *http.Request) {
	var req processRequest
	if !h.DecodeBody(w, r, &req) {
		return
	}

	result, err := h.service.Process(r.Context(), req.Text)
	if err != nil {
		h.log.Warn("обработка текста не удалась", "error", err)
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, okEnvelope{
		Status:    "ok",
		Model:     result.Model,
		Provider:  result.Provider,
		Answer:    result.Answer,
		ElapsedMS: result.ElapsedMS,
	})
}

// HandleModels возвращает псевдонимы моделей и доступных
// провайдеров с их моделями.
func (h *Handler) HandleModels(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, modelsResponse{
		Aliases:   h.service.Aliases(),
		Providers: h.service.Providers(),
	})
}
