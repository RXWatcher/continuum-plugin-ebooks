# Portal Library Auto-Provision (Full Sync) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Mirror a backend's libraries into portal presentation shelves (create/update/prune) via a manual admin endpoint and an hourly scheduled task, with a guard that makes a briefly-unavailable backend a safe no-op.

**Architecture:** A new `internal/libsync` package holds a pure `Reconcile` function and a `Sync` orchestrator (guard → fetch → list → reconcile → reuse the audited `store.ReplacePortalLibraries`). An HTTP handler and a scheduled task both call `libsync.Sync`.

**Tech Stack:** Go (chi, pgx), React + @tanstack/react-query (portal SPA).

Spec: `docs/superpowers/specs/2026-05-17-portal-library-autoprovision-design.md`
Repo: `/opt/silo_plugins/silo-plugin-ebooks` (run all commands from there). The confusingly-named sibling `/opt/silo_plugins/silo-plugin-local-ebooks` must NOT be touched.

Verified existing shapes (do not redefine):
- `store.PortalLibrary{ ID int64; Name string; MediaType string; BackendPluginID string; BackendLibraryID *int64; Enabled bool; SortOrder int }`
- `store.(*Store).ListPortalLibraries(ctx, enabledOnly bool) ([]PortalLibrary, error)`
- `store.(*Store).ReplacePortalLibraries(ctx, libs []PortalLibrary) error` — validates Name/BackendPluginID non-empty, set-replace (ID>0 update, ID==0 insert, ids absent from the slice are DELETEd).
- `store.Config` has `BackendTarget() string` and `HasBackend() bool`; `store.(*Store).GetConfig(ctx) (Config, error)`.
- `backend.LibraryInfo{ ID int64; Name string; Path string; MediaType string; Enabled bool; LastScannedAt string }`
- `backend.NewEbookBackend(host *backend.HostHTTPClient, installID string) *backend.EbookBackend`; `(*EbookBackend).ListLibraries(ctx) ([]LibraryInfo, error)`.
- `server.Server` has `deps Deps` with `deps.Store *store.Store`, `deps.Host *backend.HostHTTPClient`; helpers `writeJSON(w, code, v)`, `writeErr(w, code, msg string)` exist in package `server`.
- `scheduler.Tasks` struct fields include `Store *store.Store`, `Host *backend.HostHTTPClient`, `Log hclog.Logger`. Scheduler task = method `func (t *Tasks) X(ctx context.Context) error`; registered in `cmd/silo-plugin-ebooks/main.go` in the `scheduler.New(func() map[string]scheduler.TaskFn{...})` map.
- Frontend `web/src/lib/api.ts` exposes `api.get/api.put/api.post`; `web/src/pages/Admin.tsx` `LibrariesTab` already receives `backends: BackendOption[]` and uses `useMutation`/`useQueryClient`/`toast`/`Button`.

---

## File Structure

Created:
- `internal/libsync/libsync.go` — `SyncStats`, pure `Reconcile`, interfaces `LibStore`/`BackendLister`, `Sync` orchestrator.
- `internal/libsync/libsync_test.go` — pure Reconcile table tests + Sync guard tests.

Modified:
- `internal/server/admin_routes.go` — `handleAdminSyncLibraries` + route registration.
- `internal/server/admin_sync_test.go` — handler test (new test file, package `server`).
- `internal/scheduler/tasks.go` — `(*Tasks).PortalLibrarySync`.
- `cmd/silo-plugin-ebooks/main.go` — register `"portal_library_sync"` in the task map.
- `cmd/silo-plugin-ebooks/manifest.json` — add the `scheduled_task.v1` entry.
- `web/src/lib/api.ts` — `adminSyncLibraries`.
- `web/src/pages/Admin.tsx` — backend select + "Sync from backend" button in `LibrariesTab`.

---

## Task 1: libsync — pure Reconcile + SyncStats

**Files:**
- Create: `internal/libsync/libsync.go`
- Test: `internal/libsync/libsync_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/libsync/libsync_test.go`:

