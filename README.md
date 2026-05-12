# continuum-plugin-ebooks

Continuum plugin: customer-facing ebook portal — browse, read in browser
(epub.js), OPDS catalog feed, KOReader kosync, send to Kobo (KEPUB) and
Kindle (email), request flow, admin.

See `/opt/worktrees/continuum-rh/docs/superpowers/specs/2026-05-11-ebooks-portal-and-backends-design.md`.

## Build

```bash
cd web && pnpm install && pnpm run build
cd ..
go build ./cmd/continuum-plugin-ebooks
```

The `web/dist/` output is embedded into the Go binary via `web/embed.go`.

## Test

```bash
go test ./...        # requires Postgres for store + streaming integration tests
cd web && pnpm run build  # tsc + vite type-check / production build
```

`TEST_DATABASE_URL` overrides the default
`postgres://continuum:continuum@localhost:5432/continuum?sslmode=disable` for
the Go integration tests.

## Operator runbook

### 1. Postgres pre-flight

The plugin owns one schema in the host's Postgres database.

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

### 3. Upload + configure via admin UI

After uploading the built binary as a plugin archive, the admin UI exposes
the following config keys (see manifest.json `global_config_schema`):

| Key | Required | Notes |
|-----|----------|-------|
| `database_url` | yes | `postgres://plugin_ebooks:<pwd>@host/continuum?search_path=ebooks` |
| `cache_dir` | optional | If set, enables disk-cache streaming mode. |
| `cache_max_size_gb` | optional | Defaults to 10. |
| `cache_download_concurrency` | optional | Defaults to 4. |
| `default_streaming_mode` | optional | `proxy` (live forward) or `cache` (LRU disk). |
| `kepubify_path` | optional | Defaults to `/usr/local/bin/kepubify`. |
| `kindle_smtp_config` | optional | JSON: `{"host","port","username","password","from","tls"}` |
| `opds_realm` | optional | OPDS basic-auth realm string. |
| `path_remappings` | optional | JSON array (reserved for future use). |
| `auto_approve_requests` | optional | Skip the admin approval queue. |
| `target_backend_plugin_id` | optional | Plugin id of the active `ebook_backend.v1`. |

### 4. Capabilities exposed

* `http_routes.v1` — serves the SPA + REST API + OPDS + kosync + Kobo
* `event_consumer.v1` — listens to `request_*` from bookwarehouse-ebook + ebookdb
* `scheduled_task.v1` — `request_reconciler` (1m), `cache_evictor` (5m),
  `kobo_session_reaper` (5m), `opds_token_pruner` (daily), `kindle_send_retrier` (2m)
* `request_router.v1` — accepts ebook requests routed from the requests plugin

## Architecture overview

```
SPA  (web/)               React 19 + Tailwind v4 + shadcn (zinc, new-york)
 │
HTTP (chi router)         /api/v1/*  /opds/*  /kosync/*  /kobo/*
 │
internal/server           handlers
internal/streaming        cache mode (LRU + single-flight) + proxy mode
internal/backend          host-proxy client + typed EbookBackend facade
internal/store            pgx wrappers for all 11 portal tables
internal/scheduler        5 cron tasks
internal/consumer         event_consumer.v1 handler
internal/kindle           gomail.v2 SMTP sender
internal/auth             identity middleware (reads host-injected headers)
```
