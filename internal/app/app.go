package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"look-backend/internal/config"
	"look-backend/internal/parser"
	"look-backend/internal/provider"
	"look-backend/internal/provider/gemini"
	"look-backend/internal/provider/openrouter"
	"look-backend/internal/service"
	"look-backend/internal/transport/httpapi"
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

	// Сборка слоёв: parser → service → httpapi, маршруты — в mux.
	svc := service.New(parser.New(), registry, cfg.ProviderTimeout)
	handler := httpapi.NewHandler(svc, cfg.MaxBodyBytes, log)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	return &App{
		cfg:    cfg,
		log:    log,
		server: httpapi.NewServer(cfg.Addr, httpapi.LoggingMiddleware(log, httpapi.RecoverMiddleware(log, mux))),
	}
}

// Run запускает HTTP-сервер и корректно останавливает его по SIGINT/SIGTERM.
func (a *App) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		a.log.Info("сервер запущен", "addr", a.cfg.Addr, "api", apiURL(a.cfg.Addr))
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

// apiURL — адрес API для подключения клиентов; ":8080" означает localhost.
func apiURL(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "http://localhost" + addr + "/api/v1/process"
	}
	return "http://" + addr + "/api/v1/process"
}

func newLogger(format string) *slog.Logger {
	if format == "json" {
		return slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, nil))
}
