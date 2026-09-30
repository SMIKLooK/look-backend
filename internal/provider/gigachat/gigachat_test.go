package gigachat

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"look-backend/internal/domain"
)

// newTestServer поднимает заглушку с OAuth-эндпоинтом и чатом; возвращает
// сервер, счётчики вызовов и клиента, смотрящего на заглушку.
func newTestServer(t *testing.T, chatHandler http.HandlerFunc) (*httptest.Server, *atomic.Int64, *atomic.Int64, *Client) {
	t.Helper()
	var oauthCalls, chatCalls atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v2/oauth", func(w http.ResponseWriter, r *http.Request) {
		oauthCalls.Add(1)
		if got := r.Header.Get("RqUID"); len(got) != 36 || got[14] != '4' {
			t.Errorf("ожидался uuid4 в RqUID, получено %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Basic "+base64.StdEncoding.EncodeToString([]byte("id:secret")) {
			t.Errorf("неожиданный Authorization: %q", got)
		}
		if err := r.ParseForm(); err != nil || r.Form.Get("scope") != "GIGACHAT_API_PERS" {
			t.Errorf("ожидался scope GIGACHAT_API_PERS, получено %v", r.Form)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"token-1","expires_at":0}`)
	})
	mux.HandleFunc("POST /api/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		chatCalls.Add(1)
		chatHandler(w, r)
	})
	srv := httptest.NewServer(mux)
	client := New(Config{
		ClientID:     "id",
		ClientSecret: "secret",
		BaseURL:      srv.URL + "/api/v1",
		AuthURL:      srv.URL + "/api/v2/oauth",
		HTTPClient:   srv.Client(),
	})
	return srv, &oauthCalls, &chatCalls, client
}

func TestClient_Complete(t *testing.T) {
	srv, oauthCalls, chatCalls, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token-1" {
			t.Errorf("неожиданный Authorization: %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"model":"GigaChat"`) {
			t.Errorf("в запросе нет модели: %s", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"Ответ модели"},"finish_reason":"stop"}]}`)
	})
	defer srv.Close()

	resp, err := client.Complete(context.Background(), domain.Request{
		Model:    "gigachat",
		Messages: []domain.Message{{Role: "user", Content: "вопрос"}},
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if resp.Content != "Ответ модели" || resp.Provider != "gigachat" || resp.Model != "GigaChat" {
		t.Errorf("неожиданный ответ: %+v", resp)
	}
	if oauthCalls.Load() != 1 || chatCalls.Load() != 1 {
		t.Errorf("ожидался 1 запрос токена и 1 запрос чата, получено %d и %d", oauthCalls.Load(), chatCalls.Load())
	}
}

func TestClient_TokenReuse(t *testing.T) {
	srv, oauthCalls, chatCalls, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ок"}}]}`)
	})
	defer srv.Close()

	for i := 0; i < 3; i++ {
		if _, err := client.Complete(context.Background(), domain.Request{
			Model:    "GigaChat",
			Messages: []domain.Message{{Role: "user", Content: "вопрос"}},
		}); err != nil {
			t.Fatalf("неожиданная ошибка на вызове %d: %v", i+1, err)
		}
	}
	// токен жив ~30 минут, поэтому OAuth должен вызваться один раз
	if oauthCalls.Load() != 1 || chatCalls.Load() != 3 {
		t.Errorf("ожидались 1 запрос токена и 3 запроса чата, получено %d и %d", oauthCalls.Load(), chatCalls.Load())
	}
}

