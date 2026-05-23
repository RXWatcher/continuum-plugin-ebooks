# Scheduled tasks

Seven `scheduled_task.v1` capabilities are declared in
[`manifest.json`](../cmd/silo-plugin-ebooks/manifest.json) and
dispatched by [`internal/scheduler/`](../internal/scheduler/). The host
calls `Run(task_key)` on the cron schedule; the dispatcher maps
`plugin:<installID>:<capabilityID>` to the right `TaskFn`.

A failed `Run` returns an error, which the host treats as "retry on the
next tick". That means a task that errors every time will keep firing.
Watch the scheduler-named logger for `task error`.

| Capability id | Cron | What it does | What goes wrong |
| --- | --- | --- | --- |
| `request_reconciler` | `*/1 * * * *` | Polls the backend for non-terminal `request` rows. | Picks up "missing event" gaps. |
| `cache_evictor` | `*/5 * * * *` | LRU-evicts `ebook_file_cache` down to 95% of `cache_max_size_gb`. | Refcount-aware; in-flight reads are skipped. |
| `kobo_session_reaper` | `*/5 * * * *` | Expires stale `kobo_transfer_session` and removes its kepub temp file. | Also sweeps stray `kobo-*` temp files >6h old. |
| `opds_token_pruner` | `0 3 * * *` | Deletes `opds_token` rows revoked >30 days ago. | Deferred GC; safe to run/skip. |
| `kindle_send_retrier` | `*/2 * * * *` | Resends queued `kindle_send_log` rows older than 30s; caps at 3 attempts. | See SMTP section below. |
| `portal_library_sync` | `0 * * * *` | Mirrors backend `/catalog/libraries` into `portal_library`. | Handles missing backend gracefully. |
| `purge_expired` | `0 */6 * * *` | Drops expired `share_link` and `ebook_recommendation_cache` rows. | Idempotent on re-run. |

## request_reconciler

**Why it exists** — backend lifecycle events are at-least-once but can
also be **missed** (portal restart, host broker glitch, backend
acks-and-drops). Polling backfills.

Flow per tick:

```
ListNonTerminal(100) → for each row where ExternalID != "" and age > 30s:
   GET /api/v1/requests/{external_id} on its target_plugin_id
   → AdvanceRequestStatus(id, snap.Status, ...)
```

Skips rows fresher than 30s so it doesn't race the
`request_watcher` consumer that is probably about to write the same
status from an event.

**Pitfalls / known gotchas:**

- The reconciler uses `r.TargetPluginID` (per-row) before falling back to
  the singleton `BackendTarget()`. If a row was created when the target
  was misconfigured, the reconciler will keep polling that ghost id —
  fix the row (`UPDATE request SET target_plugin_id = '<id>' WHERE
  id = '...';`) or accept that the row stays stale.
- `GetRequestSnapshot` returns `{}` when the backend has forgotten the
  external id (e.g. ebook-requests purged its history). The reconciler
  silently skips — the row stays in its last known state forever. Cancel
  it from the admin UI.
- A row with no `external_id` (request never made it past `submitted`)
  is also skipped. That's usually a backend down at submit time; the
  customer can retry from `/me/requests`, which republishes
  `request_submitted`.
- `AdvanceRequestStatus` is terminal-guarded. Once a row is
  `fulfilled/failed/denied/cancelled`, the reconciler can't move it.

## cache_evictor

Delegates to `streaming.Manager.EvictTo(target)` where
`target = 0.95 × cache_max_size_gb × GiB`.

The manager:
1. Asks Postgres for `sum(bytes_on_disk)` across `status='ready'` rows.
2. If under target, returns.
3. Lists 500 LRU candidates, walks oldest-first.
4. Skips entries whose in-process refcount is >0 (an HTTP handler or the
   Kindle retrier is currently reading the file).
5. `os.Remove` the file. On error other than `IsNotExist`, **keeps the DB
   row** so the next sweep retries. Without that guard a transient EIO
   would orphan the file on disk forever.
6. Deletes the row.

**Pitfalls:**

- The DB total can drift from disk reality if a `Remove` fails repeatedly.
  Symptom: `ls -la $cache_dir` shows files that aren't in
  `ebook_file_cache`. Fix: run the sweep manually with the plugin stopped
  (`find $cache_dir -type f` then cross-check).
- The legacy fallback loop (no `CacheManager` wired) does NOT honor the
  refcount. It's only used when `cache_dir == ""` at Configure, which
  also disables cache mode — so it shouldn't fire in practice.

