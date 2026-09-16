package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"look-backend/internal/parser"
	"look-backend/internal/provider"
	"look-backend/internal/provider/echo"
	"look-backend/internal/service"
	"look-backend/internal/storage"
)

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	reg := provider.NewRegistry(echo.New())
	svc := service.New(parser.New(), reg, storage.NewMemoryStore(time.Hour, time.Minute), 5*time.Second, 50)
	h := NewHandler(svc, 1<<20, slog.New(slog.NewTextHandler(io.Discard, nil)))
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

func TestHandler_ProcessOK(t *testing.T) {
	h := newTestHandler(t)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/process", strings.NewReader(`{"text":"лук echo привет"}`))
	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("ожидался 200, получен %d: %s", resp.Code, resp.Body.String())
	}
	var got struct {
		Status   string `json:"status"`
		Model    string `json:"model"`
		Provider string `json:"provider"`
		Answer   string `json:"answer"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("не удалось разобрать ответ: %v", err)
	}
	if got.Status != "ok" || got.Provider != "echo" || got.Model != "echo" || !strings.Contains(got.Answer, "привет") {
		t.Fatalf("неожиданный ответ: %+v", got)
	}
}

func TestHandler_ModelNotSpecified(t *testing.T) {
	h := newTestHandler(t)

	// текст только из ключевого слова старого формата — модели нет
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/process", strings.NewReader(`{"text":"look"}`))
	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("ожидался 400, получен %d: %s", resp.Code, resp.Body.String())
	}
	var got struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("не удалось разобрать ответ: %v", err)
	}
	if got.Error.Code != "MODEL_NOT_SPECIFIED" {
		t.Fatalf("ожидался код MODEL_NOT_SPECIFIED, получен %q", got.Error.Code)
	}
}

func TestHandler_InvalidJSON(t *testing.T) {
	h := newTestHandler(t)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/process", strings.NewReader(`{`))
	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("ожидался 400, получен %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "INVALID_JSON") {
		t.Fatalf("ожидался код INVALID_JSON, получено: %s", resp.Body.String())
	}
}

func TestHandler_Health(t *testing.T) {
	h := newTestHandler(t)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("ожидался 200, получен %d: %s", resp.Code, resp.Body.String())
	}
}

func TestHandler_ChatFlow(t *testing.T) {
	h := newTestHandler(t)
	chat := func(t *testing.T, body string) map[string]any {
		t.Helper()
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/chat", strings.NewReader(body))
		h.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("ожидался 200, получен %d: %s", resp.Code, resp.Body.String())
		}
		var got map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
			t.Fatalf("не удалось разобрать ответ: %v", err)
		}
		return got
	}

	// первый ход: session_id генерируется сервером
	first := chat(t, `{"text":"echo раз"}`)
	if first["status"] != "ok" || first["session_id"] == "" {
		t.Fatalf("неожиданный первый ответ: %v", first)
	}
	if first["messages"].(float64) != 1 {
		t.Fatalf("первый ход должен содержать 1 сообщение: %v", first)
	}
	sid := first["session_id"].(string)

	// второй ход той же сессии: модель видит историю (1+2=3 сообщения)
	second := chat(t, `{"session_id":"`+sid+`","text":"echo два"}`)
	if second["session_id"] != sid {
		t.Fatalf("session_id не сохранился: %v", second)
	}
	if second["messages"].(float64) != 3 {
		t.Fatalf("второй ход должен содержать 3 сообщения: %v", second)
	}

	// история сессии
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/chat/"+sid, nil)
	h.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("ожидался 200, получен %d: %s", resp.Code, resp.Body.String())
	}
	var history struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &history); err != nil {
		t.Fatalf("не удалось разобрать историю: %v", err)
	}
	if len(history.Messages) != 4 ||
		history.Messages[0].Role != "user" || history.Messages[1].Role != "assistant" ||
		history.Messages[2].Role != "user" || history.Messages[3].Role != "assistant" {
		t.Fatalf("неожиданная история: %+v", history)
	}

	// сброс и проверка, что история опустела
	resp = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/chat/"+sid, nil)
	h.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("ожидался 200, получен %d: %s", resp.Code, resp.Body.String())
	}
	resp = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/chat/"+sid, nil)
	h.ServeHTTP(resp, req)
	if err := json.Unmarshal(resp.Body.Bytes(), &history); err != nil {
		t.Fatalf("не удалось разобрать историю: %v", err)
	}
	if len(history.Messages) != 0 {
		t.Fatalf("после сброса история должна быть пуста: %+v", history)
	}
}