func TestClient_401RefreshesTokenAndRetries(t *testing.T) {
	var chats atomic.Int64
	srv, oauthCalls, _, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if chats.Add(1) == 1 {
			// первый запрос чата — сервер считает токен просроченным
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": 401, "description": "Token expired"}})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"после обновления"}}]}`)
	})
	defer srv.Close()

	resp, err := client.Complete(context.Background(), domain.Request{
		Model:    "GigaChat",
		Messages: []domain.Message{{Role: "user", Content: "вопрос"}},
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if resp.Content != "после обновления" {
		t.Errorf("неожиданный ответ: %+v", resp)
	}
	// oauth: начальный + после 401; чат: неудачный + удачный
	if oauthCalls.Load() != 2 || chats.Load() != 2 {
		t.Errorf("ожидались 2 запроса токена и 2 запроса чата, получено %d и %d", oauthCalls.Load(), chats.Load())
	}
}

func TestClient_Supports(t *testing.T) {
	client := New(Config{})
	if !client.Supports("GigaChat") || !client.Supports("gigachat-pro") || !client.Supports("GigaChat-2-Max") {
		t.Error("модели с префиксом gigachat должны поддерживаться в любом регистре")
	}
	if client.Supports("gpt-4o") || client.Supports("google/gemini-3.8-flash") {
		t.Error("чужие модели не должны поддерживаться")
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

func TestClient_Second401IsFatal(t *testing.T) {
	srv, oauthCalls, chatCalls, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"description":"Token expired"}}`)
	})
	defer srv.Close()

	_, err := client.Complete(context.Background(), domain.Request{
		Model:    "GigaChat",
		Messages: []domain.Message{{Role: "user", Content: "вопрос"}},
	})
	if err == nil {
		t.Fatal("ожидалась ошибка, получен успех")
	}
	if !strings.Contains(err.Error(), "даже с обновлённым токеном") {
		t.Errorf("в ошибке нет объяснения про обновлённый токен: %v", err)
	}
	if oauthCalls.Load() != 2 || chatCalls.Load() != 2 {
		t.Errorf("ожидались 2 запроса токена и 2 запроса чата, получено %d и %d", oauthCalls.Load(), chatCalls.Load())
	}
}

func TestClient_ChatParseErrors(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		status   int
		wantText string
	}{
		{name: "некорректный JSON", body: `<html>ошибка</html>`, status: 200, wantText: "некорректный ответ"},
		{name: "пустой список choices", body: `{"choices":[]}`, status: 200, wantText: "пустой список choices"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			})
			_, err := client.Complete(context.Background(), domain.Request{
				Model:    "GigaChat",
				Messages: []domain.Message{{Role: "user", Content: "вопрос"}},
			})
			if err == nil {
				t.Fatal("ожидалась ошибка, получен успех")
			}
			if !strings.Contains(err.Error(), tt.wantText) {
				t.Errorf("в ошибке нет %q: %v", tt.wantText, err)
			}
		})
	}
}

func TestClient_OAuthErrors(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		wantText string
	}{
		{name: "ключ отклонён", status: 401, body: `{"error":{"description":"bad key"}}`, wantText: "проверьте ключ"},
		{name: "прочая ошибка", status: 500, body: `{"error":{"description":"boom"}}`, wantText: "ошибка авторизации"},
		{name: "некорректный JSON", status: 200, body: `<html>ошибка</html>`, wantText: "некорректный ответ"},
		{name: "пустой access_token", status: 200, body: `{"access_token":""}`, wantText: "не вернул access_token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("POST /api/v2/oauth", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			})
			srv := httptest.NewServer(mux)
			defer srv.Close()

			client := New(Config{
				ClientID:     "id",
				ClientSecret: "secret",
				BaseURL:      srv.URL + "/api/v1",
				AuthURL:      srv.URL + "/api/v2/oauth",
				HTTPClient:   srv.Client(),
			})
			_, err := client.Complete(context.Background(), domain.Request{
				Model:    "GigaChat",
				Messages: []domain.Message{{Role: "user", Content: "вопрос"}},
			})
			if err == nil {
				t.Fatal("ожидалась ошибка, получен успех")
			}
			if !strings.Contains(err.Error(), tt.wantText) {
				t.Errorf("в ошибке нет %q: %v", tt.wantText, err)
			}
		})
	}
}

func TestClient_ExpiredTokenNotCached(t *testing.T) {
	expired := time.Now().Add(-time.Minute).UnixMilli()
	var oauthCalls, chatCalls atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v2/oauth", func(w http.ResponseWriter, r *http.Request) {
		oauthCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"access_token":"токен-%d","expires_at":%d}`, oauthCalls.Load(), expired)
	})
	mux.HandleFunc("POST /api/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		chatCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ок"}}]}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := New(Config{
		ClientID:     "id",
		ClientSecret: "secret",
		BaseURL:      srv.URL + "/api/v1",
		AuthURL:      srv.URL + "/api/v2/oauth",
		HTTPClient:   srv.Client(),
	})
	for i := 0; i < 2; i++ {
		if _, err := client.Complete(context.Background(), domain.Request{
			Model:    "GigaChat",
			Messages: []domain.Message{{Role: "user", Content: "вопрос"}},
		}); err != nil {
			t.Fatalf("неожиданная ошибка на вызове %d: %v", i+1, err)
		}
	}
	if oauthCalls.Load() != 2 || chatCalls.Load() != 2 {
		t.Errorf("ожидались 2 запроса токена и 2 запроса чата, получено %d и %d", oauthCalls.Load(), chatCalls.Load())
	}
}

func TestClient_CompleteWithoutKey(t *testing.T) {
	client := New(Config{})
	_, err := client.Complete(context.Background(), domain.Request{
		Model:    "GigaChat",
		Messages: []domain.Message{{Role: "user", Content: "вопрос"}},
	})
	if err == nil {
		t.Fatal("ожидалась ошибка, получен успех")
	}
	if !strings.Contains(err.Error(), "не задан") {
		t.Errorf("в ошибке должно быть объяснение про ключ: %v", err)
	}
}

func TestClient_CompleteCanceledContext(t *testing.T) {
	client := New(Config{APIKey: "ключ"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.Complete(ctx, domain.Request{
		Model:    "GigaChat",
		Messages: []domain.Message{{Role: "user", Content: "вопрос"}},
	})
	if code := domain.ErrorCode(err); code != domain.CodeTimeout {
		t.Errorf("ожидался код %s, получен %s (%v)", domain.CodeTimeout, code, err)
	}
}

func TestClient_ProviderUnavailable(t *testing.T) {
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"токен","expires_at":0}`)
	}))
	defer oauth.Close()

	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL, deadClient := dead.URL, dead.Client()
	dead.Close()

	client := New(Config{
		ClientID:     "id",
		ClientSecret: "secret",
		BaseURL:      deadURL,
		AuthURL:      oauth.URL,
		HTTPClient:   deadClient,
	})
	_, err := client.Complete(context.Background(), domain.Request{
		Model:    "GigaChat",
		Messages: []domain.Message{{Role: "user", Content: "вопрос"}},
	})
	if code := domain.ErrorCode(err); code != domain.CodeProviderFailed {
		t.Errorf("ожидался код %s, получен %s (%v)", domain.CodeProviderFailed, code, err)
	}
}