```go
package libsync

import (
	"testing"

	"github.com/RXWatcher/silo-plugin-ebooks/internal/backend"
	"github.com/RXWatcher/silo-plugin-ebooks/internal/store"
)

func i64(v int64) *int64 { return &v }

func TestReconcile_CreateMissing(t *testing.T) {
	out, st := Reconcile(nil,
		[]backend.LibraryInfo{{ID: 5, Name: "Comics", MediaType: "comics"}}, "42")
	if st.Created != 1 || st.Updated != 0 || st.Pruned != 0 {
		t.Fatalf("stats=%+v", st)
	}
	if len(out) != 1 || out[0].ID != 0 || out[0].Name != "Comics" ||
		out[0].MediaType != "comics" || out[0].BackendPluginID != "42" ||
		out[0].BackendLibraryID == nil || *out[0].BackendLibraryID != 5 ||
		!out[0].Enabled {
		t.Fatalf("created row wrong: %+v", out[0])
	}
}

func TestReconcile_DefaultsEmptyNameAndMediaType(t *testing.T) {
	out, _ := Reconcile(nil, []backend.LibraryInfo{{ID: 9}}, "42")
	if out[0].Name != "Library 9" || out[0].MediaType != "book" {
		t.Fatalf("defaults wrong: %+v", out[0])
	}
}

func TestReconcile_UpdatePreservesOperatorFields(t *testing.T) {
	existing := []store.PortalLibrary{{
		ID: 7, Name: "Old", MediaType: "book", BackendPluginID: "42",
		BackendLibraryID: i64(5), Enabled: false, SortOrder: 3,
	}}
	out, st := Reconcile(existing,
		[]backend.LibraryInfo{{ID: 5, Name: "Comics", MediaType: "comics"}}, "42")
	if st.Updated != 1 || st.Created != 0 || st.Pruned != 0 {
		t.Fatalf("stats=%+v", st)
	}
	g := out[0]
	if g.ID != 7 || g.Name != "Comics" || g.MediaType != "comics" ||
		g.Enabled != false || g.SortOrder != 3 || g.BackendLibraryID == nil || *g.BackendLibraryID != 5 {
		t.Fatalf("update must change only name/media_type: %+v", g)
	}
}

func TestReconcile_PruneGoneBackendDerived(t *testing.T) {
	existing := []store.PortalLibrary{{
		ID: 7, Name: "Gone", MediaType: "book", BackendPluginID: "42",
		BackendLibraryID: i64(99), Enabled: true, SortOrder: 0,
	}}
	out, st := Reconcile(existing, []backend.LibraryInfo{{ID: 5, Name: "Keep"}}, "42")
	if st.Pruned != 1 || st.Created != 1 {
		t.Fatalf("stats=%+v", st)
	}
	for _, l := range out {
		if l.BackendLibraryID != nil && *l.BackendLibraryID == 99 {
			t.Fatal("pruned row must be omitted")
		}
	}
}

func TestReconcile_PassthroughUntouched(t *testing.T) {
	existing := []store.PortalLibrary{
		{ID: 1, Name: "Manual", MediaType: "book", BackendPluginID: "42", BackendLibraryID: nil, Enabled: true, SortOrder: 0},
		{ID: 2, Name: "OtherBackend", MediaType: "book", BackendPluginID: "99", BackendLibraryID: i64(5), Enabled: true, SortOrder: 1},
	}
	out, st := Reconcile(existing, []backend.LibraryInfo{{ID: 5, Name: "X"}}, "42")
	if st.Kept != 2 || st.Pruned != 0 || st.Created != 1 {
		t.Fatalf("stats=%+v", st)
	}
	var sawManual, sawOther bool
	for _, l := range out {
		if l.ID == 1 {
			sawManual = true
		}
		if l.ID == 2 {
			sawOther = true
		}
	}
	if !sawManual || !sawOther {
		t.Fatal("non-managed rows must pass through unchanged")
	}
}

func TestReconcile_Idempotent(t *testing.T) {
	bl := []backend.LibraryInfo{{ID: 5, Name: "Comics", MediaType: "comics"}}
	out1, _ := Reconcile(nil, bl, "42")
	// Simulate persistence assigning an id.
	out1[0].ID = 7
	out2, st := Reconcile(out1, bl, "42")
	if st.Created != 0 || st.Updated != 0 || st.Pruned != 0 || st.Kept != 1 {
		t.Fatalf("second run must be a no-op: %+v", st)
	}
	if len(out2) != 1 || out2[0].ID != 7 {
		t.Fatalf("idempotent run changed rows: %+v", out2)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/libsync/`
