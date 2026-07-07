# Knowledge RAG

[Русская версия](./README_RU.md) | [Development guide](./DEVELOPMENT.md)

Knowledge RAG is a self-hosted RAG service for Confluence Server/Data Center and self-hosted GitLab. It stores source connections and indexing scopes in PostgreSQL, syncs content in background workers, and provides hybrid pgvector + PostgreSQL FTS search and chat through a Go API and Vue 3 UI.

## Quick Start

1. Copy the example configuration:

```bash
cp .env.example .env
```

2. Review the first settings in `.env`:

- `EMBEDDINGS_BASE_URL`, `EMBEDDINGS_API_KEY`, `EMBEDDINGS_MODEL`, `EMBEDDINGS_DIM` - OpenAI-compatible embeddings endpoint.
- `LLM_BASE_URL`, `LLM_API_KEY`, `LLM_MODEL` - OpenAI-compatible chat completions endpoint.
- `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD` - local PostgreSQL credentials.
- `GITLAB_*` - GitLab indexing limits and exclusions.

By default the compose file points embeddings and LLM traffic to the optional `ollama` service. If you use an external provider, set the external URLs and API keys before indexing.

3. Start the stack:

```bash
docker compose up -d
```

4. Open:

- UI: <http://localhost:5173>
- API healthcheck: <http://localhost:8080/api/health>
- API observability: <http://localhost:9090/readyz>
- Worker observability: <http://localhost:9091/readyz>

To start local Ollama together with the app:

```bash
docker compose --profile ollama up -d
```

## Adding Sources

Open **Sources** in the UI:

1. Create a Confluence or GitLab connection and test it.
2. For Confluence, add a page by ID/URL, optionally with descendants, or load and select spaces.
3. For GitLab, search for a project, choose a branch or tag, and add it as a repository scope.
4. Use **Sync** for an incremental refresh or **Reindex** to rebuild embeddings.

Tokens and passwords are write-only. API responses expose only `has_token`. TLS verification is enabled by default; `skip_tls_verify` is a per-connection opt-in for self-signed internal endpoints.

GitLab tokens need enough read access for projects, branches/tags, repository trees, and raw repository files. `read_api` is usually enough; some GitLab installations may also require `read_repository`.

## Search Scope

Search and Chat support **All / Confluence / GitLab** controls. The API accepts a scope object:

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

An empty scope searches all indexed sources.

## GitLab Indexing Policy

Only configured text extensions are indexed. Binary files, dependency/build directories, generated outputs, obvious credential files, and files larger than `GITLAB_MAX_FILE_BYTES` are skipped. Configure the policy with `GITLAB_*` variables in `.env`.

## Kubernetes And Observability

The API and worker expose a separate admin listener through `OBSERVABILITY_ADDR`:

- `/startupz` reports completed process initialization.
- `/livez` reports process health without checking external dependencies.
- `/readyz` includes PostgreSQL availability.
- `/metrics` exposes Prometheus-compatible HTTP, Go runtime/process, PostgreSQL pool, and worker job metrics.

Incoming/outgoing HTTP requests and worker jobs can be traced over OTLP/gRPC. For Jaeger, set `OTEL_EXPORTER_OTLP_ENDPOINT=http://jaeger-collector:4317` and configure sampling with `OTEL_TRACES_SAMPLER_ARG` from `0` to `1`. Trace export is disabled when no endpoint is configured.

See [`deploy/kubernetes/backend.yaml`](./deploy/kubernetes/backend.yaml) for Deployment and Service examples with startup, readiness, and liveness probes. The manifest expects a `simple-rag-env` Secret with application configuration.

## Existing Confluence Installations

Migration `003_multi_source.sql` copies existing `confluence_pages` and `page_chunks` into the universal document model without deleting old tables or requiring a PostgreSQL volume reset. After migration, Confluence connections and credentials are managed through **Sources**; Confluence connection environment variables are no longer used.

## Development

Build, test, and contribution workflow are documented in [DEVELOPMENT.md](./DEVELOPMENT.md).
