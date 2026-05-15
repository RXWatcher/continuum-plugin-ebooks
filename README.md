# continuum-plugin-ebooks

Customer-facing ebook portal for Continuum. Browser-based reader (epub.js), OPDS catalog feed for third-party reader apps, KOReader kosync compatibility, send-to-Kobo (KEPUB conversion via kepubify), send-to-Kindle (SMTP), request flow, admin SPA.

This plugin is the **portal**, not a source of ebooks. Pair it with one or more
`ebook_backend.v1` providers such as `continuum.local-ebooks` for local files,
`continuum.bookwarehouse-ebook` for a managed Calibre-backed library, and
`continuum.annas-archive-downloader` for direct download requests.

## Capabilities

| Capability | Notes |
|---|---|
| `http_routes.v1` (`portal`) | SPA, REST API, OPDS feed, kosync endpoints, Kobo + Kindle send routes. Navigation label "Ebooks". |
| `event_consumer.v1` | Listens to backend `request_*` events from ebook request providers. |
| `scheduled_task.v1` × 5 | `request_reconciler` (1m), `cache_evictor` (5m), `kobo_session_reaper` (5m), `opds_token_pruner` (daily), `kindle_send_retrier` (2m). |
| `request_router.v1` | Accepts routed ebook requests from `continuum.requests`. |

## Configuration

| Key | Required | Description |
|---|---|---|
| `database_url` | yes | DSN for the `ebooks` Postgres schema. |
| `cache_dir` | no | If set, enables disk-cache streaming mode. |
| `cache_max_size_gb` | no | Disk-cache cap (default 10). |
| `cache_download_concurrency` | no | Parallel-download budget (default 4). |
| `default_streaming_mode` | no | `proxy` (live forward) or `cache` (LRU disk). |
| `kepubify_path` | no | Path to the kepubify binary (default `/usr/local/bin/kepubify`). |
| `kindle_smtp_config` | no | JSON: `{"host","port","username","password","from","tls"}` for send-to-Kindle. |
| `opds_realm` | no | Basic-auth realm string for OPDS. |
| `path_remappings` | no | Stored for deployment-specific direct-path mapping. |
| `auto_approve_requests` | no | Skip the admin approval queue. |
| `target_backend_plugin_id` | no | Default download provider plugin ID for new requests. Presentation libraries are configured separately in the admin UI. |
| `standalone_http_listen` | no | Same model as the [`audiobooks`](../continuum-plugin-audiobooks/) plugin — bind a second TCP listener for client-app surfaces (KOReader, OPDS readers). |

## Library Model

Admins define user-facing presentation libraries in the Ebooks admin UI. Each
library has a display name, media type (`book`, `comics`, `manga`, or
`documents`), source backend plugin, optional backend sub-library, enabled
state, and sort order. This lets one portal expose several library experiences
at the same time, for example ebooks from Book Warehouse and comics from Local
Ebooks.

Download providers are separate from presentation libraries. A provider can be
catalog-capable, download-capable, or both, depending on its `ebook_roles`
metadata.

## Dependencies

- Postgres role + `ebooks` schema.
- A writable cache directory if disk-cache streaming is enabled.
- `kepubify` binary on PATH for send-to-Kobo.
- SMTP credentials for send-to-Kindle.
- One or more `ebook_backend.v1` provider plugins.

## Install

### 1. Postgres pre-flight

```sql
CREATE ROLE plugin_ebooks WITH LOGIN PASSWORD '<chosen>';
CREATE SCHEMA ebooks AUTHORIZATION plugin_ebooks;
GRANT CONNECT ON DATABASE continuum TO plugin_ebooks;
```

### 2. Cache directory + kepubify

```bash
mkdir -p /var/lib/continuum/ebooks/cache
chown <continuum-user>:<continuum-group> /var/lib/continuum/ebooks/cache

# Kepubify is required for "Send to Kobo" (EPUB → KEPUB conversion).
wget -O /usr/local/bin/kepubify \
  https://github.com/pgaskin/kepubify/releases/latest/download/kepubify-linux-64bit
chmod +x /usr/local/bin/kepubify
```

### 3. Configure via admin UI

Configure `database_url`, create presentation libraries, choose a default
download provider, and set any optional feature switches you want enabled.

## Build & test

```bash
cd web && pnpm install && pnpm run build
cd ..
go build ./cmd/continuum-plugin-ebooks

go test ./...                       # requires Postgres for integration tests
cd web && pnpm run build            # tsc + vite type-check / production build
```

The `web/dist/` output is embedded into the Go binary via `web/embed.go`. `TEST_DATABASE_URL` overrides the default `postgres://continuum:continuum@localhost:5432/continuum?sslmode=disable` for the Go integration tests.

## Status

v0.1.0, beta. Browser reader, OPDS, kosync, Kobo, and Kindle send paths are all wired; expect rough edges around session lifecycle and SMTP retries.
