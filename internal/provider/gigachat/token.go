package gigachat

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"look-backend/internal/domain"
	"look-backend/internal/provider/kit"
)

// base64AuthKey собирает Authorization-ключ из пары ID + секрет:
// GigaChat ожидает его в формате base64("client_id:client_secret").
func base64AuthKey(clientID, clientSecret string) string {
	return base64.StdEncoding.EncodeToString([]byte(clientID + ":" + clientSecret))
}

func (c *Client) getToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && time.Now().Before(c.expires) {
		return c.token, nil
	}
	if c.authKey == "" {
		return "", domain.NewError(domain.CodeProviderFailed,
			"ключ GigaChat не задан: установите GIGACHAT_API_KEY или пару GIGACHAT_CLIENT_ID + GIGACHAT_CLIENT_SECRET")
	}

	// OAuth по спецификации GigaChat: POST form со scope, Basic-ключ и RqUID.
	form := url.Values{"scope": {c.scope}}
	httpReq, err := kit.NewRequest(ctx, http.MethodPost, c.authURL, []byte(form.Encode()))
	if err != nil {
		return "", domain.WrapError(domain.CodeInternal, err, "не удалось сформировать запрос токена gigachat")
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("RqUID", newUUID())
	httpReq.Header.Set("Authorization", "Basic "+c.authKey)

	result, err := kit.Do(ctx, c.httpClient, "gigachat (OAuth)", httpReq)
	if err != nil {
		return "", err
	}

	var parsed struct {
		AccessToken string `json:"access_token"`
		ExpiresAt   int64  `json:"expires_at"` // unix-время в миллисекундах
	}
	if err := json.Unmarshal(result.Body, &parsed); err != nil {
		return "", domain.WrapError(domain.CodeProviderFailed, err,
			"сервер авторизации gigachat вернул некорректный ответ (HTTP %d): %s", result.StatusCode, kit.TruncateBody(result.Body))
	}
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		msg := fmt.Sprintf("HTTP %d", result.StatusCode)
		if reply, perr := kit.ParseChat(result.Body); perr == nil && reply.ErrText != "" {
			msg += ": " + reply.ErrText
		}
		if result.StatusCode == http.StatusUnauthorized {
			return "", domain.NewError(domain.CodeProviderFailed,
				msg+" — проверьте ключ GigaChat (base64 client_id:client_secret) и scope")
		}
		return "", domain.NewError(domain.CodeProviderFailed, "ошибка авторизации gigachat: "+msg)
	}
	if parsed.AccessToken == "" {
		return "", domain.NewError(domain.CodeProviderFailed, "сервер авторизации gigachat не вернул access_token")
	}

	c.token = parsed.AccessToken
	if parsed.ExpiresAt > 0 {
		c.expires = time.UnixMilli(parsed.ExpiresAt).Add(-30 * time.Second) // небольшой запас
	} else {
		c.expires = time.Now().Add(tokenLifetime)
	}
	if time.Now().Before(c.expires) {
		return c.token, nil
	}
	// сервер прислал уже истекающий срок — не кэшируем
	c.token = ""
	return parsed.AccessToken, nil
}

// invalidateToken сбрасывает закэшированный токен; следующий запрос получит новый.
func (c *Client) invalidateToken() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = ""
	c.expires = time.Time{}
}

// newUUID генерирует uuid v4 для заголовка RqUID.
func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40 // версия 4
	b[8] = (b[8] & 0x3f) | 0x80 // вариант RFC 4122
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
