package model_aliases

var DefaultModelAliases = map[string]string{
	"gemini": "gemini-3.6-flash",
	"gemeni": "gemini-3.6-flash",
	"гемини": "gemini-3.6-flash",

	"дипсик":   "deepseek/deepseek-v4-flash",
	"deepseek": "deepseek/deepseek-v4-flash",

	"клод":   "claude-4.5-haiku",
	"claude": "claude-4.5-haiku",

	"гпт": "gpt-4o",
	"gpt": "gpt-4o",

	"гигачат":  "GigaChat",
	"gigachat": "GigaChat",

	"фри":  "openrouter/free",
	"free": "openrouter/free",
}

var DefaultFreeModelAliases = map[string]string{
	"инклинг":         "thinkingmachines/inkling:free",
	"inkling":         "thinkingmachines/inkling:free",
	"инклинг-мини":    "thinkingmachines/inkling-small:free",
	"inkling-mini":    "thinkingmachines/inkling-small:free",
	"немотрон-ультра": "nvidia/nemotron-3-ultra-550b-a55b:free",
	"nemotron-ultra":  "nvidia/nemotron-3-ultra-550b-a55b:free",
	"немотрон-супер":  "nvidia/nemotron-3-super-120b-a12b:free",
	"nemotron-super":  "nvidia/nemotron-3-super-120b-a12b:free",
	"немотрон-лайт":   "nvidia/nemotron-3.5-lightning:free",
	"nemotron-light":  "nvidia/nemotron-3.5-lightning:free",
	"немотрон-омни":   "nvidia/nemotron-3-nano-omni-30b-a3b-reasoning:free",
	"nemotron-omni":   "nvidia/nemotron-3-nano-omni-30b-a3b-reasoning:free",
	"гемма":           "google/gemma-4-31b-it:free",
	"gemma":           "google/gemma-4-31b-it:free",
	"гемма-мини":      "google/gemma-4-26b-a4b-it:free",
	"gemma-mini":      "google/gemma-4-26b-a4b-it:free",
	"лагуна":          "poolside/laguna-s-2.1:free",
	"laguna":          "poolside/laguna-s-2.1:free",
	"лагуна-мини":     "poolside/laguna-xs-2.1:free",
	"laguna-mini":     "poolside/laguna-xs-2.1:free",
	"норд-код":        "cohere/north-mini-code:free",
	"north-code":      "cohere/north-mini-code:free",
	"линг-мед":        "inclusionai/ling-3.0-flash-sante:free",
	"ling-med":        "inclusionai/ling-3.0-flash-sante:free",
	"ликвид":          "liquid/lfm-2.5-2.6b:free",
	"liquid":          "liquid/lfm-2.5-2.6b:free",
	"квэн":            "qwen/qwen3.8-27b:free",
	"qwen":            "qwen/qwen3.8-27b:free",
	"кролик":          "stealth/space-bunny-alpha",
	"bunny":           "stealth/space-bunny-alpha",
	"дотс":            "dots-studio/dots-3-note-preview:free",
	"dots":            "dots-studio/dots-3-note-preview:free",
}
