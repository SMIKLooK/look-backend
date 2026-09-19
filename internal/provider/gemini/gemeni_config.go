package gemini

import (
	"net/http"
	"time"
)

const DefaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"

var defaultModels = []string{"gemini-3.8-flash", "gemini-3.6-flash", "gemini-2.5-pro"}

var defaultModelPrefixes = []string{"gemini"}

const maxResponseBytes = 8 << 20

// Config — настройки клиента Gemini.
type Config struct {
	APIKey        string
	BaseURL       string        // по умолчанию https://generativelanguage.googleapis.com/v1beta
	Models        []string      // поддерживаемые модели; пусто — defaultModels
	ModelPrefixes []string      // префиксы моделей; пусто — ["gemini"]
	Timeout       time.Duration // таймаут HTTP-запроса; 0 — без отдельного таймаута
	MaxTokens     int           // 0 — параметр maxOutputTokens не отправляется
	HTTPClient    *http.Client  // опционально; удобно подменять в тестах
}
