# Operations (admin-facing)

This is the operator runbook: install, Postgres bootstrap, backend
selection, secrets, day‑2. End-user docs live in
[user-guide.md](user-guide.md).

## Install order

1. Install at least one backend plugin (bookwarehouse-ebook,
   ebook-requests, or local-ebooks) and finish its own setup.
2. Install `continuum.ebooks`. Set `database_url` (see below).
3. Open **Admin → Ebooks → Backend** and pick a default backend (or set
   per-shelf libraries — see [Backend selection](#backend-selection)).
4. Optional: per-media-type routing rules at
   **Admin → Ebooks → Routing**.
5. Optional: Kindle SMTP, cache settings, kepubify path, OPDS realm
   under the same admin pages.
6. Sanity check: `GET /opds/` (with a created token) returns the catalog,
   `GET /admin/cache` returns stats, and a test request reaches the
   chosen backend within ~1 minute (the reconciler tick).

## Postgres schema bootstrap

The plugin runs golang-migrate **on every Configure** against the DSN in
`database_url`. The `embed.FS` in `internal/migrate/files/` is the source
of truth; never run migrations manually.

What "bootstrap" means depends on how you scope the DSN:

### Recommended: dedicated schema, dedicated role

Operators typically run:

```sql
CREATE ROLE plugin_ebooks LOGIN PASSWORD '...';
CREATE SCHEMA ebooks AUTHORIZATION plugin_ebooks;
GRANT CONNECT ON DATABASE continuum TO plugin_ebooks;
-- migrations run as plugin_ebooks; CREATE TABLE inside the owned schema works
```

DSN:

```text
postgres://plugin_ebooks:password@postgres:5432/continuum?search_path=ebooks&sslmode=disable
```

`search_path=ebooks` is what makes all tables land in the `ebooks`
schema. Without it the migrator will write to `public`. There is no
SET search_path inside the SQL files — they intentionally rely on the
DSN.

### Antipattern: pointing at `public`

The plugin will work, but every other plugin sharing the same database
also writes to `public`, and a `DROP TABLE` from a sibling plugin can
collide. The "wrong installation ID after reinstall" failure mode (see
[debugging.md](debugging.md)) becomes much more painful when several
plugins share `public`.

### Migration table

golang-migrate creates `schema_migrations` in the same search path. If
you ever need to manually clear a dirty migration (e.g. a partially-run
SQL file), do it on that table; don't truncate the data tables.

## Backend selection

Two independent choices:

1. **Browse / file delivery backend** for the SPA, OPDS, Kobo Sync,
   Kindle send. Resolution:
   - If `portal_library` rows exist and the user/SPA targets one, that
     row's `backend_plugin_id` is used.
   - Otherwise the singleton
     `backend_config.target_backend_installation_id`
     (or `target_backend_plugin_id` as legacy fallback).
2. **Request target backend** for `POST /api/v1/me/requests`. Resolution:
   - `request_routing_rule` row matching `media_type` (defaults to
     `book`) and `enabled = TRUE`.
   - Falls back to `backend_config.BackendTarget()` if no rule matches.

`backend_config.BackendTarget()` prefers
`target_backend_installation_id` (added in migration `0013`) over
`target_backend_plugin_id`. **Always set the installation id** if the
backend supports multiple installs of the same plugin — picking the wrong
install id is the most common request-not-routing failure.

The `target_backend_installation_id` is what shows up in the admin form's
backend picker. If a backend is reinstalled, the picker still shows the
old install id until the operator re-saves; the resolver then routes
events at a dead install.

### Per-shelf libraries

`portal_library` rows hold `(name, media_type, backend_plugin_id,
backend_library_id)`. They are populated either by
`portal_library_sync` (hourly mirror of the backend's `/catalog/libraries`)
or by the admin "Libraries" page. Each shelf is independently enableable
and sortable. The SPA emits `library_id` on most catalog requests; a
zero/missing value falls back to `DefaultPortalLibrary`.

A book id seen from the customer side is sometimes a **scoped book ref**
of the form `<library_id>:<base64url(backend_book_id)>` — see
`decodeBookRef` in `internal/scheduler/tasks.go`. Kindle and Kobo send,
and the streamer, decode it so they know which shelf the request belongs
to (and therefore which backend to call). A non-scoped id falls through
to the singleton backend.

## Secrets

| Secret | Where | Notes |
| --- | --- | --- |
| `backend_config.kosync_secret` | bytea, 32 random bytes | Created on first `GetConfig`. Used to derive the kosync HMAC key. **Rotating it breaks every existing KOReader registration** — devices must re-register. |
| `backend_config.media_signing_secret` | text, base64 | HMAC key the portal signs file/cover URLs with. The backend plugin (`stream_signing_secret` on bw-ebook / local-ebooks) **must hold the same value** or every file fetch 401s. |
| OPDS tokens | bcrypt hash in `opds_token.token_hash` | The plaintext JTI is shown once on `POST /me/opds-tokens`; we only persist the bcrypt of it. Cannot be recovered — issue a new one. |
| Kobo transfer codes | bcrypt hash in `kobo_transfer_session.code_hash` | Same pattern: plaintext only shown once in the API response. |
| `kindle_smtp_config.password` | jsonb | Plain text inside the JSONB blob. Treat the DB row as a secret. |

The portal does not encrypt secrets at rest beyond what Postgres
provides. Use row-level Postgres encryption or filesystem-level encryption
if you need defence in depth.

## Cache configuration

| Setting | Effect |
| --- | --- |
| `cache_dir` | If empty, **cache mode is disabled** even if `default_streaming_mode` says `cache`. Streams fall through to proxy mode silently. |
| `cache_max_size_gb` | Soft cap. The evictor targets `0.95 × max` (5% slack to absorb new fills between sweeps). |
| `cache_download_concurrency` | Advisory — read by the single-flight Manager when stamping leadership. Concurrent reads of the *same* book never duplicate the download. |
| `default_streaming_mode` | `proxy` (default) or `cache`. See [cache-and-streaming.md](cache-and-streaming.md). |

Set `cache_dir` to a path on the same volume as the plugin process; the
manager writes `<dir>/<sha[:2]>/<sha>.part` then renames atomically to
`<dir>/<sha[:2]>/<sha>`. `os.Rename` across filesystems will fail.

## Standalone HTTP listener

`standalone_http_listen` (e.g. `0.0.0.0:5051`) makes the plugin also bind
a direct HTTP listener with the **same** handler. Use it when you want a
reverse proxy (Caddy/Nginx/Traefik) to terminate a separate hostname for
reader apps:

```
ebooks.example.com → reverse proxy → :5051 → /opds/, /kosync/, /kobo/...
app.example.com    → continuum host       → /api/v1/plugins/<id>/...
```

The listener is bound **once** on first Configure. Changing the value
later logs a warning and is ignored until the plugin restarts. Verify
this if a config change "didn't take effect".

## Day-2 operations

### Inspecting current state

| Question | Where to look |
| --- | --- |
| Is a request stuck? | `SELECT id, status, target_plugin_id, external_id, updated_at FROM request WHERE status NOT IN ('fulfilled','failed','denied','cancelled');` |
| Is the cache full? | `SELECT pg_size_pretty(sum(bytes_on_disk)::bigint), count(*), status FROM ebook_file_cache GROUP BY status;` |
| Stale Kobo sessions? | `SELECT id, status, expires_at FROM kobo_transfer_session WHERE expires_at < now();` |
| OPDS tokens last used? | `SELECT user_id, label, last_used_at, revoked_at FROM opds_token ORDER BY last_used_at DESC;` |
| Kindle queue health? | `SELECT status, count(*) FROM kindle_send_log GROUP BY status;` |
| Sync state per user? | `SELECT user_id, count(*), max(timestamp) FROM kosync_progress GROUP BY user_id;` |

### Forcing a backend re-sync

`portal_library_sync` runs hourly. To force it now without restarting:

```
POST /admin/libraries/sync
```

(Admin route.)

### Revoking an OPDS token

```
DELETE /admin/opds-tokens/{id}            # immediate, soft revoke
```

Soft revoke sets `revoked_at`. The row stays for 30 days so audit logs
work, then `opds_token_pruner` deletes it daily at 03:00 (cron `0 3 * *
*`). Once deleted, the JTI can be re-issued.

### Revoking a kosync registration

```
DELETE /admin/kosync-users/{username}
```

The user must re-register on every device.

### Clearing the file cache

The evictor is gentle by design (it skips files with in-flight readers).
To force a full wipe, stop the plugin, `rm -rf $cache_dir/*`, and
`TRUNCATE ebook_file_cache`. The plugin will refill on first read.

### Health probe checklist

- Plugin process logs show `configured cache_dir=... target_backend=<id>` on Configure.
- Every scheduled task logs at least once per its interval (scheduler logger is `Logger.Named("scheduler")`).
- `/admin/providers/{installID}/health` returns `ok: true` with `formats: [...]`.
- `/admin/providers/{installID}/test-search?q=test` returns ≤5 items.
- A round-trip request shows up in `request` rows within 1 minute.
