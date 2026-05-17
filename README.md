# Ebooks Portal for Continuum

`continuum.ebooks` is Continuum's user-facing ebook portal. It provides the web
app, request flow, OPDS, KOReader sync, Kobo transfer, Kindle send support, and
cache management while delegating catalog and file access to ebook backend
plugins.

Install this plugin when you want a single reader-facing ebook experience that
can sit in front of local libraries, BookWarehouse, or external download
providers.

## Features

- Authenticated Ebooks web app for browsing, searching, requesting, and
  downloading titles.
- OPDS feeds for reader apps.
- KOReader sync support.
- Kobo integration routes.
- Kindle email/send queue with retry.
- Optional file cache with size limits and download concurrency control.
- Request routing to a configured ebook backend or download provider.
- Optional standalone HTTP listener for reverse-proxied OPDS, Kobo, KOReader,
  and Kindle routes.
- Scheduled cleanup for cache, request reconciliation, Kobo sessions, OPDS
  tokens, and Kindle send retries.

## Architecture

The portal is separate from ebook source providers:

- `continuum.ebooks` owns the UI, request state, OPDS/Kobo/KOReader/Kindle
  surfaces, caching, and user workflows.
- Source providers such as `continuum.local-ebooks` or
  `continuum.bookwarehouse-ebook` own catalog and file access.
- Download providers such as `continuum.annas-archive-downloader` can be
  selected as request targets.

## Configuration

| Key | Required | Description |
|---|---|---|
| `database_url` | yes | Postgres DSN for the `ebooks` schema. |
| `cache_dir` | no | Local directory used for cached ebook files. |
| `cache_max_size_gb` | no | Maximum cache size in GB. |
| `cache_download_concurrency` | no | Number of concurrent backend downloads into cache. |
| `default_streaming_mode` | no | Default file delivery behavior. |
| `kepubify_path` | no | Path to `kepubify` for Kobo-compatible conversion. |
| `kindle_smtp_config` | no | JSON SMTP config for Kindle send. |
| `opds_realm` | no | Realm shown to OPDS clients. |
| `path_remappings` | no | JSON path remapping rules for local file access. |
| `auto_approve_requests` | no | Auto-approve new requests instead of requiring admin review. |
| `target_backend_plugin_id` | no | Default ebook request/download provider plugin ID. |
| `target_backend_installation_id` | no | Optional installed instance ID for the default provider. |
| `standalone_http_listen` | no | Optional direct listener for client-app routes. |

Example DSN:

```text
postgres://plugin_ebooks:password@postgres:5432/continuum?search_path=ebooks&sslmode=disable
```

## Database Setup

```sql
CREATE ROLE plugin_ebooks WITH LOGIN PASSWORD '<chosen>';
CREATE SCHEMA ebooks AUTHORIZATION plugin_ebooks;
GRANT CONNECT ON DATABASE continuum TO plugin_ebooks;
```

## Provider Setup

1. Install at least one ebook backend plugin, such as `continuum.local-ebooks`
   or `continuum.bookwarehouse-ebook`.
2. Configure the Ebooks portal database and optional cache/client settings.
3. Select a default request provider if requests should be forwarded to a
   downloader or monitoring backend.
4. Configure standalone HTTP if OPDS/Kobo/KOReader clients should connect
   through a dedicated reverse-proxied hostname.

## Build And Test

```bash
go test ./...
cd web && npm run build
make build
```
