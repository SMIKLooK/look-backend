package config

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"look-backend/internal/keys"
)

func TestEnv(t *testing.T) {
	if got := env("LOOK_ENV_TEST_MISSING", "по умолчанию"); got != "по умолчанию" {
		t.Errorf("отсутствующая переменная должна давать значение по умолчанию, получено %q", got)
	}
	t.Setenv("LOOK_ENV_TEST_SET", "значение")
	if got := env("LOOK_ENV_TEST_SET", "по умолчанию"); got != "значение" {
		t.Errorf("заданная переменная должна переопределять значение, получено %q", got)
	}
}

func TestEnvList(t *testing.T) {
	def := []string{"a", "b"}
	if got, err := envList("LOOK_ENV_TEST_MISSING", def); err != nil || !reflect.DeepEqual(got, def) {
		t.Errorf("отсутствующая переменная должна давать значение по умолчанию, получено %v (%v)", got, err)
	}

	t.Setenv("LOOK_ENV_TEST_LIST", " x , ,y,z ")
	got, err := envList("LOOK_ENV_TEST_LIST", def)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if want := []string{"x", "y", "z"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ожидался список %v без пустых элементов и пробелов, получено %v", want, got)
	}

	t.Setenv("LOOK_ENV_TEST_EMPTY", " , , ")
	if got, err := envList("LOOK_ENV_TEST_EMPTY", def); err != nil || !reflect.DeepEqual(got, def) {
		t.Errorf("пустой список должен давать значение по умолчанию, получено %v (%v)", got, err)
	}
}

func TestEnvDuration(t *testing.T) {
	if got, err := envDuration("LOOK_ENV_TEST_MISSING", 5*time.Second); err != nil || got != 5*time.Second {
		t.Errorf("ожидалось значение по умолчанию, получено %v (%v)", got, err)
	}
	t.Setenv("LOOK_ENV_TEST_DURATION", "90s")
	if got, err := envDuration("LOOK_ENV_TEST_DURATION", 0); err != nil || got != 90*time.Second {
		t.Errorf("ожидалось 90s, получено %v (%v)", got, err)
	}

	t.Setenv("LOOK_ENV_TEST_DURATION", "не длительность")
	if _, err := envDuration("LOOK_ENV_TEST_DURATION", 0); err == nil || !strings.Contains(err.Error(), "LOOK_ENV_TEST_DURATION") {
		t.Errorf("ошибка должна называть переменную, получено %v", err)
	}
}

func TestEnvIntAndInt64(t *testing.T) {
	if got, err := envInt("LOOK_ENV_TEST_MISSING", 42); err != nil || got != 42 {
		t.Errorf("ожидалось значение по умолчанию, получено %d (%v)", got, err)
	}
	t.Setenv("LOOK_ENV_TEST_INT", "-7")
	if got, err := envInt("LOOK_ENV_TEST_INT", 0); err != nil || got != -7 {
		t.Errorf("ожидалось -7, получено %d (%v)", got, err)
	}

	t.Setenv("LOOK_ENV_TEST_INT", "не число")
	if _, err := envInt("LOOK_ENV_TEST_INT", 0); err == nil || !strings.Contains(err.Error(), "LOOK_ENV_TEST_INT") {
		t.Errorf("ошибка должна называть переменную, получено %v", err)
	}

	t.Setenv("LOOK_ENV_TEST_INT64", "1048576")
	if got, err := envInt64("LOOK_ENV_TEST_INT64", 0); err != nil || got != 1048576 {
		t.Errorf("ожидалось 1048576, получено %d (%v)", got, err)
	}
}

func TestEnvBool(t *testing.T) {
	if got, err := envBool("LOOK_ENV_TEST_MISSING", true); err != nil || !got {
		t.Errorf("ожидалось значение по умолчанию, получено %v (%v)", got, err)
	}
	for _, v := range []string{"1", "t", "True", "TRUE"} {
		t.Setenv("LOOK_ENV_TEST_BOOL", v)
		if got, err := envBool("LOOK_ENV_TEST_BOOL", false); err != nil || !got {
			t.Errorf("%q должно разбираться как true, получено %v (%v)", v, got, err)
		}
	}
	t.Setenv("LOOK_ENV_TEST_BOOL", "может быть")
	if _, err := envBool("LOOK_ENV_TEST_BOOL", false); err == nil {
		t.Error("ожидалась ошибка разбора")
	}
}

