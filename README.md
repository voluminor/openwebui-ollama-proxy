# openwebui-ollama-proxy

Прокси-сервер, который транслирует вызовы Ollama API в формат OpenAI и перенаправляет их
на [Open WebUI](https://github.com/open-webui/open-webui). Позволяет нативным Ollama-клиентам работать с моделями,
размещёнными на Open WebUI, без модификации клиентов.

## Зачем это нужно

```
┌──────────────────────┐           ┌───────────────────────┐          ┌──────────────┐
│   Ollama-клиенты     │  Ollama   │  openwebui-ollama-    │  OpenAI  │              │
│                      │   API     │       proxy           │   API    │  Open WebUI  │
│  Ollie, Enchanted,   ├──────────►│                       ├─────────►│              │
│  CLI, другие...      │           │  Трансляция форматов  │          │  (upstream)  │
└──────────────────────┘           └───────────────────────┘          └──────────────┘
```

Open WebUI предоставляет OpenAI-совместимый API, а многие десктопные и мобильные клиенты поддерживают только Ollama API.
Этот прокси решает проблему совместимости — клиенты подключаются к прокси как к обычному Ollama-серверу, а прокси
транслирует запросы на Open WebUI.

## Возможности

- Streaming и non-streaming режимы для `/api/chat` и `/api/generate`
- Мультимодальность: изображения пробрасываются как OpenAI content parts с автоопределением MIME-типа
- Трёхуровневый кеш списка моделей: память → диск → upstream
- Кеш метаданных моделей с настраиваемым TTL
- AES-256-GCM шифрование дискового кеша с SHA-256 integrity check
- Автоматическое управление JWT-токенами с зашифрованным persistent хранением сессии
- Rate limiting (token bucket)
- CORS с настраиваемыми origins
- Graceful shutdown по SIGINT / SIGTERM
- Нулевые внешние зависимости — только Go stdlib
- Кросс-платформенная сборка на 35+ целей

## Быстрый старт

### Установка из релиза

Скачать бинарник для вашей платформы со [страницы релизов](../../releases).

### Сборка из исходников

```bash
git clone <repo-url>
cd openwebui-ollama-proxy
go generate ./...
go build -o openwebui-ollama-proxy ./
```

### Запуск

```bash
./openwebui-ollama-proxy \
  --openwebui-url https://your-openwebui.example.com \
  --email user@example.com \
  --password yourpassword
```

Прокси запустится на `0.0.0.0:11434` (стандартный порт Ollama). В клиенте укажите адрес прокси вместо адреса
Ollama-сервера.

## Архитектура

### Общая схема работы

```mermaid
flowchart TB
    subgraph Клиенты
        A1[Ollie]
        A2[Enchanted]
        A3[CLI / curl]
        A4[Другие Ollama-клиенты]
    end

    subgraph Proxy["openwebui-ollama-proxy :11434"]
        direction TB
        CORS[CORS middleware]
        RL[Rate Limiter]
        Router[HTTP Router]
        Router --> H_CHAT["/api/chat"]
        Router --> H_GEN["/api/generate"]
        Router --> H_TAGS["/api/tags"]
        Router --> H_SHOW["/api/show"]
        Router --> H_VER["/api/version"]
        Router --> H_PS["/api/ps"]
        Router --> H_STUB["Stubs: pull, push,\ncreate, delete, copy"]
    end

    subgraph Backend["Open WebUI"]
        OAI_CHAT["/api/chat/completions"]
        OAI_MODELS["/api/models"]
        OAI_AUTH["/api/v1/auths/signin"]
    end

    A1 & A2 & A3 & A4 -->|Ollama API| CORS
    CORS --> RL --> Router
    H_CHAT -->|OpenAI формат| OAI_CHAT
    H_GEN -->|OpenAI формат| OAI_CHAT
    H_TAGS -->|GET| OAI_MODELS
    H_SHOW -.->|stub ответ| H_SHOW
    Proxy -.->|JWT auth| OAI_AUTH
```

### Обработка запроса `/api/chat`

```mermaid
sequenceDiagram
    participant C as Ollama-клиент
    participant P as Proxy
    participant A as Auth
    participant U as Open WebUI
    C ->> P: POST /api/chat (Ollama формат)
    P ->> P: Декодировать JSON, валидация
    P ->> A: EnsureToken()
    alt Токен валиден
        A -->> P: JWT токен
    else Токен истёк / нет
        A ->> U: POST /api/v1/auths/signin
        U -->> A: JWT токен
        A ->> A: Сохранить сессию (AES-256-GCM)
        A -->> P: JWT токен
    end

    P ->> P: Конвертация Ollama → OpenAI
    Note over P: messages, options, images,<br/>format → OpenAI ChatRequest
    P ->> U: POST /api/chat/completions (OpenAI формат)

    alt Streaming
        loop SSE-события
            U -->> P: data: {"choices":[...]}
            P -->> C: NDJSON {"message":{"content":"..."}}
        end
        U -->> P: data: [DONE]
        P -->> C: {"done":true, "done_reason":"stop"}
    else Non-streaming
        U -->> P: {"choices":[{"message":{"content":"..."}}]}
        P -->> C: {"message":{"content":"..."}, "done":true}
    end
```

### Трёхуровневый кеш моделей (`/api/tags`)

```mermaid
flowchart TB
    REQ["GET /api/tags"] --> L1{"L1: In-memory кеш\n(RWMutex)"}
    L1 -->|Hit + TTL валиден| RES_L1["Ответ из памяти"]
    L1 -->|Miss| LOCK["tagsFetchMu.Lock()\n(защита от thundering herd)"]
    LOCK --> L1_RE{"L1 повторная\nпроверка"}
    L1_RE -->|Hit| RES_L1_2["Ответ из памяти\n(загружен другой горутиной)"]
    L1_RE -->|Miss| L2{"L2: Диск\n(AES-256-GCM)"}
    L2 -->|Hit + TTL валиден| UPD_L1_FROM_DISK["Обновить L1"] --> RES_L2["Ответ с диска"]
    L2 -->|Miss / expired| L3["L3: Open WebUI\nGET /api/models"]
    L3 --> UPD_ALL["Обновить L1 + L2"]
    UPD_ALL --> RES_L3["Ответ из upstream"]
    style L1 fill: #4a9, stroke: #333
    style L2 fill: #49a, stroke: #333
    style L3 fill: #a94, stroke: #333
```

### Управление JWT-сессией

```mermaid
stateDiagram-v2
    [*] --> НетТокена: Старт
    НетТокена --> ЧтениеКеша: loadSession()
    ЧтениеКеша --> ТокенВалиден: Кеш найден,\nне истёк
    ЧтениеКеша --> Логин: Кеш отсутствует\nили mismatch
    ТокенВалиден --> ОтдатьТокен: TTL > 30 сек
    ТокенВалиден --> Логин: TTL ≤ 30 сек\n(grace period)
    Логин --> СохранитьКеш: Успех
    Логин --> Ошибка: Неверные\nкреденшлы / сеть
    СохранитьКеш --> ОтдатьТокен
    ОтдатьТокен --> [*]
    Ошибка --> [*]
```

### Формат зашифрованного кеша

```mermaid
block-beta
    columns 4
    A["Magic\n2 байта"]:1
    B["Nonce\n12 байт"]:1
    C["Ciphertext\n(AES-256-GCM)\nпеременная длина"]:1
    D["Integrity\nSHA-256\n32 байта"]:1
    style A fill: #e74, stroke: #333, color: #fff
    style B fill: #47b, stroke: #333, color: #fff
    style C fill: #4a9, stroke: #333, color: #fff
    style D fill: #a84, stroke: #333, color: #fff
```

| Magic   | Тип данных        | Файл              |
|---------|-------------------|-------------------|
| `CA 01` | Сессия (JWT)      | `session.bin`     |
| `CA 02` | Список моделей    | `tags.bin`        |
| `CA 03` | Метаданные модели | `show_<hash>.bin` |

Ключ AES выводится как `SHA-256(integrity_hash + commit_hash)`. При каждой новой сборке кеш инвалидируется
автоматически.

### Конвертация форматов

```mermaid
flowchart LR
    subgraph Ollama["Ollama запрос"]
        O_MODEL[model]
        O_MSG[messages]
        O_SYS[system]
        O_IMG[images]
        O_OPT[options]
        O_FMT[format]
        O_STREAM[stream]
    end

    subgraph OpenAI["OpenAI запрос"]
        A_MODEL[model]
        A_MSG[messages]
        A_STREAM[stream]
        A_TEMP["temperature"]
        A_TOPP["top_p"]
        A_MAX["max_tokens"]
        A_STOP["stop"]
        A_SEED["seed"]
        A_FP["frequency_penalty"]
        A_PP["presence_penalty"]
        A_RF["response_format"]
    end

    O_MODEL --> A_MODEL
    O_MSG --> A_MSG
    O_SYS -->|" prepend as system\nmessage "| A_MSG
    O_IMG -->|" base64 → data URL\nс автоMIME "| A_MSG
    O_STREAM --> A_STREAM
    O_OPT -->|temperature| A_TEMP
    O_OPT -->|top_p| A_TOPP
    O_OPT -->|num_predict| A_MAX
    O_OPT -->|stop| A_STOP
    O_OPT -->|seed| A_SEED
    O_OPT -->|frequency_penalty| A_FP
    O_OPT -->|presence_penalty| A_PP
    O_FMT -->|" 'json' → json_object\nschema → json_schema "| A_RF
```

## Конфигурация

### Обязательные параметры

| Флаг                  | Описание               |
|-----------------------|------------------------|
| `--openwebui-url URL` | URL сервера Open WebUI |
| `--email EMAIL`       | Email для авторизации  |
| `--password PASSWORD` | Пароль для авторизации |

### Дополнительные параметры

| Флаг                    | По умолчанию         | Описание                                     |
|-------------------------|----------------------|----------------------------------------------|
| `--host`                | `0.0.0.0`            | Адрес привязки                               |
| `--port`                | `11434`              | Порт (стандартный для Ollama)                |
| `--cache-dir`           | `./cache`            | Директория для кеш-файлов                    |
| `--tags-ttl`            | `10m`                | TTL кеша списка моделей                      |
| `--show-ttl`            | `30m`                | TTL кеша метаданных модели                   |
| `--timeout`             | `30s`                | Таймаут для non-streaming запросов           |
| `--stream-idle-timeout` | `5m`                 | Таймаут бездействия стрима (0 = отключён)    |
| `--shutdown-timeout`    | `5s`                 | Таймаут graceful shutdown                    |
| `--max-body`            | `104857600` (100 МБ) | Максимальный размер тела запроса             |
| `--max-error-body`      | `1048576` (1 МБ)     | Максимальный размер тела ошибки upstream     |
| `--cors-origins`        | `*`                  | CORS Allow-Origin (пусто = отключён)         |
| `--rate-limit`          | `0`                  | Глобальный лимит запросов/сек (0 = отключён) |
| `--ollama-version`      | `0.5.4`              | Версия Ollama API для клиентов               |

### Информационные флаги

| Флаг                | Описание                                      |
|---------------------|-----------------------------------------------|
| `-i`                | Вывести информацию о сборке (текст) и выйти   |
| `--info text\|json` | Вывести информацию о сборке в формате и выйти |
| `-h`, `--help`      | Справка по флагам                             |

## API-эндпоинты

### Поддерживаемые

| Эндпоинт        | Метод     | Описание                                     |
|-----------------|-----------|----------------------------------------------|
| `/`             | GET, HEAD | Health check. Возвращает `Ollama is running` |
| `/api/version`  | GET       | Версия Ollama API (настраиваемая)            |
| `/api/tags`     | GET       | Список моделей (трёхуровневый кеш)           |
| `/api/show`     | POST      | Метаданные модели (stub + кеш)               |
| `/api/ps`       | GET       | Запущенные модели (пустой список)            |
| `/api/chat`     | POST      | Чат (streaming / non-streaming)              |
| `/api/generate` | POST      | Генерация текста (streaming / non-streaming) |

### Заблокированные (403 Forbidden)

| Эндпоинт      | Метод  | Причина                                    |
|---------------|--------|--------------------------------------------|
| `/api/pull`   | POST   | Управление моделями через прокси запрещено |
| `/api/push`   | POST   | —                                          |
| `/api/create` | POST   | —                                          |
| `/api/delete` | DELETE | —                                          |
| `/api/copy`   | POST   | —                                          |

### Не реализованные (501 Not Implemented)

| Эндпоинт          | Метод | Причина                      |
|-------------------|-------|------------------------------|
| `/api/embed`      | POST  | Embeddings не поддерживаются |
| `/api/embeddings` | POST  | —                            |

## Структура проекта

```
openwebui-ollama-proxy/
├── main.go                  # Точка входа, CLI-аргументы, graceful shutdown
├── server.go                # HTTP-сервер, роутинг, CORS, rate limiter
├── handler_chat.go          # POST /api/chat (streaming + non-streaming)
├── handler_generate.go      # POST /api/generate (streaming + non-streaming)
├── handler_models.go        # GET /api/tags, POST /api/show, GET /api/ps
├── handler_stubs.go         # Заглушки для запрещённых/неподдерживаемых эндпоинтов
├── stream.go                # SSE-парсер (Open WebUI → NDJSON Ollama)
├── util.go                  # Хелперы: JSON, конвертация форматов, MIME
├── gen.go                   # go:generate директивы
│
├── auth/
│   ├── auth.go              # JWT-авторизация, auto-refresh, session persistence
│   └── auth_test.go
│
├── cache/
│   ├── cache.go             # AES-256-GCM шифрование, Read[T]/Write[T] generics
│   ├── session.go           # Кеш сессии (magic: CA 01)
│   ├── tags.go              # Кеш списка моделей (magic: CA 02)
│   ├── show.go              # Кеш метаданных модели (magic: CA 03)
│   └── cache_test.go
│
├── ollama/
│   └── types.go             # Типы данных Ollama API
│
├── openai/
│   └── types.go             # Типы данных OpenAI API
│
├── target/
│   └── value_project.go     # Генерируемые константы сборки (не редактировать)
│
├── _run/                    # Скрипты сборки и CI
│   ├── scripts/
│   │   ├── sys.sh           # Управление версиями
│   │   ├── git.sh           # Git-хуки
│   │   └── go_creator_const.sh  # Генерация target/value_project.go
│   └── values/
│       ├── name.txt         # Имя проекта
│       └── ver.txt          # Текущая версия
│
├── .github/
│   ├── workflows/
│   │   ├── tests.yml        # Тесты на PR (3 платформы)
│   │   ├── pr-merge.yml     # Auto-bump patch при merge
│   │   └── release.yml      # Релиз: тесты + сборка + GitHub Release
│   └── actions/             # Переиспользуемые CI-экшены
│
├── go.mod                   # Go-модуль (без внешних зависимостей)
├── CLAUDE.md                # Стандарты кодирования
└── LICENSE
```

## Примеры использования

### Базовый запуск

```bash
./openwebui-ollama-proxy \
  --openwebui-url https://webui.example.com \
  --email admin@example.com \
  --password secret123
```

### С кастомным портом и rate limiting

```bash
./openwebui-ollama-proxy \
  --openwebui-url https://webui.example.com \
  --email admin@example.com \
  --password secret123 \
  --port 8080 \
  --rate-limit 10 \
  --cors-origins "https://myapp.example.com"
```

### С агрессивным кешированием

```bash
./openwebui-ollama-proxy \
  --openwebui-url https://webui.example.com \
  --email admin@example.com \
  --password secret123 \
  --tags-ttl 1h \
  --show-ttl 2h \
  --cache-dir /var/cache/ollama-proxy
```

### Проверка chat через curl

```bash
# Non-streaming
curl http://localhost:11434/api/chat -d '{
  "model": "gpt-4o",
  "messages": [{"role": "user", "content": "Hello!"}],
  "stream": false
}'

# Streaming
curl http://localhost:11434/api/chat -d '{
  "model": "gpt-4o",
  "messages": [{"role": "user", "content": "Hello!"}]
}'

# С изображением
curl http://localhost:11434/api/chat -d '{
  "model": "gpt-4o",
  "messages": [{
    "role": "user",
    "content": "What is in this image?",
    "images": ["'$(base64 -w0 photo.jpg)'"]
  }],
  "stream": false
}'
```

### Список моделей

```bash
curl http://localhost:11434/api/tags
```

### Информация о сборке

```bash
./openwebui-ollama-proxy -i
./openwebui-ollama-proxy --info json
```

## CI/CD

```mermaid
flowchart LR
    subgraph PR["Pull Request"]
        direction TB
        P1[Prepare] --> T1["Test\n(ubuntu)"]
        P1 --> T2["Test\n(macOS)"]
        P1 --> T3["Test\n(windows)"]
    end

    subgraph Merge["PR Merge → main"]
        direction TB
        M1[Prepare] --> MT1["Test\n(3 платформы)"]
        MT1 --> MB[Build Check]
        MB --> MV["Patch bump\n+ commit"]
    end

    subgraph Release["Release (manual)"]
        direction TB
        R1[Prepare] --> RT["Garble Test\n(3 платформы)"]
        RT --> RV["Minor bump"]
        RV --> RB["Build\n(35+ платформ)"]
        RB --> RR["GitHub\nRelease"]
    end

    PR -->|merge| Merge
    Merge -.->|dispatch| Release
```

Тесты запускаются с `-race` флагом и включают бенчмарки. Релизные сборки обфусцируются
через [garble](https://github.com/burrowers/garble).

## Поддерживаемые платформы

| ОС      | Архитектуры                                                                                       |
|---------|---------------------------------------------------------------------------------------------------|
| Linux   | amd64, arm64, arm/v7, arm/v6, 386, mips, mipsle, mips64, mips64le, ppc64, ppc64le, riscv64, s390x |
| Windows | amd64, arm64, arm/v7, 386                                                                         |
| macOS   | amd64 (Intel), arm64 (Apple Silicon)                                                              |
| FreeBSD | amd64, arm64, arm/v6, arm/v7, 386                                                                 |
| OpenBSD | amd64, arm64, arm/v6, arm/v7, 386                                                                 |
| NetBSD  | amd64, arm64, arm/v6, arm/v7, 386                                                                 |
| Android | amd64, arm64, arm/v7, 386                                                                         |

## Совместимые клиенты

Любой клиент, поддерживающий Ollama API:

- [Ollie](https://github.com/nicepkg/ollie) (macOS)
- [Enchanted](https://github.com/AugustDev/enchanted) (iOS / macOS)
- [Ollama CLI](https://github.com/ollama/ollama)
- Любые другие клиенты с настраиваемым Ollama endpoint

## Разработка

### Требования

- Go 1.25+

### Запуск тестов

```bash
go test -race -v ./...
```

### Бенчмарки

```bash
go test -bench . -run=NONE -v ./...
```

### Генерация кода

```bash
go generate ./...
```

## Лицензия

См. файл [LICENSE](LICENSE).