Expected: build failure — `undefined: Reconcile`, `undefined: SyncStats`.

- [ ] **Step 3: Write the implementation**

Create `internal/libsync/libsync.go`:

```go
// Package libsync mirrors a backend's libraries into portal presentation
// shelves. Reconcile is pure (the single destructive decision); Sync wraps
// it behind a guard that refuses to run on a failed/empty backend fetch.
package libsync

import (
	"context"
	"errors"
	"fmt"

	"github.com/RXWatcher/silo-plugin-ebooks/internal/backend"
	"github.com/RXWatcher/silo-plugin-ebooks/internal/store"
)

// SyncStats summarizes a reconcile pass.
type SyncStats struct {
	Created int
	Updated int
	Pruned  int
	Kept    int
}

// Reconcile computes the full desired portal_library set for backendID.
//
// Sync-managed = rows with BackendPluginID==backendID AND BackendLibraryID!=nil.
// Non-managed rows (operator-created nil-BackendLibraryID, or other backend)
// pass through untouched. Managed rows are matched to backend libraries by
// *BackendLibraryID == LibraryInfo.ID: matched -> update Name/MediaType only
// (ID/Enabled/SortOrder preserved); unmatched backend lib -> create; managed
// row with no backend lib -> prune (omitted). Pure & deterministic.
func Reconcile(existing []store.PortalLibrary, backendLibs []backend.LibraryInfo, backendID string) ([]store.PortalLibrary, SyncStats) {
	var out []store.PortalLibrary
	var st SyncStats

	managed := make(map[int64]store.PortalLibrary)
	maxSort := -1
	for _, e := range existing {
		if e.SortOrder > maxSort {
			maxSort = e.SortOrder
		}
		if e.BackendPluginID == backendID && e.BackendLibraryID != nil {
			managed[*e.BackendLibraryID] = e
		} else {
			out = append(out, e)
			st.Kept++
		}
	}

	seen := make(map[int64]bool, len(backendLibs))
	for _, bl := range backendLibs {
		seen[bl.ID] = true
		name := bl.Name
		if name == "" {
			name = fmt.Sprintf("Library %d", bl.ID)
		}
		mt := bl.MediaType
		if mt == "" {
			mt = "book"
		}
		if cur, ok := managed[bl.ID]; ok {
			changed := cur.Name != name || cur.MediaType != mt
			cur.Name = name
			cur.MediaType = mt
			out = append(out, cur)
			if changed {
				st.Updated++
			} else {
				st.Kept++
			}
			continue
		}
		maxSort++
		idCopy := bl.ID
		out = append(out, store.PortalLibrary{
			ID:               0,
			Name:             name,
			MediaType:        mt,
			BackendPluginID:  backendID,
			BackendLibraryID: &idCopy,
			Enabled:          true,
			SortOrder:        maxSort,
		})
		st.Created++
	}

	for blID := range managed {
		if !seen[blID] {
			st.Pruned++
		}
	}
	return out, st
}

// LibStore is the store surface Sync needs (satisfied by *store.Store).
type LibStore interface {
	ListPortalLibraries(ctx context.Context, enabledOnly bool) ([]store.PortalLibrary, error)
	ReplacePortalLibraries(ctx context.Context, libs []store.PortalLibrary) error
}

// BackendLister fetches the backend's libraries (satisfied by
// *backend.EbookBackend).
type BackendLister interface {
	ListLibraries(ctx context.Context) ([]backend.LibraryInfo, error)
}

// Sync fetches backendID's libraries and reconciles them into portal
// shelves. THE GUARD: a fetch error OR zero libraries aborts with an error
// and ZERO store writes, so a briefly-unavailable/empty backend can never
// mass-prune operator config.
func Sync(ctx context.Context, st LibStore, lister BackendLister, backendID string) (SyncStats, error) {
	if backendID == "" {
		return SyncStats{}, errors.New("no backend configured")
	}
	libs, err := lister.ListLibraries(ctx)
	if err != nil {
		return SyncStats{}, fmt.Errorf("fetch backend libraries: %w", err)
	}
	if len(libs) == 0 {
		return SyncStats{}, errors.New("refusing to sync: backend returned no libraries")
	}
	existing, err := st.ListPortalLibraries(ctx, false)
	if err != nil {
		return SyncStats{}, fmt.Errorf("list portal libraries: %w", err)
	}
	desired, stats := Reconcile(existing, libs, backendID)
	if err := st.ReplacePortalLibraries(ctx, desired); err != nil {
		return SyncStats{}, fmt.Errorf("replace portal libraries: %w", err)
	}
	return stats, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/libsync/`
