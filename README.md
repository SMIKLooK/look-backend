# look-backend

HTTP-сервис на Go: принимает JSON с текстом, берёт из него имя модели
нейросети и запрос, пересылает запрос выбранной модели и возвращает её ответ.

## Формат текста

```text
<модель> <запрос>
```

Примеры:

```text
gemini какая погода сегодня?
gemeni расскажи анекдот
```

Правила разбора:

- **модель — первое слово текста** (один токен до пробела), далее — запрос;
  знаки препинания между моделью и запросом допустимы (`gemeni, привет`,
  `gemeni: привет`);
- короткие имена работают из коробки: `дипсик`/`deepseek` (через OpenRouter) и `гемини`/`gemeni`/`gemini`
  (напрямую в Google).Полный список—в `internal/model_aliases/model_aliases.go`,
  свои добавляются через `LOOK_MODEL_ALIASES`.

## Быстрый старт

```bash
go run ./cmd/server
```

### Реальный ответ от модели

Получите бесплатный ключ в Google AI Studio ([aistudio.google.com/apikey](https://aistudio.google.com/apikey))
и впишите его в файл [internal/keys/keys.go](internal/keys/keys.go):

```go
var (
	Gemini    = "AIza..."
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
`gemini-2.5-flash`); точное имя видно в поле `model` ответа, (`GEMINI_API_KEY` и т.п.) по-прежнему работают и имеют приоритет — удобно
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

## API

### `POST /api/v1/process`

Тело: `{"text": "<модель> <запрос>"}`.

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


### `GET /api/v1/models`

Список псевдонимов моделей и провайдеров с их моделями.

### `GET /healthz`

Проверка живости.

## Конфигурация

Все настройки — через переменные окружения (см. [.env.example](.env.example)):

| Переменная | По умолчанию | Описание |
|---|---|---|
| `LOOK_ADDR` | `:8080` | адрес HTTP-сервера |
| `LOOK_MODEL_ALIASES` | см. `internal/model_aliases/model_aliases.go` | дополнительные псевдонимы моделей `имя=модель` |
| `LOOK_PROVIDER_TIMEOUT` | `60s` | таймаут одного обращения к провайдеру |
| `LOOK_MAX_BODY_BYTES` | `1048576` | лимит размера тела запроса |
| `LOOK_LOG_FORMAT` | `text` | формат логов: `text` или `json` |
| `GEMINI_API_KEY` | из `internal/keys/keys.go` | без ключа провайдер gemini отключён |
| `GEMINI_BASE_URL` | из `keys.go` или официальный API | адрес прокси в поддерживаемом регионе |
| `GEMINI_MODELS` | `gemini-3.8-flash,gemini-3.6-flash,gemini-2.5-pro` | список моделей |
| `GEMINI_MAX_TOKENS` | `0` (не отправлять) | лимит `maxOutputTokens` |
| `GEMINI_TIMEOUT` | — | отдельный таймаут HTTP-клиента |
| `OPENROUTER_API_KEY` | из `internal/keys/keys.go` | без ключа провайдер openrouter отключён |
| `OPENROUTER_BASE_URL` | `https://openrouter.ai/api/v1` | базовый URL API |
| `OPENROUTER_MODELS` | популярные модели (см. README) | список слагов `вендор/модель` |
| `OPENROUTER_MAX_TOKENS` | `0` (не отправлять) | лимит токенов ответа |
| `OPENROUTER_TIMEOUT` | — | отдельный таймаут HTTP-клиента |

Модель попадает к провайдеру, если совпадает с одной из `*_MODELS`
(регистр не важен) или начинается с одного из префиксов провайдера
(`gemini` — у gemini;)

Зависимости направлены строго в одну сторону:

```
transport/httpapi → service → parser, provider/* → domain
```

Благодаря контракту `provider.Provider` каждая нейросеть — самостоятельный
пакет, который ничего не знает об HTTP и соседях. Тестовый провайдер `echo`,
подменный HTTP-сервер в тестах `gemini` и заглушки в тестах `service`
позволяют проверять сервис без ключей и сети.

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
