# Debugging runbook

Operator-facing. Symptoms ordered by how often they show up. Each entry
points at the rough code path or table to inspect first.

## Backend selection / request routing

### Request stays in `pending` forever

Symptoms:
- Customer submitted a request; status never moves past `pending`.
- `request` row shows `status='pending'`, `external_id=''`.

Causes (in likelihood order):

1. **`auto_approve_requests = false`** and no admin has approved it.
   Approve at **Admin → Ebooks → Requests** (or set
   `auto_approve_requests=true`). On approve, `submitRequest` updates
   status to `submitted` and publishes `request_submitted` to the
   target.
2. **No backend configured.** `backend_config.target_backend_*` empty
   AND no matching `request_routing_rule`. The user-facing
   `POST /me/requests` would actually return 412 in this case, so this
   only applies to rows submitted before the backend was unset.
3. **Target backend is dead / wrong install id.** The event publisher
   sends to `request_submitted` on the wrong topic; the backend never
   sees it. Check `/admin/providers/{installID}/health`.

### Request reaches `submitted` but stays there

The backend received `request_submitted` but never emitted
`request_acknowledged`. Most often:

- The backend's HTTP/event subscription is unhealthy — restart it.
- The backend acks-and-drops with no external id, then the reconciler
  has nothing to poll. Check the backend's logs for the publish.

If the backend supports HTTP polling, the reconciler will eventually
sync state on the 1-minute tick — but only for rows where the backend
populated `external_id` first. Without `external_id`, polling can't
target the right row.

### Request goes to the wrong backend

The router resolves in this order (see
[`store/routing_rule.go::ResolveRequestRoutingRule`](../internal/store/routing_rule.go)):

1. `request_routing_rule` row matching the request's `media_type`,
   `enabled=true`.
2. `backend_config.target_backend_installation_id`
3. `backend_config.target_backend_plugin_id` (legacy fallback)

To check what would happen for a media type without submitting:

```
GET /api/v1/requests/routing/preview?media_type=book
```

The response includes `target_plugin_id` and a `source` of `default`
or `rule`. Useful for diagnosing whether routing rules are masking the
expected target.

### `target_plugin_id` doesn't match a real install

Common after a backend plugin is uninstalled and reinstalled — the
install id is regenerated. The `request` row keeps the old id and the
reconciler polls a ghost target. Fix:

```sql
UPDATE request SET target_plugin_id = '<new id>'
WHERE id = '<request id>';
```

Or cancel and re-submit from the customer side.

## Cache / streaming

### Streams get truncated mid-download (410 / EOF)

Pre-refcount era: the `cache_evictor` deleted a file while a slow
reader was still copying it. Modern builds use the refcount registry,
so this should not happen unless the cache manager isn't wired
(`cache_dir` empty at Configure time disables it).

Confirm `default_streaming_mode='cache'`, `cache_dir` non-empty, and
the plugin started with cache mode enabled (the log line on Configure
shows `cache_dir=...`).

### Cache uses more disk than configured

The 95% target only fires every 5 minutes. New fills can push you over
briefly. Sustained over-target:

- `os.Remove` failing on the on-disk file (the evictor keeps the DB row
  to avoid orphans — symptom is rows in `ebook_file_cache` that the
  evictor revisits but never deletes). Check filesystem.
- Orphaned on-disk files with no DB row: diff `ls $cache_dir` against
  `SELECT relative_path FROM ebook_file_cache`. Safe to delete extras
  with the plugin running.

### `ebook_file_cache` rows stuck in `pending`

The leader goroutine crashed mid-download. The next reader becomes
leader, inserts a **new** row with the same `cache_key` — which
violates the `cache_key UNIQUE` constraint and fails.

To recover, delete the stuck row:

```sql
DELETE FROM ebook_file_cache WHERE cache_key='<sha>' AND status='pending';
```

The next read recreates it cleanly.

### Cache mode "doesn't work"

`cache_dir` empty at Configure time leaves `Manager` nil; `ResolveMode`
still returns `cache` because that's just a string read, but the
streaming layer falls through to proxy semantics because there's no
manager. Fix by setting `cache_dir`.

## OPDS

### Every OPDS request returns 401

- Token expired (`revoked_at IS NOT NULL`) — issue a new one.
- User entered the wrong continuum user id as the Basic username.
  The JTI is correct but the row's `user_id != username` check fails.
- App is sending the JTI as the username and continuum id as the
  password. Some readers UI's leak. Swap.

### "no backend" 412 from OPDS

The portal has no `BackendTarget()` configured. Set one in admin. OPDS
respects the singleton — there is no per-shelf OPDS yet.

### Marvin / Moon+Reader 404 on `/opds`

Missing trailing slash. Configure the app with `/opds/`.

## KOReader kosync

### KOReader registration always says "username taken"

A previous public registration with the same username exists. There is
no merge path between synthetic (`kosync:<username>`) and
authenticated (`<continuum-user-id>`) rows. Either:

