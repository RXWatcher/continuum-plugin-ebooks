# Reader integrations

Four external reader surfaces, all served by the same plugin binary, all
with their own auth model. They are independent of the SPA's
host-authenticated `/api/v1/*` API.

| Path | Public access | Auth model |
| --- | --- | --- |
| `/opds/*` | yes | HTTP Basic against `opds_token` (hashed). |
| `/kosync/*` | yes | `x-auth-user`/`x-auth-key` against `kosync_user` (hashed). |
| `/kobo/{code}` | yes | bcrypt-comparison against `kobo_transfer_session.code_hash`. |
| Kindle send | n/a (server-only) | Outbound SMTP. |

The three public namespaces are declared with `access: "public"` in the
manifest, which means the host injects **no identity** into the request.
The portal authenticates them itself.

---

## OPDS

OPDS is the protocol Marvin/Moon+Reader/KOReader/etc. use to browse and
acquire books from external libraries. Code: 
[`internal/server/opds_kosync_routes.go::mountOPDS`](../internal/server/opds_kosync_routes.go).

### Token issuance

A logged-in user creates an OPDS token through the SPA, which calls:

```
POST /api/v1/me/opds-tokens
{ "label": "Marvin on iPad" }

→ 201
{ "id": "<ulid>", "label": "Marvin on iPad",
  "jti_shown_once": "<24-byte base64url, padding stripped>" }
```

Code in `handleCreateOPDSToken`:

1. Generates 24 random bytes (192 bits of entropy) → base64url, no
   padding.
2. Bcrypts the JTI with `bcrypt.DefaultCost`.
3. Stores `{id, user_id, jti, token_hash, label}` in `opds_token`.
4. Returns the **plaintext JTI** in the response. Once only.

The reader app then sends every OPDS request with HTTP Basic where:
- `username` = silo user id
- `password` = the JTI (the value `jti_shown_once`)

### Auth on every request

`opdsAuth(r)`:

```go
user, pass, ok := r.BasicAuth()
t := GetOPDSTokenByJTI(pass)         // lookup-by-jti, revoked rows excluded
if t.UserID != user → fail
bcrypt.CompareHashAndPassword(t.TokenHash, pass) → must pass
TouchOPDSToken(pass)                  // updates last_used_at
return t.UserID
```

The `jti` column is unique (and indexed) but the JTI itself is also the
plaintext password — both lookup and verification happen with the same
string. That's safe because `GetOPDSTokenByJTI` already filters out
revoked tokens at the SQL level and bcrypt is the constant-time
defence against timing across distinct rows.

### Revocation

Two paths:

- User: `DELETE /api/v1/me/opds-tokens/{id}` — owner-scoped soft revoke.
- Admin: `DELETE /admin/opds-tokens/{id}` — soft revoke without
  user-id check.

Both set `revoked_at`. The row stays for 30 days so audit log queries
work, then `opds_token_pruner` deletes it daily at 03:00.

A revoked token causes every future request to 401 immediately — the
`WHERE revoked_at IS NULL` in `GetOPDSTokenByJTI` makes the lookup
return `ErrNotFound`. The reader app should re-issue.

### Catalog & search routes

| Route | Behaviour |
| --- | --- |
| `GET /opds/` | OPDS service document. Links to `/opds/catalog` and the OpenSearch description. |
| `GET /opds/catalog` | Acquisition feed, `?cursor=` paginated, `?limit=` clamped to 200 (default 50). Backend's `PageEnvelope.NextCursor` emitted as `rel="next"`. |
| `GET /opds/search` | Returns OpenSearch description if `q` is missing; otherwise forwards to the backend's `/api/v1/catalog/search`. |
| `GET /opds/book/{id}` | Single-entry feed for a book. |
| `GET /opds/book/{id}/download/{format}` | Streams the file from the backend (proxy semantics — no caching). |

The realm shown on the WWW-Authenticate challenge comes from
`backend_config.opds_realm` (default `Silo Library`).

### OPDS pitfalls

- **Apps that don't strip the trailing `/` from the URL** see a 404 on
  `/opds`. Configure them with `/opds/`.
- A 412 "no backend" from `/opds/catalog` means `cfg.HasBackend()` was
  false at request time. Set a default in admin.
- Cover/thumbnail URLs in the feed are backend-relative; they're
  served through the same proxy and need a signed token. Some OPDS
  apps don't follow these properly and show generic icons. Not a bug
  in the portal.

---

## KOReader kosync