Expected: `ok  github.com/RXWatcher/silo-plugin-ebooks/internal/libsync`

- [ ] **Step 5: Commit**

```bash
git add internal/libsync/
git -c user.email=agent@anthropic.com -c user.name="Claude Code" commit -m "feat(libsync): pure Reconcile + Sync orchestrator with empty/error guard

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: libsync.Sync guard tests

**Files:**
- Modify: `internal/libsync/libsync_test.go` (append)

- [ ] **Step 1: Write the failing test**

Append to `internal/libsync/libsync_test.go`:

```go
import (
	"context"
	"errors"
)

type fakeStore struct {
	existing   []store.PortalLibrary
	replaced   bool
	replacedTo []store.PortalLibrary
}

func (f *fakeStore) ListPortalLibraries(_ context.Context, _ bool) ([]store.PortalLibrary, error) {
	return f.existing, nil
}
func (f *fakeStore) ReplacePortalLibraries(_ context.Context, libs []store.PortalLibrary) error {
	f.replaced = true
	f.replacedTo = libs
	return nil
}

type fakeLister struct {
	libs []backend.LibraryInfo
	err  error
}

func (f *fakeLister) ListLibraries(_ context.Context) ([]backend.LibraryInfo, error) {
	return f.libs, f.err
}

func TestSync_GuardBackendErrorNoWrite(t *testing.T) {
	fs := &fakeStore{}
	_, err := Sync(context.Background(), fs, &fakeLister{err: errors.New("upstream down")}, "42")
	if err == nil {
		t.Fatal("expected error on backend fetch failure")
	}
	if fs.replaced {
		t.Fatal("store must NOT be written when the backend fetch failed")
	}
}

func TestSync_GuardZeroLibrariesNoWrite(t *testing.T) {
	fs := &fakeStore{existing: []store.PortalLibrary{{ID: 1, Name: "X", BackendPluginID: "42"}}}
	_, err := Sync(context.Background(), fs, &fakeLister{libs: nil}, "42")
	if err == nil {
		t.Fatal("expected error when backend returns zero libraries")
	}
	if fs.replaced {
		t.Fatal("store must NOT be written (catastrophic-prune guard)")
	}
}

func TestSync_HappyPathWritesReconciled(t *testing.T) {
	fs := &fakeStore{}
	stats, err := Sync(context.Background(), fs,
		&fakeLister{libs: []backend.LibraryInfo{{ID: 5, Name: "Comics", MediaType: "comics"}}}, "42")
	if err != nil {
		t.Fatal(err)
	}
	if !fs.replaced || len(fs.replacedTo) != 1 || stats.Created != 1 {
		t.Fatalf("expected one created row persisted; replaced=%v to=%+v stats=%+v", fs.replaced, fs.replacedTo, stats)
	}
}

