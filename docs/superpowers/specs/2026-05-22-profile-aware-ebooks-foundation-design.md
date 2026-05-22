# Profile-Aware Ebooks Foundation — Design

Date: 2026-05-22
Plugin: `continuum-plugin-ebooks`

## Problem

The ebooks plugin predates continuum's user-profile model. Identity is
user-level only — `auth.Identity` carries `UserID` but no profile. The public
reader integrations authenticate with their own per-user credentials (OPDS
against a hashed `opds_token`, kosync against a `kosync_user` password), and
collections, reading progress, and content restriction are all keyed by
`user_id`. A continuum user with sub-profiles (e.g. `jim` with a `laura`
profile) cannot give each profile its own collections or its own reading
identity on an external reader app — everything collapses to the account.

## Goal

Make the ebooks plugin profile-aware. Identity carries the profile; the public
reader routes authenticate as a specific profile using the established
`username#profile` convention; manual and smart collections become per-profile;
kosync becomes profile-aware; and the plugin's separate content-restriction
system is removed in favour of collections as the curation mechanism.

This is sub-project A — the foundation. Native Kobo sync (sub-project B) builds
on it and is specced separately.

## Scope

In scope:
- Profile-aware `Identity` (`X-Continuum-Profile-Id`).
- OPDS authentication via the core `ValidateProfileCredential` RPC.
- kosync made profile-aware with a portal-managed credential.
- Per-profile manual (`collection`) and smart (`smart_collection`) collections.
- Removal of the `content_restriction` system.

Out of scope:
- Kobo native sync — sub-project B.
- Profile-scoping the Readest web-reader reading state, reading goals, share
  links, and notification preferences. These *will* become profile-scoped, but
  in a later follow-up sub-project, not here. Until then they remain
  user-scoped — a deliberate, temporary seam.

## Dependencies

- The core `RuntimeHost.ValidateProfileCredential` RPC and the SDK
  `runtimehost.Client.ValidateProfileCredential` helper (continuum core +
  `continuum-plugin-sdk`). Spec:
  `continuum/docs/superpowers/specs/2026-05-13-profile-aware-third-party-auth-design.md`.
- The host stamping `X-Continuum-Profile-Id` on proxied SPA requests.

Both are implemented; the SDK side ships in SDK PR #5.

## Design

### 1. Profile-aware identity

`auth.Identity` gains a `ProfileID string` field. The identity middleware reads
it from the `X-Continuum-Profile-Id` header, which the host now injects on
authenticated SPA / `/api/v1/*` requests alongside the existing
`X-Continuum-User-*` headers.

`ProfileID == ""` denotes the **primary profile** — the canonical identity,
matching core's `ValidateProfileCredential` contract and the host's header
behaviour (a bare-username login, a `user#primary` login, and the primary
profile selected in the browser all resolve to `""`).

The plugin's working identity becomes the pair `(UserID, ProfileID)`.
Profile-scoped data keys on the pair; account-level data keeps keying on
`UserID` alone. Because `""` is unique only *within* a user (every user's
primary profile carries `""`), all profile-scoped lookups MUST scope on the
full `(user_id, profile_id)` pair, never on `profile_id` alone. Real
sub-profile ids are globally unique core text keys, but the pair-scoping rule
is applied uniformly so there is no special case.

### 2. OPDS authentication

Today OPDS uses HTTP Basic auth resolved by `opdsAuth` against a hashed
`opds_token` JTI. This is replaced.

OPDS Basic auth now carries `username` = `jim` or `jim#laura` and `password` =
the continuum account password, optionally `password#pin` when the named
profile is PIN-gated. `opdsAuth` calls `ValidateProfileCredential(username,
password)` and resolves `(userID, profileID)`.

To keep the handler testable, the plugin defines a small `CredentialValidator`
interface — `ValidateProfileCredential(ctx, username, password) (userID,
profileID string, err error)` — implemented by a thin wrapper over the SDK
`runtimehost.Client`. OPDS handler tests fake this interface; no live host is
needed.

The `opds_token` table, its store code, the `handleCreate/List/RevokeOPDSToken`
handlers, the `/api/v1/me/opds-tokens` routes, and the SPA token-management UI
are all removed. A migration drops the `opds_token` table.

### 3. kosync — profile-aware, portal-managed credential

KOReader's kosync protocol hashes the password client-side (`sha1(password)`)
before sending it as `x-auth-key`. The plugin therefore never receives the
plaintext password and cannot verify it against core's bcrypt. kosync cannot
use `ValidateProfileCredential`; it must keep a kosync-specific credential the
plugin can verify against the hash. This is a protocol constraint, not a design
choice — it is the one reader surface that does not use the real continuum
password.

kosync becomes profile-aware as follows:

- **Registration moves to the SPA only.** A kosync credential is created from
  `POST /api/v1/me/kosync/register`, where the host-injected identity makes the
  `(userID, profileID)` known. The public `POST /kosync/users/create` endpoint —
  which could never know the profile and produced synthetic `kosync:<username>`
  accounts — is retired.
