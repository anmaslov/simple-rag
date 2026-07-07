# Development

[Русская версия](./DEVELOPMENT_RU.md) | [Project README](./README.md)

This guide is for contributors and maintainers who build, test, or change Knowledge RAG locally.

## Requirements

- Docker with Docker Compose.
- Go 1.23 for backend development outside containers.
- Node.js 22 for frontend development outside containers.

## Development Stack

Use `docker-compose.dev.yml` when you want images built from local sources:

```bash
cp .env.example .env
docker compose -f docker-compose.dev.yml up -d --build
```

Useful endpoints:

- UI: <http://localhost:5173>
- API: <http://localhost:8080>
- API readiness: <http://localhost:9090/readyz>
- Worker readiness: <http://localhost:9091/readyz>

Rebuild only application services after code changes:

```bash
docker compose -f docker-compose.dev.yml up -d --build --force-recreate backend-api backend-worker frontend
```

Start optional local Ollama:

```bash
docker compose -f docker-compose.dev.yml --profile ollama up -d --build
```

Start optional Adminer:

```bash
docker compose -f docker-compose.dev.yml --profile tools up -d
```

Reset the local database:

```bash
docker compose -f docker-compose.dev.yml down -v
docker compose -f docker-compose.dev.yml up -d --build
```

This removes the local PostgreSQL volume. It is not required for normal upgrades: migration `003_multi_source.sql` preserves and copies existing Confluence data.

## Configuration

Start from `.env.example`. The settings most often changed during development are:

- `EMBEDDINGS_BASE_URL`, `EMBEDDINGS_API_KEY`, `EMBEDDINGS_MODEL`, `EMBEDDINGS_DIM`.
- `LLM_BASE_URL`, `LLM_API_KEY`, `LLM_MODEL`.
- `GITLAB_MAX_FILE_BYTES`, `GITLAB_EXCLUDED_DIRS`, `GITLAB_EXCLUDED_FILES`, `GITLAB_TEXT_EXTENSIONS`.
- `OTEL_EXPORTER_OTLP_ENDPOINT` and `OTEL_TRACES_SAMPLER_ARG` when tracing is needed.

Confluence and GitLab connections are created in the **Sources** UI. Source credentials should not be added to `.env`.

## Build

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

Production Docker images are defined by:

- `backend/Dockerfile` - builds both `/api` and `/worker`.
- `frontend/Dockerfile.prod` - builds static assets and serves them through nginx.

The user-facing `docker-compose.yml` pulls:

```text
anmaslov/simple-rag-backend:${IMAGE_TAG:-latest}
anmaslov/simple-rag-frontend:${IMAGE_TAG:-latest}
```

## Tests

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

Run both before opening a pull request.

## Contributing

1. Create a branch from the current `master`.
2. Keep changes scoped to one feature, fix, or documentation update.
3. Update README/development docs when behavior, setup, configuration, or API usage changes.
4. Add or update tests for backend behavior changes. For frontend changes, at least make sure `npm run build` passes.
5. Do not commit local `.env`, database volumes, generated build output, or dependency directories.
6. Before requesting review, run `go test ./...` in `backend` and `npm run build` in `frontend`.

Secrets must not appear in commits, logs, fixtures, screenshots, or documentation examples. Saved source tokens are deliberately omitted from API responses and logs.

## Architecture

Backend packages:

- `internal/domain` - interfaces and domain contracts.
- `internal/db` - PostgreSQL adapter.
- `internal/search` - hybrid search logic.
- `internal/rag` - RAG orchestration.
- `internal/jobs` - sync worker.
- `internal/confluence` and `internal/gitlab` - source-specific REST clients.
- `internal/http` - HTTP transport.

Frontend:

- Vue 3.
- Vite.
- Pinia.
- Vue Router.

## CI

GitHub Actions workflow: `.github/workflows/ci.yml`.

On push and pull request to `master`, CI runs:

- `go test ./...`
- `npm run build`

On release tags, CI also builds multi-platform Docker images for:

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
- `POST /api/search`, `/api/chat`, `/api/chat/stream` with the universal `scope` object

## Common Issues

`no matching manifest for linux/arm64/v8`

The selected Docker image tag was built without an arm64 image. Use a newer multi-platform image tag, then pull again:

```bash
docker compose pull
docker compose up -d
```

`vector dimension mismatch`

The migrations create `page_chunks.embedding vector(1024)` and `document_chunks.embedding vector(1024)`. `EMBEDDINGS_DIM` must match the stored vector size. By default the app also sends this value as the OpenAI-compatible embeddings `dimensions` parameter (`EMBEDDINGS_SEND_DIMENSION=true`). If your provider rejects `dimensions`, set `EMBEDDINGS_SEND_DIMENSION=false`; then the model must natively return 1024 dimensions, or you must change the vector column dimensions and reindex.

`LLM/embeddings status 404`

Check that the configured provider supports OpenAI-compatible `/v1/embeddings` and `/v1/chat/completions` endpoints.

`certificate signed by unknown authority`

For local LLM/embeddings testing, set:

```env
EMBEDDINGS_SKIP_TLS_VERIFY=true
LLM_SKIP_TLS_VERIFY=true
```

Confluence and GitLab TLS settings are managed per connection on the Sources page. Keep verification enabled when possible. For a long-lived environment, add the required CA certificate to the container image instead.
