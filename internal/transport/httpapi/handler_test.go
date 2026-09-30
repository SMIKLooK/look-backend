package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"look-backend/internal/domain"
	"look-backend/internal/parser"
	"look-backend/internal/provider"
	"look-backend/internal/service"
)

type fakeProvider struct {
	name   string
	models []string
	answer string
	err    error
	sleep  time.Duration
}

func (f *fakeProvider) Name() string     { return f.name }
func (f *fakeProvider) Models() []string { return append([]string(nil), f.models...) }

func (f *fakeProvider) Supports(model string) bool {
	for _, known := range f.models {
		if strings.EqualFold(known, model) {
			return true
		}
	}
	return false
}

func (f *fakeProvider) Complete(ctx context.Context, req domain.Request) (domain.Response, error) {
	if f.sleep > 0 {
		select {
		case <-time.After(f.sleep):
		case <-ctx.Done():
			return domain.Response{}, domain.WrapError(domain.CodeTimeout, ctx.Err(), "fake: контекст отменён")
		}
	}
	if f.err != nil {
		return domain.Response{}, f.err
	}
	return domain.Response{Model: req.Model, Provider: f.name, Content: f.answer}, nil
}

func newTestMux(p *fakeProvider, timeout time.Duration, maxBody int64) *http.ServeMux {
	registry := provider.NewRegistry()
	if p != nil {
		registry.Register(p)
		registry.SetAliases(map[string]string{"тест": "fake-model"})
	}
	svc := service.New(parser.New(), registry, timeout)

	mux := http.NewServeMux()
	NewHandler(svc, maxBody, slog.Default()).RegisterRoutes(mux)
	return mux
}

func postProcess(t *testing.T, mux *http.ServeMux, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, ProcessPath, strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func decodeEnvelope(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("ответ должен быть корректным JSON (%q): %v", rec.Body.String(), err)
	}
	return m
}

