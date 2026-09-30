// Package gemini — провайдер Google Gemini (Generative Language API).
package gemini

import (
	"net/http"
	"strings"

	"look-backend/internal/provider/kit"
)

type Client struct {
	apiKey     string
	baseURL    string
	models     []string
	prefixes   []string
	maxTokens  int
	httpClient *http.Client
}

func New(cfg Config) *Client {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	models := cfg.Models
	if len(models) == 0 {
		models = defaultModels
	}
	prefixes := cfg.ModelPrefixes
	if len(prefixes) == 0 {
		prefixes = defaultModelPrefixes
	}
	httpClient := kit.NewHTTPClient(cfg.Timeout, cfg.HTTPClient)
	return &Client{
		apiKey:     cfg.APIKey,
		baseURL:    baseURL,
		models:     models,
		prefixes:   prefixes,
		maxTokens:  cfg.MaxTokens,
		httpClient: httpClient,
	}
}

func (c *Client) Name() string { return "gemini" }

func (c *Client) Models() []string { return append([]string(nil), c.models...) }

func (c *Client) Supports(model string) bool {
	if kit.ContainsFold(c.models, model) {
		return true
	}
	m := strings.ToLower(model)
	for _, p := range c.prefixes {
		if strings.HasPrefix(m, strings.ToLower(p)) {
			return true
		}
	}
	return false
}
