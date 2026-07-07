# Knowledge RAG

[English version](./README.md) | [Документация для разработки](./DEVELOPMENT_RU.md)

Knowledge RAG - self-hosted RAG-сервис для Confluence Server/Data Center и self-hosted GitLab. Он хранит подключения и области индексации в PostgreSQL, синхронизирует контент фоновыми worker'ами и дает гибридный поиск pgvector + PostgreSQL FTS, а также чат через Go API и Vue 3 UI.

## Быстрый старт

1. Скопируйте пример конфигурации:

```bash
cp .env.example .env
```

2. В первую очередь проверьте настройки в `.env`:

- `EMBEDDINGS_BASE_URL`, `EMBEDDINGS_API_KEY`, `EMBEDDINGS_MODEL`, `EMBEDDINGS_DIM` - OpenAI-compatible endpoint для embeddings.
- `LLM_BASE_URL`, `LLM_API_KEY`, `LLM_MODEL` - OpenAI-compatible endpoint для chat completions.
- `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD` - доступы к локальному PostgreSQL.
- `GITLAB_*` - лимиты и исключения для индексации GitLab.

По умолчанию compose направляет embeddings и LLM-запросы в опциональный сервис `ollama`. Если используете внешний провайдер, укажите внешние URL и API keys до индексации.

3. Поднимите стек:

```bash
docker compose up -d
```

4. Откройте:

- UI: <http://localhost:5173>
- API healthcheck: <http://localhost:8080/api/health>
- API observability: <http://localhost:9090/readyz>
- Worker observability: <http://localhost:9091/readyz>

Чтобы поднять локальную Ollama вместе с приложением:

```bash
docker compose --profile ollama up -d
```

## Добавление источников

Откройте **Sources** в UI:

1. Создайте подключение Confluence или GitLab и проверьте его.
2. Для Confluence добавьте страницу по ID/URL, при необходимости с дочерними страницами, либо загрузите и выберите spaces.
3. Для GitLab найдите проект, выберите branch или tag и добавьте repository scope.
4. Используйте **Sync** для incremental refresh или **Reindex**, чтобы пересобрать embeddings.

Токены и пароли доступны только на запись. API возвращает только `has_token`. Проверка TLS включена по умолчанию; `skip_tls_verify` включается отдельно на подключении и нужен только для внутренних endpoints с self-signed сертификатами.

GitLab token должен иметь права чтения проектов, branches/tags, repository tree и raw repository files. Обычно достаточно `read_api`; в некоторых установках GitLab может понадобиться `read_repository`.

## Scope поиска

Search и Chat поддерживают переключатель **All / Confluence / GitLab**. API принимает объект `scope`:

```json
{
  "query": "deployment timeout",
  "scope": {
    "source_types": ["gitlab"],
    "connection_ids": [],
    "scope_ids": [12]
  },
  "top_k": 10
}
```

Пустой `scope` ищет по всем проиндексированным источникам.

## Политика индексации GitLab

Индексируются только настроенные текстовые расширения. Бинарные файлы, dependency/build directories, generated outputs, очевидные credential-файлы и файлы больше `GITLAB_MAX_FILE_BYTES` пропускаются. Политика настраивается переменными `GITLAB_*` в `.env`.

## Kubernetes и observability

API и worker поднимают отдельный служебный listener через `OBSERVABILITY_ADDR`:

- `/startupz` - процесс полностью инициализирован.
- `/livez` - процесс жив, без проверки внешних зависимостей.
- `/readyz` - процесс готов к работе, включая доступность PostgreSQL.
- `/metrics` - Prometheus-compatible HTTP, Go runtime/process, PostgreSQL pool и worker job metrics.

Входящие/исходящие HTTP-запросы и worker jobs могут экспортировать трейсы по OTLP/gRPC. Для Jaeger задайте `OTEL_EXPORTER_OTLP_ENDPOINT=http://jaeger-collector:4317`, sampling настраивается через `OTEL_TRACES_SAMPLER_ARG` от `0` до `1`. Если endpoint не задан, экспорт трейсов отключен.

Пример Deployment и Service со startup, readiness и liveness probes находится в [`deploy/kubernetes/backend.yaml`](./deploy/kubernetes/backend.yaml). Манифест ожидает Secret `simple-rag-env` с конфигурацией приложения.

## Существующие установки Confluence

Миграция `003_multi_source.sql` копирует существующие `confluence_pages` и `page_chunks` в универсальную модель документов, не удаляет старые таблицы и не требует сброса PostgreSQL volume. После миграции подключения и credentials Confluence управляются через **Sources**; env-переменные подключения Confluence больше не используются.

## Разработка

Сборка, тесты и contribution workflow описаны в [DEVELOPMENT_RU.md](./DEVELOPMENT_RU.md).
