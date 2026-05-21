# Architecture

The portal is a **stateless front door**. It owns user state (progress,
annotations, requests, reader sync, OPDS tokens, send queues) but it never
owns catalog data or ebook files. Catalog and bytes always come from an
upstream backend plugin reachable through the Continuum host proxy.

```
                                        ┌─────────────────────────────┐
   browser / reader app                 │  continuum host             │
   ──────────────────────►              │  /api/v1/plugins/<id>/...   │
   /opds /kosync /kobo /api/v1          │  (auth + proxy)             │
                                        └────────────┬────────────────┘
                                                     │
              ┌──────────────────────────┐           ▼
              │ continuum.ebooks         │   ┌─────────────────────────┐
              │ (this plugin)            │──►│ ebook backend plugin    │
              │  HTTP + scheduler        │   │  bookwarehouse-ebook    │
              │  Postgres (ebooks schema)│   │  ebook-requests         │
              └──────────────────────────┘   │  local-ebooks           │
                                              └─────────────────────────┘
```

## Capabilities and what each does

| Capability | ID | Role |
| --- | --- | --- |
| `http_routes.v1` | `spa` | The entire HTTP surface: SPA, OPDS, kosync, Kobo, Kindle send, admin, customer API. |
| `event_consumer.v1` | `request_watcher` | Applies backend lifecycle events to the local `request` row. |
| `request_router.v1` | `ebooks` | Targets the configured provider when an `ebooks` request enters the host. |
| `scheduled_task.v1` × 7 | various | See [scheduled-tasks.md](scheduled-tasks.md). |

## The two roles of "backend"

There are **two distinct uses** of `target_backend_*` and they both resolve
through the same code path but should not be confused:

1. **Browse / file delivery backend** — where the portal goes to render the
   library and stream bytes. Always one provider per portal library
   (`portal_library.backend_plugin_id`). If no per‑library libraries are
   configured, the singleton `backend_config.target_backend_*` is used.
2. **Request target backend** — where new requests are dispatched. Resolved
   in this order at submit time:
   1. `request_routing_rule` row for the request's `media_type` (enabled).
   2. `backend_config.BackendTarget()` — install id if set, else plugin id.

   The lookup lives in [`store/routing_rule.go::ResolveRequestRoutingRule`](../internal/store/routing_rule.go).

The two roles can be the same plugin (bookwarehouse-ebook serves both) or
different ones (browse from bookwarehouse-ebook, route requests to
ebook-requests). The portal does not enforce that they match.

## Addressing the backend

Every call to a backend goes through the host proxy:

```
GET http://localhost:8080/api/v1/plugins/<install_id_or_plugin_id>/api/v1/...
Authorization: Bearer <CONTINUUM_PLUGIN_TOKEN>
```

Code path: [`internal/backend/client.go::HostHTTPClient.do`](../internal/backend/client.go).

The portal **does not** call backend plugins directly over their gRPC
sockets. The host token is set via `CONTINUUM_PLUGIN_TOKEN`; the base URL is
`CONTINUUM_HOST_BASE_URL` (defaults to `http://localhost:8080`).

`validInstallID` rejects anything outside `[A-Za-z0-9._-]` to defend against
path traversal in install-id values that came out of config/DB. Redirects
are not followed (`CheckRedirect` returns `ErrUseLastResponse`) so a backend
can't point the host bearer at an arbitrary URL.

### CallPluginHTTP vs HTTP proxy

The SDK exposes a runtime-host RPC (`CallPluginHTTP`) but the current
Continuum host does **not** implement it — wiring it makes every backend
call fail with `code = Unimplemented`. `main.go` therefore leaves
`runtimeHost` nil and uses the HTTP proxy. Don't re-enable it without
verifying host support.

## State ownership

| Table | Owner | Purpose |
| --- | --- | --- |
| `backend_config` | singleton (id=1) | Portal configuration and the `kosync_secret` / `media_signing_secret`. |
| `request`, `request_routing_rule` | portal | Request inbox + routing rules. |
| `portal_library` | portal | Presentation shelves with per-shelf backend target. |
| `user_data`, `annotation` | portal | Per-user progress, ratings, highlights. |
| `kosync_user`, `kosync_progress`, `kosync_book_link` | portal | KOReader sync. |
| `opds_token` | portal | OPDS basic-auth tokens (hashed). |
| `kobo_transfer_session` | portal | Short-lived Kobo Sync deliveries. |
| `kindle_send_log` | portal | Send-to-Kindle queue/log. |
| `ebook_file_cache` | portal | Index of files cached under `cache_dir`. |
| `share_link`, `ebook_recommendation_cache` | portal | Optional features with TTL. |
| `ereader_device`, `reading_goal`, `notification_pref`, `custom_font`, … | portal | Per-user preferences. |

The portal never reads or writes a backend's catalog tables.

## HTTP surface (manifest)

| Path | Access | Notes |
| --- | --- | --- |
| `/` | public | SPA shell. |
| `/assets/*`, `/admin/assets/*` | public | Bundled SPA static files. |
| `/opds/*` | public | OPDS feed; HTTP Basic against `opds_token`. |
| `/kosync/*` | public | KOReader kosync protocol; `x-auth-user` / `x-auth-key` headers. |
| `/kobo/{code}` | public | One-shot Kobo Sync transfer URL. |
| `/api/v1/*` | authenticated | Customer SPA backend. |
| `/admin/*` | admin | Admin SPA + admin API. |

The `/api/v1/*` routes are authenticated by the host before the portal
sees the request; `auth.FromContext(ctx)` returns the host-injected
identity. The three `public` integration namespaces (`/opds`, `/kosync`,
`/kobo`) authenticate themselves — see [reader-integrations.md](reader-integrations.md).

## Optional standalone listener

If `standalone_http_listen` (e.g. `0.0.0.0:5051`) is set, the portal binds
a second HTTP server with the same handler on first Configure. Intended
for reverse proxying `ebooks.example.com` directly at the OPDS / kosync /
Kobo surfaces without going through the host's path-prefixed proxy. The
listener is bound **once**; changing the value requires a plugin restart
(a warning is logged). See `cmd/continuum-plugin-ebooks/main.go`.

## Process model

Single Go process. The SDK runtime owns the gRPC sockets for the four
capability servers (`Runtime`, `HttpRoutes`, `EventConsumer`,
`ScheduledTask`). Configure is called once per host restart and on every
config change; it (re)builds a fresh `pgxpool`, runs migrations, swaps
the pool atomically, and re-publishes the dependency snapshots that the
capability servers read on each call. Stale dependencies aren't possible
because every server uses an atomic `*Pointer` read.

`pgxpool.MaxConns` is clamped to ≥16 so the portal + OPDS + kosync +
Kobo + Kindle retrier + scheduler mix doesn't starve under the pgx
default (which scales with `GOMAXPROCS` and can be as low as 4).
Operators can raise via the DSN: `?pool_max_conns=N`.
