# Cache and streaming

The portal has **two modes** for delivering bytes to a client:

1. `proxy` — forward the backend's response live, headers and body.
   No persistence. `default_streaming_mode = proxy`.
2. `cache` — single-flight fill into `cache_dir`, then `http.ServeFile`.
   Subsequent reads of the same book hit disk.

`ResolveMode(cfg)` in [`internal/streaming/resolver.go`](../internal/streaming/resolver.go)
picks the mode per request. Today this is just the singleton config; a
per-user override is not implemented yet.

## Proxy mode

`ProxyStream` in [`internal/streaming/proxy.go`](../internal/streaming/proxy.go):

- Forwards `Range` and `If-None-Match` request headers.
- Strips hop-by-hop response headers; everything else is preserved
  (`Content-Type`, `Content-Length`, `Content-Range`, `Accept-Ranges`,
  `ETag`, ...).
- The backend file path is signed with `mediatoken.Mint` using
  `media_signing_secret` (HMAC) — the backend's public file route
  rejects unsigned requests.

Range requests "just work" if the backend supports them. EPUB reader
SPAs that issue range reads (e.g. PDF.js, some readers) get
byte-accurate responses.

## Cache mode

[`internal/streaming/cache.go::Manager`](../internal/streaming/cache.go) owns:

- The on-disk layout: `<cache_dir>/<sha[:2]>/<sha>`.
- A `sync.Map` keyed by `cache_key` for in-process single-flight.
- DB transitions on `ebook_file_cache.status` (`pending → ready/failed`).
- A refcount of currently-reading entries that gates eviction.

### Cache key

```go
sha256(bookID + "|" + installID + "|" + libraryID)
```

`libraryID` is included so two `portal_library` shelves pointing at the
same backend with different access policies cannot collide on the same
file. Format is **not** keyed: bookwarehouse-ebook and ebook-requests
both store one file per book and ignore `?format=` on the byte route.

### Single-flight fill

`StartOrJoin(cacheKey, ...)`:

1. Fast-path: `Lookup` returns the ready row → done.
2. Slow-path: `LoadOrStore` in the inflight map.
   - **Leader** (the goroutine that wins the LoadOrStore):
     - Decouples its work from the request context — uses
       `context.WithoutCancel(ctx)` + 15min timeout. If the client that
       happened to win leadership drops, the followers (still
       connected) keep waiting on the leader's `done` channel.
     - Inserts the `ebook_file_cache` row in status `pending`.
     - Streams to `<path>.part`, fsync's via Close, atomic-renames.
     - Updates status to `ready` and `bytes_on_disk`.
   - **Followers**: block on `<-leader.done`, then return the leader's
     entry and serve from disk.

A failed download is recorded as `status='failed'` so an operator can
inspect why. The next caller for the same key will NOT see this row in
`Lookup` (only `ready` is considered hit) and will become leader again.
This is deliberately retry-friendly.

### Refcount and the read/delete race

`Acquire(id)` increments a refcount before the HTTP handler starts
streaming the cached file; `release()` decrements once at the end.
`EvictTo` checks the count before unlinking — entries with count >0
are skipped and reconsidered next sweep.

This closes the race where the LRU sweep deletes a file mid `io.Copy`
to a slow reader, producing mid-transfer 410/EOF errors. The
`kindle_send_retrier` uses the same `Acquire` so SMTP attaches don't
race the sweep either.

The release closure is idempotent — calling it twice is a no-op. Always
`defer release()` immediately after `Acquire`.

### Eviction algorithm

`EvictTo(targetBytes)`:

1. `SELECT sum(bytes_on_disk) FROM ebook_file_cache WHERE status='ready'`.
2. If under target, return.
3. `ListCacheLRU(500)` — oldest by `last_accessed_at`.
4. Walk in order, skipping busy entries (refcount > 0).
5. `os.Remove` the file. If unlink fails with anything other than
   `IsNotExist`, **keep the DB row** — deleting it would orphan the
   on-disk file forever (no other code path references the sha-keyed
   name).
6. Delete the row, decrement running total.

Default target is 95% of `cache_max_size_gb` (5% slack to absorb
fills between 5-minute sweeps).

## Disk pressure symptoms

| Symptom | Likely cause | Where to check |
| --- | --- | --- |
| Eviction logs but disk usage doesn't drop | `os.Remove` returning ENOTSUP / EBUSY | Check filesystem, mounted noexec/ro? |
| `cache_dir` size > `cache_max_size_gb` | Orphaned files (DB row gone but file present) | `find $cache_dir -type f -size +0` and diff against `SELECT relative_path FROM ebook_file_cache`. Manual cleanup safe. |
| `bytes_on_disk` total in DB < actual disk usage | Same as above. | Same. |
| 410 / "context canceled" mid-stream | Pre-refcount era OR cache mode not enabled (refcount not used). | Confirm `default_streaming_mode = cache` and a manager is wired. |
| Followers see "context canceled" on a download | Leader's context was the request — fixed (`WithoutCancel`). Verify build. | Confirm running ≥ the build that introduced `WithoutCancel`. |
| Disk fills with `kobo-*.epub` files | `kobo_session_reaper` not running OR conversion failed before session insert. | Check scheduler logs; sweep window is 6h. |

## kepubify temp files

`handleSendToKobo` writes `kobo-<ulid>.epub` then runs
`kepubify -o kobo-<ulid>.kepub.epub kobo-<ulid>.epub`, deletes the
source, and stores the kepub path in `kobo_transfer_session.source_path`.

If the session row is committed successfully, the reaper unlinks
`source_path` when the session expires (default 30min) or is consumed.

If the row insert fails after conversion (e.g. DB blip), the kepub is
**orphaned** until the 6h sweep in `kobo_session_reaper` picks it up.
Symptom: stray `kobo-*.kepub.epub` in `cache_dir`. The sweep window is
deliberately long (>>WriteTimeout) so it never deletes a
mid-conversion file.

If `cache_dir` is empty, `handleSendToKobo` falls back to `/tmp`. The
reaper does **not** walk `/tmp`. Configure `cache_dir` to keep cleanup
self-managed.

## Cache lifecycle: a fully worked example

User clicks "Read" on a book the portal has never seen:

1. SPA → `GET /api/v1/me/books/{id}/file?format=epub`.
2. `handleStreamFile` resolves the book ref to `(libraryID,
   backend_book_id)` and the right backend install.
3. `ResolveMode(cfg) == "cache"` → `ComputeCacheKey(...)`.
4. `Manager.Lookup(cacheKey)` → no hit.
5. Calling goroutine becomes leader, inserts a row at status `pending`,
   begins streaming the upstream via `host.GetStream` to
   `<dir>/ab/abcdef....part`.
6. A second user clicks the same book at the same time. Their goroutine
   loses the `LoadOrStore`, blocks on the leader's `done`.
7. Leader's body finishes, rename, status `ready`, `bytes_on_disk` set.
8. Both goroutines now have the same `CacheEntry`. They each call
   `Acquire(id)`.
9. Each handler `http.ServeFile`s the on-disk path with `Range`
   semantics. `release()` runs on `defer`.
10. Five minutes later the `cache_evictor` fires. The book is fresh
    (`last_accessed_at` updated on `Touch`) so it's at the bottom of
    the LRU list and survives.
11. Next month, the user reads many other books. This entry rises to
    the top of the LRU list, refcount is 0, the evictor unlinks the
    file and deletes the row.