func TestEnvAliases(t *testing.T) {
	def := map[string]string{"встроенный": "модель-1"}
	if got, err := envAliases("LOOK_ENV_TEST_MISSING", def); err != nil || !reflect.DeepEqual(got, def) {
		t.Errorf("отсутствующая переменная должна давать значение по умолчанию, получено %v (%v)", got, err)
	}

	t.Setenv("LOOK_ENV_TEST_ALIASES", " один=модель-1 , два = модель-2 ")
	got, err := envAliases("LOOK_ENV_TEST_ALIASES", def)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	want := map[string]string{"один": "модель-1", "два": "модель-2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ожидалось %v, получено %v", want, got)
	}

	t.Setenv("LOOK_ENV_TEST_ALIASES", "без-равно")
	if _, err := envAliases("LOOK_ENV_TEST_ALIASES", def); err == nil || !strings.Contains(err.Error(), "LOOK_ENV_TEST_ALIASES") {
		t.Errorf("ошибка должна называть переменную и формат, получено %v", err)
	}

	for _, bad := range []string{"=модель", "имя="} {
		t.Setenv("LOOK_ENV_TEST_ALIASES", bad)
		if _, err := envAliases("LOOK_ENV_TEST_ALIASES", def); err == nil {
			t.Errorf("пара %q должна считаться некорректной", bad)
		}
	}

	t.Setenv("LOOK_ENV_TEST_ALIASES", " , , ")
	if got, err := envAliases("LOOK_ENV_TEST_ALIASES", def); err != nil || !reflect.DeepEqual(got, def) {
		t.Errorf("пустой список должен давать значение по умолчанию, получено %v (%v)", got, err)
	}
}

func clearAll(t *testing.T) {
	t.Helper()
	vars := []string{
		"LOOK_ADDR", "LOOK_LOG_FORMAT", "LOOK_PROVIDER_TIMEOUT", "LOOK_MAX_BODY_BYTES",
		"LOOK_MODEL_ALIASES",
		"GEMINI_API_KEY", "GEMINI_BASE_URL", "GEMINI_MODELS", "GEMINI_MAX_TOKENS", "GEMINI_TIMEOUT",
		"OPENROUTER_API_KEY", "OPENROUTER_BASE_URL", "OPENROUTER_MODELS", "OPENROUTER_MAX_TOKENS", "OPENROUTER_TIMEOUT",
		"GPTUNNEL_API_KEY", "GPTUNNEL_BASE_URL", "GPTUNNEL_MODELS", "GPTUNNEL_MAX_TOKENS", "GPTUNNEL_TIMEOUT",
		"GIGACHAT_API_KEY", "GIGACHAT_CLIENT_ID", "GIGACHAT_CLIENT_SECRET", "GIGACHAT_BASE_URL",
		"GIGACHAT_SCOPE", "GIGACHAT_AUTH_URL", "GIGACHAT_MODELS", "GIGACHAT_MAX_TOKENS", "GIGACHAT_TIMEOUT",
		"GIGACHAT_TLS_SKIP_VERIFY",
	}
	for _, v := range vars {
		t.Setenv(v, "")
	}
}

func TestLoad_Defaults(t *testing.T) {
	clearAll(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}

	if cfg.Addr != ":8080" {
		t.Errorf("Addr = %q, ожидалось :8080", cfg.Addr)
	}
	if cfg.LogFormat != "text" {
		t.Errorf("LogFormat = %q, ожидалось text", cfg.LogFormat)
	}
	if cfg.ProviderTimeout != 60*time.Second {
		t.Errorf("ProviderTimeout = %v, ожидалось 60s", cfg.ProviderTimeout)
	}
	if cfg.MaxBodyBytes != 1<<20 {
		t.Errorf("MaxBodyBytes = %d, ожидалось %d", cfg.MaxBodyBytes, int64(1<<20))
	}
	if cfg.Gemini.APIKey != keys.Gemini {
		t.Errorf("Gemini.APIKey должен браться из keys.go")
	}
	if !reflect.DeepEqual(cfg.Gemini.Models, keys.GeminiModels) {
		t.Errorf("Gemini.Models = %v, ожидалось %v (keys.go)", cfg.Gemini.Models, keys.GeminiModels)
	}
}

