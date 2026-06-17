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

This removes the local PostgreSQL volume.

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

The migration creates `page_chunks.embedding vector(1024)`. If your embeddings model returns a different vector size, update `EMBEDDINGS_DIM`, adjust the migration, and recreate the dev database volume.

`LLM/embeddings status 404`

Check that the configured provider supports OpenAI-compatible `/v1/embeddings` and `/v1/chat/completions` endpoints.

`certificate signed by unknown authority`

For quick local testing, set:

```env
CONFLUENCE_SKIP_TLS_VERIFY=true
EMBEDDINGS_SKIP_TLS_VERIFY=true
LLM_SKIP_TLS_VERIFY=true
```

For a long-lived environment, add the required CA certificate to the container image instead.
