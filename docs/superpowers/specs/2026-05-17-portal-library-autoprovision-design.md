# Portal Library Auto-Provision (Full Sync) — Design

Date: 2026-05-17
Status: Approved (design); pending spec review before implementation planning.
Plugin: `continuum-plugin-ebooks` (the portal)

## 1. Problem & Goal

A backend (e.g. `continuum-plugin-local-ebooks`) can expose many libraries,
each with its own media type. The portal does **not** present them
automatically — an admin must hand-create one `portal_library`
("presentation shelf") per backend library and map each to the right
`backend_library_id`. Goal: a one-action **full sync** that mirrors a
backend's libraries into portal presentation shelves (create / update /
prune), available both as a manual admin button and an hourly scheduled
task, **without ever destroying operator-managed or other-backend config,
even when the backend is briefly unavailable.**

## 2. Decisions (from brainstorming)

- **Full sync**: create missing, update changed, prune backend-derived
  shelves whose backend library no longer exists.
- **Triggers**: manual admin button **and** an hourly scheduled task.
- **Safety**: the destructive logic is one pure, exhaustively-tested
  function gated behind one explicit guard.

## 3. Architecture (Approach A)

Pure reconcile function → orchestrator (backend + store) → reused by an HTTP
handler and a scheduled task. Persistence reuses the existing audited
transactional `store.ReplacePortalLibraries` (id-preserving update; `id==0`
insert; omitted rows pruned).

### 3.1 Pure function

`reconcilePortalLibraries(existing []store.PortalLibrary, backendLibs []backend.LibraryInfo, backendID string) ([]store.PortalLibrary, SyncStats)`

`SyncStats{ Created, Updated, Pruned, Kept int }`. No I/O; deterministic;
fully unit-testable. It is only ever called after the orchestrator has
confirmed a successful, non-empty backend fetch.

**Scoping boundary (the safety property):** a row is *sync-managed* iff
`BackendPluginID == backendID && BackendLibraryID != nil`. All other rows —
operator-created shelves (`BackendLibraryID == nil`) and shelves for a
different backend — are passed through unchanged and counted `Kept`. Sync
can never disturb manual or other-backend configuration.

**Rules over sync-managed rows, matched by `*BackendLibraryID == backendLib.ID`:**

Field defaulting (applied to both Create and Update, because
`store.ReplacePortalLibraries` rejects the **entire batch** if any row has
an empty `Name` or `BackendPluginID`): `Name` = `backendLib.Name`, or
`fmt.Sprintf("Library %d", backendLib.ID)` when the backend sends an empty
name; `MediaType` = `backendLib.MediaType`, or `"book"` when empty;
`BackendPluginID` = `backendID` (already guaranteed non-empty by the guard).

- match → **Update**: set `Name`, `MediaType` (with the defaulting above)
  from the backend library; **preserve** that row's `ID`, `Enabled`,
  `SortOrder` (the operator owns visibility/order; scheduled ticks must not
  fight operator edits).
- no portal row for a backend library → **Create**:
  `{ID:0, Name, MediaType, BackendPluginID:backendID,
  BackendLibraryID:&backendLib.ID, Enabled:true, SortOrder: maxSortOrder+1}`
  (Name/MediaType per the defaulting above).
- sync-managed row whose `*BackendLibraryID` is absent from the fetched
  backend list → **Prune** (omitted from the result slice).

Result = passthrough(non-managed) ++ updated ++ created. Idempotent: an
unchanged backend yields `Created==Updated==Pruned==0`.

### 3.2 Orchestrator

`(s *Server) syncBackendLibraries(ctx, backendID string) (SyncStats, error)`

1. `backendID == ""` → error (no work).
2. `libs, err := backend.NewEbookBackend(s.deps.Host, backendID).ListLibraries(ctx)`.
3. **GUARD:** `if err != nil` → return error, **zero DB writes**.
   `if len(libs) == 0` → return a "refusing to sync: backend returned no
   libraries" error, **zero DB writes**. This single guard makes a
   briefly-unavailable/empty backend a safe no-op and is the only thing
   standing between a backend hiccup and a mass prune.
4. `existing, _ := s.deps.Store.ListPortalLibraries(ctx, false)` (all rows,
   not enabled-only).
5. `desired, stats := reconcilePortalLibraries(existing, libs, backendID)`.
6. `s.deps.Store.ReplacePortalLibraries(ctx, desired)` (existing audited
   transactional set-replace: kept/updated keep their id → UPDATE; `id==0`
   → INSERT; omitted → DELETE = prune).
7. return `stats`.

## 4. Triggers / Surface

### 4.1 HTTP endpoint

`POST /admin/libraries/sync?backend_plugin_id=<id>` mounted in
`internal/server/admin_routes.go` next to the existing library routes
(admin-gated by the manifest `admin` route). Responses:
`200 {created,updated,pruned,kept}`; `400` if `backend_plugin_id` empty;
`502` if the fetch fails or returns zero (guard tripped — clear message,
never a silent prune).

### 4.2 Scheduled task

New `scheduled_task.v1` `portal_library_sync` (hourly cron) in
`cmd/continuum-plugin-ebooks/manifest.json`, dispatched in `main.go`'s
scheduler map alongside `request_reconciler`/`cache_evictor`. Syncs the
configured target backend via `cfg.BackendTarget()` (the resolver added
earlier this session). No backend configured → no-op. Same orchestrator →
same guard, so an unavailable backend at tick time is a safe, logged no-op.

### 4.3 Frontend

`adminSyncLibraries(backendPluginID)` in `web/src/lib/api.ts`; a "Sync from
backend" button in `Admin.tsx` `LibrariesTab`. The tab already lists
`backends`; the button uses the backend the operator is working with
(single → that one; multiple → small picker defaulting to the configured
target). On success: toast `"Synced: N created, M updated, P pruned"` and
invalidate the `["admin","libraries"]` query.

## 5. Error Handling

- Fetch error / zero libraries → abort, zero writes, return error. HTTP:
  502. Scheduled: log + return the error so the host records a *failed*
  run (never a clean "did nothing" success that hides the guard tripping).
- `ReplacePortalLibraries` validation errors (name/backend required) →
  surfaced to the caller.
- Empty `cfg.BackendTarget()` in the scheduled path → skip silently.

## 6. Testing

- Pure `reconcilePortalLibraries` — table tests: create-missing; update
  (name/media_type changed, id/enabled/sort_order preserved); prune-gone;
  passthrough (other-backend row + nil-BackendLibraryID row); empty
  existing; idempotent re-run (all-zero stats).
- Orchestrator guard (fake backend + fake store): backend error → **no
  store write**; zero libs → **no store write**.
- Endpoint: 200 + stats; empty `backend_plugin_id` → 400; backend down →
  502.
- Scheduled task registered + dispatches; no-backend-configured → no-op.

## 7. Isolation

pure `reconcilePortalLibraries` (no deps) ↔ `syncBackendLibraries`
(backend + store) ↔ HTTP handler ↔ scheduled task. The one destructive
decision lives in one pure tested function behind one explicit guard.

## 8. Out of scope

- The separate, known minor inconsistency that a *disabled* local-ebooks
  library's books still appear in an unfiltered backend catalog pull
  (tracked separately; not part of this feature).
- Per-shelf selective sync / partial mapping UI (YAGNI).