KOReader uses the kosync protocol (originally from the Koreader Sync
Server project) for cross-device reading-progress sync. Routes:

| Route | Method | Notes |
| --- | --- | --- |
| `/kosync/users/create` | POST | Register a kosync username. |
| `/kosync/users/auth` | POST | Probe credentials. |
| `/kosync/syncs/progress/{document}` | GET | Read latest progress for `document`. |
| `/kosync/syncs/progress` | PUT | Upsert progress. |

KOReader's `document` value is a SHA1 of the EPUB file's binary
contents — completely opaque to us. We store `(user_id, document) →
progress` and let clients on different devices converge.

### User registration

Two entry points to `handleKosyncCreate`:

1. **Public** `POST /kosync/users/create` — used directly by KOReader.
   The host injects no identity. The portal stores the kosync row keyed
   by a **synthetic user id** of the form `kosync:<username>`. This is
   load-bearing: without it, every KOReader-registered user would
   collapse to `user_id=""` and share/clobber each other's progress.
   `CreateKosyncUserStrict` is `INSERT … ON CONFLICT DO NOTHING` and
   returns `ErrKosyncUsernameTaken` on duplicate.
2. **Authenticated** `POST /api/v1/me/kosync/register` — used by the
   SPA when a logged-in silo user wants to link KOReader. The
   row is keyed by the silo user id, and `UpsertKosyncUser` allows
   the **owner only** to rotate the password. Cross-user takeover is
   prevented by an owner-scoped `DO UPDATE` clause.

Password handling is two-stage:

1. KOReader client hashes `password → sha1(password)`, sends the
   40-char hex string in the wire request.
2. We bcrypt the hex string and store the bcrypt hash.

On auth, the client sends the sha1-hex in `x-auth-key`; we
`bcrypt.CompareHashAndPassword` against the stored hash.

### Per-user sync state

`kosync_progress` is `PRIMARY KEY (user_id, document, device_id)` (per
migration `0006`). That means **each device gets its own row**; the
"current progress" for a book is whichever device wrote most recently.
KOReader's "send progress immediately" feature converges everyone.

`UpsertKosyncProgress` takes `user_id` from the authenticated kosync
session, never from the request body. A malicious client cannot lie about
their user via the JSON payload.

### kosync_book_link

A separate table `(document, user_id) → (book_id, format)` exists so
the SPA can show the kosync progress next to the corresponding book in
the silo catalog. Populated by
`POST /api/v1/me/books/{id}/kosync-link` once the user has read a
chapter in KOReader.

### Kosync pitfalls

- A user who registered via the public `/kosync/users/create` and then
  later logs into silo will see "Not registered" on the SPA's
  KOReader page — the SPA only finds rows keyed by the silo user
  id. They need to register again from the SPA. We don't merge
  synthetic accounts.
- KOReader's "Use HTTP" toggle is required when reverse-proxying without
  TLS; some KOReader versions silently refuse to send Basic auth over
  plain HTTP otherwise.
- `kosync_secret` is bytea on `backend_config`; rotating it invalidates
  every device's stored credentials.

---

## Kobo Sync

Code: [`internal/server/kindle_kobo.go::handleSendToKobo`](../internal/server/kindle_kobo.go).
Public delivery: [`opds_kosync_routes.go::mountKobo → handleKoboServeFile`](../internal/server/opds_kosync_routes.go).

The user requests a Kobo-format copy of a book; the portal converts via
`kepubify` and returns a short, one-time transfer URL.

### Lifecycle

```
POST /api/v1/me/books/{id}/send-to-kobo            (SPA)
  → fetch EPUB from backend (signed media URL)
  → write to <cache_dir>/kobo-<ulid>.epub
  → exec kepubify -o <...>.kepub.epub <...>.epub
  → unlink the source EPUB
  → randCode(10) over ABCDEFGHJKMNPQRSTUVWXYZ23456789
       (~8e14 keyspace, no ambiguous 0/O/1/I/L)
  → bcrypt the code, store only the hash
  → INSERT kobo_transfer_session (user_id, book_id, format='kepub',
      code_hash, source_path, expires_at = now()+30min, status='pending')
  → response: { transfer_code, transfer_url=/kobo/<code>, expires_at }

GET /kobo/{code}                                  (browser on Kobo)
  → list active sessions
  → bcrypt-compare {code} against every CodeHash (linear scan)
  → on match: register reader in koboref.Registry (refcount)
  → http.ServeFile the kepub path with the right MIME
  → release refcount on close
```

