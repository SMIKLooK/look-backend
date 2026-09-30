package kit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"look-backend/internal/domain"
)

func TestNewHTTPClient(t *testing.T) {
	if c := NewHTTPClient(5*time.Second, nil); c.Timeout != 5*time.Second {
		t.Errorf("таймаут должен попасть в клиент, получено %v", c.Timeout)
	}
	if c := NewHTTPClient(0, nil); c.Timeout != 0 {
		t.Errorf("нулевой таймаут должен остаться нулевым, получено %v", c.Timeout)
	}
	custom := &http.Client{Timeout: 7 * time.Second}
	if c := NewHTTPClient(5*time.Second, custom); c != custom {
		t.Error("подменённый клиент должен возвращаться как есть")
	}
}

func TestDo_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, "привет")
	}))
	defer srv.Close()

	req, err := NewRequest(context.Background(), http.MethodPost, srv.URL, []byte("{}"))
	if err != nil {
		t.Fatalf("не удалось создать запрос: %v", err)
	}
	result, err := Do(context.Background(), srv.Client(), "тест", req)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if result.StatusCode != http.StatusCreated || string(result.Body) != "привет" {
		t.Errorf("неожиданный результат: %d %q", result.StatusCode, string(result.Body))
	}
}

func TestDo_ContextCanceledBeforeRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req, err := NewRequest(ctx, http.MethodGet, "http://example.invalid", nil)
	if err != nil {
		t.Fatalf("не удалось создать запрос: %v", err)
	}
	_, err = Do(ctx, http.DefaultClient, "тест", req)
	if code := domain.ErrorCode(err); code != domain.CodeTimeout {
		t.Errorf("ожидался код %s, получен %s (%v)", domain.CodeTimeout, code, err)
	}
}

func TestDo_ClientTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}))
	defer srv.Close()

	req, err := NewRequest(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("не удалось создать запрос: %v", err)
	}
	client := NewHTTPClient(30*time.Millisecond, nil)
	_, err = Do(context.Background(), client, "тест", req)
	if code := domain.ErrorCode(err); code != domain.CodeTimeout {
		t.Errorf("ожидался код %s, получен %s (%v)", domain.CodeTimeout, code, err)
	}
}

func TestDo_ConnectionRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	client := srv.Client()
	srv.Close()

	req, err := NewRequest(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("не удалось создать запрос: %v", err)
	}
	_, err = Do(context.Background(), client, "тест", req)
	if code := domain.ErrorCode(err); code != domain.CodeProviderFailed {
		t.Errorf("ожидался код %s, получен %s (%v)", domain.CodeProviderFailed, code, err)
	}
}

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(errReader{}),
		Header:     http.Header{},
	}, nil
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("обрыв соединения") }
func (errReader) Close() error             { return nil }

func TestDo_BodyReadError(t *testing.T) {
	req, err := NewRequest(context.Background(), http.MethodGet, "http://example.invalid", nil)
	if err != nil {
		t.Fatalf("не удалось создать запрос: %v", err)
	}
	_, err = Do(context.Background(), &http.Client{Transport: failingTransport{}}, "тест", req)
	if code := domain.ErrorCode(err); code != domain.CodeProviderFailed {
		t.Errorf("ожидался код %s, получен %s (%v)", domain.CodeProviderFailed, code, err)
	}
	if !strings.Contains(err.Error(), "не удалось прочитать ответ") {
		t.Errorf("в ошибке должен быть контекст чтения тела: %v", err)
	}
}

func TestDo_TruncatesHugeBody(t *testing.T) {
	huge := bytes.Repeat([]byte("a"), MaxResponseBytes+1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(huge)
	}))
	defer srv.Close()

	req, err := NewRequest(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("не удалось создать запрос: %v", err)
	}
	result, err := Do(context.Background(), srv.Client(), "тест", req)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(result.Body) != MaxResponseBytes {
		t.Errorf("тело должно обрезаться до %d байт, получено %d", MaxResponseBytes, len(result.Body))
	}
}

func TestTruncateBody(t *testing.T) {
	long := strings.Repeat("a", 600)
	got := TruncateBody([]byte(long))
	if len(got) != 512+3 || !strings.HasSuffix(got, "...") {
		t.Errorf("длинное тело должно обрезаться до 512 байт с многоточием, получено %d байт", len(got))
	}
	if got := TruncateBody([]byte("коротко")); got != "коротко" {
		t.Errorf("короткое тело должно оставаться без изменений, получено %q", got)
	}
}