func TestLoad_Overrides(t *testing.T) {
	t.Setenv("LOOK_ADDR", ":9999")
	t.Setenv("LOOK_LOG_FORMAT", "json")
	t.Setenv("LOOK_PROVIDER_TIMEOUT", "90s")
	t.Setenv("LOOK_MAX_BODY_BYTES", "2048")
	t.Setenv("LOOK_MODEL_ALIASES", "новый=vendor/model")

	t.Setenv("GEMINI_API_KEY", "gem-key")
	t.Setenv("GEMINI_BASE_URL", "http://gemini-proxy")
	t.Setenv("GEMINI_MODELS", "gemini-a, gemini-b")
	t.Setenv("GEMINI_MAX_TOKENS", "100")
	t.Setenv("GEMINI_TIMEOUT", "3s")

	t.Setenv("OPENROUTER_API_KEY", "or-key")
	t.Setenv("OPENROUTER_BASE_URL", "http://or-proxy")
	t.Setenv("OPENROUTER_MODELS", "vendor/model-1")
	t.Setenv("OPENROUTER_MAX_TOKENS", "200")
	t.Setenv("OPENROUTER_TIMEOUT", "4s")

	t.Setenv("GPTUNNEL_API_KEY", "gt-key")
	t.Setenv("GPTUNNEL_BASE_URL", "http://gt-proxy")
	t.Setenv("GPTUNNEL_MODELS", "gpt-4o")
	t.Setenv("GPTUNNEL_MAX_TOKENS", "300")
	t.Setenv("GPTUNNEL_TIMEOUT", "5s")

	t.Setenv("GIGACHAT_API_KEY", "gc-key")
	t.Setenv("GIGACHAT_CLIENT_ID", "id")
	t.Setenv("GIGACHAT_CLIENT_SECRET", "secret")
	t.Setenv("GIGACHAT_BASE_URL", "http://gc-proxy")
	t.Setenv("GIGACHAT_SCOPE", "GIGACHAT_API_B2B")
	t.Setenv("GIGACHAT_AUTH_URL", "http://gc-auth")
	t.Setenv("GIGACHAT_MODELS", "GigaChat")
	t.Setenv("GIGACHAT_MAX_TOKENS", "400")
	t.Setenv("GIGACHAT_TIMEOUT", "6s")
	t.Setenv("GIGACHAT_TLS_SKIP_VERIFY", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if cfg.Addr != ":9999" || cfg.LogFormat != "json" {
		t.Errorf("неожиданные Addr/LogFormat: %s %s", cfg.Addr, cfg.LogFormat)
	}
	if cfg.ProviderTimeout != 90*time.Second {
		t.Errorf("ProviderTimeout = %v, ожидалось 90s", cfg.ProviderTimeout)
	}
	if cfg.MaxBodyBytes != 2048 {
		t.Errorf("MaxBodyBytes = %d, ожидалось 2048", cfg.MaxBodyBytes)
	}
	if cfg.ModelAliases["новый"] != "vendor/model" {
		t.Errorf("псевдонимы из окружения не применились: %v", cfg.ModelAliases)
	}

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"Gemini.APIKey", cfg.Gemini.APIKey, "gem-key"},
		{"Gemini.BaseURL", cfg.Gemini.BaseURL, "http://gemini-proxy"},
		{"Gemini.Models", cfg.Gemini.Models, []string{"gemini-a", "gemini-b"}},
		{"Gemini.MaxTokens", cfg.Gemini.MaxTokens, 100},
		{"Gemini.Timeout", cfg.Gemini.Timeout, 3 * time.Second},
		{"OpenRouter.APIKey", cfg.OpenRouter.APIKey, "or-key"},
		{"OpenRouter.BaseURL", cfg.OpenRouter.BaseURL, "http://or-proxy"},
		{"OpenRouter.Models", cfg.OpenRouter.Models, []string{"vendor/model-1"}},
		{"OpenRouter.MaxTokens", cfg.OpenRouter.MaxTokens, 200},
		{"OpenRouter.Timeout", cfg.OpenRouter.Timeout, 4 * time.Second},
		{"GPTunnel.APIKey", cfg.GPTunnel.APIKey, "gt-key"},
		{"GPTunnel.BaseURL", cfg.GPTunnel.BaseURL, "http://gt-proxy"},
		{"GPTunnel.Models", cfg.GPTunnel.Models, []string{"gpt-4o"}},
		{"GPTunnel.MaxTokens", cfg.GPTunnel.MaxTokens, 300},
		{"GPTunnel.Timeout", cfg.GPTunnel.Timeout, 5 * time.Second},
		{"GigaChat.APIKey", cfg.GigaChat.APIKey, "gc-key"},
		{"GigaChat.ClientID", cfg.GigaChat.ClientID, "id"},
		{"GigaChat.ClientSecret", cfg.GigaChat.ClientSecret, "secret"},
		{"GigaChat.BaseURL", cfg.GigaChat.BaseURL, "http://gc-proxy"},
		{"GigaChat.Scope", cfg.GigaChat.Scope, "GIGACHAT_API_B2B"},
		{"GigaChat.AuthURL", cfg.GigaChat.AuthURL, "http://gc-auth"},
		{"GigaChat.Models", cfg.GigaChat.Models, []string{"GigaChat"}},
		{"GigaChat.MaxTokens", cfg.GigaChat.MaxTokens, 400},
		{"GigaChat.Timeout", cfg.GigaChat.Timeout, 6 * time.Second},
		{"GigaChat.TLSSkipVerify", cfg.GigaChat.TLSSkipVerify, true},
	}
	for _, c := range checks {
		if !reflect.DeepEqual(c.got, c.want) {
			t.Errorf("%s = %v, ожидалось %v", c.name, c.got, c.want)
		}
	}
}

