// Package app собирает компоненты приложения и управляет его жизненным циклом.
// Здесь выполняется вся компоновка зависимостей: конфигурация → провайдеры →
// реестр → сервис → HTTP-обработчики → сервер.
package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"look-backend/internal/config"
	"look-backend/internal/parser"
	"look-backend/internal/provider"
	"look-backend/internal/provider/anthropic"
	"look-backend/internal/provider/echo"
	"look-backend/internal/provider/gemini"
	"look-backend/internal/provider/openai"
	"look-backend/internal/provider/openrouter"
	"look-backend/internal/service"
	"look-backend/internal/storage"
	httpapi "look-backend/internal/transport/httpapi"
)

// App — собранное приложение: зависимости, логгер, HTTP-сервер.
type App struct {
	cfg    config.Config
	log    *slog.Logger
	server *httpapi.Server
}

// New собирает приложение из конфигурации.
func New(cfg config.Config) *App {
	log := newLogger(cfg.LogFormat)

	registry := provider.NewRegistry()
	registry.Register(echo.New()) // тестовый провайдер доступен всегда

	if cfg.OpenAI.APIKey != "" {
		registry.Register(openai.New(openai.Config{
			APIKey:    cfg.OpenAI.APIKey,
			BaseURL:   cfg.OpenAI.BaseURL,
			Models:    cfg.OpenAI.Models,
			MaxTokens: cfg.OpenAI.MaxTokens,
			Timeout:   cfg.OpenAI.Timeout,
		}))
		log.Info("провайдер подключён", "provider", "openai", "base_url", cfg.OpenAI.BaseURL)
	} else {
		log.Info("провайдер отключён: не задан API-ключ", "provider", "openai", "env", "OPENAI_API_KEY")
	}

	if cfg.Anthropic.APIKey != "" {
		registry.Register(anthropic.New(anthropic.Config{
			APIKey:    cfg.Anthropic.APIKey,
			BaseURL:   cfg.Anthropic.BaseURL,
			Models:    cfg.Anthropic.Models,
			MaxTokens: cfg.Anthropic.MaxTokens,
			Timeout:   cfg.Anthropic.Timeout,
		}))
		log.Info("провайдер подключён", "provider", "anthropic", "base_url", cfg.Anthropic.BaseURL)
	} else {
		log.Info("провайдер отключён: не задан API-ключ", "provider", "anthropic", "env", "ANTHROPIC_API_KEY")
	}

	if cfg.Gemini.APIKey != "" {
		registry.Register(gemini.New(gemini.Config{
			APIKey:    cfg.Gemini.APIKey,
			BaseURL:   cfg.Gemini.BaseURL,
			Models:    cfg.Gemini.Models,
			MaxTokens: cfg.Gemini.MaxTokens,
			Timeout:   cfg.Gemini.Timeout,
		}))
		log.Info("провайдер подключён", "provider", "gemini", "base_url", cfg.Gemini.BaseURL)
	} else {
		log.Info("провайдер отключён: не задан API-ключ", "provider", "gemini", "env", "GEMINI_API_KEY")
	}

	// OpenRouter регистрируется последним: прямые провайдеры приоритетнее,
	// а он подхватывает всё в формате "vendor/model".
	if cfg.OpenRouter.APIKey != "" {
		registry.Register(openrouter.New(openrouter.Config{
			APIKey:    cfg.OpenRouter.APIKey,
			BaseURL:   cfg.OpenRouter.BaseURL,
			Models:    cfg.OpenRouter.Models,
			MaxTokens: cfg.OpenRouter.MaxTokens,
			Timeout:   cfg.OpenRouter.Timeout,
		}))
		log.Info("провайдер подключён", "provider", "openrouter", "base_url", cfg.OpenRouter.BaseURL)
	} else {
		log.Info("провайдер отключён: не задан API-ключ", "provider", "openrouter", "env", "OPENROUTER_API_KEY")
	}

	registry.SetAliases(cfg.ModelAliases)

	// История диалогов живёт в памяти процесса; для персистентности
	// (PostgreSQL/SQLite) реализуйте storage.Store и подставьте сюда.
	historyStore := storage.NewMemoryStore(24*time.Hour, 10*time.Minute)

	svc := service.New(parser.New(cfg.Keywords...), registry, historyStore, cfg.ProviderTimeout, cfg.MaxHistoryMessages)

	mux := http.NewServeMux()
	httpapi.NewHandler(svc, cfg.MaxBodyBytes, log).RegisterRoutes(mux)
	handler := httpapi.LoggingMiddleware(log, httpapi.RecoverMiddleware(log, mux))

	return &App{cfg: cfg, log: log, server: httpapi.NewServer(cfg.Addr, handler)}
}

// Run запускает HTTP-сервер и корректно останавливает его по SIGINT/SIGTERM.
func (a *App) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		a.log.Info("сервер запущен", "addr", a.cfg.Addr, "keywords", a.cfg.Keywords)
		errCh <- a.server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		a.log.Info("получен сигнал остановки, завершаем работу")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := a.server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		a.log.Info("сервер остановлен")
		return nil
	}
}

func newLogger(format string) *slog.Logger {
	if format == "json" {
		return slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, nil))
}