- Pick a different username, or
- `DELETE FROM kosync_user WHERE kosync_username='<name>';` (admin) and
  re-register.

### Two devices show different progress for the same book

Expected if both devices are online with different cursor positions.
KOReader pushes on every page; whoever wrote most recently wins
`GET /kosync/syncs/progress/{document}` reads. The per-device row in
`kosync_progress` (PK = `(user_id, document, device_id)`) records each
device's view; the GET returns the newest.

### Sync seems to work but the SPA shows "Not registered"

The user registered via the public path (synthetic id) but is logged
into continuum with a different user. Re-register from
**SPA → Settings → KOReader**.

## Kobo Sync

### `/kobo/{code}` returns 404 or 401

- The session expired (`expires_at < now()`). User must request a new
  transfer URL.
- The session is already reaped (>5min past expiry). Same fix.
- The code is wrong. We bcrypt-compare against the full active set, so
  a single typo fails. Generate again.

### "kepubify failed" 500 on send-to-Kobo

`kepubify_path` is wrong or the binary is missing. Default is
`/usr/local/bin/kepubify`. The convention is to install via the
plugin's container/image; if running outside one, ensure the binary is
on disk and executable.

If the binary is fine, check that `cache_dir` is writable — we write
the source EPUB there before exec'ing kepubify.

### Stale `kobo-*` files in `cache_dir`

Conversions that failed before inserting a session row. The
`kobo_session_reaper` walks the cache dir every 5min and deletes
`kobo-*` files older than 6 hours. Wait or unlink manually.

## Kindle send

### Customer says "send to Kindle worked but no book on device"

99% of the time: the customer didn't allow-list the `from` address in
their Amazon account. Send them to:

  https://www.amazon.com/myk → Preferences → Personal Document Settings
  → Approved Personal Document E-mail List → Add `from` value from
  `kindle_smtp_config`.

### Kindle queue grows but nothing sends

`kindle_smtp_config` is `{}`. The retrier no-ops silently. Set host,
port, from at minimum.

### Kindle sends fail with "smtp host/port missing"

The JSONB exists but has missing required keys. The sender refuses to
dial. Check the `host` and `port` fields.

### Kindle row stuck in `queued` with attempt:max

3 retries failed. Investigate the error_text for the underlying SMTP
issue, fix, then re-queue:

```sql
UPDATE kindle_send_log SET status='queued', error_text=''
WHERE id='<row id>';
```

## Postgres / migrations

### "migrate: ..." on Configure

A migration file failed to apply. Check the plugin logs for the
specific SQL error. The `schema_migrations` table may show a dirty
state (`dirty=true`). To clear:

```sql
UPDATE schema_migrations SET dirty=false WHERE version=<N>;
```

…**only after** you've manually applied or reverted the half-applied
file. Don't TRUNCATE migration files or data tables.

### "permission denied for schema" on first start

The DSN's role doesn't own the schema. Either grant
`AUTHORIZATION plugin_ebooks` to the schema, or run the migration as a
superuser once to create the tables then revert to the limited role.

### Multiple plugins clobbering each other in `public`

Symptom: tables vanish, type mismatches, "relation does not exist"
after a sibling plugin's migration. Move each plugin to its own schema
via `search_path` in the DSN.

## Scheduled tasks (general)

### A task isn't firing

- Cron is in `manifest.json`; the host won't pick up changes until
  reinstall. Force a host restart of the install.
- "plugin not configured yet" errors in logs are expected during
  startup. Persistent errors mean Configure is failing — investigate
  migrations or DSN.

### A task runs but does nothing

- `cfg.HasBackend()` is false. Most tasks short-circuit. Configure a
  backend.
- `cache_dir` empty, `kindle_smtp_config` empty, etc. Each task's
  guards are in
  [scheduled-tasks.md](scheduled-tasks.md).

### Foreign request events being logged

Backends shared across multiple portals deliver `request_*` events to
every subscriber. The portal ACK-drops events for `request_id`s it
doesn't own (`ErrNotFound` branch in
[`consumer/handler.go`](../internal/consumer/handler.go)). This is
**not** an error — it's defensive design. The log level is debug, so
you only see it if you've turned that up.

## Standalone listener

### Listener doesn't bind a new address after config change

The listener is bound **once** on first Configure. Subsequent changes
log a warning ("standalone_http_listen changed; restart the plugin to
apply") and are ignored. Restart the plugin process.

### Reverse proxy returns 404 on `/opds/` but the SPA works

The reverse proxy is forwarding to the host's prefixed path
(`/api/v1/plugins/<id>/opds`) instead of the standalone listener.
Either:

- Configure the reverse proxy to forward to the standalone listener,
  not the host's plugin proxy.
- Or strip the path prefix at the proxy.

The standalone listener serves the routes at their bare paths
(`/opds/...`) because the manifest declares them that way.
