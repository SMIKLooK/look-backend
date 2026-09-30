package gigachat

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"look-backend/internal/domain"
	"look-backend/internal/provider/kit"
)

const (
	DefaultBaseURL = "https://gigachat.devices.sberbank.ru/api/v1"
	DefaultAuthURL = "https://ngw.devices.sberbank.ru:9443/api/v2/oauth"
	DefaultScope   = "GIGACHAT_API_PERS"
)

// tokenLifetime — время жизни access-токена, если сервер не прислал expires_at.
const tokenLifetime = 25 * time.Minute

var defaultModels = []string{
	"GigaChat",
	"GigaChat-Pro",
	"GigaChat-Max",
}

// errTokenRejected — внутренний признак того, что сервер отклонил токен (401):
// нужен, чтобы sendWithToken отличал эту ситуацию от прочих ошибок провайдера.
var errTokenRejected = errors.New("gigachat: токен отклонён")

// Config — настройки клиента GigaChat.
type Config struct {
	APIKey        string // Authorization-ключ из консоли (base64 client_id:client_secret)
	ClientID      string // альтернатива ключу: пара ID + секрет
	ClientSecret  string
	Scope         string        // по умолчанию GIGACHAT_API_PERS
	BaseURL       string        // по умолчанию https://gigachat.devices.sberbank.ru/api/v1
	AuthURL       string        // по умолчанию https://ngw.devices.sberbank.ru:9443/api/v2/oauth
	Models        []string      // поддерживаемые модели; пусто — defaultModels
	MaxTokens     int           // 0 — параметр max_tokens не отправляется
	Timeout       time.Duration // таймаут HTTP-запроса; 0 — без отдельного таймаута
	TLSSkipVerify bool          // true — не проверять TLS-сертификаты (нужны НУЦ Минцифры)
	HTTPClient    *http.Client  // опционально; удобно подменять в тестах
}

type Client struct {
	authKey    string // готовое значение для заголовка Basic (base64 client_id:client_secret)
	scope      string
	authURL    string
	baseURL    string
	models     []string
	maxTokens  int
	httpClient *http.Client

	mu      sync.Mutex
	token   string
	expires time.Time
}

func New(cfg Config) *Client {
	authKey := cfg.APIKey
	if authKey == "" && cfg.ClientID != "" && cfg.ClientSecret != "" {
		authKey = base64AuthKey(cfg.ClientID, cfg.ClientSecret)
	}
	scope := cfg.Scope
	if scope == "" {
		scope = DefaultScope
	}
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	authURL := strings.TrimRight(cfg.AuthURL, "/")
	if authURL == "" {
		authURL = DefaultAuthURL
	}
	models := cfg.Models
	if len(models) == 0 {
		models = defaultModels
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		// GigaChat требует сертификатов НУЦ Минцифры: разрешаем отключить
		// проверку TLS, чтобы клиент работал до установки сертификатов.
		transport := http.DefaultTransport.(*http.Transport).Clone()
		if cfg.TLSSkipVerify {
			transport.TLSClientConfig.InsecureSkipVerify = true
		}
		httpClient = &http.Client{Transport: transport}
		if cfg.Timeout > 0 {
			httpClient.Timeout = cfg.Timeout
		}
	}
	return &Client{
		authKey:    authKey,
		scope:      scope,
		authURL:    authURL,
		baseURL:    baseURL,
		models:     models,
		maxTokens:  cfg.MaxTokens,
		httpClient: httpClient,
	}
}

func (c *Client) Name() string { return "gigachat" }

func (c *Client) Models() []string { return append([]string(nil), c.models...) }

// Supports принимает точные имена моделей GigaChat в любом регистре
// и любое имя с префиксом "gigachat" (GigaChat-2-Pro и т.п.).
func (c *Client) Supports(model string) bool {
	if kit.ContainsFold(c.models, model) {
		return true
	}
	return strings.HasPrefix(strings.ToLower(model), "gigachat")
}

// resolveModel приводит имя к точному ID из каталога (регистр не важен).
func (c *Client) resolveModel(model string) string {
	return kit.ResolveFold(c.models, model)
}

// Complete отправляет запрос в /chat/completions и возвращает текст ответа.
// При 401 (токен истёк) токен запрашивается заново и запрос повторяется один раз.
func (c *Client) Complete(ctx context.Context, req domain.Request) (domain.Response, error) {
	if err := ctx.Err(); err != nil {
		return domain.Response{}, domain.WrapError(domain.CodeTimeout, err, "контекст отменён до запроса к gigachat")
	}
	if c.authKey == "" {
		return domain.Response{}, domain.NewError(domain.CodeProviderFailed,
			"ключ GigaChat не задан: установите GIGACHAT_API_KEY или пару GIGACHAT_CLIENT_ID + GIGACHAT_CLIENT_SECRET")
	}

	model := c.resolveModel(req.Model)
	body, err := kit.MarshalChat(model, req.Messages, c.maxTokens)
	if err != nil {
		return domain.Response{}, domain.WrapError(domain.CodeInternal, err, "не удалось сериализовать запрос к gigachat")
	}

	content, err := c.sendWithToken(ctx, body)
	if err != nil {
		return domain.Response{}, err
	}
	return domain.Response{
		Model:    model,
		Provider: c.Name(),
		Content:  content,
	}, nil
}

// sendWithToken отправляет запрос чата; при 401 обновляет токен и повторяет.
func (c *Client) sendWithToken(ctx context.Context, body []byte) (string, error) {
	content, err := c.sendChat(ctx, body)
	if err == nil {
		return content, nil
	}
	if !errors.Is(err, errTokenRejected) {
		return "", err
	}
	// токен истёк на стороне сервера — сбрасываем и пробуем ещё раз
	c.invalidateToken()
	content, err = c.sendChat(ctx, body)
	if err == nil {
		return content, nil
	}
	if errors.Is(err, errTokenRejected) {
		// повторная попытка с новым токеном тоже не прошла — проблема не в токене
		return "", domain.NewError(domain.CodeProviderFailed,
			"gigachat отклонил запрос даже с обновлённым токеном: проверьте ключ и scope")
	}
	return "", err
}

// sendChat выполняет один запрос чата с текущим токеном.
func (c *Client) sendChat(ctx context.Context, body []byte) (string, error) {
	token, err := c.getToken(ctx)
	if err != nil {
		return "", err
	}

	httpReq, err := kit.NewRequest(ctx, http.MethodPost, c.baseURL+"/chat/completions", body)
	if err != nil {
		return "", domain.WrapError(domain.CodeInternal, err, "не удалось сформировать запрос к gigachat")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	result, err := kit.Do(ctx, c.httpClient, c.Name(), httpReq)
	if err != nil {
		return "", err
	}

	reply, parseErr := kit.ParseChat(result.Body)
	if parseErr != nil {
		return "", domain.WrapError(domain.CodeProviderFailed, parseErr,
			"gigachat вернул некорректный ответ (HTTP %d): %s", result.StatusCode, kit.TruncateBody(result.Body))
	}
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		msg := fmt.Sprintf("HTTP %d", result.StatusCode)
		if reply.ErrText != "" {
			msg += ": " + reply.ErrText
		}
		if result.StatusCode == http.StatusUnauthorized {
			return "", domain.WrapError(domain.CodeProviderFailed, errTokenRejected, msg)
		}
		return "", domain.NewError(domain.CodeProviderFailed, "ошибка gigachat: "+msg)
	}
	if reply.Choices == 0 {
		return "", domain.NewError(domain.CodeProviderFailed, "gigachat вернул пустой список choices")
	}
	return reply.Content, nil
}
