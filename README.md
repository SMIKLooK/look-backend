# look-backend

HTTP-сервис на Go: принимает JSON с текстом, берёт из него имя модели
нейросети и запрос, пересылает запрос выбранной модели и возвращает её ответ.

## Формат текста

```text
<модель> <запрос>
```

Примеры:

```text
gpt-4o Привет, кто ты?
gemini какая погода сегодня?
gemeni расскажи анекдот
claude-3-sonnet переведи "привет" на английский
```

Правила разбора:

- **модель — первое слово текста** (один токен до пробела), далее — запрос;
  знаки препинания между моделью и запросом допустимы (`gpt-4o, привет`,
  `gpt-4o: привет`);
- ключевые слова `look`/`лук` **больше не требуются**: если старый формат
  (`look gpt-4o привет`) ещё приходит, ключевое слово в начале текста
  просто игнорируется — набор настраивается переменной `LOOK_KEYWORDS`;
- короткие имена работают из коробки: `гпт`/`gpt`, `клод`/`claude`,
  `дипсик`/`deepseek` (через OpenRouter) и `гемини`/`gemeni`/`gemini`
  (напрямую в Google). Полный список — в `internal/config/config.go`,
  свои добавляются через `LOOK_MODEL_ALIASES`.

## Быстрый старт

```bash
go run ./cmd/server
```

Проверка без API-ключей (провайдер `echo` возвращает запрос как есть):

```bash
curl -s -X POST http://localhost:8080/api/v1/process \
  -H 'Content-Type: application/json' \
  -d '{"text": "echo Привет! Кто ты?"}'
```

```json
{
  "status": "ok",
  "model": "echo",
  "provider": "echo",
  "answer": "echo → Привет! Кто ты?",
  "elapsed_ms": 0
}
```

### Реальный ответ от модели

