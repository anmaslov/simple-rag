# Knowledge RAG

Self-hosted RAG for Confluence Server/Data Center and self-hosted GitLab. It stores managed connections and indexing scopes in PostgreSQL, synchronizes them in background jobs, and provides hybrid pgvector + PostgreSQL FTS search and RAG chat through a Go API and Vue 3 UI.

## Quick start

```bash
cp .env.example .env
docker compose up -d
```

Open the UI at <http://localhost:5173> and the healthcheck at <http://localhost:8080/api/health>.

## Adding sources

Open **Sources**:

1. Create a Confluence or GitLab connection and test it.
2. For Confluence, add a page by ID/URL (optionally including descendants), or load and select spaces.
3. For GitLab, search for a project, select a branch/tag, and add it as a repository scope.
4. Use Sync for incremental refresh or Reindex to force rebuilding embeddings.

Tokens/passwords are write-only: API responses contain only `has_token`. TLS verification is enabled by default. `skip_tls_verify` is a per-connection opt-in intended only for self-signed corporate certificates.

GitLab tokens need API read access sufficient for projects, branches/tags, repository tree, and raw repository files (`read_api` is typically sufficient; deployments may also require `read_repository`).

## GitLab indexing policy

Only configured text extensions are indexed. Binary files, dependency/build directories, generated outputs, obvious credential files, and files larger than `GITLAB_MAX_FILE_BYTES` are skipped. Configure exclusions with the `GITLAB_*` variables in `.env.example`.

## Existing Confluence installations

Migration `003_multi_source.sql` copies existing `confluence_pages` and `page_chunks` into the universal document model without deleting the old tables or requiring a PostgreSQL volume reset. After migration, all Confluence connections and credentials are managed through **Sources**; Confluence connection environment variables are no longer used.

## Search scope

Search and Chat provide **All / Confluence / GitLab** controls. The API accepts:

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

## Development and tests

See [DEVELOPMENT.md](./DEVELOPMENT.md). Russian documentation is in [README_RU.md](./README_RU.md).
