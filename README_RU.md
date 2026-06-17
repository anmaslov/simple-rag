# Confluence RAG

Небольшой self-hosted RAG для Confluence. Он индексирует страницы Confluence в PostgreSQL с pgvector, а затем даёт поиск и чат по проиндексированным материалам через Go backend и Vue frontend.

Обычный `docker-compose.yml` предназначен для пользователей и тянет готовые образы из Docker Hub.

## Сервисы

- `backend-api` - HTTP API.
- `backend-worker` - воркер синхронизации и индексации.
- `frontend` - web UI.
- `postgres` - PostgreSQL с pgvector.
- `ollama` - опциональный локальный OpenAI-compatible провайдер.
- `adminer` - опциональный UI для базы.

## Быстрый старт

```bash
cp .env.example .env
docker compose up -d
```

После запуска:

- UI: http://localhost:5173
- API healthcheck: http://localhost:8080/api/health

По умолчанию используются образы:

```text
anmaslov/simple-rag-backend:${IMAGE_TAG:-latest}
anmaslov/simple-rag-frontend:${IMAGE_TAG:-latest}
```

Чтобы запустить конкретную версию:

```bash
IMAGE_TAG=v0.1.0 docker compose up -d
```

## Настройка

Перед первым запуском поправьте `.env`:

```env
CONFLUENCE_BASE_URL=https://confluence.company.ru
CONFLUENCE_TOKEN=...
CONFLUENCE_AUTH_TYPE=bearer
CONFLUENCE_ROOT_PAGE_IDS=123456,789012
```

Для basic auth:

```env
CONFLUENCE_AUTH_TYPE=basic
CONFLUENCE_USERNAME=user@company.ru
CONFLUENCE_TOKEN=api-token-or-password
```

Проект ожидает OpenAI-compatible endpoints для embeddings и chat completions. `.env.example` по умолчанию настроен под Ollama.

## Локальная Ollama

```bash
docker compose --profile ollama up -d
docker exec -it kedo-rag-ollama-1 ollama pull bge-m3
docker exec -it kedo-rag-ollama-1 ollama pull qwen2.5:14b
```

## Использование

1. Откройте http://localhost:5173/sync и запустите индексацию.
2. Используйте http://localhost:5173/search для поиска.
3. Используйте http://localhost:5173/chat для RAG-ответов с источниками.

## Для разработчиков

Разработка, тесты, публикация релизов и детали Docker-образов описаны в [DEVELOPMENT.md](./DEVELOPMENT.md).
