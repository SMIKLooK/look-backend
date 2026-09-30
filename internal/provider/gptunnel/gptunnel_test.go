package gptunnel

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
		if r.URL.Path != "/chat/completions" {
			t.Errorf("неожиданный путь запроса: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("неожиданный Authorization: %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		// имя модели в любом регистре должно уйти точным ID из каталога
		if !strings.Contains(string(body), `"model":"gpt-4o"`) {
			t.Errorf("в запросе нет модели: %s", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"Ответ модели"},"finish_reason":"stop"}],"usage":{"total_cost":0.02}}`)
	}))
	defer srv.Close()

	client := New(Config{APIKey: "test-key", BaseURL: srv.URL, HTTPClient: srv.Client()})
	resp, err := client.Complete(context.Background(), domain.Request{
		Model:    "GPT-4O",
		Messages: []domain.Message{{Role: "user", Content: "вопрос"}},
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if resp.Content != "Ответ модели" || resp.Provider != "gptunnel" || resp.Model != "gpt-4o" {
		t.Errorf("неожиданный ответ: %+v", resp)
	}
}

func TestClient_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = io.WriteString(w, `{"error":{"message":"Insufficient funds","code":"insufficient_funds"}}`)
	}))
	defer srv.Close()

	client := New(Config{APIKey: "test-key", BaseURL: srv.URL, HTTPClient: srv.Client()})
	_, err := client.Complete(context.Background(), domain.Request{
		Model:    "gpt-4o",
		Messages: []domain.Message{{Role: "user", Content: "вопрос"}},
	})
	if err == nil {
		t.Fatal("ожидалась ошибка, получен успех")
	}
	if code := domain.ErrorCode(err); code != domain.CodeProviderFailed {
		t.Fatalf("ожидался код %s, получен %s (%v)", domain.CodeProviderFailed, code, err)
	}
	if !strings.Contains(err.Error(), "Insufficient funds") {
		t.Errorf("в ошибке нет сообщения провайдера: %v", err)
	}
}

func TestClient_Supports(t *testing.T) {
	client := New(Config{})
	if !client.Supports("gpt-4o") || !client.Supports("GPT-4O") {
		t.Error("модели из каталога должны поддерживаться в любом регистре")
	}
	if client.Supports("gemini-3.8-flash") || client.Supports("google/gemini-3.8-flash") {
		t.Error("чужие модели не должны поддерживаться")
	}
}