func TestSync_EmptyBackendIDErrors(t *testing.T) {
	fs := &fakeStore{}
	if _, err := Sync(context.Background(), fs, &fakeLister{}, ""); err == nil {
		t.Fatal("empty backendID must error")
	}
	if fs.replaced {
		t.Fatal("no write on empty backendID")
	}
}
```

(Merge the new `import (...)` block into the existing import group at the top of the file — do not create a second `import` statement.)

- [ ] **Step 2: Run test to verify it fails then passes**

Run: `go test ./internal/libsync/ -run TestSync`
Expected: compiles and PASSES (Sync already implemented in Task 1; these lock the guard). If any FAIL, fix `libsync.go` until green.

- [ ] **Step 3: Commit**

```bash
git add internal/libsync/libsync_test.go
git -c user.email=agent@anthropic.com -c user.name="Claude Code" commit -m "test(libsync): guard tests — no store write on backend error/empty

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Admin HTTP endpoint `POST /admin/libraries/sync`

**Files:**
- Modify: `internal/server/admin_routes.go`
- Test: `internal/server/admin_sync_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/server/admin_sync_test.go`:

```go
package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleAdminSyncLibraries_MissingBackendID(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/admin/libraries/sync", nil)
	s.handleAdminSyncLibraries(rec, req)
	if rec.Code != 400 {
		t.Fatalf("status=%d want 400 (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "backend_plugin_id") {
		t.Fatalf("body should mention backend_plugin_id: %s", rec.Body.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestHandleAdminSyncLibraries`
Expected: build failure — `s.handleAdminSyncLibraries undefined`.

- [ ] **Step 3: Write the implementation**

In `internal/server/admin_routes.go`, add the route inside `mountAdminRoutes` immediately after the existing `r.Get("/admin/backend-libraries", s.handleAdminBackendLibraries)` line:

```go
	r.Post("/admin/libraries/sync", s.handleAdminSyncLibraries)
```

Add the handler function at the end of `internal/server/admin_routes.go`:

```go
func (s *Server) handleAdminSyncLibraries(w http.ResponseWriter, r *http.Request) {
	backendID := r.URL.Query().Get("backend_plugin_id")
	if backendID == "" {
		writeErr(w, 400, "backend_plugin_id required")
		return
	}
	stats, err := libsync.Sync(r.Context(), s.deps.Store,
		backend.NewEbookBackend(s.deps.Host, backendID), backendID)
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"created": stats.Created,
		"updated": stats.Updated,
		"pruned":  stats.Pruned,
		"kept":    stats.Kept,
	})
}
```

Add the import to `internal/server/admin_routes.go`'s import block (it already imports `backend`; add `libsync`):

```go
	"github.com/RXWatcher/silo-plugin-ebooks/internal/libsync"
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/server/ -run TestHandleAdminSyncLibraries`
Expected: PASS.

- [ ] **Step 5: Build + vet + full server tests**

Run: `go build ./... && go vet ./... && go test ./internal/server/`
Expected: build/vet clean; PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/server/admin_routes.go internal/server/admin_sync_test.go
git -c user.email=agent@anthropic.com -c user.name="Claude Code" commit -m "feat(server): POST /admin/libraries/sync endpoint

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Scheduled task `PortalLibrarySync`

**Files:**
- Modify: `internal/scheduler/tasks.go`
- Modify: `cmd/silo-plugin-ebooks/main.go`

- [ ] **Step 1: Add the task method**

Append to `internal/scheduler/tasks.go` (after `RequestReconciler`, same file, same `package scheduler`):

```go
// PortalLibrarySync mirrors the configured target backend's libraries into
// portal presentation shelves. No backend configured -> no-op. The
// libsync.Sync guard makes a briefly-unavailable backend a safe (logged)
// no-op rather than a destructive prune.
func (t *Tasks) PortalLibrarySync(ctx context.Context) error {
	cfg, err := t.Store.GetConfig(ctx)
	if err != nil {
		return fmt.Errorf("get config: %w", err)
	}
	if !cfg.HasBackend() {
		return nil
	}
	target := cfg.BackendTarget()
	if _, err := libsync.Sync(ctx, t.Store,
		backend.NewEbookBackend(t.Host, target), target); err != nil {
		t.Log.Warn("portal_library_sync", "err", err)
		return err
	}
	return nil
}
```

