package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"look-backend/internal/domain"
	"look-backend/internal/provider"
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
	mux.HandleFunc("POST /api/v1/chat", h.handleChat)
	mux.HandleFunc("GET /api/v1/chat/{session_id}", h.handleChatHistory)
	mux.HandleFunc("DELETE /api/v1/chat/{session_id}", h.handleChatReset)
	mux.HandleFunc("GET /api/v1/models", h.handleModels)
	mux.HandleFunc("GET /healthz", h.handleHealth)
}

type processRequest struct {
	Text string `json:"text"`
}

// handleProcess принимает {"text": "..."} и возвращает ответ модели.
// Одиночный запрос без истории диалога.
func (h *Handler) handleProcess(w http.ResponseWriter, r *http.Request) {
	var req processRequest
	if !h.decodeBody(w, r, &req) {
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

type chatRequest struct {
	SessionID string `json:"session_id"` // пусто — сервер создаст новую сессию
	Text      string `json:"text"`       // "<модель> <запрос>"
}

type chatEnvelope struct {
	Status    string `json:"status"`
	SessionID string `json:"session_id"`
	Model     string `json:"model"`
	Provider  string `json:"provider"`
	Answer    string `json:"answer"`
	ElapsedMS int64  `json:"elapsed_ms"`
	Messages  int    `json:"messages"` // сколько сообщений ушло модели вместе с историей
}

// handleChat принимает {"session_id": "...", "text": "<модель> <запрос>"}:
// модели уходят предыдущие сообщения сессии плюс новый запрос, ответ и
// запрос сохраняются в историю.
func (h *Handler) handleChat(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	if !h.decodeBody(w, r, &req) {
		return
	}

	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		sessionID = newSessionID()
	}

	result, err := h.service.Chat(r.Context(), sessionID, req.Text)
	if err != nil {
		h.log.Warn("диалоговый запрос не удался", "error", err, "session_id", sessionID)
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, chatEnvelope{
		Status:    "ok",
		SessionID: result.SessionID,
		Model:     result.Model,
		Provider:  result.Provider,
		Answer:    result.Answer,
		ElapsedMS: result.ElapsedMS,
		Messages:  result.Messages,
	})
}

type historyEnvelope struct {
	Status    string           `json:"status"`
	SessionID string           `json:"session_id"`
	Messages  []domain.Message `json:"messages"`
}

// handleChatHistory возвращает сохранённые сообщения сессии.
func (h *Handler) handleChatHistory(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("session_id")

	messages, err := h.service.History(sessionID)
	if err != nil {
		h.log.Warn("получение истории не удалось", "error", err, "session_id", sessionID)
		writeError(w, err)
		return
	}
	if messages == nil {
		messages = []domain.Message{}
	}

	writeJSON(w, http.StatusOK, historyEnvelope{
		Status:    "ok",
		SessionID: sessionID,
		Messages:  messages,
	})
}

// handleChatReset очищает историю сессии.
func (h *Handler) handleChatReset(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("session_id")

	if err := h.service.Reset(sessionID); err != nil {
		h.log.Warn("очистка истории не удалась", "error", err, "session_id", sessionID)
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":     "ok",
		"session_id": sessionID,
	})
}

// decodeBody читает и разбирает JSON-тело запроса; при ошибке сама
// формирует ответ и возвращает false.
func (h *Handler) decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
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

// newSessionID генерирует идентификатор сессии, если клиент не прислал свой.
func newSessionID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return time.Now().UTC().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(b)
}

type modelsResponse struct {
	Keywords  []string          `json:"keywords"`
	Aliases   map[string]string `json:"aliases"`
	Providers []provider.Info   `json:"providers"`
}

// handleModels возвращает ключевые слова, псевдонимы моделей
// и доступных провайдеров с моделями.
func (h *Handler) handleModels(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, modelsResponse{
		Keywords:  h.service.Keywords(),
		Aliases:   h.service.Aliases(),
		Providers: h.service.Providers(),
	})
}

// handleHealth — проверка живости сервиса.
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
