// Package config читает конфигурацию приложения из переменных окружения.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
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
}

type Config struct {
	Addr               string        // адрес HTTP-сервера, например ":8080"
	ProviderTimeout    time.Duration // таймаут одного обращения к провайдеру
	MaxBodyBytes       int64         // лимит размера тела запроса
	LogFormat          string        // text | json
	MaxHistoryMessages int           // сколько последних сообщений сессии отправлять модели; 0 — без ограничения

	ModelAliases map[string]string

	Gemini     AiConfig
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
	if cfg.ModelAliases, err = envAliases("LOOK_MODEL_ALIASES", aliases); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// envList читает список значений через запятую; пустая или отсутствующая
// переменная даёт значение по умолчанию.
func envList(key string, def []string) ([]string, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return def, nil
	}
	return out, nil
}

func envDuration(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return d, nil
}

func envInt(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return n, nil
}

func envInt64(key string, def int64) (int64, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return n, nil
}

// envAliases читает псевдонимы моделей в формате "псевдоним=модель"
// через запятую; пустая или отсутствующая переменная даёт значение
// по умолчанию.
func envAliases(key string, def map[string]string) (map[string]string, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	out := make(map[string]string)
	for _, pair := range strings.Split(v, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		alias, model, found := strings.Cut(pair, "=")
		alias, model = strings.TrimSpace(alias), strings.TrimSpace(model)
		if !found || alias == "" || model == "" {
			return nil, fmt.Errorf(`%s: ожидается формат "псевдоним=модель" (сейчас %q)`, key, pair)
		}
		out[alias] = model
	}
	if len(out) == 0 {
		return def, nil
	}
	return out, nil
}
