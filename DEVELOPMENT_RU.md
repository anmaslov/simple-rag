# Разработка

[English version](./DEVELOPMENT.md) | [README проекта](./README_RU.md)

Этот документ для тех, кто локально собирает, тестирует или меняет Knowledge RAG.

## Требования

- Docker с Docker Compose.
- Go 1.23 для разработки backend вне контейнеров.
- Node.js 22 для разработки frontend вне контейнеров.

## Dev-стек

Используйте `docker-compose.dev.yml`, если нужно собрать образы из локального кода:

```bash
cp .env.example .env
docker compose -f docker-compose.dev.yml up -d --build
```

Полезные endpoints:

- UI: <http://localhost:5173>
- API: <http://localhost:8080>
- API readiness: <http://localhost:9090/readyz>
- Worker readiness: <http://localhost:9091/readyz>

Пересобрать только сервисы приложения после изменений в коде:

```bash
docker compose -f docker-compose.dev.yml up -d --build --force-recreate backend-api backend-worker frontend
```

Поднять опциональную локальную Ollama:

```bash
docker compose -f docker-compose.dev.yml --profile ollama up -d --build
```

Поднять опциональный Adminer:

```bash
docker compose -f docker-compose.dev.yml --profile tools up -d
```

Сбросить локальную базу:

```bash
docker compose -f docker-compose.dev.yml down -v
docker compose -f docker-compose.dev.yml up -d --build
```

Команда удаляет локальный PostgreSQL volume. Для обычного обновления это не требуется: миграция `003_multi_source.sql` сохраняет и копирует существующие данные Confluence.

## Конфигурация

Начинайте с `.env.example`. Чаще всего при разработке меняются:

- `EMBEDDINGS_BASE_URL`, `EMBEDDINGS_API_KEY`, `EMBEDDINGS_MODEL`, `EMBEDDINGS_DIM`.
- `LLM_BASE_URL`, `LLM_API_KEY`, `LLM_MODEL`.
- `GITLAB_MAX_FILE_BYTES`, `GITLAB_EXCLUDED_DIRS`, `GITLAB_EXCLUDED_FILES`, `GITLAB_TEXT_EXTENSIONS`.
- `OTEL_EXPORTER_OTLP_ENDPOINT` и `OTEL_TRACES_SAMPLER_ARG`, если нужны трейсы.

Подключения Confluence и GitLab создаются в UI на странице **Sources**. Credentials источников не нужно добавлять в `.env`.

## Сборка

Backend binaries:

```bash
cd backend
go build ./cmd/api
go build ./cmd/worker
```

Frontend production bundle:

```bash
cd frontend
npm install
npm run build
```

Production Docker images описаны в:

- `backend/Dockerfile` - собирает оба бинаря, `/api` и `/worker`.
- `frontend/Dockerfile.prod` - собирает static assets и отдает их через nginx.

Пользовательский `docker-compose.yml` тянет:

```text
anmaslov/simple-rag-backend:${IMAGE_TAG:-latest}
anmaslov/simple-rag-frontend:${IMAGE_TAG:-latest}
```

## Тесты

Backend:

```bash
cd backend
go test ./...
```

Frontend:

```bash
cd frontend
npm install
npm run build
```

Запускайте оба перед pull request.

## Как контрибьютить

1. Создайте branch от актуального `master`.
2. Держите изменения в рамках одной feature, fix или документационной правки.
3. Обновляйте README/development docs, если меняются поведение, запуск, конфигурация или использование API.
4. Для изменений backend-логики добавляйте или обновляйте тесты. Для frontend изменений минимум проверьте `npm run build`.
5. Не коммитьте локальный `.env`, database volumes, build output и dependency directories.
6. Перед review запустите `go test ./...` в `backend` и `npm run build` в `frontend`.

Secrets не должны попадать в commits, logs, fixtures, screenshots или примеры в документации. Сохраненные source tokens намеренно не возвращаются в API responses и logs.

## Архитектура

Backend packages:

- `internal/domain` - interfaces and domain contracts.
- `internal/db` - PostgreSQL adapter.
- `internal/search` - hybrid search logic.
- `internal/rag` - RAG orchestration.
- `internal/jobs` - sync worker.
- `internal/confluence` и `internal/gitlab` - source-specific REST clients.
- `internal/http` - HTTP transport.

Frontend:

- Vue 3.
- Vite.
- Pinia.
- Vue Router.

## CI

GitHub Actions workflow: `.github/workflows/ci.yml`.

На push и pull request в `master` CI запускает:

- `go test ./...`
- `npm run build`

На release tags CI также собирает multi-platform Docker images для:

- `linux/amd64`
- `linux/arm64`

## Source API

- `GET|POST /api/connections`, `PUT|DELETE /api/connections/{id}`
- `POST /api/connections/{id}/test`
- `GET /api/connections/{id}/confluence/spaces`
- `GET /api/connections/{id}/gitlab/projects|branches|tags`
- `GET|POST /api/scopes`, `DELETE /api/scopes/{id}`
- `POST /api/scopes/{id}/sync`
- `GET /api/jobs`
- `GET /api/documents`
- `POST /api/search`, `/api/chat`, `/api/chat/stream` с универсальным объектом `scope`

## Частые проблемы

`no matching manifest for linux/arm64/v8`

Выбранный Docker image tag был собран без arm64 image. Используйте более новый multi-platform tag и перезапустите:

```bash
docker compose pull
docker compose up -d
```

`vector dimension mismatch`

Миграции создают `page_chunks.embedding vector(1024)` и `document_chunks.embedding vector(1024)`. `EMBEDDINGS_DIM` должен совпадать с размерностью сохраненных vectors. По умолчанию приложение также отправляет это значение как OpenAI-compatible embeddings parameter `dimensions` (`EMBEDDINGS_SEND_DIMENSION=true`). Если провайдер не принимает `dimensions`, задайте `EMBEDDINGS_SEND_DIMENSION=false`; тогда модель должна нативно возвращать 1024 dimensions, либо нужно менять размерность vector columns и переиндексировать данные.

`LLM/embeddings status 404`

Проверьте, что выбранный provider поддерживает OpenAI-compatible endpoints `/v1/embeddings` и `/v1/chat/completions`.

`certificate signed by unknown authority`

Для локальной проверки LLM/embeddings можно задать:

```env
EMBEDDINGS_SKIP_TLS_VERIFY=true
LLM_SKIP_TLS_VERIFY=true
```

TLS-настройки Confluence и GitLab управляются отдельно на подключении в Sources. По возможности оставляйте verification включенной. Для постоянного окружения лучше добавить нужный CA certificate в container image.
