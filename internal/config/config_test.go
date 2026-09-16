package config

import (
	"testing"

	"look-backend/internal/keys"
)

// TestLoad_KeysAndModelsFromCode проверяет, что ключи и списки моделей,
// вписанные в internal/keys/keys.go, попадают в конфигурацию без
// переменных окружения.
func TestLoad_KeysAndModelsFromCode(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GEMINI_MODELS", "")

	oldGemini, oldOpenAI, oldAnthropic := keys.Gemini, keys.OpenAI, keys.Anthropic
	oldGeminiModels := keys.GeminiModels
	keys.Gemini, keys.OpenAI, keys.Anthropic = "gemini-key", "openai-key", "anthropic-key"
	keys.GeminiModels = []string{"gemini-2.5-pro"}
	defer func() {
		keys.Gemini, keys.OpenAI, keys.Anthropic = oldGemini, oldOpenAI, oldAnthropic
		keys.GeminiModels = oldGeminiModels
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if cfg.Gemini.APIKey != "gemini-key" || cfg.OpenAI.APIKey != "openai-key" || cfg.Anthropic.APIKey != "anthropic-key" {
		t.Fatalf("ключи из keys.go не подхватились: gemini=%q openai=%q anthropic=%q",
			cfg.Gemini.APIKey, cfg.OpenAI.APIKey, cfg.Anthropic.APIKey)
	}
	if len(cfg.Gemini.Models) != 1 || cfg.Gemini.Models[0] != "gemini-2.5-pro" {
		t.Fatalf("список моделей из keys.go не подхватился: %v", cfg.Gemini.Models)
	}
}

// TestLoad_EnvOverridesCode проверяет приоритет переменных окружения
// над значениями из internal/keys/keys.go.
func TestLoad_EnvOverridesCode(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "env-key")

	old := keys.OpenAI
	keys.OpenAI = "code-key"
	defer func() { keys.OpenAI = old }()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if cfg.OpenAI.APIKey != "env-key" {
		t.Fatalf("переменная окружения должна иметь приоритет: %q", cfg.OpenAI.APIKey)
	}
}

// TestLoad_FreeModelAliases проверяет короткие имена бесплатных моделей
// OpenRouter (выборочно — по одному от каждого вендора).
func TestLoad_FreeModelAliases(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for alias, model := range map[string]string{
		"инклинг":         "thinkingmachines/inkling:free",
		"inkling-mini":    "thinkingmachines/inkling-small:free",
		"немотрон-ультра": "nvidia/nemotron-3-ultra-550b-a55b:free",
		"немотрон-лайт":   "nvidia/nemotron-3.5-lightning:free",
		"гемма":           "google/gemma-4-31b-it:free",
		"gemma-mini":      "google/gemma-4-26b-a4b-it:free",
		"лагуна-мини":     "poolside/laguna-xs-2.1:free",
		"north-code":      "cohere/north-mini-code:free",
		"линг-фин":        "inclusionai/ling-3.0-flash-fin:free",
		"ликвид":          "liquid/lfm-2.5-2.6b:free",
		"nex":             "nex-agi/nex-n2.5-pro:free",
		"дотс":            "dots-studio/dots-3-note-preview:free",
		"фри":             "openrouter/free",
	} {
		if cfg.ModelAliases[alias] != model {
			t.Fatalf("псевдоним %q = %q, ожидалось %q", alias, cfg.ModelAliases[alias], model)
		}
	}
}

// TestLoad_ExtraAliasesFromCode проверяет, что псевдонимы из keys.go
// дополняют базовые, а не заменяют их.
func TestLoad_ExtraAliasesFromCode(t *testing.T) {
	t.Setenv("LOOK_MODEL_ALIASES", "")

	old := keys.ExtraModelAliases
	keys.ExtraModelAliases = map[string]string{"gpt": "gpt-4o"}
	defer func() { keys.ExtraModelAliases = old }()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	// псевдоним из keys.go перекрывает одноимённый базовый
	if cfg.ModelAliases["gpt"] != "gpt-4o" {
		t.Fatalf("псевдоним из keys.go не подхватился: %v", cfg.ModelAliases)
	}
	// остальные базовые псевдонимы на месте
	if cfg.ModelAliases["гпт"] != "openai/gpt-5.6-terra" {
		t.Fatalf("базовые псевдонимы потерялись: %v", cfg.ModelAliases)
	}
}

// TestLoad_DefaultModelAliases проверяет встроенные короткие имена
// (русские и английские), которые не требуют никакой настройки.
func TestLoad_DefaultModelAliases(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for alias, model := range map[string]string{
		"gemini":   "gemini-3.6-flash",
		"gemeni":   "gemini-3.6-flash",
		"гемини":   "gemini-3.6-flash",
		"gpt":      "openai/gpt-5.6-terra",
		"гпт":      "openai/gpt-5.6-terra",
		"клод":     "anthropic/claude-sonnet-5",
		"claude":   "anthropic/claude-sonnet-5",
		"дипсик":   "deepseek/deepseek-v4-flash",
		"deepseek": "deepseek/deepseek-v4-flash",
		"фри":      "openrouter/free",
		"free":     "openrouter/free",
	} {
		if cfg.ModelAliases[alias] != model {
			t.Fatalf("псевдоним %q = %q, ожидалось %q", alias, cfg.ModelAliases[alias], model)
		}
	}
}
