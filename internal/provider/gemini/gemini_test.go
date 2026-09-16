package gemini

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"look-backend/internal/domain"
)

func TestClient_Complete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/models/gemini-2.5-flash:generateContent") {
			t.Errorf("неожиданный путь запроса: %s", r.URL.Path)
		}
		if got := r.Header.Get("x-goog-api-key"); got != "test-key" {
			t.Errorf("неожиданный API-ключ: %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"text":"вопрос"`) {
			t.Errorf("в запросе нет текста пользователя: %s", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"Ответ модели"}]}}]}`)
	}))
	defer srv.Close()

	client := New(Config{APIKey: "test-key", BaseURL: srv.URL, HTTPClient: srv.Client()})
	resp, err := client.Complete(context.Background(), domain.Request{
		Model:    "gemini-2.5-flash",
		Messages: []domain.Message{{Role: "user", Content: "вопрос"}},
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if resp.Content != "Ответ модели" || resp.Provider != "gemini" || resp.Model != "gemini-2.5-flash" {
		t.Errorf("неожиданный ответ: %+v", resp)
	}
}

func TestClient_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"code":429,"message":"quota exceeded","status":"RESOURCE_EXHAUSTED"}}`)
	}))
	defer srv.Close()

	client := New(Config{APIKey: "test-key", BaseURL: srv.URL, HTTPClient: srv.Client()})
	_, err := client.Complete(context.Background(), domain.Request{
		Model:    "gemini-2.5-flash",
		Messages: []domain.Message{{Role: "user", Content: "вопрос"}},
	})
	if err == nil {
		t.Fatal("ожидалась ошибка, получен успех")
	}
	if code := domain.ErrorCode(err); code != domain.CodeProviderFailed {
		t.Fatalf("ожидался код %s, получен %s (%v)", domain.CodeProviderFailed, code, err)
	}
	if !strings.Contains(err.Error(), "quota exceeded") {
		t.Errorf("в ошибке нет сообщения провайдера: %v", err)
	}
}

func TestClient_Supports(t *testing.T) {
	client := New(Config{})
	if !client.Supports("gemini-2.5-flash") || !client.Supports("GEMINI-2.0-FLASH") {
		t.Error("ожидалось, что модели gemini-* поддерживаются")
	}
	if client.Supports("gpt-4o") || client.Supports("claude-3") {
		t.Error("чужие модели не должны поддерживаться")
	}
}
