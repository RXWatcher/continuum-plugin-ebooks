# Ebooks Portal for Continuum

`continuum.ebooks` is the customer-facing ebooks portal. It serves the reader SPA, OPDS feed, KOReader/Kobo/Kindle integrations, and routes ebook requests to the configured backend provider (BookWarehouse, local libraries, or external downloader).

## Category

Lives under **Books/Ebooks**.

## Capabilities

| Type | ID | Purpose |
| --- | --- | --- |
| `http_routes.v1` | `spa` | Customer-facing portal: reader SPA, OPDS, KOReader kosync, Kobo Sync, Kindle send, and admin UI. |
| `event_consumer.v1` | `request_watcher` | Watches request lifecycle events from `continuum.ebook-requests` and `continuum.bookwarehouse-ebook` and updates the portal request table. |
| `scheduled_task.v1` | `request_reconciler` | Polls backends every minute for missed request events and reconciles stale rows. |
| `scheduled_task.v1` | `cache_evictor` | LRU-evicts cached ebook files every 5 minutes to keep the on-disk cache under budget. |
| `scheduled_task.v1` | `kobo_session_reaper` | Expires stale Kobo Sync transfer sessions every 5 minutes. |
| `scheduled_task.v1` | `opds_token_pruner` | Deletes revoked OPDS authentication tokens daily at 03:00. |
| `scheduled_task.v1` | `kindle_send_retrier` | Retries queued Kindle send-to-device deliveries every 2 minutes. |
| `scheduled_task.v1` | `portal_library_sync` | Mirrors backend libraries into the portal presentation DB hourly. |
| `scheduled_task.v1` | `purge_expired` | Drops expired share links and recommendation cache rows every 6 hours. |
| `request_router.v1` | `ebooks` | Forwards ebook requests to the configured request provider (BookWarehouse for owned-library or ebook-requests for downloader-style). |

## Dependencies

The portal does not own catalog or file storage. It consumes status events from backend plugins and forwards requests through the `ebooks` request router capability.

- [`continuum-plugin-bookwarehouse-ebook`](https://github.com/RXWatcher/continuum-plugin-bookwarehouse-ebook) — BookWarehouse/Calibre backend; emits `request_acknowledged`, `request_failed`, `request_status_changed`, `request_fulfilled`.
- [`continuum-plugin-ebook-requests`](https://github.com/RXWatcher/continuum-plugin-ebook-requests) — Anna's-Archive-style request provider; emits the same lifecycle events.
- [`continuum-plugin-local-ebooks`](https://github.com/RXWatcher/continuum-plugin-local-ebooks) — local filesystem backend (optional alternative source provider).

At least one backend must be installed and selected as the request target before the portal can fulfil requests.

Host app: [`ContinuumApp/continuum`](https://github.com/ContinuumApp/continuum). SDK: [`ContinuumApp/continuum-plugin-sdk`](https://github.com/ContinuumApp/continuum-plugin-sdk).

## External services

- **Postgres** — dedicated `ebooks` schema for portal state (requests, cache index, Kobo sessions, OPDS tokens, share links, recommendation cache, library mirror).
- **Kobo Sync API** — outbound Kobo device protocol terminated locally; no upstream Kobo dependency.
- **Kindle send** — outbound SMTP delivery to a user's `@kindle.com` address (configured per-installation via `kindle_smtp_config`).
- **Embedding service** (optional) — used by the similar-books recommender when `EMBEDDING_BASE_URL` / `EMBEDDING_MODEL` are set; otherwise the similar endpoint returns empty results.
- **Host HTTP API** — backend proxy calls go through the host at `CONTINUUM_HOST_BASE_URL` using `CONTINUUM_PLUGIN_TOKEN`.

## Reader integrations

- **Reader SPA** — in-browser EPUB reader (Readest-lite based) for authenticated users.
- **OPDS** — public OPDS catalog at `/opds/*` for external reader apps (Moon+ Reader, Marvin, etc.), token-authenticated.
- **KOReader kosync** — reading progress sync at `/kosync/*` for KOReader devices.
- **Kobo Sync** — Kobo native sync protocol at `/kobo/*`, with optional `kepubify` conversion for Kobo-compatible EPUBs.
- **Kindle send** — server-side SMTP queue that emails titles to a user's Kindle address, with automatic retry.

An optional `standalone_http_listen` direct listener lets the OPDS/Kobo/KOReader/Kindle surfaces be reverse-proxied on a dedicated hostname, separate from the main host.

## Configuration

| Key | Required | Description |
| --- | --- | --- |
| `database_url` | yes | Postgres DSN for the `ebooks` schema. |
| `cache_dir` | no | Local directory used for cached ebook files. |
| `cache_max_size_gb` | no | Maximum cache size in GB. |
| `cache_download_concurrency` | no | Number of concurrent backend downloads into cache. |
| `default_streaming_mode` | no | `proxy` or `cache` — default file delivery behavior. |
| `kepubify_path` | no | Path to `kepubify` binary for Kobo-compatible conversion. |
| `kindle_smtp_config` | no | JSON SMTP config for Kindle send-to-device. |
| `opds_realm` | no | Realm shown to OPDS clients on auth challenge. |
| `path_remappings` | no | JSON path remapping rules for local file access. |
| `auto_approve_requests` | no | Auto-approve new requests instead of requiring admin review. |
| `target_backend_plugin_id` | no | Default ebook request/download provider plugin ID. |
| `target_backend_installation_id` | no | Optional installed instance ID for the default provider. |
| `standalone_http_listen` | no | Optional direct `host:port` listener for reverse-proxied client-app routes. |

Example DSN:

```text
postgres://plugin_ebooks:password@postgres:5432/continuum?search_path=ebooks&sslmode=disable
```

## Event subscriptions

The `request_watcher` consumer subscribes to lifecycle events from both supported backends:

- `plugin.continuum.ebook-requests.request_acknowledged`
- `plugin.continuum.ebook-requests.request_failed`
- `plugin.continuum.ebook-requests.request_status_changed`
- `plugin.continuum.ebook-requests.request_fulfilled`
- `plugin.continuum.bookwarehouse-ebook.request_acknowledged`
- `plugin.continuum.bookwarehouse-ebook.request_failed`
- `plugin.continuum.bookwarehouse-ebook.request_status_changed`
- `plugin.continuum.bookwarehouse-ebook.request_fulfilled`

The `request_reconciler` scheduled task polls the same backends to recover state for events that were missed (e.g. during portal restart).

## Detailed docs

See [`docs/`](docs/) for operator, debugging, and integration docs:

- [Architecture](docs/architecture.md) — components, request flow, state ownership.
- [Operations](docs/operations.md) — install, Postgres bootstrap, backend selection, day-2.
- [Scheduled tasks](docs/scheduled-tasks.md) — what each task does and how it fails.
- [Cache and streaming](docs/cache-and-streaming.md) — proxy vs cache, LRU eviction, kepubify temp files.
- [Reader integrations](docs/reader-integrations.md) — OPDS / KOReader / Kobo / Kindle deep dives.
- [Debugging](docs/debugging.md) — symptom → root cause runbook.
- [User guide](docs/user-guide.md) — customer-facing how-to.

## Build and release

```bash
make build   # builds the web SPA (pnpm) then the Go binary
make test    # go test ./...
```

CI builds linux-amd64 binaries on push to main via the reusable workflow in [RXWatcher/continuum-plugin-repository](https://github.com/RXWatcher/continuum-plugin-repository) and publishes them to the catalog at [`./binaries/`](https://github.com/RXWatcher/continuum-plugin-repository/tree/main/binaries).