Ensure `internal/scheduler/tasks.go` imports `libsync` (it already imports `backend`, `fmt`, `store`):

```go
	"github.com/RXWatcher/silo-plugin-ebooks/internal/libsync"
```

- [ ] **Step 2: Register the task key in main.go**

In `cmd/silo-plugin-ebooks/main.go`, in the `scheduler.New(func() map[string]scheduler.TaskFn { ... })` returned map, add this entry alongside the others:

```go
			"portal_library_sync": t.PortalLibrarySync,
```

- [ ] **Step 3: Build + vet**

Run: `go build ./... && go vet ./...`
Expected: exit 0.

- [ ] **Step 4: Commit**

```bash
git add internal/scheduler/tasks.go cmd/silo-plugin-ebooks/main.go
git -c user.email=agent@anthropic.com -c user.name="Claude Code" commit -m "feat(scheduler): portal_library_sync task (configured backend, guarded)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: Manifest scheduled_task entry

**Files:**
- Modify: `cmd/silo-plugin-ebooks/manifest.json`

- [ ] **Step 1: Add the capability**

In `cmd/silo-plugin-ebooks/manifest.json`, in the `capabilities` array, add this object immediately after the existing `{"type":"scheduled_task.v1","id":"kindle_send_retrier", ...}` entry:

```json
    {"type": "scheduled_task.v1", "id": "portal_library_sync",
     "display_name": "Sync presentation libraries from backend",
     "cron": "0 * * * *"}
```

(Match the surrounding formatting/commas; `0 * * * *` = top of every hour.)

- [ ] **Step 2: Validate JSON + build**

Run: `python3 -c "import json;json.load(open('cmd/silo-plugin-ebooks/manifest.json'))" && echo JSON_OK && go build ./...`
Expected: `JSON_OK`, build exit 0.

- [ ] **Step 3: Commit**

```bash
git add cmd/silo-plugin-ebooks/manifest.json
git -c user.email=agent@anthropic.com -c user.name="Claude Code" commit -m "feat(manifest): hourly portal_library_sync scheduled task

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: Frontend API client

**Files:**
- Modify: `web/src/lib/api.ts`

- [ ] **Step 1: Add the API function**

In `web/src/lib/api.ts`, immediately after the existing `adminListBackendLibraries` export, add:

```ts
export const adminSyncLibraries = (backendPluginID: string) =>
  api.post<{ created: number; updated: number; pruned: number; kept: number }>(
    `/api/v1/admin/libraries/sync?backend_plugin_id=${encodeURIComponent(backendPluginID)}`,
  );
```

