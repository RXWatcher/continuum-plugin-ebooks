# Profile-Aware Ebooks Foundation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the ebooks plugin profile-aware — identity carries the profile, OPDS authenticates via the core `ValidateProfileCredential` RPC, kosync becomes profile-aware, collections become per-profile, and the content-restriction system is removed.

**Architecture:** A `ProfileID` is threaded through `auth.Identity` (from `X-Continuum-Profile-Id`) and resolved on the public OPDS route through a new `CredentialValidator` that wraps the SDK runtimehost client. Collections and kosync tables gain a `profile_id` column and scope every query on `(user_id, profile_id)`, where `''` is the canonical primary-profile key. The unused `content_restriction` system is deleted.

**Tech Stack:** Go (chi, pgx, golang-migrate), the continuum-plugin-sdk runtimehost client, React + TypeScript + Vite, PostgreSQL.

**Conventions:** Work directly on `main`. Backend tests use a per-PID Postgres schema via `newTestStore`/migrations; run with `go test`. Frontend verified with `pnpm build` (`tsc -b` + Vite). The ebooks plugin resolves the local SDK through the existing `/opt/continuum_plugins/go.work`, which already includes `continuum-plugin-sdk` — so the new `runtimehost.Client.ValidateProfileCredential` helper (SDK PR #5) is available without a dependency bump. Append `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>` to every commit.

**Spec:** `docs/superpowers/specs/2026-05-22-profile-aware-ebooks-foundation-design.md`

**Out of scope:** Kobo native sync (sub-project B); profile-scoping the Readest reader state, reading goals, share links, notification preferences (a later follow-up).

---

## File Structure

Migrations (`internal/migrate/files/`):
- `0032_collection_profile.up.sql` / `.down.sql` — create: `profile_id` on `collection` + `smart_collection`.
- `0033_kosync_profile.up.sql` / `.down.sql` — create: `profile_id` on `kosync_user` / `kosync_progress` / `kosync_book_link` + PK changes.
- `0034_drop_opds_token.up.sql` / `.down.sql` — create: drop `opds_token`.
- `0035_drop_content_restriction.up.sql` / `.down.sql` — create: drop `content_restriction`.

Backend:
- `internal/auth/identity.go` — modify: `ProfileID` field + header read.
- `internal/server/credential.go` — create: `CredentialValidator` interface + `hostCredentialValidator`.
- `internal/server/server.go` — modify: `Deps` gains `Credentials`; route registration drops opds-token + content-restriction mounts.
- `internal/server/opds_kosync_routes.go` — modify: `opdsAuth` via `CredentialValidator`; kosync handlers profile-aware; drop public `/kosync/users/create`; OPDS Collections feed.
- `internal/server/opds_token.go` (handlers) + `internal/store/opds_token.go` — delete.
- `internal/server/content_restriction.go` + `internal/store/content_restriction.go` — delete.
- `internal/store/collection.go`, `internal/store/smart_collection.go`, `internal/store/kosync.go` — modify: `profile_id` dimension.
- `internal/server/user_routes.go`, `internal/server/smart_collection_handler.go` — modify: thread `profileID`.
- `cmd/continuum-plugin-ebooks/main.go` — modify: construct `hostCredentialValidator` into `Deps`.

Frontend (`web/src/`):
- `pages/Apps.tsx` — modify: remove `OPDSSection`, keep `KOReaderSection`.
- `pages/admin/ContentRestrictions.tsx` — delete.
- `pages/Admin.tsx` — modify: remove the restrictions tab.
- `lib/api.ts` — modify: remove OPDS-token and content-restriction types/functions.

---

## Task 1: Profile-aware identity

**Files:**
- Modify: `internal/auth/identity.go`
- Test: `internal/auth/identity_test.go`

- [x] **Step 1: Write the failing test**

Add to `internal/auth/identity_test.go`:

```go
func TestMiddlewareReadsProfileID(t *testing.T) {
	var got Identity
	h := Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got, _ = FromContext(r.Context())
	}))
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Continuum-User-Id", "u-1")
	r.Header.Set("X-Continuum-Profile-Id", "p-9")
	h.ServeHTTP(httptest.NewRecorder(), r)
	if got.UserID != "u-1" || got.ProfileID != "p-9" {
		t.Errorf("identity = %+v, want user u-1 profile p-9", got)
	}
}
```

If `net/http`/`net/http/httptest` are not imported in the test file, add them.

- [x] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/auth/ -run TestMiddlewareReadsProfileID`
Expected: FAIL — `Identity` has no field `ProfileID`.

- [x] **Step 3: Add the field and header read**

In `internal/auth/identity.go`, add `ProfileID string` to the `Identity` struct after `UserID string`. In `Middleware`, add to the `Identity{...}` literal:

```go
		ProfileID: r.Header.Get("X-Continuum-Profile-Id"),
```

- [x] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/auth/ -run TestMiddlewareReadsProfileID`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add internal/auth/identity.go internal/auth/identity_test.go
git commit -m "feat(ebooks): profile id on Identity"
```

---

## Task 2: CredentialValidator interface and host wrapper

**Files:**
- Create: `internal/server/credential.go`
- Modify: `internal/server/server.go` (`Deps`)
- Modify: `cmd/continuum-plugin-ebooks/main.go`

- [x] **Step 1: Create the interface and wrapper**

Create `internal/server/credential.go`:

```go
package server

import (
	"context"
	"errors"

	sdkruntime "github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginsdk/runtime"
)

// CredentialValidator resolves a third-party "user#profile" / "password#pin"
// login to (userID, profileID) by delegating to the continuum host. profileID
// is "" for the primary profile. Defined as an interface so reader-route
// handlers can be tested with a fake.
type CredentialValidator interface {
	ValidateProfileCredential(ctx context.Context, username, password string) (userID, profileID string, err error)
}

// hostCredentialValidator is the production CredentialValidator — it calls the
// host RuntimeHost.ValidateProfileCredential RPC through the SDK runtime host.
type hostCredentialValidator struct{}

// NewHostCredentialValidator returns a CredentialValidator backed by the host.
func NewHostCredentialValidator() CredentialValidator { return hostCredentialValidator{} }

func (hostCredentialValidator) ValidateProfileCredential(
	ctx context.Context, username, password string,
) (string, string, error) {
	host := sdkruntime.Host()
	if host == nil {
		return "", "", errors.New("runtime host unavailable")
	}
	cred, err := host.ValidateProfileCredential(ctx, username, password)
	if err != nil {
		return "", "", err
	}
	return cred.UserID, cred.ProfileID, nil
}
```

- [x] **Step 2: Add Credentials to Deps**

In `internal/server/server.go`, add a field to the `Deps` struct:

```go
	Credentials CredentialValidator
```

- [x] **Step 3: Wire it in main**

In `cmd/continuum-plugin-ebooks/main.go`, in the `server.Deps{...}` literal where the server is constructed, add:

```go
		Credentials: server.NewHostCredentialValidator(),
```

- [x] **Step 4: Verify the build**

Run: `go build ./...`
Expected: success. (If `sdkruntime.Host()` returns a type without `ValidateProfileCredential`, the SDK working tree is not on the branch carrying SDK PR #5 — check `git -C ../continuum-plugin-sdk log --oneline -1`.)

- [x] **Step 5: Commit**

```bash
git add internal/server/credential.go internal/server/server.go cmd/continuum-plugin-ebooks/main.go
git commit -m "feat(ebooks): host-backed credential validator"
```

---

## Task 3: Drop the opds_token store and migration

**Files:**
- Create: `internal/migrate/files/0034_drop_opds_token.up.sql` / `.down.sql`
- Delete: `internal/store/opds_token.go`

- [x] **Step 1: Write the up migration**

Create `internal/migrate/files/0034_drop_opds_token.up.sql`:

```sql
-- OPDS auth moves to the core ValidateProfileCredential RPC; the per-user
-- token table is no longer used.
DROP TABLE IF EXISTS opds_token;
```

- [x] **Step 2: Write the down migration**

Create `internal/migrate/files/0034_drop_opds_token.down.sql` — recreate the table shape from `0004_opds_kosync.up.sql`. Copy the exact `CREATE TABLE opds_token (...)` statement (and its indexes) from `internal/migrate/files/0004_opds_kosync.up.sql` into this down file.

- [x] **Step 3: Delete the store file**

Delete `internal/store/opds_token.go`.

Run: `git rm internal/store/opds_token.go`

- [x] **Step 4: Verify nothing else references it**

Run: `grep -rn "OPDSToken\|opds_token\|GetOPDSTokenByJTI" internal/ --include=*.go`
Expected: only matches in `internal/server/opds_kosync_routes.go` (the handlers + `opdsAuth`, rewritten in Task 4) and their tests. If other files reference it, note them for Task 4.

- [x] **Step 5: Commit** (after Task 4 — the store deletion does not build alone). Skip the commit here; Task 4 commits both together.

---

## Task 4: OPDS auth via ValidateProfileCredential

**Files:**
- Modify: `internal/server/opds_kosync_routes.go`
- Modify: `internal/server/server.go` (drop opds-token routes)
- Test: `internal/server/opds_auth_test.go` (create)

- [x] **Step 1: Rewrite `opdsAuth`**

In `internal/server/opds_kosync_routes.go`, replace the `opdsAuth` function body. It currently does `r.BasicAuth()` → `GetOPDSTokenByJTI` → bcrypt and returns a `userID string`. Change it to return `(userID, profileID string)` and resolve via the validator:

```go
// opdsAuth validates OPDS Basic-Auth and returns the resolved (userID,
// profileID). Both are "" when the request is unauthorized. profileID is ""
// for the primary profile. The error return distinguishes a bad credential
// (nil error, empty ids) from an auth-service failure (non-nil error).
func (s *Server) opdsAuth(r *http.Request) (string, string, error) {
	user, pass, ok := r.BasicAuth()
	if !ok || user == "" || pass == "" {
		return "", "", nil
	}
	if s.deps.Credentials == nil {
		return "", "", errors.New("credential validator not configured")
	}
	userID, profileID, err := s.deps.Credentials.ValidateProfileCredential(r.Context(), user, pass)
	if err != nil {
		if status.Code(err) == codes.Unauthenticated {
			return "", "", nil // bad credential — not a service error
		}
		return "", "", err // transport / Unimplemented — service error
	}
	return userID, profileID, nil
}
```

Add imports to the file if missing: `"errors"`, `"google.golang.org/grpc/codes"`, `"google.golang.org/grpc/status"`.

- [x] **Step 2: Update every `opdsAuth` call site**

Every OPDS handler in `opds_kosync_routes.go` currently does `if s.opdsAuth(r) == "" { s.opdsChallenge(w, r); return }`. Replace each with:

```go
		userID, profileID, autherr := s.opdsAuth(r)
		if autherr != nil {
			writeErr(w, http.StatusBadGateway, "auth service unavailable")
			return
		}
		if userID == "" {
			s.opdsChallenge(w, r)
			return
		}
```

Handlers that ignore the user id (`handleOPDSRoot`, `handleOPDSCatalog`, `handleOPDSSearch`, `handleOPDSBookEntry`) keep `userID`/`profileID` only as needed — use `_` for unused returns. `handleOPDSDownload` already uses `userID`; it now also has `profileID` available (unused until Task 8). Apply Go's unused-variable rule: name only what you use.

- [x] **Step 3: Delete the OPDS-token handlers**

In `internal/server/opds_kosync_routes.go`, delete `handleListOPDSTokens`, `handleCreateOPDSToken`, and `handleRevokeOPDSToken` entirely.

- [x] **Step 4: Drop the opds-token routes**

In `internal/server/server.go` (or `user_routes.go`, wherever they are mounted), remove the three route registrations:

```go
r.Get("/me/opds-tokens", s.handleListOPDSTokens)
r.Post("/me/opds-tokens", s.handleCreateOPDSToken)
r.Delete("/me/opds-tokens/{id}", s.handleRevokeOPDSToken)
```

Run `grep -rn "opds-tokens" internal/` to find the exact lines.

- [x] **Step 5: Write the auth test**

Create `internal/server/opds_auth_test.go`:

```go
package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeCredentials struct {
	userID, profileID string
	err               error
}

func (f fakeCredentials) ValidateProfileCredential(context.Context, string, string) (string, string, error) {
	return f.userID, f.profileID, f.err
}

func basicReq(user, pass string) *http.Request {
	r := httptest.NewRequest("GET", "/opds/catalog", nil)
	r.SetBasicAuth(user, pass)
	return r
}

func TestOPDSAuth_Resolves(t *testing.T) {
	s := &Server{deps: Deps{Credentials: fakeCredentials{userID: "u-1", profileID: "p-9"}}}
	uid, pid, err := s.opdsAuth(basicReq("jim#laura", "pw"))
	if err != nil || uid != "u-1" || pid != "p-9" {
		t.Errorf("got (%q,%q,%v)", uid, pid, err)
	}
}

func TestOPDSAuth_BadCredentialIsNotServiceError(t *testing.T) {
	s := &Server{deps: Deps{Credentials: fakeCredentials{err: status.Error(codes.Unauthenticated, "no")}}}
	uid, _, err := s.opdsAuth(basicReq("jim", "bad"))
	if err != nil || uid != "" {
		t.Errorf("want empty uid + nil err, got (%q,%v)", uid, err)
	}
}

func TestOPDSAuth_ServiceErrorPropagates(t *testing.T) {
	s := &Server{deps: Deps{Credentials: fakeCredentials{err: status.Error(codes.Unavailable, "down")}}}
	if _, _, err := s.opdsAuth(basicReq("jim", "pw")); err == nil {
		t.Error("want service error, got nil")
	}
}

func TestOPDSAuth_NoHeader(t *testing.T) {
	s := &Server{deps: Deps{Credentials: fakeCredentials{}}}
	uid, _, err := s.opdsAuth(httptest.NewRequest("GET", "/opds/catalog", nil))
	if err != nil || uid != "" {
		t.Errorf("want empty + nil, got (%q,%v)", uid, err)
	}
}
```

(If the `Server` struct or `Deps` field names differ, adjust — confirm `Server` has a `deps Deps` field; the explore confirmed `New(d Deps)` sets `deps: d`.)

- [x] **Step 6: Run tests and build**

Run: `go test ./internal/server/ -run TestOPDSAuth -v` and `go build ./...`
Expected: tests PASS; build succeeds. Existing OPDS feed tests still pass (`buildOPDSCatalogFeed` is unchanged).

- [x] **Step 7: Commit**

```bash
git add internal/migrate/files/0034_drop_opds_token.up.sql internal/migrate/files/0034_drop_opds_token.down.sql internal/store/opds_token.go internal/server/opds_kosync_routes.go internal/server/server.go internal/server/opds_auth_test.go
git commit -m "feat(ebooks): OPDS auth via ValidateProfileCredential; drop opds_token"
```

---

## Task 5: Collections migration — profile_id columns

**Files:**
- Create: `internal/migrate/files/0032_collection_profile.up.sql` / `.down.sql`

- [x] **Step 1: Write the up migration**

Create `internal/migrate/files/0032_collection_profile.up.sql`:

```sql
-- Collections become per-profile. profile_id '' is the primary profile;
-- existing rows default to it, which matches pre-profile behaviour. Ownership
-- is the (user_id, profile_id) pair — '' is unique only within a user.
ALTER TABLE collection
  ADD COLUMN profile_id TEXT NOT NULL DEFAULT '';
ALTER TABLE smart_collection
  ADD COLUMN profile_id TEXT NOT NULL DEFAULT '';

DROP INDEX IF EXISTS collection_user_pinned_idx;
CREATE INDEX collection_user_pinned_idx
  ON collection (user_id, profile_id, is_pinned DESC, name);

DROP INDEX IF EXISTS smart_collection_user_idx;
CREATE INDEX smart_collection_user_idx
  ON smart_collection (user_id, profile_id, is_pinned DESC, name);
```

- [x] **Step 2: Write the down migration**

Create `internal/migrate/files/0032_collection_profile.down.sql`:

```sql
DROP INDEX IF EXISTS collection_user_pinned_idx;
CREATE INDEX collection_user_pinned_idx
  ON collection (user_id, is_pinned DESC, name);
DROP INDEX IF EXISTS smart_collection_user_idx;
CREATE INDEX smart_collection_user_idx
  ON smart_collection (user_id, is_pinned DESC, name);
ALTER TABLE collection DROP COLUMN IF EXISTS profile_id;
ALTER TABLE smart_collection DROP COLUMN IF EXISTS profile_id;
```

(Confirm the index names against `0002_collections.up.sql` and `0017_smart_collection.up.sql` — they are `collection_user_pinned_idx` and `smart_collection_user_idx`. Adjust if different.)

- [x] **Step 3: Verify the migration applies**

Run: `go test ./internal/store/ -run TestMigrations -count=1` if such a test exists; otherwise `go test ./internal/store/ -count=1` (the test harness runs all migrations on a fresh schema — a green store package proves `0032` applies).
Expected: PASS.

- [x] **Step 4: Commit**

```bash
git add internal/migrate/files/0032_collection_profile.up.sql internal/migrate/files/0032_collection_profile.down.sql
git commit -m "feat(ebooks): profile_id columns on collection tables"
```

---

## Task 6: Collections store — profile scoping

**Files:**
- Modify: `internal/store/collection.go`
- Modify: `internal/store/smart_collection.go`
- Test: `internal/store/collection_test.go`

- [x] **Step 1: Write the failing test**

Add to `internal/store/collection_test.go` (create the file if absent, `package store_test`, using `newTestStore`):

```go
func TestCollectionsScopedByProfile(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Same user, two profiles.
	must(t, s.CreateCollection(ctx, store.Collection{ID: "c-primary", UserID: "u-1", ProfileID: "", Name: "Primary"}))
	must(t, s.CreateCollection(ctx, store.Collection{ID: "c-laura", UserID: "u-1", ProfileID: "p-laura", Name: "Laura"}))
	// A different user's primary collection — also profile_id '' — must not leak.
	must(t, s.CreateCollection(ctx, store.Collection{ID: "c-other", UserID: "u-2", ProfileID: "", Name: "Other"}))

	primary, err := s.ListCollectionsByProfile(ctx, "u-1", "")
	if err != nil {
		t.Fatalf("list primary: %v", err)
	}
	if len(primary) != 1 || primary[0].ID != "c-primary" {
		t.Errorf("u-1 primary = %+v, want only c-primary", primary)
	}
	laura, _ := s.ListCollectionsByProfile(ctx, "u-1", "p-laura")
	if len(laura) != 1 || laura[0].ID != "c-laura" {
		t.Errorf("u-1 laura = %+v, want only c-laura", laura)
	}
}
```

Add a `must` helper if the package lacks one: `func must(t *testing.T, err error) { t.Helper(); if err != nil { t.Fatal(err) } }`.

- [x] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/store/ -run TestCollectionsScopedByProfile`
Expected: FAIL — `Collection` has no `ProfileID`; `ListCollectionsByProfile` undefined.

- [x] **Step 3: Update `collection.go`**

In `internal/store/collection.go`:
- Add `ProfileID string` to the `Collection` struct after `UserID string`.
- `CreateCollection`: add `profile_id` to the `INSERT` column list and bind `c.ProfileID`.
- Rename `ListCollectionsByUser(ctx, userID)` to `ListCollectionsByProfile(ctx, userID, profileID string)`; change the query to `WHERE user_id = $1 AND profile_id = $2`, scan `profile_id` into `ProfileID`.
- `DeleteCollection`, `UpdateCollection`: add a `profileID` parameter; change `WHERE id=$1 AND user_id=$2` to `WHERE id=$1 AND user_id=$2 AND profile_id=$3`.
- `AddItemForUser`, `RemoveItemForUser`, `ListItemsForUser`: add a `profileID` parameter; in the `collection`-join sub-clause add `AND profile_id = $N` so item ops also respect the profile.
- The id-only helpers (`AddItem`, `RemoveItem`, `ListItems`) and `ListPublicCollections` are unchanged.

Show the full new `CreateCollection` and `ListCollectionsByProfile` exactly:

```go
func (s *Store) CreateCollection(ctx context.Context, c Collection) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO collection (id, user_id, profile_id, name, color, is_public, is_pinned, cover_book_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		c.ID, c.UserID, c.ProfileID, c.Name, c.Color, c.IsPublic, c.IsPinned, c.CoverBookID)
	return err
}

func (s *Store) ListCollectionsByProfile(ctx context.Context, userID, profileID string) ([]Collection, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, profile_id, name, COALESCE(color,''), is_public, is_pinned, COALESCE(cover_book_id,''), created_at
		 FROM collection WHERE user_id = $1 AND profile_id = $2
		 ORDER BY is_pinned DESC, name`, userID, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Collection
	for rows.Next() {
		var c Collection
		if err := rows.Scan(&c.ID, &c.UserID, &c.ProfileID, &c.Name, &c.Color, &c.IsPublic, &c.IsPinned, &c.CoverBookID, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
```

(Match the existing `SELECT` column list / `COALESCE` usage from the current `ListCollectionsByUser` — adjust the scan if the current code differs.)

- [x] **Step 4: Update `smart_collection.go`**

In `internal/store/smart_collection.go`:
- Add `ProfileID string` to `SmartCollection`.
- `UpsertSmartCollection`: add `profile_id` to the insert.
- `ListSmartCollections(ctx, userID, limit)` → `ListSmartCollections(ctx, userID, profileID string, limit int)`; change `WHERE user_id = $1 OR is_public = TRUE` to `WHERE (user_id = $1 AND profile_id = $2) OR is_public = TRUE` and the `ORDER BY (user_id = $1)` term to `(user_id = $1 AND profile_id = $2)`.
- `DeleteSmartCollection`: add `profileID`; `WHERE id=$1 AND user_id=$2 AND profile_id=$3`.
- `GetSmartCollection` stays id-only (handler enforces ownership — see Task 7).

- [x] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/store/ -run TestCollectionsScopedByProfile -v`
Expected: PASS.

- [x] **Step 6: Commit**

```bash
git add internal/store/collection.go internal/store/smart_collection.go internal/store/collection_test.go
git commit -m "feat(ebooks): scope collection store queries by profile"
```

---

## Task 7: Collection handlers — thread profileID

**Files:**
- Modify: `internal/server/user_routes.go`
- Modify: `internal/server/smart_collection_handler.go`

- [x] **Step 1: Update manual-collection handlers**

In `internal/server/user_routes.go`, every collection handler (`handleListMyCollections`, `handleCreateCollection`, `handleUpdateCollection`, `handleDeleteCollection`, `handleListCollectionItems`, `handleAddCollectionItem`, `handleRemoveCollectionItem`) currently does `id, _ := auth.FromContext(r.Context())` and passes `id.UserID`. Change each to also pass `id.ProfileID`:
- `handleListMyCollections`: `s.deps.Store.ListCollectionsByProfile(r.Context(), id.UserID, id.ProfileID)`.
- `handleCreateCollection`: set `ProfileID: id.ProfileID` on the `Collection` struct.
- `handleUpdateCollection`, `handleDeleteCollection`: pass `id.ProfileID` to the renamed store calls.
- `handleListCollectionItems`, `handleAddCollectionItem`, `handleRemoveCollectionItem`: pass `id.ProfileID` to `ListItemsForUser`/`AddItemForUser`/`RemoveItemForUser`.

- [x] **Step 2: Update smart-collection handlers**

In `internal/server/smart_collection_handler.go`:
- `handleListSmartCollections`: `ListSmartCollections(r.Context(), userID, profileID, 200)`.
- `handleGetSmartCollection`: after `GetSmartCollection`, the ownership check becomes `if c.UserID != userID || c.ProfileID != profileID { if !c.IsPublic { 404/403 } }` — i.e. a non-public collection is only visible to its owning `(user, profile)`.
- `handleUpdateSmartCollection`: ownership check `existing.UserID != userID || existing.ProfileID != profileID`.
- `handleDeleteSmartCollection`: pass `profileID`.
- `persistSmartCollection`: set `ProfileID: profileID` on new rows; read `profileID` from `auth.FromContext`.

- [x] **Step 3: Verify the build**

Run: `go build ./...`
Expected: success (the renamed store methods now have all call sites updated).

- [x] **Step 4: Run the server + store tests**

Run: `go test ./internal/server/ ./internal/store/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add internal/server/user_routes.go internal/server/smart_collection_handler.go
git commit -m "feat(ebooks): thread profile id through collection handlers"
```

---

## Task 8: OPDS Collections feed

**Files:**
- Modify: `internal/server/opds_kosync_routes.go`
- Test: `internal/server/opds_collections_test.go` (create)

- [x] **Step 1: Write the failing test**

Create `internal/server/opds_collections_test.go`:

```go
package server

import (
	"strings"
	"testing"
	"time"

	"github.com/RXWatcher/continuum-plugin-ebooks/internal/store"
)

func TestBuildOPDSCollectionsFeed(t *testing.T) {
	cols := []store.Collection{
		{ID: "c-1", Name: "Nancy Drew"},
		{ID: "c-2", Name: "Sci-Fi"},
	}
	feed := buildOPDSCollectionsFeed(cols, time.Unix(1700000000, 0))
	if len(feed.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(feed.Entries))
	}
	if !strings.Contains(feed.Entries[0].Links[0].Href, "c-1") {
		t.Errorf("entry 0 href = %q, want it to reference c-1", feed.Entries[0].Links[0].Href)
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/server/ -run TestBuildOPDSCollectionsFeed`
Expected: FAIL — `buildOPDSCollectionsFeed` undefined.

- [x] **Step 3: Implement the feed builder and routes**

In `internal/server/opds_kosync_routes.go`:

Add a navigation feed builder (mirrors `buildOPDSCatalogFeed`'s style — navigation entries link to sub-feeds):

```go
// buildOPDSCollectionsFeed renders the authenticated profile's collections as
// an OPDS navigation feed; each entry links to that collection's acquisition
// feed at /opds/collection/{id}.
func buildOPDSCollectionsFeed(cols []store.Collection, now time.Time) opdsFeed {
	feed := opdsFeed{
		XMLNS:   "http://www.w3.org/2005/Atom",
		ID:      "tag:continuum:ebooks:opds:collections",
		Title:   "My Collections",
		Updated: now.UTC().Format(time.RFC3339),
		Links: []opdsLink{
			{Rel: "self", Type: "application/atom+xml;profile=opds-catalog", Href: "/opds/collections"},
		},
	}
	for _, c := range cols {
		feed.Entries = append(feed.Entries, opdsEntry{
			ID:      "tag:continuum:ebooks:collection:" + c.ID,
			Title:   c.Name,
			Updated: now.UTC().Format(time.RFC3339),
			Links: []opdsLink{{
				Rel:  "subsection",
				Type: "application/atom+xml;profile=opds-catalog;kind=acquisition",
				Href: "/opds/collection/" + c.ID,
			}},
		})
	}
	return feed
}
```

Add two handlers: `handleOPDSCollections` (lists the profile's collections via `buildOPDSCollectionsFeed`) and `handleOPDSCollection` (an acquisition feed for one collection's items — reuse the `opdsEntry`/acquisition-link pattern from `handleOPDSCatalog`, resolving items through `Store.ListItemsForUser(ctx, userID, profileID, collectionID)` and the backend `GetBook`). Both call `s.opdsAuth(r)` for `(userID, profileID)` with the same error handling as Task 4 step 2.

In `mountOPDS`, register:

```go
	r.Get("/collections", s.handleOPDSCollections)
	r.Get("/collection/{id}", s.handleOPDSCollection)
```

In `handleOPDSRoot`, add a navigation link to the service document's `Links` so readers discover it:

```go
		{Rel: "subsection", Type: "application/atom+xml;profile=opds-catalog", Href: "/opds/collections"},
```

- [x] **Step 4: Run the test and build**

Run: `go test ./internal/server/ -run TestBuildOPDSCollectionsFeed -v` and `go build ./...`
Expected: PASS; build succeeds.

- [x] **Step 5: Commit**

```bash
git add internal/server/opds_kosync_routes.go internal/server/opds_collections_test.go
git commit -m "feat(ebooks): expose per-profile collections over OPDS"
```

---

## Task 9: kosync migration — profile_id columns

**Files:**
- Create: `internal/migrate/files/0033_kosync_profile.up.sql` / `.down.sql`

- [x] **Step 1: Write the up migration**

Create `internal/migrate/files/0033_kosync_profile.up.sql`:

```sql
-- kosync becomes per-profile. Existing rows default to '' (primary profile).
ALTER TABLE kosync_user
  ADD COLUMN profile_id TEXT NOT NULL DEFAULT '';
ALTER TABLE kosync_progress
  ADD COLUMN profile_id TEXT NOT NULL DEFAULT '';
ALTER TABLE kosync_book_link
  ADD COLUMN profile_id TEXT NOT NULL DEFAULT '';

-- Re-key progress and book-link on the profile. kosync_progress PK is
-- (user_id, document, device_id) — set by migration 0006 to isolate progress
-- per device; device_id MUST be kept. kosync_book_link PK is (document, user_id).
ALTER TABLE kosync_progress DROP CONSTRAINT kosync_progress_pkey;
ALTER TABLE kosync_progress
  ADD PRIMARY KEY (user_id, profile_id, document, device_id);
ALTER TABLE kosync_book_link DROP CONSTRAINT kosync_book_link_pkey;
ALTER TABLE kosync_book_link
  ADD PRIMARY KEY (document, user_id, profile_id);
```

(Confirm the constraint names against `0004_opds_kosync.up.sql`. PostgreSQL's default name for a table `t`'s primary key is `t_pkey`, so `kosync_progress_pkey` / `kosync_book_link_pkey` are correct unless the migration named them explicitly — check and adjust.)

- [x] **Step 2: Write the down migration**

Create `internal/migrate/files/0033_kosync_profile.down.sql`:

```sql
ALTER TABLE kosync_progress DROP CONSTRAINT kosync_progress_pkey;
ALTER TABLE kosync_progress ADD PRIMARY KEY (user_id, document);
ALTER TABLE kosync_book_link DROP CONSTRAINT kosync_book_link_pkey;
ALTER TABLE kosync_book_link ADD PRIMARY KEY (document, user_id);
ALTER TABLE kosync_user DROP COLUMN IF EXISTS profile_id;
ALTER TABLE kosync_progress DROP COLUMN IF EXISTS profile_id;
ALTER TABLE kosync_book_link DROP COLUMN IF EXISTS profile_id;
```

- [x] **Step 3: Verify the migration applies**

Run: `go test ./internal/store/ -count=1`
Expected: PASS (the harness applies all migrations on a fresh schema; existing kosync store tests still pass against the new columns since the store code is updated in Task 10 — if the store package does not build yet, defer this check to Task 10 step 5).

- [x] **Step 4: Commit**

```bash
git add internal/migrate/files/0033_kosync_profile.up.sql internal/migrate/files/0033_kosync_profile.down.sql
git commit -m "feat(ebooks): profile_id columns on kosync tables"
```

---

## Task 10: kosync store — profile scoping

**Files:**
- Modify: `internal/store/kosync.go`
- Test: `internal/store/kosync_test.go`

- [x] **Step 1: Write the failing test**

Add to `internal/store/kosync_test.go` (create if absent):

```go
func TestKosyncProgressScopedByProfile(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	must(t, s.UpsertKosyncProgress(ctx, store.KosyncProgress{
		UserID: "u-1", ProfileID: "", Document: "doc-a", Progress: "10", DeviceID: "d1",
	}))
	must(t, s.UpsertKosyncProgress(ctx, store.KosyncProgress{
		UserID: "u-1", ProfileID: "p-laura", Document: "doc-a", Progress: "90", DeviceID: "d1",
	}))

	primary, _ := s.GetKosyncProgress(ctx, "u-1", "", "doc-a")
	laura, _ := s.GetKosyncProgress(ctx, "u-1", "p-laura", "doc-a")
	if primary.Progress != "10" || laura.Progress != "90" {
		t.Errorf("primary=%q laura=%q, want 10/90", primary.Progress, laura.Progress)
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/store/ -run TestKosyncProgressScopedByProfile`
Expected: FAIL — `KosyncProgress` has no `ProfileID`; `GetKosyncProgress` signature mismatch.

- [x] **Step 3: Update `kosync.go`**

In `internal/store/kosync.go`:
- Add `ProfileID string` to `KosyncUser`, `KosyncProgress`, and `KosyncBookLink`.
- `UpsertKosyncUser` / `CreateKosyncUserStrict`: add `profile_id` to the insert column list and bind it.
- `GetKosyncUserByUsername`: add `profile_id` to the `SELECT` and scan it (lookup stays by `kosync_username` — the username is the self-identifying key).
- `UpsertKosyncProgress`: add `profile_id` to the insert; change the `ON CONFLICT` target to `(user_id, profile_id, document)`.
- `GetKosyncProgress(ctx, userID, document)` → `GetKosyncProgress(ctx, userID, profileID, document string)`; query `WHERE user_id=$1 AND profile_id=$2 AND document=$3`.
- `UpsertKosyncBookLink`: add `profile_id`; `ON CONFLICT (document, user_id, profile_id)`.
- `FindKosyncBookLinkByBook(ctx, userID, bookID)` → add `profileID`; `WHERE user_id=$1 AND profile_id=$2 AND book_id=$3`.
- `ListKosyncUsers`: add `profile_id` to the `SELECT`/scan (admin listing — no filter change).
- `DeleteKosyncUser`: unchanged (deletes by username, which is unique).

- [x] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/store/ -run TestKosyncProgressScopedByProfile -v`
Expected: PASS.

- [x] **Step 5: Run the full store package**

Run: `go test ./internal/store/ -count=1`
Expected: PASS — confirms migrations `0032`/`0033` apply and all store code builds.

- [x] **Step 6: Commit**

```bash
git add internal/store/kosync.go internal/store/kosync_test.go
git commit -m "feat(ebooks): scope kosync store by profile"
```

---

## Task 11: kosync handlers — profile-aware, portal-only registration

**Files:**
- Modify: `internal/server/opds_kosync_routes.go`

- [x] **Step 1: Retire the public registration endpoint**

In `mountKosync`, remove the `POST /users/create` route. KOReader registration is portal-only now. Delete the public-path branch of `handleKosyncCreate` (the `id.UserID == ""` synthetic-account branch); keep only the authenticated path. Rename the function to reflect it is the authenticated handler (it is already reached via `handleKosyncRegister` for `/api/v1/me/kosync/register`).

- [x] **Step 2: Make registration profile-aware**

In the authenticated kosync registration handler, the identity now carries `(UserID, ProfileID)` and the host injects `X-Continuum-User-Name` / `X-Continuum-Profile-Name`. Compute the kosync username as the `user#profile` string — `username` for the primary profile (`ProfileID == ""`), `username#profileName` otherwise — read from the request headers `X-Continuum-User-Name` and `X-Continuum-Profile-Name`. Store the `KosyncUser` with `UserID`, `ProfileID`, and that computed `KosyncUsername`. The kosync password handling (sha1 → bcrypt) is unchanged.

- [x] **Step 3: Make auth and progress profile-aware**

In `kosyncAuthHeader`, after `GetKosyncUserByUsername` resolves the row, it now yields `(UserID, ProfileID)` directly from the row — no `#` parsing needed, the username lookup is self-identifying. `handleKosyncGetProgress` / `handleKosyncPutProgress` pass `u.ProfileID` through to `GetKosyncProgress` / `UpsertKosyncProgress`. The kosync `document` book-link handler passes `ProfileID` to `UpsertKosyncBookLink` / `FindKosyncBookLinkByBook`.

- [x] **Step 4: Verify the build and tests**

Run: `go build ./...` and `go test ./internal/server/ ./internal/store/`
Expected: build succeeds; tests pass. Update any existing kosync handler test that posted to `/kosync/users/create` — point it at the authenticated `/api/v1/me/kosync/register` path, or remove it if it specifically tested the retired public path.

- [x] **Step 5: Commit**

```bash
git add internal/server/opds_kosync_routes.go
git commit -m "feat(ebooks): profile-aware kosync, portal-only registration"
```

---

## Task 12: Remove content restriction

**Files:**
- Create: `internal/migrate/files/0035_drop_content_restriction.up.sql` / `.down.sql`
- Delete: `internal/store/content_restriction.go`, `internal/server/content_restriction.go`
- Modify: `internal/server/server.go`

Note: the explore confirmed `ApplyContentRestriction` is defined but never called — no catalog/search/OPDS handler applies a restriction filter. Removal is therefore pure deletion; there are no filter call sites to unwire.

- [x] **Step 1: Write the drop migration**

Create `internal/migrate/files/0035_drop_content_restriction.up.sql`:

```sql
-- Content restriction is removed; curation is collections, not blocking.
DROP TABLE IF EXISTS content_restriction;
```

Create `0035_drop_content_restriction.down.sql` — copy the exact `CREATE TABLE content_restriction (...)` from `internal/migrate/files/0019_content_restriction.up.sql`.

- [x] **Step 2: Delete the store and server files**

```bash
git rm internal/store/content_restriction.go internal/server/content_restriction.go
```

- [x] **Step 3: Remove the route mount**

In `internal/server/server.go`, remove the line `s.mountContentRestrictionRoutes(r)`.

- [x] **Step 4: Confirm no dangling references**

Run: `grep -rn "ContentRestriction\|content_restriction\|content-restriction" internal/ --include=*.go`
Expected: no matches. If any remain (e.g. an admin-routes reference), delete them.

- [x] **Step 5: Build and test**

Run: `go build ./...` and `go test ./internal/server/ ./internal/store/ -count=1`
Expected: build succeeds; tests pass (drop the content-restriction store/server tests as part of the file deletions).

- [x] **Step 6: Commit**

```bash
git add -A internal/migrate/files/0035_drop_content_restriction.up.sql internal/migrate/files/0035_drop_content_restriction.down.sql internal/store/content_restriction.go internal/server/content_restriction.go internal/server/server.go
git commit -m "feat(ebooks): remove content restriction"
```

---

## Task 13: Frontend — remove OPDS-token UI

**Files:**
- Modify: `web/src/pages/Apps.tsx`
- Modify: `web/src/lib/api.ts`

- [x] **Step 1: Remove the OPDS section**

In `web/src/pages/Apps.tsx`, delete the `OPDSSection` component entirely and remove its render from the page body. Keep `KOReaderSection` — kosync stays. Remove the now-unused imports (`listOPDSTokens`, `createOPDSToken`, `revokeOPDSToken`).

- [x] **Step 2: Remove the API functions**

In `web/src/lib/api.ts`, delete the `OPDSToken` type and the `listOPDSTokens`, `createOPDSToken`, `revokeOPDSToken` functions.

- [x] **Step 3: Verify the build**

Run: `cd web && pnpm build`
Expected: `tsc -b` + Vite build succeed with no unused-symbol or missing-import errors.

- [x] **Step 4: Commit**

```bash
git add web/src/pages/Apps.tsx web/src/lib/api.ts
git commit -m "feat(ebooks): remove OPDS token management UI"
```

---

## Task 14: Frontend — remove content-restriction admin UI

**Files:**
- Delete: `web/src/pages/admin/ContentRestrictions.tsx`
- Modify: `web/src/pages/Admin.tsx`
- Modify: `web/src/lib/api.ts`

- [x] **Step 1: Delete the admin page**

```bash
git rm web/src/pages/admin/ContentRestrictions.tsx
```

- [x] **Step 2: Remove the tab**

In `web/src/pages/Admin.tsx`, remove the `ContentRestrictionsTab` import and the `<TabsContent value="restrictions">...</TabsContent>` block, plus the matching `<TabsTrigger>` for that tab.

- [x] **Step 3: Remove the API functions**

In `web/src/lib/api.ts`, delete the `ContentRestriction` type and the `listContentRestrictions`, `putContentRestriction`, `deleteContentRestriction` functions.

- [x] **Step 4: Verify the build**

Run: `cd web && pnpm build`
Expected: build succeeds.

- [x] **Step 5: Commit**

```bash
git add -A web/src/pages/admin/ContentRestrictions.tsx web/src/pages/Admin.tsx web/src/lib/api.ts
git commit -m "feat(ebooks): remove content-restriction admin UI"
```

---

## Task 15: Full verification

**Files:** none (verification only)

- [x] **Step 1: Backend — build, vet, test**

Run: `go build ./... && go vet ./internal/... && go test ./internal/... -count=1`
Expected: PASS for every package.

- [x] **Step 2: Frontend — build**

Run: `cd web && pnpm build`
Expected: build succeeds.

- [x] **Step 3: Frontend — unit tests**

Run: `cd web && pnpm test` (if `vitest` has suites; skip if none).
Expected: PASS.

- [x] **Step 4: Manual smoke check (optional but recommended)**

Deploy with `/opt/continuum_plugins/install-plugin.sh continuum-plugin-ebooks` and verify in a browser: an OPDS reader authenticates with `user` / `user#profile` + the continuum password; a profile's collections show only that profile's collections in the SPA and over `/opds/collections`; the Apps page no longer shows OPDS tokens; the admin area no longer shows content restrictions.

- [x] **Step 5: Final commit (only if step 4 surfaced fixes)**

```bash
git add -A
git commit -m "fix(ebooks): smoke-test fixes for profile-aware foundation"
```

---

## Notes for the implementer

- `profile_id = ''` is the primary profile and is unique only within a user — every profile-scoped query MUST scope on `(user_id, profile_id)`, never `profile_id` alone.
- The ebooks plugin builds against the local SDK through `/opt/continuum_plugins/go.work`; `runtimehost.Client.ValidateProfileCredential` is available because the SDK working tree carries SDK PR #5. If `go build` cannot find that method, check the SDK checkout's branch.
- Migration numbers `0032`–`0035` assume `0031` is the current highest. If another branch has added migrations, renumber to stay sequential.
- `opdsAuth` now returns three values; every call site in `opds_kosync_routes.go` must be updated together or the package will not compile — Task 4 step 2 covers them all.
