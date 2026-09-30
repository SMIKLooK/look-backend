// Package config читает конфигурацию приложения из переменных окружения.
package config

import (
	"fmt"
	"time"

	"look-backend/internal/keys"
	"look-backend/internal/model_aliases"
)

type AiConfig struct {
	APIKey    string
	BaseURL   string
	Models    []string
	MaxTokens int
	Timeout   time.Duration

	// Только для провайдеров с OAuth-авторизацией (gigachat):
	// ключ можно задать целиком (APIKey) или парой ID + секрет.
	ClientID      string
	ClientSecret  string
	Scope         string
	AuthURL       string
	TLSSkipVerify bool
}

type Config struct {
	Addr            string        // адрес HTTP-сервера, например ":8080"
	ProviderTimeout time.Duration // таймаут одного обращения к провайдеру
	MaxBodyBytes    int64         // лимит размера тела запроса
	LogFormat       string        // text | json

	ModelAliases map[string]string

	Gemini     AiConfig
	GPTunnel   AiConfig
	GigaChat   AiConfig
	OpenRouter AiConfig
}

// Load собирает конфигурацию из окружения, подставляя значения по умолчанию.
func Load() (Config, error) {
	var cfg Config
	var err error

	cfg.Addr = env("LOOK_ADDR", ":8080")
	cfg.LogFormat = env("LOOK_LOG_FORMAT", "text")

	if cfg.LogFormat != "text" && cfg.LogFormat != "json" {
		return Config{}, fmt.Errorf("LOOK_LOG_FORMAT: допустимые значения text, json (сейчас %q)", cfg.LogFormat)
	}
	if cfg.ProviderTimeout, err = envDuration("LOOK_PROVIDER_TIMEOUT", 60*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.MaxBodyBytes, err = envInt64("LOOK_MAX_BODY_BYTES", 1<<20); err != nil {
		return Config{}, err
	}

	cfg.Gemini.APIKey = env("GEMINI_API_KEY", keys.Gemini)
	cfg.Gemini.BaseURL = env("GEMINI_BASE_URL", keys.GeminiBaseURL)
	if cfg.Gemini.Models, err = envList("GEMINI_MODELS", keys.GeminiModels); err != nil {
		return Config{}, err
	}
	if cfg.Gemini.MaxTokens, err = envInt("GEMINI_MAX_TOKENS", 0); err != nil {
		return Config{}, err
	}
	if cfg.Gemini.Timeout, err = envDuration("GEMINI_TIMEOUT", 0); err != nil {
		return Config{}, err
	}

	cfg.OpenRouter.APIKey = env("OPENROUTER_API_KEY", keys.OpenRouter)
	cfg.OpenRouter.BaseURL = env("OPENROUTER_BASE_URL", keys.OpenRouterBaseURL)
	if cfg.OpenRouter.Models, err = envList("OPENROUTER_MODELS", keys.OpenRouterModels); err != nil {
		return Config{}, err
	}
	if cfg.OpenRouter.MaxTokens, err = envInt("OPENROUTER_MAX_TOKENS", 0); err != nil {
		return Config{}, err
	}
	if cfg.OpenRouter.Timeout, err = envDuration("OPENROUTER_TIMEOUT", 0); err != nil {
		return Config{}, err
	}

	cfg.GPTunnel.APIKey = env("GPTUNNEL_API_KEY", keys.GPTunnel)
	cfg.GPTunnel.BaseURL = env("GPTUNNEL_BASE_URL", keys.GPTunnelBaseURL)
	if cfg.GPTunnel.Models, err = envList("GPTUNNEL_MODELS", keys.GPTunnelModels); err != nil {
		return Config{}, err
	}
	if cfg.GPTunnel.MaxTokens, err = envInt("GPTUNNEL_MAX_TOKENS", 0); err != nil {
		return Config{}, err
	}
	if cfg.GPTunnel.Timeout, err = envDuration("GPTUNNEL_TIMEOUT", 0); err != nil {
		return Config{}, err
	}

	cfg.GigaChat.APIKey = env("GIGACHAT_API_KEY", keys.GigaChat)
	cfg.GigaChat.ClientID = env("GIGACHAT_CLIENT_ID", keys.GigaChatClientID)
	cfg.GigaChat.ClientSecret = env("GIGACHAT_CLIENT_SECRET", keys.GigaChatClientSecret)
	cfg.GigaChat.BaseURL = env("GIGACHAT_BASE_URL", keys.GigaChatBaseURL)
	cfg.GigaChat.Scope = env("GIGACHAT_SCOPE", "")
	if cfg.GigaChat.Models, err = envList("GIGACHAT_MODELS", keys.GigaChatModels); err != nil {
		return Config{}, err
	}
	if cfg.GigaChat.MaxTokens, err = envInt("GIGACHAT_MAX_TOKENS", 0); err != nil {
		return Config{}, err
	}
	if cfg.GigaChat.Timeout, err = envDuration("GIGACHAT_TIMEOUT", 0); err != nil {
		return Config{}, err
	}
	if cfg.GigaChat.TLSSkipVerify, err = envBool("GIGACHAT_TLS_SKIP_VERIFY", false); err != nil {
		return Config{}, err
	}
	cfg.GigaChat.AuthURL = env("GIGACHAT_AUTH_URL", "")

	cfg.ModelAliases, err = loadAliases()
	if err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// loadAliases собирает карту псевдонимов: встроенные списки плюс
// переопределения из LOOK_MODEL_ALIASES.
func loadAliases() (map[string]string, error) {
	aliases := make(map[string]string, len(model_aliases.DefaultModelAliases)+
		len(model_aliases.DefaultFreeModelAliases)+
		len(keys.ExtraModelAliases))

	for alias, model := range model_aliases.DefaultModelAliases {
		aliases[alias] = model
	}
	for alias, model := range model_aliases.DefaultFreeModelAliases {
		aliases[alias] = model
	}
	for alias, model := range keys.ExtraModelAliases {
		aliases[alias] = model
	}
	return envAliases("LOOK_MODEL_ALIASES", aliases)
}
