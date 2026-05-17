# Ebooks Portal Setup, Debugging, And Flows

Plugin ID: `continuum.ebooks`
Version documented: `0.1.0`

## Purpose

user-facing ebook web app with OPDS, KOReader sync, Kobo routes, Kindle send, cache, and
request workflow.

## Runtime Dependencies

- Continuum plugin host
- Postgres schema for this plugin
- At least one ebook backend such as local-ebooks or bookwarehouse-ebook
- Optional request/download provider such as annas-archive-downloader

## Setup Checklist

1. Create schema and configure database_url.
2. Configure cache_dir and cache limits if the portal should cache backend files.
3. Configure OPDS/Kobo/KOReader/Kindle settings as needed.
4. Install ebook backend plugins and select defaults in admin settings.
5. Optionally configure standalone_http_listen for reader apps behind a reverse proxy.
6. Run browse, OPDS, download, and request smoke tests.

## Configuration Reference

- `database_url`
- `cache_dir`
- `cache_max_size_gb`
- `cache_download_concurrency`
- `default_streaming_mode`
- `kepubify_path`
- `kindle_smtp_config`
- `opds_realm`
- `path_remappings`
- `auto_approve_requests`
- `target_backend_plugin_id`
- `target_backend_installation_id`
- `standalone_http_listen`

Use the plugin manifest/admin form as the source of truth for field validation and defaults. Keep database credentials scoped to the plugin schema unless a plugin explicitly needs read access to Continuum core tables.

## Exposed Routes

- `GET /assets/* [public]`
- `GET /admin/assets/* [public]`
- `GET / [public]`
- `* /opds/* [public]`
- `* /kosync/* [public]`
- `* /kobo/* [public]`
- `* /api/v1/* [authenticated]`
- `GET /admin/* [admin]`
- `GET /* [authenticated]`

## Capabilities

- `http_routes.v1 (spa) - Customer-facing ebooks portal: SPA + OPDS + KOReader kosync + Kobo + Kindle send + admin.`
- `event_consumer.v1 (request_watcher) - Watch backend request events`
- `scheduled_task.v1 (request_reconciler) - Reconcile ebook requests`
- `scheduled_task.v1 (cache_evictor) - Evict LRU cached ebook files`
- `scheduled_task.v1 (kobo_session_reaper) - Expire stale Kobo transfer sessions`
- `scheduled_task.v1 (opds_token_pruner) - Prune revoked OPDS tokens`
- `scheduled_task.v1 (kindle_send_retrier) - Retry queued Kindle sends`
- `request_router.v1 (ebooks) - Route ebook requests`

## Operational Flows

### Browse/download

1. User opens the Ebooks SPA or OPDS/Kobo client.
2. The portal calls selected ebook_backend.v1 providers for catalog/detail/search/file data.
3. Files may be streamed directly from a backend or cached in cache_dir before delivery.
4. Kindle sends are queued and retried by scheduled tasks.

### Request

1. User submits a request.
2. The portal stores it and either auto-approves or waits for admin approval.
3. The selected provider receives request_submitted and returns status events.
4. The portal reconciler keeps request state current.

## How This Plugin Communicates

- Calls ebook_backend.v1 providers for catalog/file operations.
- Emits request events to download/request providers.
- Consumes request status events from those providers.

## Debugging Runbook

- If OPDS/Kobo clients fail, test standalone_http_listen and reverse proxy headers/paths.
- If downloads are slow or stale, inspect cache_dir permissions, max size, and concurrency.
- If Kindle sends fail, validate kindle_smtp_config and retrier logs.
- If requests do not move, confirm target_backend_plugin_id/installation_id and provider event logs.
- Check scheduled task logs for cache eviction, token pruning, and request reconciliation.

## Log And Health Checks

- Start with Continuum Admin -> Plugins and confirm the installation is enabled.
- Check the plugin process logs around startup for manifest loading, migration, and route registration.
- Check scheduled task logs when a workflow depends on polling or reconciliation.
- Confirm the plugin routes are reachable through Continuum using the access level shown above.
- For database-backed plugins, verify the configured role can connect, create/migrate tables in its schema, and read/write expected rows.

## Common Failure Patterns

- Wrong installation ID selected in a portal or router setting after reinstalling a plugin.
- Plugin database URL points at the public schema instead of the dedicated plugin schema.
- Reverse proxy forwards the SPA route but not `/api/*`, `/api/v1/*`, `/assets/*`, or provider-specific public routes.
- Network checks are run from the operator laptop instead of from the Continuum/plugin runtime network.
- Secrets are regenerated during restart, invalidating signed URLs, encrypted fields, or login state.

## Verification After Changes

1. Restart or reload the plugin installation.
2. Open the plugin route or admin page in Continuum.
3. Exercise the smallest workflow that crosses a plugin boundary.
4. Confirm both the source plugin and destination plugin record the same request/session/login identifier.
5. Leave the scheduled reconciler enough time to run, then confirm terminal state or a useful error.