func TestContainsFoldAndResolveFold(t *testing.T) {
	models := []string{"GigaChat", "GigaChat-Pro"}
	if !ContainsFold(models, "gigachat") || ContainsFold(models, "gpt-4o") {
		t.Error("ContainsFold должен находить модель без учёта регистра и не находить чужие")
	}
	if got := ResolveFold(models, "GIGACHAT-PRO"); got != "GigaChat-Pro" {
		t.Errorf("ResolveFold должен возвращать каноническое имя, получено %q", got)
	}
	if got := ResolveFold(models, "неизвестная"); got != "неизвестная" {
		t.Errorf("ResolveFold должен возвращать модель как есть, получено %q", got)
	}
}

func TestMarshalChat(t *testing.T) {
	body, err := MarshalChat("gpt-4o", []domain.Message{
		{Role: "user", Content: "вопрос"},
		{Role: "assistant", Content: "ответ"},
	}, 0)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("тело должно быть корректным JSON: %v", err)
	}
	if parsed["model"] != "gpt-4o" {
		t.Errorf("model = %v, ожидалось gpt-4o", parsed["model"])
	}
	if _, ok := parsed["max_tokens"]; ok {
		t.Error("max_tokens=0 не должен попадать в запрос")
	}
	messages, ok := parsed["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("ожидалось 2 сообщения, получено %v", parsed["messages"])
	}
	first := messages[0].(map[string]any)
	if first["role"] != "user" || first["content"] != "вопрос" {
		t.Errorf("неожиданное первое сообщение: %v", first)
	}

	body, err = MarshalChat("m", nil, 512)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(string(body), `"max_tokens":512`) {
		t.Errorf("max_tokens должен попасть в запрос: %s", string(body))
	}
}

func TestParseChat(t *testing.T) {
	t.Run("успешный ответ", func(t *testing.T) {
		reply, err := ParseChat([]byte(`{"choices":[{"message":{"content":"  ответ  "}}]}`))
		if err != nil {
			t.Fatalf("неожиданная ошибка: %v", err)
		}
		if reply.Content != "ответ" || reply.Choices != 1 || reply.ErrText != "" {
			t.Errorf("неожиданный разбор: %+v", reply)
		}
	})
	t.Run("ошибка-объект с message", func(t *testing.T) {
		reply, err := ParseChat([]byte(`{"error":{"message":"нет денег"}}`))
		if err != nil {
			t.Fatalf("неожиданная ошибка: %v", err)
		}
		if reply.ErrText != "нет денег" {
			t.Errorf("ErrText = %q, ожидалось %q", reply.ErrText, "нет денег")
		}
	})
	t.Run("ошибка-объект с description", func(t *testing.T) {
		reply, err := ParseChat([]byte(`{"error":{"description":"истёк токен"}}`))
		if err != nil {
			t.Fatalf("неожиданная ошибка: %v", err)
		}
		if reply.ErrText != "истёк токен" {
			t.Errorf("ErrText = %q, ожидалось %q", reply.ErrText, "истёк токен")
		}
	})
	t.Run("ошибка-строка", func(t *testing.T) {
		reply, err := ParseChat([]byte(`{"error":"Access denied by security policy"}`))
		if err != nil {
			t.Fatalf("неожиданная ошибка: %v", err)
		}
		if reply.ErrText != "Access denied by security policy" {
			t.Errorf("ErrText = %q", reply.ErrText)
		}
	})
	t.Run("ошибка неизвестной формы игнорируется", func(t *testing.T) {
		reply, err := ParseChat([]byte(`{"error":{"unknown_field":1}}`))
		if err != nil {
			t.Fatalf("неожиданная ошибка: %v", err)
		}
		if reply.ErrText != "" {
			t.Errorf("ErrText должен быть пустым, получено %q", reply.ErrText)
		}
	})
	t.Run("пустой список choices", func(t *testing.T) {
		reply, err := ParseChat([]byte(`{"choices":[]}`))
		if err != nil {
			t.Fatalf("неожиданная ошибка: %v", err)
		}
		if reply.Choices != 0 || reply.Content != "" {
			t.Errorf("неожиданный разбор: %+v", reply)
		}
	})
	t.Run("некорректный JSON", func(t *testing.T) {
		if _, err := ParseChat([]byte(`<html>ошибка</html>`)); err == nil {
			t.Error("ожидалась ошибка разбора")
		}
	})
}

func TestNewRequest(t *testing.T) {
	req, err := NewRequest(context.Background(), http.MethodPost, "http://example.invalid/api", []byte("тело"))
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if req.Method != http.MethodPost || req.URL.String() != "http://example.invalid/api" {
		t.Errorf("неожиданный запрос: %s %s", req.Method, req.URL)
	}
	got, err := io.ReadAll(req.Body)
	if err != nil || string(got) != "тело" {
		t.Errorf("тело запроса потерялось: %q (%v)", string(got), err)
	}
}