func TestHandler_Process_Success(t *testing.T) {
	mux := newTestMux(&fakeProvider{name: "fake", models: []string{"fake-model"}, answer: "ответ модели"}, 5*time.Second, 1<<20)

	rec := postProcess(t, mux, `{"text":"тест привет"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("ожидался 200, получен %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("неожиданный Content-Type: %q", ct)
	}
	m := decodeEnvelope(t, rec)
	if m["status"] != "ok" || m["model"] != "fake-model" || m["provider"] != "fake" || m["answer"] != "ответ модели" {
		t.Errorf("неожиданный конверт: %v", m)
	}
	if elapsed, ok := m["elapsed_ms"].(float64); !ok || elapsed < 0 {
		t.Errorf("elapsed_ms должен быть неотрицательным числом, получено %v", m["elapsed_ms"])
	}
}

func TestHandler_Process_ErrorMapping(t *testing.T) {
	tests := []struct {
		name     string
		provider *fakeProvider
		body     string
		maxBody  int64
		wantHTTP int
		wantCode string
	}{
		{
			name:     "некорректный JSON",
			body:     `{не json`,
			wantHTTP: http.StatusBadRequest,
			wantCode: "INVALID_JSON",
		},
		{
			name:     "text не строка",
			body:     `{"text":123}`,
			wantHTTP: http.StatusBadRequest,
			wantCode: "INVALID_JSON",
		},
		{
			name:     "пустой text",
			body:     `{"text":"  "}`,
			wantHTTP: http.StatusBadRequest,
			wantCode: "EMPTY_TEXT",
		},
		{
			name:     "нет модели",
			body:     `{"text":",,, : !"}`,
			wantHTTP: http.StatusBadRequest,
			wantCode: "MODEL_NOT_SPECIFIED",
		},
		{
			name:     "нет запроса",
			body:     `{"text":"тест"}`,
			wantHTTP: http.StatusBadRequest,
			wantCode: "PROMPT_NOT_SPECIFIED",
		},
		{
			name:     "неизвестная модель",
			body:     `{"text":"нету привет"}`,
			wantHTTP: http.StatusBadRequest,
			wantCode: "UNKNOWN_MODEL",
		},
		{
			name:     "тело больше лимита",
			body:     `{"text":"` + strings.Repeat("а", 200) + `"}`,
			maxBody:  50,
			wantHTTP: http.StatusRequestEntityTooLarge,
			wantCode: "PAYLOAD_TOO_LARGE",
		},
		{
			name:     "ошибка провайдера",
			provider: &fakeProvider{name: "fake", models: []string{"fake-model"}, err: domain.NewError(domain.CodeProviderFailed, "провайдер перегружен")},
			body:     `{"text":"тест привет"}`,
			wantHTTP: http.StatusBadGateway,
			wantCode: "PROVIDER_ERROR",
		},
		{
			name:     "таймаут провайдера",
			provider: &fakeProvider{name: "fake", models: []string{"fake-model"}, sleep: 300 * time.Millisecond},
			body:     `{"text":"тест привет"}`,
			wantHTTP: http.StatusGatewayTimeout,
			wantCode: "TIMEOUT",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maxBody := tt.maxBody
			if maxBody == 0 {
				maxBody = 1 << 20
			}
			timeout := 5 * time.Second
			if tt.wantCode == "TIMEOUT" {
				timeout = 30 * time.Millisecond
			}
			mux := newTestMux(tt.provider, timeout, maxBody)

			rec := postProcess(t, mux, tt.body)
			if rec.Code != tt.wantHTTP {
				t.Fatalf("ожидался HTTP %d, получен %d: %s", tt.wantHTTP, rec.Code, rec.Body.String())
			}
			m := decodeEnvelope(t, rec)
			if m["status"] != "error" {
				t.Errorf("ожидался status=error, получено %v", m["status"])
			}
			errBody, ok := m["error"].(map[string]any)
			if !ok || errBody["code"] != tt.wantCode {
				t.Errorf("ожидался код %s, получено %v", tt.wantCode, m["error"])
			}
		})
	}
}

func TestHandler_Process_MethodNotAllowed(t *testing.T) {
	mux := newTestMux(&fakeProvider{name: "fake", models: []string{"fake-model"}}, 5*time.Second, 1<<20)

	req := httptest.NewRequest(http.MethodGet, ProcessPath, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("ожидался 405, получен %d", rec.Code)
	}
}

func TestHandler_Models(t *testing.T) {
	mux := newTestMux(&fakeProvider{name: "fake", models: []string{"fake-model"}}, 5*time.Second, 1<<20)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/models", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ожидался 200, получен %d", rec.Code)
	}

	m := decodeEnvelope(t, rec)
	providers, ok := m["providers"].([]any)
	if !ok || len(providers) != 1 {
		t.Fatalf("ожидался один провайдер, получено %v", m["providers"])
	}
	p := providers[0].(map[string]any)
	if p["name"] != "fake" {
		t.Errorf("неожиданный провайдер: %v", p)
	}
	aliases, ok := m["aliases"].(map[string]any)
	if !ok || aliases["тест"] != "fake-model" {
		t.Errorf("ожидался псевдоним тест→fake-model, получено %v", m["aliases"])
	}
}

func TestHandler_Health(t *testing.T) {
	mux := newTestMux(nil, 5*time.Second, 1<<20)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ожидался 200, получен %d", rec.Code)
	}
	if m := decodeEnvelope(t, rec); m["status"] != "ok" {
		t.Errorf("ожидался status=ok, получено %v", m["status"])
	}
}

func TestStatusByCode(t *testing.T) {
	tests := map[domain.Code]int{
		domain.CodeInvalidJSON:    http.StatusBadRequest,
		domain.CodeEmptyText:      http.StatusBadRequest,
		domain.CodeModelMissing:   http.StatusBadRequest,
		domain.CodePromptMissing:  http.StatusBadRequest,
		domain.CodeUnknownModel:   http.StatusBadRequest,
		domain.CodeTimeout:        http.StatusGatewayTimeout,
		domain.CodeProviderFailed: http.StatusBadGateway,
		domain.CodeInternal:       http.StatusInternalServerError,
		"неизвестный код":         http.StatusInternalServerError,
	}
	for code, want := range tests {
		if got := statusByCode(code); got != want {
			t.Errorf("statusByCode(%s) = %d, ожидалось %d", code, got, want)
		}
	}
}

func TestWriteJSON_EncodesPayload(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, map[string]string{"status": "ok"})
	if rec.Code != http.StatusOK {
		t.Errorf("ожидался 200, получен %d", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), `"status":"ok"`) {
		t.Errorf("неожиданное тело: %s", string(body))
	}
}
