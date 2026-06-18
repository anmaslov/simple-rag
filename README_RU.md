# Knowledge RAG

Self-hosted RAG для Confluence Server/Data Center и self-hosted GitLab. Подключения и области индексации хранятся в PostgreSQL, синхронизация выполняется фоновыми jobs, а поиск объединяет pgvector и PostgreSQL FTS.

## Быстрый старт

```bash
cp .env.example .env
docker compose up -d
```

UI: <http://localhost:5173>, healthcheck: <http://localhost:8080/api/health>.

## Добавление источников

На странице **Sources**:

1. Создайте и проверьте подключение Confluence или GitLab.
2. Для Confluence добавьте страницу по ID/URL (с дочерними страницами или без них) либо загрузите и выберите пространства.
3. Для GitLab найдите проект, выберите branch/tag и добавьте repository scope.
4. `Sync` запускает повторную/incremental-синхронизацию, `Reindex` принудительно пересоздаёт embeddings.

Токены и пароли доступны только на запись: API возвращает лишь `has_token`. Проверка TLS включена по умолчанию. `skip_tls_verify` включается отдельно для конкретного подключения и предназначен только для self-signed сертификатов.

GitLab token должен иметь права чтения API и репозитория (`read_api`, а при требованиях вашей установки также `read_repository`).

## Ограничения GitLab

Индексируются только настроенные текстовые расширения. Пропускаются бинарные файлы, dependency/build/vendor directories, generated artifacts, очевидные credential-файлы и файлы больше `GITLAB_MAX_FILE_BYTES`. Политика настраивается переменными `GITLAB_*` из `.env.example`.

## Миграция со старой конфигурации

Миграция `003_multi_source.sql` безопасно переносит существующие `confluence_pages/page_chunks` в `documents/document_chunks`, не удаляет старые таблицы и не требует пересоздания PostgreSQL volume. После миграции подключения и credentials Confluence управляются только через страницу **Sources**; переменные подключения Confluence в env больше не используются.

## Поиск

Search и Chat имеют переключатель **Все / Confluence / GitLab** и фильтры подключений/scopes. Пустой scope означает поиск по всем проиндексированным источникам.

Команды разработки и тестов описаны в [DEVELOPMENT.md](./DEVELOPMENT.md).
