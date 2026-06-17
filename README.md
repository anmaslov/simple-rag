# Confluence RAG

Небольшой self-hosted RAG для Confluence. Поднимается локально через Docker Compose: backend на Go, frontend на Vue, PostgreSQL с pgvector и любой OpenAI-compatible провайдер для embeddings/LLM. По умолчанию в compose есть Ollama, если хочется всё гонять без внешних API.

Идея простая: забираем страницы из Confluence, чистим HTML, режем текст на чанки, считаем embeddings, кладём всё в Postgres и потом ищем по этому индексу из поиска или чата.

## Что внутри

- `backend-api` - HTTP API.
- `backend-worker` - воркер, который забирает sync jobs и индексирует страницы.
- `postgres` - страницы, чанки, вектора, история чата и статусы задач.
- `frontend` - UI на Vue/Vite.
- `ollama` - опционально, локальный OpenAI-compatible endpoint.

В backend код разнесён по слоям:

- `internal/domain` - интерфейсы и доменные контракты.
- `internal/db` - PostgreSQL adapter.
- `internal/search`, `internal/rag`, `internal/jobs` - основная логика.
- `internal/http` - transport layer.

## Быстрый старт

```bash
cp .env.example .env
docker compose up -d
```

После запуска:

- frontend: http://localhost:5173
- backend healthcheck: http://localhost:8080/api/health

Если меняли `.env`, пересоздайте контейнеры. Чтобы точно пересобрать образы и поднять сервисы уже с новыми значениями:

```bash
docker compose up -d --build --force-recreate backend-api backend-worker frontend
```

Если меняли только переменные для Postgres из `.env`, старый volume уже создан со старыми значениями. Для dev-окружения проще пересоздать его:

```bash
docker compose down -v
docker compose up -d --build
```

Команда удалит локальную базу, так что для нужных данных сначала сделайте backup.

## Настройка Confluence

Основные поля в `.env`:

```env
CONFLUENCE_BASE_URL=https://confluence.company.ru
CONFLUENCE_TOKEN=...
CONFLUENCE_AUTH_TYPE=bearer
CONFLUENCE_ROOT_PAGE_IDS=123456,789012
CONFLUENCE_SPACE_KEYS=
```

Обычно достаточно указать `CONFLUENCE_ROOT_PAGE_IDS`. Worker возьмёт эти страницы, проиндексирует их и рекурсивно пройдёт по дочерним. `CONFLUENCE_SPACE_KEYS` оставлен как запасной ручной сценарий для sync по space.

Для basic auth:

```env
CONFLUENCE_AUTH_TYPE=basic
CONFLUENCE_USERNAME=user@company.ru
CONFLUENCE_TOKEN=api-token-or-password
```

Если Confluence живёт за корпоративным self-signed сертификатом, временно можно поставить:

```env
CONFLUENCE_SKIP_TLS_VERIFY=true
```

Нормальный вариант для постоянного окружения - добавить корпоративный CA в контейнер.

## Локальные модели через Ollama

Если хотите запускать embeddings и LLM локально:

```bash
docker compose --profile ollama up -d
docker exec -it confluence-rag-ollama-1 ollama pull bge-m3
docker exec -it confluence-rag-ollama-1 ollama pull qwen2.5:14b
```

В `.env.example` уже стоят значения под Ollama:

```env
EMBEDDINGS_BASE_URL=http://ollama:11434/v1
EMBEDDINGS_MODEL=bge-m3
EMBEDDINGS_DIM=1024

LLM_BASE_URL=http://ollama:11434/v1
LLM_MODEL=qwen2.5:14b
```

Если подключаете другой OpenAI-compatible endpoint, проверьте base URL, API key, модель embeddings и размерность `EMBEDDINGS_DIM`.

## Индексация

Откройте http://localhost:5173/sync.

Там есть несколько режимов:

- `Sync configured roots` - основной вариант, индексирует `CONFLUENCE_ROOT_PAGE_IDS` и дочерние страницы.
- `Sync space` - ручной sync одного space, скорее fallback.
- `Run CQL` - произвольный CQL.
- `Incremental` - MVP-инкремент по страницам, изменённым за последние 7 дней. Если заданы root page ids, запрос ограничивается этим scope.

Если одна страница упала с ошибкой, job не валится целиком: ошибка попадёт в лог, а страница будет помечена как skipped.

## Поиск и чат

После sync можно идти в:

- http://localhost:5173/search
- http://localhost:5173/chat

`/search` показывает найденные страницы и чанки. `/chat` делает hybrid search, собирает контекст и отдаёт его в LLM вместе с источниками.

Если подходящего контекста нет, backend отвечает:

```text
В проиндексированных материалах Confluence я не нашёл ответа
```

## API

- `GET /api/health`
- `GET /api/spaces`
- `POST /api/sync`
- `GET /api/sync/status`
- `GET /api/pages`
- `GET /api/pages/{id}`
- `POST /api/search`
- `POST /api/chat`
- `GET /api/settings`
- `PUT /api/settings`

`PUT /api/settings` в MVP возвращает read-only ошибку.

## Частые проблемы

`vector dimension mismatch`

Миграция создаёт `page_chunks.embedding vector(1024)`. Если модель embeddings возвращает другую размерность, поменяйте `EMBEDDINGS_DIM`, поправьте миграцию и пересоздайте dev volume.

`LLM/embeddings status 404`

Проверьте, что endpoint правда поддерживает OpenAI-compatible ручки `/v1/embeddings` и `/v1/chat/completions`.

`Confluence status 401`

Проверьте `CONFLUENCE_TOKEN`, `CONFLUENCE_AUTH_TYPE` и `CONFLUENCE_USERNAME`, если используете basic auth.

`certificate signed by unknown authority`

Для быстрой локальной проверки можно включить:

```env
CONFLUENCE_SKIP_TLS_VERIFY=true
EMBEDDINGS_SKIP_TLS_VERIFY=true
LLM_SKIP_TLS_VERIFY=true
```

Для нормального окружения лучше добавить нужный CA в образ.

Миграции не применились на старой базе

`docker compose up` не переигрывает уже применённые миграции и не пересоздаёт volume. В dev можно снести volume:

```bash
docker compose down -v
docker compose up -d --build
```
