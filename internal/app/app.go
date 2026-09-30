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
	"look-backend/internal/provider/gigachat"
	"look-backend/internal/provider/gptunnel"
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

// providerSpec — описание провайдера для регистрации: имя для логов,
// переменная окружения с ключом, условие включения и конструктор.
type providerSpec struct {
	name    string
	env     string
	baseURL string
	enabled bool
	make    func() provider.Provider
}

// New собирает приложение из конфигурации.
func New(cfg config.Config) *App {
	log := newLogger(cfg.LogFormat)
	registry := provider.NewRegistry()

	// Порядок = приоритет: прямые провайдеры раньше, OpenRouter последним —
	// он подхватывает всё в формате "vendor/model". Новый провайдер добавляется
	// записью в таблицу и секцией в config.Load (см. README → «Как добавить»).
	specs := []providerSpec{
		{
			name: "gemini", env: "GEMINI_API_KEY", baseURL: cfg.Gemini.BaseURL,
			enabled: cfg.Gemini.APIKey != "",
			make: func() provider.Provider {
				return gemini.New(gemini.Config{
					APIKey:    cfg.Gemini.APIKey,
					BaseURL:   cfg.Gemini.BaseURL,
					Models:    cfg.Gemini.Models,
					MaxTokens: cfg.Gemini.MaxTokens,
					Timeout:   cfg.Gemini.Timeout,
				})
			},
		},
		{
			name: "gptunnel", env: "GPTUNNEL_API_KEY", baseURL: cfg.GPTunnel.BaseURL,
			enabled: cfg.GPTunnel.APIKey != "",
			make: func() provider.Provider {
				return gptunnel.New(gptunnel.Config{
					APIKey:    cfg.GPTunnel.APIKey,
					BaseURL:   cfg.GPTunnel.BaseURL,
					Models:    cfg.GPTunnel.Models,
					MaxTokens: cfg.GPTunnel.MaxTokens,
					Timeout:   cfg.GPTunnel.Timeout,
				})
			},
		},
		{
			name: "gigachat", env: "GIGACHAT_API_KEY", baseURL: cfg.GigaChat.BaseURL,
			enabled: cfg.GigaChat.APIKey != "" || (cfg.GigaChat.ClientID != "" && cfg.GigaChat.ClientSecret != ""),
			make: func() provider.Provider {
				return gigachat.New(gigachat.Config{
					APIKey:        cfg.GigaChat.APIKey,
					ClientID:      cfg.GigaChat.ClientID,
					ClientSecret:  cfg.GigaChat.ClientSecret,
					Scope:         cfg.GigaChat.Scope,
					BaseURL:       cfg.GigaChat.BaseURL,
					AuthURL:       cfg.GigaChat.AuthURL,
					Models:        cfg.GigaChat.Models,
					MaxTokens:     cfg.GigaChat.MaxTokens,
					Timeout:       cfg.GigaChat.Timeout,
					TLSSkipVerify: cfg.GigaChat.TLSSkipVerify,
				})
			},
		},
		{
			name: "openrouter", env: "OPENROUTER_API_KEY", baseURL: cfg.OpenRouter.BaseURL,
			enabled: cfg.OpenRouter.APIKey != "",
			make: func() provider.Provider {
				return openrouter.New(openrouter.Config{
					APIKey:    cfg.OpenRouter.APIKey,
					BaseURL:   cfg.OpenRouter.BaseURL,
					Models:    cfg.OpenRouter.Models,
					MaxTokens: cfg.OpenRouter.MaxTokens,
					Timeout:   cfg.OpenRouter.Timeout,
				})
			},
		},
	}

	for _, s := range specs {
		if !s.enabled {
			log.Info("провайдер отключён: не задан API-ключ", "provider", s.name, "env", s.env)
			continue
		}
		registry.Register(s.make())
		log.Info("провайдер подключён", "provider", s.name, "base_url", s.baseURL)
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
	return a.run(ctx)
}

func (a *App) run(ctx context.Context) error {
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
		shutdownCtx, cancel := context.WithTimeout(context.Background(), a.shutdownGrace())
		defer cancel()
		if err := a.server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		a.log.Info("сервер остановлен")
		return nil
	}
}

const (
	defaultShutdownGrace = 60 * time.Second
	shutdownWriteBuffer  = 5 * time.Second
)

func (a *App) shutdownGrace() time.Duration {
	if a.cfg.ProviderTimeout <= 0 {
		return defaultShutdownGrace
	}
	return a.cfg.ProviderTimeout + shutdownWriteBuffer
}

// apiURL — адрес API для подключения клиентов; ":8080" означает localhost.
func apiURL(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "http://localhost" + addr + httpapi.ProcessPath
	}
	return "http://" + addr + httpapi.ProcessPath
}

func newLogger(format string) *slog.Logger {
	if format == "json" {
		return slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, nil))
}
