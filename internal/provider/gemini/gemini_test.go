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

func TestClient_AssistantRoleBecomesModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"role":"model"`) {
			t.Errorf("роль assistant должна превратиться в model: %s", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"ок"}]}}]}`)
	}))
	defer srv.Close()

	client := New(Config{APIKey: "test-key", BaseURL: srv.URL, HTTPClient: srv.Client()})
	_, err := client.Complete(context.Background(), domain.Request{
		Model: "gemini-2.5-flash",
		Messages: []domain.Message{
			{Role: "user", Content: "вопрос"},
			{Role: "assistant", Content: "предыдущий ответ"},
		},
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestClient_MaxTokensInPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"maxOutputTokens":100`) {
			t.Errorf("в запросе нет лимита токенов: %s", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"ок"}]}}]}`)
	}))
	defer srv.Close()

	client := New(Config{APIKey: "test-key", BaseURL: srv.URL, HTTPClient: srv.Client(), MaxTokens: 100})
	if _, err := client.Complete(context.Background(), domain.Request{
		Model:    "gemini-2.5-flash",
		Messages: []domain.Message{{Role: "user", Content: "вопрос"}},
	}); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestClient_ParseErrors(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantText []string
	}{
		{
			name:     "блокировка запроса",
			body:     `{"promptFeedback":{"blockReason":"SAFETY"}}`,
			wantText: []string{"блокировал", "SAFETY"},
		},
		{
			name:     "пустой список кандидатов",
			body:     `{"candidates":[]}`,
			wantText: []string{"не вернул текст"},
		},
		{
			name:     "некорректный JSON",
			body:     `<html>ошибка</html>`,
			wantText: []string{"некорректный ответ", "HTTP 200"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, tt.body)
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
				t.Errorf("ожидался код %s, получен %s (%v)", domain.CodeProviderFailed, code, err)
			}
			for _, want := range tt.wantText {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("в ошибке нет %q: %v", want, err)
				}
			}
		})
	}
}

func TestClient_Models(t *testing.T) {
	client := New(Config{})
	models := client.Models()
	if len(models) == 0 {
		t.Fatal("без каталога должен использоваться встроенный список моделей")
	}
	models[0] = "испорчено"
	if client.Models()[0] == "испорчено" {
		t.Error("Models вернул не копию: мутации видны в клиенте")
	}
}

func TestClient_ProviderUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url, httpClient := srv.URL, srv.Client()
	srv.Close()

	client := New(Config{APIKey: "test-key", BaseURL: url, HTTPClient: httpClient})
	_, err := client.Complete(context.Background(), domain.Request{
		Model:    "gemini-2.5-flash",
		Messages: []domain.Message{{Role: "user", Content: "вопрос"}},
	})
	if code := domain.ErrorCode(err); code != domain.CodeProviderFailed {
		t.Errorf("ожидался код %s, получен %s (%v)", domain.CodeProviderFailed, code, err)
	}
}
