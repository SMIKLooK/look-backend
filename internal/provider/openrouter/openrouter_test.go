package openrouter

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
		// короткое имя "gpt-5.6-terra" должно развернуться в полный слаг
		if !strings.Contains(string(body), `"model":"openai/gpt-5.6-terra"`) {
			t.Errorf("в запросе нет полного слага модели: %s", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"Ответ модели"}}]}`)
	}))
	defer srv.Close()

	client := New(Config{
		APIKey:     "test-key",
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
	})
	resp, err := client.Complete(context.Background(), domain.Request{
		Model:    "gpt-5.6-terra",
		Messages: []domain.Message{{Role: "user", Content: "вопрос"}},
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if resp.Content != "Ответ модели" || resp.Provider != "openrouter" {
		t.Errorf("неожиданный ответ: %+v", resp)
	}
	if resp.Model != "openai/gpt-5.6-terra" {
		t.Errorf("в ответе ожидался полный слаг, получено %q", resp.Model)
	}
}

func TestClient_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"No auth credentials found","code":401}}`)
	}))
	defer srv.Close()

	client := New(Config{APIKey: "bad", BaseURL: srv.URL, HTTPClient: srv.Client()})
	_, err := client.Complete(context.Background(), domain.Request{
		Model:    "openai/gpt-5.6-terra",
		Messages: []domain.Message{{Role: "user", Content: "вопрос"}},
	})
	if err == nil {
		t.Fatal("ожидалась ошибка, получен успех")
	}
	if code := domain.ErrorCode(err); code != domain.CodeProviderFailed {
		t.Fatalf("ожидался код %s, получен %s (%v)", domain.CodeProviderFailed, code, err)
	}
	if !strings.Contains(err.Error(), "No auth credentials found") {
		t.Errorf("в ошибке нет сообщения провайдера: %v", err)
	}
}

// TestClient_WAFBlock проверяет, что отказ WAF OpenRouter (403 с error-строкой
// вместо объекта) превращается в понятную ошибку с подсказкой о прокси.
func TestClient_WAFBlock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{ "success": false, "error": "Access denied by security policy." }`)
	}))
	defer srv.Close()

	client := New(Config{APIKey: "test-key", BaseURL: srv.URL, HTTPClient: srv.Client()})
	_, err := client.Complete(context.Background(), domain.Request{
		Model:    "openrouter/free",
		Messages: []domain.Message{{Role: "user", Content: "вопрос"}},
	})
	if err == nil {
		t.Fatal("ожидалась ошибка, получен успех")
	}
	if code := domain.ErrorCode(err); code != domain.CodeProviderFailed {
		t.Fatalf("ожидался код %s, получен %s (%v)", domain.CodeProviderFailed, code, err)
	}
	for _, want := range []string{"WAF", "OpenRouterBaseURL"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("в ошибке нет подсказки %q: %v", want, err)
		}
	}
}

func TestClient_Supports(t *testing.T) {
	client := New(Config{})
	if !client.Supports("google/gemini-3.8-flash") {
		t.Error("слаг vendor/model должен поддерживаться")
	}
	if !client.Supports("GEMINI-3.8-FLASH") {
		t.Error("короткое имя из списка Models должно поддерживаться")
	}
	if !client.Supports("glm-5.3-flash") {
		t.Error("короткое имя z-ai/glm-5.3-flash должно поддерживаться")
	}
	if client.Supports("gpt-4o") {
		t.Error("неизвестное короткое имя не должно поддерживаться")
	}
}

func TestClient_ResolveSlug(t *testing.T) {
	client := New(Config{})
	cases := []struct {
		in, want string
	}{
		{"google/gemini-3.8-flash", "google/gemini-3.8-flash"},
		{"gemini-3.8-flash", "google/gemini-3.8-flash"},
		{"GEMINI-3.8-FLASH", "google/gemini-3.8-flash"},
		{"unknown/model", "unknown/model"},
		{"unknown", "unknown"},
	}
	for _, tc := range cases {
		if got := client.resolveSlug(tc.in); got != tc.want {
			t.Errorf("resolveSlug(%q) = %q, ожидалось %q", tc.in, got, tc.want)
		}
	}
}