### Why bcrypt over a 10-char code?

The public `/kobo/{code}` endpoint is brute-forceable. The original 4-char
code (~9e5 space) was trivially enumerable inside the 30-minute window.
10 chars over a 31-char alphabet (~8e14) makes online enumeration
infeasible, and bcrypt's per-comparison cost (~50ms) caps the rate at
which an attacker can probe even if they obtain the hashes.

### Session reaper interaction

`kobo_session_reaper` runs every 5 minutes and:

1. Lists expired sessions (with a 5-minute grace beyond `expires_at` to
   never kill an active download).
2. For each, checks `koboref.Registry.InUse(id)`. If a download is
   active, defer.
3. Otherwise, status=expired and `os.Remove(source_path)`.

There is a separate 6-hour sweep for stray `kobo-*` temp files — see
[cache-and-streaming.md](cache-and-streaming.md).

### Kobo pitfalls

- `kepubify_path` defaults to `/usr/local/bin/kepubify`. If the binary
  is missing, `cmd.Run()` returns immediately and the user sees a 500
  "kepubify failed". Install the binary or set the path.
- Conversion failures leave the source EPUB on disk (we delete the
  source after kepubify succeeds). The 6-hour sweep cleans it.
- If `cache_dir` is empty, conversions land in `/tmp`. The reaper does
  **not** walk `/tmp`. Configure `cache_dir`.
- The session URL is single-stop: bookmark it and reload only inside
  the 30-minute window. The browser doesn't "consume" the session on
  success; multiple downloads work until expiry.

---

## Kindle send

Code:
- Enqueue: [`internal/server/kindle_kobo.go::handleSendToKindle`](../internal/server/kindle_kobo.go)
- Retry/send: [`internal/scheduler/tasks.go::KindleSendRetrier`](../internal/scheduler/tasks.go)
- SMTP: [`internal/kindle/sender.go`](../internal/kindle/sender.go)

The portal queues a row and a background task delivers it. Synchronous
SMTP from the request handler would tie up the HTTP connection for the
duration of the upstream upload to Amazon — not acceptable.

### Enqueue

```
POST /api/v1/me/books/{id}/send-to-kindle
{ "to_address": "<id>@kindle.com", "format": "epub" }

→ validateKindleAddress(to_address)
  - must be a bare address (no display name)
  - no CR/LF (header injection defence)
  - domain must be kindle.com, kindle.cn, or free.kindle.com
→ INSERT kindle_send_log (status='queued', to_address, ...)
→ 202
```

The Amazon domain allowlist turns an authenticated relay into a
Kindle-only delivery path. Adding domains requires a code change
(intentional — operators should not be able to flip this to "send
anywhere").

### SMTP config shape

```json
{
  "host": "smtp.example.com",
  "port": 587,
  "username": "user",
  "password": "...",
  "from":     "kindle-sender@example.com",
  "tls":      "starttls"
}
```

Stored in `backend_config.kindle_smtp_config`. Empty JSON (`{}`) makes
the retrier a no-op.

### Queue and retries

The retrier runs every 2 minutes. Per row:

1. Skip rows updated <30s ago (don't race the SPA).
2. Count attempts from the `error_text` prefix (`| attempt:N:...`).
   If >3 → terminal `failed`.
3. Fetch the EPUB via the streaming cache (single-flight on miss).
4. SMTP send via gomail.v2 (`DialAndSend`).
5. On success: `status='sent'`, `sent_at=now()`.
6. On failure: append `| attempt:N:<stage>:<err>`, re-queue.

`SetAddressHeader` (not `SetHeader`) is used for "To" so a crafted
recipient cannot inject extra headers via CR/LF, defense in depth behind
the validator.

### Kindle pitfalls

- **Most common failure**: the user's Kindle account doesn't have our
  `from` address allow-listed in
  https://www.amazon.com/myk → Preferences → Personal Document Settings.
  Symptom: SMTP send succeeds but the book never appears on the Kindle.
  Tell the user to allow-list the sender.
- TLS field: the operator may need `starttls` (port 587), `implicit`
  (port 465), or `none` (port 25). gomail.v2 picks STARTTLS by
  default; we don't currently surface the `tls` field to the dialer.
- Attempts cap at 3; once `failed`, the row is terminal. Re-queue
  manually with `UPDATE kindle_send_log SET status='queued',
  error_text='' WHERE id='...';` if the underlying SMTP issue is
  resolved.