Получите бесплатный ключ в Google AI Studio ([aistudio.google.com/apikey](https://aistudio.google.com/apikey))
и впишите его в файл [internal/keys/keys.go](internal/keys/keys.go):

```go
var (
	Gemini    = "AIza..."
	OpenAI    = ""
	Anthropic = ""
)
```

Пересоберите и запустите — переменные окружения перед запуском задавать
не нужно:

```bash
go run ./cmd/server

curl -s -X POST http://localhost:8080/api/v1/process \
  -H 'Content-Type: application/json' \
  -d '{"text": "gemeni какая погода сегодня?"}'
```

```json
{
  "status": "ok",
  "model": "gemini-3.6-flash",
  "provider": "gemini",
  "answer": "<реальный ответ Gemini>",
  "elapsed_ms": 1234
}
```

`gemeni` — псевдоним, к Google уйдёт модель `gemini-3.6-flash` (Google
рекомендует её новым ключам вместо выведенного из обращения
`gemini-2.5-flash`); точное имя видно в поле `model` ответа. Аналогично работают провайдеры `openai` и
`anthropic` — впишите их ключи в тот же файл. Переменные окружения
(`GEMINI_API_KEY` и т.п.) по-прежнему работают и имеют приоритет — удобно
для деплоя, не меняя код.

### Много моделей через OpenRouter

[OpenRouter](https://openrouter.ai/) — агрегатор: один ключ
([openrouter.ai/settings/keys](https://openrouter.ai/settings/keys)) даёт
доступ к моделям десятков вендоров, а заодно решает проблему
`User location is not supported` — запросы идут через инфраструктуру
OpenRouter. Впишите ключ в `keys.go` (`OpenRouter = "sk-or-..."`) — и
обращайтесь к моделям короткими именами или слагом `вендор/модель`
из каталога [openrouter.ai/models](https://openrouter.ai/models):

| Текст | Кому уйдёт |
|---|---|
| `гпт какая погода сегодня?` | `openai/gpt-5.6-terra` |
| `gpt какая погода сегодня?` | `openai/gpt-5.6-terra` |
| `клод расскажи анекдот` | `anthropic/claude-sonnet-5` |
| `claude расскажи анекдот` | `anthropic/claude-sonnet-5` |
| `дипсик посчитай 2^20` | `deepseek/deepseek-v4-flash` |
| `deepseek посчитай 2^20` | `deepseek/deepseek-v4-flash` |
| `google/gemini-3.8-flash привет` | любой слаг каталога |

Кто отвечает — видно в ответе API (`"provider": "openrouter"`,
`"model": "<полный слаг>"`). Модель по умолчанию для каждого короткого
имени меняется в `internal/config/config.go` (`defaultModelAliases`),
свои псевдонимы добавляются в `ExtraModelAliases` в keys.go:

```go
ExtraModelAliases = map[string]string{
	"лучший": "anthropic/claude-opus-5",
	"грок":   "x-ai/grok-4.6",
}
```

**Бесплатные модели.** У OpenRouter есть модели с нулевой ценой — их ID
оканчиваются на `:free` (фильтр на [openrouter.ai/models](https://openrouter.ai/models):
Max Price → 0). Для каждой встроено короткое имя — русское и английское:

| Короткое имя | ID модели | Что это |
|---|---|---|
| `фри` / `free` | `openrouter/free` | роутер: случайная бесплатная модель |
| `инклинг` / `inkling` | `thinkingmachines/inkling:free` | мультимодальная MoE 975B |
| `инклинг-мини` / `inkling-mini` | `thinkingmachines/inkling-small:free` | мультимодальная MoE 276B |
| `немотрон-ультра` / `nemotron-ultra` | `nvidia/nemotron-3-ultra-550b-a55b:free` | NVIDIA 550B |
| `немотрон-супер` / `nemotron-super` | `nvidia/nemotron-3-super-120b-a12b:free` | NVIDIA 120B |
| `немотрон-лайт` / `nemotron-light` | `nvidia/nemotron-3.5-lightning:free` | NVIDIA, быстрая 30B |
| `немотрон-омни` / `nemotron-omni` | `nvidia/nemotron-3-nano-omni-30b-a3b-reasoning:free` | мультимодальная omni |
| `гемма` / `gemma` | `google/gemma-4-31b-it:free` | компактная Gemma 4 |
| `гемма-мини` / `gemma-mini` | `google/gemma-4-26b-a4b-it:free` | Gemma 4 26B MoE |
| `лагуна` / `laguna` | `poolside/laguna-s-2.1:free` | код-агент 118B |
| `лагуна-мини` / `laguna-mini` | `poolside/laguna-xs-2.1:free` | код 33B |
| `норд-код` / `north-code` | `cohere/north-mini-code:free` | код 30B |
| `линг-мед` / `ling-med` | `inclusionai/ling-3.0-flash-sante:free` | медицина |
| `линг-фин` / `ling-fin` | `inclusionai/ling-3.0-flash-fin:free` | финансы |
| `ликвид` / `liquid` | `liquid/lfm-2.5-2.6b:free` | ультракомпактная 2.6B |
| `некс` / `nex` | `nex-agi/nex-n2.5-pro:free` | агентная MoE |
| `некс-мини` / `nex-mini` | `nex-agi/nex-n2.5-mini:free` | агентная MoE, младшая |
| `дотс` / `dots` | `dots-studio/dots-3-note-preview:free` | 280B MoE, превью до 30.09.2026 |

Пример: `инклинг какая погода сегодня?` → `thinkingmachines/inkling:free`.
Список задаётся в `internal/config/config.go` (`defaultFreeAliases`) —
бесплатные модели периодически отключают и добавляют, при ошибке от
OpenRouter сверяйте ID с каталогом. Модель `nemotron-3.5-content-safety:free`
— это фильтр контента, а не чат-модель, для неё псевдонима нет. У
бесплатных моделей есть лимиты запросов — актуальные смотрите на странице
конкретной модели.

Короткие имена без слэша, не входящие в псевдонимы, тоже работают, если
модель есть в `OpenRouterModels` (в keys.go или `OPENROUTER_MODELS`):
`gpt-5.6-terra ...` развернётся в `openai/gpt-5.6-terra`.

> Если OpenRouter отвечает `Access denied by security policy` — их защита
> блокирует регион/IP сети, откуда работает сервер. Обходится тем же
> способом, что и гео-блок Google: бесплатный Cloudflare Worker с кодом
>
> ```js
> export default {
>   async fetch(request) {
>     const url = new URL(request.url);
>     url.host = "openrouter.ai";
>     return fetch(new Request(url, request));
>   }
> }
> ```
>
> и адрес воркера в keys.go: `OpenRouterBaseURL = "https://имя.workers.dev/api/v1"`.

> ⚠️ Файл с ключами нельзя коммитить в публичный репозиторий — если код
> уходит в открытый доступ, держите ключи только в переменных окружения.

Если Google отвечает `User location is not supported for the API use`,
регион, откуда уходят запросы, не поддерживается Gemini API. В этом случае
впишите в [internal/keys/keys.go](internal/keys/keys.go) адрес прокси
в поддерживаемом регионе (`GeminiBaseURL`), а ключ оставьте как есть.
Варианты: собственный реверс-прокси на VPS/Cloudflare Worker, который
форвардит запросы в `generativelanguage.googleapis.com`, либо агрегаторы
вроде OpenRouter — для них уже готов провайдер `openai`: впишите
`OpenAIBaseURL = "https://openrouter.ai/api/v1"` и ключ OpenRouter в `OpenAI`.

## API

### `POST /api/v1/process`

Тело: `{"text": "<keyword> <модель> <запрос>"}`.

Успех — `200 OK`:

```json
{"status": "ok", "model": "...", "provider": "...", "answer": "...", "elapsed_ms": 123}
```

Ошибка — JSON с кодом и сообщением:

```json
{"status": "error", "error": {"code": "MODEL_NOT_SPECIFIED", "message": "..."}}
```

| HTTP | Код | Причина |
|---|---|---|
| 400 | `INVALID_JSON` | тело запроса — не корректный JSON |
| 400 | `EMPTY_TEXT` | поле `text` пустое |
| 400 | `MODEL_NOT_SPECIFIED` | в тексте нет модели |
| 400 | `PROMPT_NOT_SPECIFIED` | после модели нет запроса |
| 400 | `UNKNOWN_MODEL` | модель не обслуживает ни один провайдер |
| 413 | `PAYLOAD_TOO_LARGE` | тело запроса больше `LOOK_MAX_BODY_BYTES` |
| 502 | `PROVIDER_ERROR` | провайдер вернул ошибку / недоступен |
| 504 | `TIMEOUT` | истёк таймаут обращения к провайдеру |
| 500 | `INTERNAL` | внутренняя ошибка сервера |

### `POST /api/v1/chat` — диалог с памятью

Сама модель ничего не помнит: диалог — это массив сообщений, который клиент
присылает целиком при каждом запросе. Этот эндпоинт делает это за вас:
сохраняет историю сессии и отправляет её модели вместе с новым запросом.

Тело: `{"session_id": "...", "text": "<модель> <запрос>"}`. Если
`session_id` не указан — сервер сгенерирует новый и вернёт его; дальше
передавайте его в каждом запросе, чтобы модель видела предыдущие реплики.

```json
{
  "status": "ok",
  "session_id": "9f2c7e1a4b0d",
  "model": "openai/gpt-5.6-terra",
  "provider": "openrouter",
  "answer": "…",
  "elapsed_ms": 512,
  "messages": 5
}
```

`messages` — сколько сообщений ушло модели (история + новый запрос);
верхняя граница настраивается `LOOK_MAX_HISTORY_MESSAGES`, чтобы контекст
не разрастался бесконечно. Старые запросы через `POST /api/v1/process`
работают как раньше — без истории.

### `GET /api/v1/chat/{session_id}` — история сессии

```json
{"status": "ok", "session_id": "9f2c7e1a4b0d", "messages": [{"role": "user", "content": "…"}, {"role": "assistant", "content": "…"}]}
```

### `DELETE /api/v1/chat/{session_id}` — очистить историю

### `GET /api/v1/models`

Список ключевых слов, псевдонимов моделей и провайдеров с их моделями.

### `GET /healthz`

Проверка живости.

## Конфигурация

Все настройки — через переменные окружения (см. [.env.example](.env.example)):

| Переменная | По умолчанию | Описание |
|---|---|---|
| `LOOK_ADDR` | `:8080` | адрес HTTP-сервера |
| `LOOK_KEYWORDS` | `look,лук` | ключевые слова старого формата, игнорируются перед моделью |
| `LOOK_MODEL_ALIASES` | `gemini=gemini-3.6-flash,gemeni=gemini-3.6-flash,claude=claude-sonnet-4-20250514` | псевдонимы моделей `имя=модель` |
| `LOOK_PROVIDER_TIMEOUT` | `60s` | таймаут одного обращения к провайдеру |
| `LOOK_MAX_BODY_BYTES` | `1048576` | лимит размера тела запроса |
| `LOOK_MAX_HISTORY_MESSAGES` | `40` | максимум сообщений сессии, отправляемых модели (`0` — без ограничения) |
| `LOOK_LOG_FORMAT` | `text` | формат логов: `text` или `json` |
| `GEMINI_API_KEY` | из `internal/keys/keys.go` | без ключа провайдер gemini отключён |
| `GEMINI_BASE_URL` | из `keys.go` или официальный API | адрес прокси в поддерживаемом регионе |
| `GEMINI_MODELS` | `gemini-3.8-flash,gemini-3.6-flash,gemini-2.5-pro` | список моделей |
| `GEMINI_MAX_TOKENS` | `0` (не отправлять) | лимит `maxOutputTokens` |
| `GEMINI_TIMEOUT` | — | отдельный таймаут HTTP-клиента |
| `OPENAI_API_KEY` | из `internal/keys/keys.go` | без ключа провайдер openai отключён |
| `OPENAI_BASE_URL` | `https://api.openai.com/v1` | любой OpenAI-совместимый API |
| `OPENAI_MODELS` | `gpt-4o,gpt-4o-mini` | список моделей |
| `OPENAI_MAX_TOKENS` | `0` (не отправлять) | лимит токенов ответа |
| `OPENAI_TIMEOUT` | — | отдельный таймаут HTTP-клиента |
| `ANTHROPIC_API_KEY` | из `internal/keys/keys.go` | без ключа провайдер anthropic отключён |
| `ANTHROPIC_BASE_URL` | `https://api.anthropic.com` | базовый URL API |
| `ANTHROPIC_MODELS` | `claude-sonnet-4-20250514,...` | список моделей |
| `ANTHROPIC_MAX_TOKENS` | `1024` | обязателен для API Anthropic |
| `ANTHROPIC_TIMEOUT` | — | отдельный таймаут HTTP-клиента |
| `OPENROUTER_API_KEY` | из `internal/keys/keys.go` | без ключа провайдер openrouter отключён |
| `OPENROUTER_BASE_URL` | `https://openrouter.ai/api/v1` | базовый URL API |
| `OPENROUTER_MODELS` | популярные модели (см. README) | список слагов `вендор/модель` |
| `OPENROUTER_MAX_TOKENS` | `0` (не отправлять) | лимит токенов ответа |
| `OPENROUTER_TIMEOUT` | — | отдельный таймаут HTTP-клиента |

Модель попадает к провайдеру, если совпадает с одной из `*_MODELS`
(регистр не важен) или начинается с одного из префиксов провайдера
(`gemini` — у gemini; `gpt`, `o1`, `o3`, `o4` — у openai; `claude` — у anthropic).

## Архитектура

```
cmd/server/            точка входа: флаги, конфигурация, запуск app
internal/
  domain/              общие типы и доменные ошибки (код ошибки → HTTP-статус)
  keys/                API-ключи и списки моделей, вписанные в код
  config/              чтение конфигурации (keys.go по умолчанию, env поверх)
  parser/              разбор текста: keyword → модель + запрос (+ тесты)
  provider/            контракт Provider и Registry (в т.ч. псевдонимы моделей)
    echo/              тестовый провайдер
    gemini/            Google Gemini (/models/{model}:generateContent) (+ тесты)
    openai/            OpenAI-совместимые API (/chat/completions)
    anthropic/         Anthropic Claude (/v1/messages)
    openrouter/        агрегатор OpenRouter — модели всех вендоров (+ тесты)
  service/             бизнес-логика: parser + registry → ответ модели
  transport/httpapi/   HTTP-слой: обработчики, JSON, middleware, сервер
  storage/             история диалогов: контракт Store + реализация в памяти
  app/                 компоновка всего приложения + graceful shutdown
```

Зависимости направлены строго в одну сторону:

```
transport/httpapi → service → parser, provider/* → domain
```

Благодаря контракту `provider.Provider` каждая нейросеть — самостоятельный
пакет, который ничего не знает об HTTP и соседях. Тестовый провайдер `echo`,
подменный HTTP-сервер в тестах `gemini` и заглушки в тестах `service`
позволяют проверять сервис без ключей и сети.

## История диалога и PostgreSQL

История хранится за интерфейсом `storage.Store`; сейчас реализация —
в памяти процесса (`internal/storage/memory.go`): неактивные сессии
автоматически чистятся (24 часа / проверка раз в 10 минут), при перезапуске
сервера история теряется.

Если нужна персистентность или несколько инстансов за балансировщиком —
добавьте PostgreSQL: установите драйвер (`go get github.com/jackc/pgx/v5`),
создайте `internal/storage/postgres/postgres.go`, реализующий тот же
интерфейс (`Append` → `INSERT`, `History` → `SELECT ... ORDER BY id`,
`Reset` → `DELETE`), и подставьте его в `internal/app/app.go` вместо
`storage.NewMemoryStore`. Таблицы для старта:

```sql
CREATE TABLE messages (
    id         BIGSERIAL PRIMARY KEY,
    session_id TEXT        NOT NULL,
    role       TEXT        NOT NULL,
    content    TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON messages (session_id, id);
```

## Как добавить нового провайдера

1. Создайте пакет `internal/provider/<имя>/` и реализуйте интерфейс
   `provider.Provider` (`Name`, `Models`, `Supports`, `Complete`) —
   ориентир: `internal/provider/gemini/gemini.go`.
2. Добавьте в `internal/config/config.go` чтение настроек нового провайдера,
   а его ключ — в `internal/keys/keys.go`.
3. Зарегистрируйте провайдера в `internal/app/app.go`
   (`registry.Register(...)`); при необходимости — псевдонимы в
   `LOOK_MODEL_ALIASES`.
4. Добавьте тесты с подменным HTTP-сервером (`httptest.NewServer`).

HTTP-слой, парсер и сервис при этом менять не нужно.

## Тестирование через Postman

В репозитории есть готовая коллекция [postman_collection.json](postman_collection.json):
Postman → **Import** → выбрать файл (или перетащить его в окно Postman).
Внутри — healthz, список моделей и запросы к `/api/v1/process`: успешный
(echo), реальный ответ Gemini и типовые ошибки, с автопроверками ответов.

Адрес сервера задаётся переменной коллекции `baseUrl` (по умолчанию
`http://localhost:8080`); меняется в Collection → Variables, если сервер
запущен на другом порту (`-addr` / `LOOK_ADDR`).

Вручную: метод `POST`, URL `http://localhost:8080/api/v1/process`,
Body → raw → JSON:

```json
{"text": "gemeni какая погода сегодня?"}
```

## Разработка

```bash
make run    # запустить сервер
make test   # все тесты
make vet    # статический анализ
make build  # бинарник в bin/look-server
```
