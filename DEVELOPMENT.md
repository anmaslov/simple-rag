# Development

This document is for contributors and maintainers.

## Local Development

Use `docker-compose.dev.yml` to build application containers from local sources:

```bash
cp .env.example .env
docker compose -f docker-compose.dev.yml up -d --build
```

Rebuild only the application services after code changes:

```bash
docker compose -f docker-compose.dev.yml up -d --build --force-recreate backend-api backend-worker frontend
```

Run the optional local Ollama service:

```bash
docker compose -f docker-compose.dev.yml --profile ollama up -d --build
```

Reset the local database:

```bash
docker compose -f docker-compose.dev.yml down -v
docker compose -f docker-compose.dev.yml up -d --build
```

This removes the local PostgreSQL volume. It is not required when upgrading: migration `003_multi_source.sql` preserves and copies existing Confluence data.

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

## Docker Images

User-facing `docker-compose.yml` pulls images from Docker Hub:

```text
anmaslov/simple-rag-backend:${IMAGE_TAG:-latest}
anmaslov/simple-rag-frontend:${IMAGE_TAG:-latest}
```

The frontend production image is built with `frontend/Dockerfile.prod` and serves static files through nginx. The nginx config proxies `/api/` to `backend-api:8080`.

The backend image contains both binaries:

- `/api`
- `/worker`

## CI

GitHub Actions workflow: `.github/workflows/ci.yml`.

On push and pull request to `master`, CI runs:

- `go test ./...`
- `npm run build`

On release tags, CI also builds multi-platform Docker images for:

- `linux/amd64`
- `linux/arm64`

## Common Issues

`no matching manifest for linux/arm64/v8`

The selected Docker image tag was built without an arm64 image. Use a newer multi-platform image tag, then pull again:

```bash
docker compose pull
docker compose up -d
```

`vector dimension mismatch`

The migration creates `page_chunks.embedding vector(1024)` and `document_chunks.embedding vector(1024)`. `EMBEDDINGS_DIM` must match the stored vector size. By default the app also sends this value as the OpenAI-compatible embeddings `dimensions` parameter (`EMBEDDINGS_SEND_DIMENSION=true`), so compatible providers can return 1024-dimensional vectors from larger models. If your provider rejects `dimensions`, set `EMBEDDINGS_SEND_DIMENSION=false`; then the model must natively return 1024 dimensions, or you must change the vector column dimensions and reindex.

`LLM/embeddings status 404`

Check that the configured provider supports OpenAI-compatible `/v1/embeddings` and `/v1/chat/completions` endpoints.

`certificate signed by unknown authority`

For LLM/embeddings local testing, set:

```env
EMBEDDINGS_SKIP_TLS_VERIFY=true
LLM_SKIP_TLS_VERIFY=true
```

Confluence and GitLab TLS settings are managed per connection on the Sources page. Keep verification enabled when possible; use the warning-protected opt-in only for a self-signed endpoint. For a long-lived environment, add the required CA certificate to the container image instead.

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

Saved secrets are deliberately omitted from responses and logs.