- `kosync_user` gains a `profile_id` column; a kosync account is owned by
  `(user_id, profile_id)`. Its `kosync_username` stores the `jim#laura` string
  (bare `jim` for the primary profile), computed at registration from the
  account username and profile name the host supplies.
- `kosync_progress` gains `profile_id`; its primary key becomes
  `(user_id, profile_id, document, device_id)`, so each profile keeps its own
  reading position.
- KOReader authenticates with `x-auth-user` = `jim#laura` and `x-auth-key` =
  `sha1(kosync-password)`. `kosyncAuthHeader` looks the `kosync_user` row up by
  `kosync_username` (a direct, self-identifying lookup — no parsing needed) and
  bcrypt-compares the key, yielding `(user_id, profile_id)`.

### 4. Per-profile collections

`collection` and `smart_collection` each gain a
`profile_id TEXT NOT NULL DEFAULT ''` column alongside the existing `user_id`. A
collection is owned by `(user_id, profile_id)`.

Existing rows: the `DEFAULT ''` assigns every current collection to its owner's
primary profile, which is correct — before profiles a user's collections were
effectively the primary profile's. No data backfill is required.

The collection and smart-collection store methods change their identity
argument from `userID` to `(userID, profileID)` and scope every query on the
pair. Handlers read `profileID` from the section-1 identity — `X-Continuum-
Profile-Id` on SPA routes, the `ValidateProfileCredential` result on OPDS. The
SPA's collection views become per-profile on their own, since the browser
already sends the active profile through to the plugin.

The OPDS root service document gains a "Collections" navigation entry beside the
catalog, listing the authenticated profile's collections, each browsable as its
own acquisition feed.

`is_public` is unchanged — a collection is owned by a profile, but a public one
stays discoverable as it is today.

### 5. Remove content restriction

The `content_restriction` system is removed entirely:

- A migration drops the `content_restriction` table. The down migration
  recreates the empty table for tooling reversibility; the rows are not
  preserved — this is a feature removal.
- The `content_restriction` store code is deleted.
- The server-side filtering that drops matching items from catalog, search, and
  discover-section responses and from OPDS feeds is removed.
- The admin endpoint and the SPA admin page for editing restrictions are
  removed.

Consequence, deliberate: after this the ebooks plugin performs no content
gating of any kind. A child profile sees the entire backend catalog the same as
any other. Curation is collections, not blocking. Core's profile-level
`max_content_rating` / `is_child` is not consulted by the plugin and never was.
A future "kid-safe library" need would be a fresh, profile-aware feature, not
this table.

## Error handling

OPDS authentication distinguishes two failure modes:

- A `codes.Unauthenticated` from `ValidateProfileCredential` is a bad
  credential → the existing `401` with the `WWW-Authenticate` Basic challenge.
- Any other RPC error — a transport failure, or `codes.Unimplemented` from a
  host that predates the validator — is an auth-service problem → `502` with a
  clear message, so a reader app does not wrongly report the password as wrong.

The `CredentialValidator` interface (section 2) isolates this so it is unit
tested without a live host.

kosync and collection error handling is unchanged in shape — invalid
credentials `401`, invalid input `400`, store failures `500`.

## Testing

Backend tests use the plugin's existing testcontainers-Postgres + `go test`
setup.

- Store: per-profile collection and smart-collection CRUD scoped on
  `(user_id, profile_id)`, including that two different users' primary-profile
  (`''`) collections stay separate; kosync user + progress keyed by
  `(user_id, profile_id, …)`.
- OPDS: `opdsAuth` resolves `(userID, profileID)` via a fake
  `CredentialValidator`; `Unauthenticated` → 401, other errors → 502; the
  Collections navigation feed renders the authenticated profile's collections.
- kosync: SPA registration stores a `(user_id, profile_id)` row; KOReader auth
  by `jim#laura` resolves the profile; progress is profile-isolated.
- Content-restriction removal is verified by deletion — existing catalog/search
  tests keep passing (now returning unfiltered) and the restriction-specific
  tests are removed.

Frontend changes (per-profile collection views, removed OPDS-token and
content-restriction admin UI) are verified with `pnpm build`.

## Migration / rollout notes

New migrations carry: the `profile_id` columns on `collection` and
`smart_collection`; the `profile_id` column and primary-key change on
`kosync_user` / `kosync_progress`; the drop of `opds_token`; and the drop of
`content_restriction`. Exact migration numbers are assigned in the plan.

- Existing OPDS readers re-enter credentials once — the new `jim#laura` +
  password instead of a pasted token.
- Existing `kosync_user` rows get `profile_id = ''` (primary profile) and keep
  their current freeform `kosync_username`; KOReader continues to work with the
  old username because the lookup is by the literal username string. New
  registrations use the `jim#laura` form. Synthetic `kosync:<username>` accounts
  from the retired public-registration path keep working until the user
  re-registers from the SPA.
- The collection and `content_restriction` migrations are a column-add with a
  default and a table drop — both safe and fast.
