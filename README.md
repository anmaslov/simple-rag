# Confluence RAG

[![CI](https://img.shields.io/github/actions/workflow/status/anmaslov/simple-rag/ci.yml?branch=master&style=flat-square&logo=github&label=CI)](https://github.com/anmaslov/simple-rag/actions/workflows/ci.yml)
[![Docker pulls](https://img.shields.io/docker/pulls/anmaslov/simple-rag-backend?style=flat-square&logo=docker&label=pulls)](https://hub.docker.com/r/anmaslov/simple-rag-backend)
[![Multi arch](https://img.shields.io/badge/linux-amd64%20%7C%20arm64-555?style=flat-square&logo=linux)](./DEVELOPMENT.md)

Self-hosted RAG for Confluence. It indexes Confluence pages into PostgreSQL with pgvector, then provides search and chat over the indexed content through a Go backend and a Vue frontend.

The default Docker Compose file is meant for regular users and pulls ready-to-run images from Docker Hub.

## Services

- `backend-api` - HTTP API.
- `backend-worker` - background sync and indexing worker.
- `frontend` - web UI.
- `postgres` - PostgreSQL with pgvector.
- `ollama` - optional local OpenAI-compatible provider.
- `adminer` - optional database UI.

## Quick Start

```bash
cp .env.example .env
docker compose up -d
```

Open:

- UI: http://localhost:5173
- API healthcheck: http://localhost:8080/api/health

The default images are:

```text
anmaslov/simple-rag-backend:${IMAGE_TAG:-latest}
anmaslov/simple-rag-frontend:${IMAGE_TAG:-latest}
```

To run a specific release:

```bash
IMAGE_TAG=v0.1.0 docker compose up -d
```

## Configuration

Edit `.env` before the first run:

```env
CONFLUENCE_BASE_URL=https://confluence.company.com
CONFLUENCE_TOKEN=...
CONFLUENCE_AUTH_TYPE=bearer
CONFLUENCE_ROOT_PAGE_IDS=123456,789012
```

For basic auth:

```env
CONFLUENCE_AUTH_TYPE=basic
CONFLUENCE_USERNAME=user@company.com
CONFLUENCE_TOKEN=api-token-or-password
```

The project expects OpenAI-compatible embeddings and chat completion endpoints. `.env.example` is configured for Ollama by default.

## Local Ollama

```bash
docker compose --profile ollama up -d
docker exec -it kedo-rag-ollama-1 ollama pull bge-m3
docker exec -it kedo-rag-ollama-1 ollama pull qwen2.5:14b
```

## Usage

1. Open http://localhost:5173/sync and start indexing.
2. Use http://localhost:5173/search for semantic search.
3. Use http://localhost:5173/chat for RAG answers with sources.

## Developer Docs

Development setup, tests, release publishing, and Docker image details are documented in [DEVELOPMENT.md](./DEVELOPMENT.md).

Russian documentation is available in [README_RU.md](./README_RU.md).