## kobo_session_reaper

Two jobs:

1. Expire `kobo_transfer_session` rows with `expires_at < now() - 5min`.
   The 5-minute grace window is deliberate: the server's `WriteTimeout`
   is 120s, so a transfer that started just before expiry can still be
   mid-copy. Refcount registry (`koboref.Registry`) is the primary
   defence; the grace is defence-in-depth for the case where
   `KoboRefs` is nil.
2. Walk `$cache_dir` for files matching `kobo-*` older than 6 hours and
   unlink them. These are kepub temp files produced by
   `handleSendToKobo` that failed before inserting a session row (so the
   refcount registry can't see them and the DB-driven cleanup never
   touches them).

**Pitfalls:**

- A kepubify run hanging will leave a `kobo-<ulid>.epub` AND a
  `kobo-<ulid>.kepub.epub` (we delete the source after conversion). The
  6-hour window catches the latter. The 30-minute session expiry catches
  the former via the DB unlink path.
- The reaper does **not** run on a non-`cache_dir` filesystem — temp
  files in `/tmp` (the cache_dir fallback in `handleSendToKobo`) are
  cleaned only when their session row expires.

## opds_token_pruner

`DELETE FROM opds_token WHERE revoked_at IS NOT NULL AND revoked_at <
now() - interval '30 days'`. Idempotent. Active tokens are not touched.

Audit windows: revoked tokens live for 30 days so you can answer "did
that ex-user's token authenticate after revocation?" — `last_used_at`
on a revoked row tells you.

## kindle_send_retrier

Reads `kindle_send_log` rows in status `queued`, updated >30s ago, up to
10 per tick. For each:

1. Counts past attempts from the error_text prefix (the retrier appends
   `| attempt:N:...` on every failure). If >3, mark `failed`.
2. Fetches the EPUB via the streaming layer — cache hit if available,
   otherwise a single-flight cache fill via the same Manager the SPA
   uses. Falls back to a temp file if no cache manager is wired.
3. Sends via `internal/kindle.Sender` (gomail.v2). On success → `sent`;
   on failure → re-queue with appended attempt counter.

**Pitfalls:**

- `kindle_smtp_config` of `{}` (default) makes the retrier a no-op.
  Symptom: queued rows accumulate, status never changes. Set
  `host`/`port`/`from` and either `username`/`password` (auth) or none
  (anonymous relay).
- The 30s minimum age is so the customer's tab doesn't race the retrier;
  a freshly queued send is normally picked up by the next tick (≤2min).
- The `cleanup` closure released on success/failure release the cache
  refcount; if the retrier panics, the refcount leaks for the life of
  the process. Restart the plugin if you see entries stuck in
  `last_accessed_at` ordering.

## portal_library_sync

Calls `libsync.Sync(store, backend, target)`. Hourly. No backend
configured → no-op. On a backend that doesn't implement
`/catalog/libraries` (older local-ebooks?), `isPluginHTTPUnsupported`
catches the `code = Unimplemented` and logs at info level.

`libsync.Sync` is **upsert-only**: it never deletes shelves that no
longer match a backend library. Operators delete shelves from the admin
UI. This is intentional — a transient `/catalog/libraries` failure
should not nuke the user's curated shelves.

## purge_expired

Runs every 6h. Drops:

- `share_link` rows past `expires_at`.
- `ebook_recommendation_cache` rows past their TTL (used by the
  `/similar` recommender when embedding service is configured).

Logs an info line with the count when >0. Safe to skip ticks; the rows
are guarded at read time anyway, this just keeps the tables small.

## Common scheduled-task pitfalls

- **"Reconciler missing events"** — Either the consumer NACKed
  (real DB error in `AdvanceRequestStatus`), or the event was foreign
  (a backend shared across portals delivers events from a sibling
  portal's `request_id`). The latter is ACK-dropped — see the comment
  in `consumer/handler.go`. Real DB errors keep redelivering forever
  until you fix them; check the plugin logs for handler errors.
- **Tasks logging but never doing work** — Almost always
  `cfg.HasBackend()` is false because `target_backend_*` is unset. The
  task returns nil. Set the backend in admin.
- **Tasks erroring with "plugin not configured yet"** — The host called
  a scheduler tick before Configure finished. Expected on startup; the
  next tick should succeed. If persistent, Configure is failing —
  check for migration errors.
