package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"look-backend/internal/config"
	"look-backend/internal/transport/httpapi"
)

func TestApiURL(t *testing.T) {
	tests := []struct {
		addr string
		want string
	}{
		{":8080", "http://localhost:8080" + httpapi.ProcessPath},
		{"127.0.0.1:9999", "http://127.0.0.1:9999" + httpapi.ProcessPath},
	}
	for _, tt := range tests {
		if got := apiURL(tt.addr); got != tt.want {
			t.Errorf("apiURL(%q) = %q, ожидалось %q", tt.addr, got, tt.want)
		}
	}
}

func TestShutdownGrace(t *testing.T) {
	tests := []struct {
		providerTimeout time.Duration
		want            time.Duration
	}{
		{0, defaultShutdownGrace},
		{60 * time.Second, 65 * time.Second},
		{10 * time.Second, 15 * time.Second},
	}
	for _, tt := range tests {
		a := &App{cfg: config.Config{ProviderTimeout: tt.providerTimeout}}
		if got := a.shutdownGrace(); got != tt.want {
			t.Errorf("shutdownGrace(%v) = %v, ожидалось %v", tt.providerTimeout, got, tt.want)
		}
	}
}

func TestNewLogger(t *testing.T) {
	for _, format := range []string{"text", "json", "что-то ещё"} {
		if newLogger(format) == nil {
			t.Errorf("newLogger(%q) не должен возвращать nil", format)
		}
	}
}

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("не удалось найти свободный порт: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}

func TestApp_Run_ServesAndStops(t *testing.T) {
	cfg := config.Config{
		Addr:            freeAddr(t),
		ProviderTimeout: 5 * time.Second,
		MaxBodyBytes:    1 << 20,
		LogFormat:       "text",
	}
	a := New(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- a.run(ctx) }()

	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, err := http.Get("http://" + cfg.Addr + "/healthz")
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK && strings.Contains(string(body), "ok") {
				break
			}
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("сервер не поднялся за 5 секунд")
		}
		time.Sleep(50 * time.Millisecond)
	}

	body, _ := json.Marshal(map[string]string{"text": "тест привет"})
	resp, err := http.Post("http://"+cfg.Addr+"/api/v1/process", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("запрос к /api/v1/process не удался: %v", err)
	}
	responseBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(responseBody), "UNKNOWN_MODEL") {
		t.Errorf("ожидался 400 UNKNOWN_MODEL, получено %d %s", resp.StatusCode, string(responseBody))
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("ошибка при остановке сервера: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("сервер не остановился за 15 секунд")
	}
}

func TestApp_Run_ListenError(t *testing.T) {
	addr := freeAddr(t)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("не удалось занять порт: %v", err)
	}
	defer func() { _ = l.Close() }()

	a := New(config.Config{Addr: addr, MaxBodyBytes: 1 << 20})
	if err := a.run(context.Background()); err == nil {
		t.Error("ожидалась ошибка запуска на занятом порту")
	}
}

func TestApp_New_RegistersProvidersWithKeys(t *testing.T) {
	cfg := config.Config{
		Addr:            freeAddr(t),
		ProviderTimeout: 5 * time.Second,
		MaxBodyBytes:    1 << 20,
	}
	cfg.Gemini.APIKey = "ключ"
	cfg.GPTunnel.APIKey = "ключ"
	cfg.GigaChat.ClientID = "id"
	cfg.GigaChat.ClientSecret = "secret"
	cfg.OpenRouter.APIKey = "ключ"
	a := New(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = a.run(ctx) }()

	var body []byte
	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, err := http.Get("http://" + cfg.Addr + "/api/v1/models")
		if err == nil {
			body, _ = io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("сервер не поднялся за 5 секунд")
		}
		time.Sleep(50 * time.Millisecond)
	}

	var parsed struct {
		Providers []struct {
			Name string `json:"name"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("некорректный ответ /api/v1/models (%q): %v", string(body), err)
	}
	got := map[string]bool{}
	for _, p := range parsed.Providers {
		got[p.Name] = true
	}
	for _, want := range []string{"gemini", "gptunnel", "gigachat", "openrouter"} {
		if !got[want] {
			t.Errorf("провайдер %s не зарегистрировался, получено %v", want, parsed.Providers)
		}
	}
}

func TestApp_Run_SignalShutdown(t *testing.T) {
	cfg := config.Config{
		Addr:            freeAddr(t),
		ProviderTimeout: 5 * time.Second,
		MaxBodyBytes:    1 << 20,
	}
	a := New(cfg)

	errCh := make(chan error, 1)
	go func() { errCh <- a.Run() }()

	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, err := http.Get("http://" + cfg.Addr + "/healthz")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("сервер не поднялся за 5 секунд")
		}
		time.Sleep(50 * time.Millisecond)
	}

	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("не удалось послать SIGTERM: %v", err)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("ожидалась тихая остановка, получено: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("сервер не остановился по SIGTERM за 15 секунд")
	}
}
