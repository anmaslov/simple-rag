# Confluence RAG

[![CI](https://img.shields.io/github/actions/workflow/status/anmaslov/simple-rag/ci.yml?branch=master&style=flat-square&logo=github&label=CI)](https://github.com/anmaslov/simple-rag/actions/workflows/ci.yml)
[![Go Report](https://goreportcard.com/badge/github.com/anmaslov/simple-rag?style=flat-square)](https://goreportcard.com/report/github.com/anmaslov/simple-rag)
[![Backend image](https://img.shields.io/docker/v/anmaslov/simple-rag-backend?sort=semver&style=flat-square&logo=docker&label=backend)](https://hub.docker.com/r/anmaslov/simple-rag-backend)
[![Frontend image](https://img.shields.io/docker/v/anmaslov/simple-rag-frontend?sort=semver&style=flat-square&logo=docker&label=frontend)](https://hub.docker.com/r/anmaslov/simple-rag-frontend)
[![Docker pulls](https://img.shields.io/docker/pulls/anmaslov/simple-rag-backend?style=flat-square&logo=docker&label=pulls)](https://hub.docker.com/r/anmaslov/simple-rag-backend)
[![Go](https://img.shields.io/badge/Go-1.23-00ADD8?style=flat-square&logo=go)](https://go.dev/)
[![Vue](https://img.shields.io/badge/Vue-3-42b883?style=flat-square&logo=vuedotjs)](https://vuejs.org/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-pgvector-4169E1?style=flat-square&logo=postgresql)](https://github.com/pgvector/pgvector)
[![OpenAI compatible](https://img.shields.io/badge/OpenAI--compatible-LLM-412991?style=flat-square)](https://platform.openai.com/docs/api-reference)
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