func TestLoad_InvalidValues(t *testing.T) {
	tests := []struct {
		name string
		key  string
		val  string
	}{
		{"недопустимый формат логов", "LOOK_LOG_FORMAT", "xml"},
		{"недопустимый таймаут", "LOOK_PROVIDER_TIMEOUT", "завтра"},
		{"недопустимый лимит тела", "LOOK_MAX_BODY_BYTES", "много"},
		{"недопустимый лимит токенов", "GEMINI_MAX_TOKENS", "много"},
		{"недопустимый таймаут провайдера", "GEMINI_TIMEOUT", "вчера"},
		{"недопустимый булев", "GIGACHAT_TLS_SKIP_VERIFY", "может быть"},
		{"недопустимые псевдонимы", "LOOK_MODEL_ALIASES", "без-равно"},
		{"недопустимый таймаут openrouter", "OPENROUTER_TIMEOUT", "не время"},
		{"недопустимый лимит токенов gptunnel", "GPTUNNEL_MAX_TOKENS", "не число"},
		{"недопустимый таймаут gigachat", "GIGACHAT_TIMEOUT", "не время"},
		{"недопустимый таймаут openrouter max_tokens", "OPENROUTER_MAX_TOKENS", "не число"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.key, tt.val)
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), tt.key) {
				t.Errorf("ожидалась ошибка с именем %s, получено %v", tt.key, err)
			}
		})
	}
}

func TestLoadAliases_EnvOverridesDefaults(t *testing.T) {
	clearAll(t)
	aliases, err := loadAliases()
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if aliases["gemeni"] != "gemini-3.6-flash" {
		t.Errorf("встроенный псевдоним gemeni потерялся: %v", aliases["gemeni"])
	}

	t.Setenv("LOOK_MODEL_ALIASES", "gemeni=другая-модель")
	aliases, err = loadAliases()
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if aliases["gemeni"] != "другая-модель" {
		t.Errorf("переопределение из окружения не применилось: %v", aliases)
	}

	t.Setenv("LOOK_MODEL_ALIASES", "без-равно")
	if _, err = loadAliases(); err == nil {
		t.Error("ожидалась ошибка разбора псевдонимов")
	}
}