(`api.post<T>(path, body?)` already exists in this file's `api` object — same client used by other admin mutations; no body is needed for this endpoint.)

- [ ] **Step 2: Typecheck**

Run: `cd web && (pnpm run build 2>/dev/null || npm run build) ; cd ..`
Expected: `✓ built`, zero TS errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/lib/api.ts
git -c user.email=agent@anthropic.com -c user.name="Claude Code" commit -m "feat(web): adminSyncLibraries API client

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: Frontend "Sync from backend" button

**Files:**
- Modify: `web/src/pages/Admin.tsx`

- [ ] **Step 1: Import the API function**

In `web/src/pages/Admin.tsx`, add `adminSyncLibraries` to the existing `@/lib/api` import group (the same group that already imports `adminReplaceLibraries`, `adminListBackendLibraries`).

- [ ] **Step 2: Add sync state + mutation + UI in `LibrariesTab`**

In `web/src/pages/Admin.tsx`, inside `function LibrariesTab(...)`, directly after the existing `const save = useMutation({ ... })` block, add:

```tsx
  const [syncBackend, setSyncBackend] = useState<string>(
    backends[0] ? String(backends[0].id) : "",
  );
  const sync = useMutation({
    mutationFn: () => adminSyncLibraries(syncBackend),
    onSuccess: (r: { created: number; updated: number; pruned: number }) => {
      toast.success(
        `Synced: ${r.created} created, ${r.updated} updated, ${r.pruned} pruned`,
      );
      qc.invalidateQueries({ queryKey: ["admin", "libraries"] });
    },
    onError: (e: Error) => toast.error(e.message),
  });
```

Then, in the JSX header block that contains the `Add library` / `Save libraries` `<Button>`s (the `<div className="flex gap-2">` wrapping them), add — immediately before the `Add library` button — this select + button:

```tsx
              <select
                className="h-9 rounded-md border border-border bg-background px-2 text-sm"
                value={syncBackend}
                onChange={(e) => setSyncBackend(e.target.value)}
                aria-label="Backend to sync from"
              >
                {backends.map((b) => (
                  <option key={b.id} value={String(b.id)}>
                    {b.display_name} ({b.plugin_id})
                  </option>
                ))}
              </select>
              <Button
                type="button"
                variant="outline"
                onClick={() => sync.mutate()}
                disabled={sync.isPending || !syncBackend}
              >
                Sync from backend
              </Button>
```

(`useState` is already imported in this file; `backends` is a prop of `LibrariesTab` with elements shaped `{ id, plugin_id, display_name }` — `BackendOption` — already used by `LibraryEditorRow`. `qc`, `toast`, `useMutation`, `Button` are already in scope/imported in this file.)

- [ ] **Step 3: Typecheck + build**

Run: `cd web && (pnpm run build 2>/dev/null || npm run build) ; cd .. && go build ./...`
Expected: `✓ built`, zero TS errors; go build exit 0.

- [ ] **Step 4: Commit**

```bash
git add web/src/pages/Admin.tsx
git -c user.email=agent@anthropic.com -c user.name="Claude Code" commit -m "feat(web): Sync-from-backend button in Libraries tab

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 8: Final verification

- [ ] **Step 1: Backend**

Run: `go build ./... && go vet ./... && go test ./... 2>&1 | grep -E 'FAIL|panic' || echo NO_FAILURES`
Expected: build/vet clean; `NO_FAILURES` (store may SKIP without Postgres — acceptable).

- [ ] **Step 2: Frontend + manifest**

Run: `cd web && (pnpm run build 2>/dev/null || npm run build) ; cd .. && python3 -c "import json;json.load(open('cmd/silo-plugin-ebooks/manifest.json'))" && echo OK`
Expected: `✓ built`; `OK`.

- [ ] **Step 3: Commit any residue**

```bash
git status --porcelain
git add -A && git -c user.email=agent@anthropic.com -c user.name="Claude Code" commit -m "chore: portal library auto-provision — final verification" || true
```

---

## Self-Review (completed by plan author)

- **Spec coverage:** §3.1 pure Reconcile + scoping + defaulting → Task 1 (tests cover create/update-preserve/prune/passthrough/idempotent/empty-name). §3.2 orchestrator + guard → Tasks 1–2 (guard tests assert zero store write on error/empty). §4.1 endpoint → Task 3. §4.2 scheduled task + manifest + `BackendTarget()` + no-backend no-op → Tasks 4–5. §4.3 frontend → Tasks 6–7. §5 error handling → Tasks 1/3/4 (502 / scheduler logs+returns err / empty target no-op). §6 testing → Tasks 1–3. No uncovered requirement.
- **Placeholder scan:** none — every step has full code/commands/expected output. (Task 2's "fails then passes": Sync exists from Task 1, so these lock behavior; explicitly stated, not a placeholder.)
- **Type consistency:** `SyncStats{Created,Updated,Pruned,Kept}`, `Reconcile`, `Sync`, `LibStore`, `BackendLister` defined Task 1 and used identically in Tasks 2–4; `handleAdminSyncLibraries` consistent Task 3↔ self; task key `"portal_library_sync"` consistent Tasks 4↔5; `adminSyncLibraries` consistent Tasks 6↔7; `store.PortalLibrary`/`backend.LibraryInfo` field names match the verified shapes.
