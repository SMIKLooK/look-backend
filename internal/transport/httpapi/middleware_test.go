package httpapi

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"look-backend/internal/domain"
)

func TestServer_ListenAndServeAndShutdown(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("не удалось найти свободный порт: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()

	srv := NewServer(addr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, err := http.Get("http://" + addr + "/")
		if err == nil {
			_ = resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("сервер не поднялся за 5 секунд")
		}
		time.Sleep(20 * time.Millisecond)
	}

	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatalf("неожиданная ошибка Shutdown: %v", err)
	}
	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("ожидался http.ErrServerClosed, получено %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ListenAndServe не завершился после Shutdown")
	}
}

func newBufferLogger() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return slog.New(slog.NewTextHandler(buf, nil)), buf
}

func TestLoggingMiddleware_LogsStatusAndDuration(t *testing.T) {
	log, buf := newBufferLogger()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("создано"))
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/process", nil)
	rec := httptest.NewRecorder()
	LoggingMiddleware(log, next).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("middleware не должен менять статус, получен %d", rec.Code)
	}
	entry := buf.String()
	for _, want := range []string{"method=POST", "path=/api/v1/process", "status=201"} {
		if !strings.Contains(entry, want) {
			t.Errorf("в записи лога нет %q: %s", want, entry)
		}
	}
	if !strings.Contains(entry, "duration_ms=") {
		t.Errorf("в записи лога нет длительности: %s", entry)
	}
}

func TestLoggingMiddleware_DefaultStatusIs200(t *testing.T) {
	log, buf := newBufferLogger()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ок"))
	})

	LoggingMiddleware(log, next).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if !strings.Contains(buf.String(), "status=200") {
		t.Errorf("ожидался статус 200 по умолчанию: %s", buf.String())
	}
}

func TestRecoverMiddleware_PanicBecomes500(t *testing.T) {
	log, buf := newBufferLogger()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("что-то взорвалось")
	})

	rec := httptest.NewRecorder()
	RecoverMiddleware(log, next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/process", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("ожидался 500, получен %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), string(domain.CodeInternal)) {
		t.Errorf("клиент должен получить INTERNAL, получено %s", rec.Body.String())
	}
	if !strings.Contains(buf.String(), "что-то взорвалось") {
		t.Errorf("паника должна попасть в лог: %s", buf.String())
	}
}

func TestRecoverMiddleware_PassesNormalResponse(t *testing.T) {
	log, _ := newBufferLogger()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ок"))
	})

	rec := httptest.NewRecorder()
	RecoverMiddleware(log, next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "ок" {
		t.Errorf("middleware не должен менять успешный ответ, получено %d %q", rec.Code, rec.Body.String())
	}
}

func TestStatusWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: rec, status: http.StatusOK}

	sw.WriteHeader(http.StatusTeapot)
	if sw.status != http.StatusTeapot {
		t.Errorf("statusWriter должен запоминать статус, получен %d", sw.status)
	}
	if rec.Code != http.StatusTeapot {
		t.Errorf("статус должен пробрасываться в ответ, получен %d", rec.Code)
	}

	sw2 := &statusWriter{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}
	_, _ = sw2.Write([]byte("данные"))
	if sw2.status != http.StatusOK {
		t.Errorf("без явного WriteHeader статус должен быть 200, получен %d", sw2.status)
	}
}

func TestNewServer_Timeouts(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	srv := NewServer(":8080", handler)

	if srv.srv.Addr != ":8080" || srv.srv.Handler == nil {
		t.Errorf("неожиданные адрес/обработчик: %s %v", srv.srv.Addr, srv.srv.Handler)
	}
	if srv.srv.ReadHeaderTimeout != 5*time.Second {
		t.Errorf("ReadHeaderTimeout = %v", srv.srv.ReadHeaderTimeout)
	}
	if srv.srv.ReadTimeout != 15*time.Second {
		t.Errorf("ReadTimeout = %v", srv.srv.ReadTimeout)
	}
	if srv.srv.WriteTimeout != 90*time.Second {
		t.Errorf("WriteTimeout = %v", srv.srv.WriteTimeout)
	}
	if srv.srv.IdleTimeout != 120*time.Second {
		t.Errorf("IdleTimeout = %v", srv.srv.IdleTimeout)
	}
}
