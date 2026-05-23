# Readest Reader Port Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the current `epubjs` reader with a Readest-derived, Foliate-powered reader that opens Readest-supported formats and preserves Readest reader features while using Silo server state as the source of truth.

**Architecture:** Keep the existing Go plugin backend and current catalog/admin Vite app. Port the Readest reader runtime into the Vite frontend behind a `SiloReaderService` adapter, and expand backend persistence to store Readest-compatible reader config and booknotes. Do not import Tauri, Next, Readest cloud auth, or Readest library-management flows.

**Tech Stack:** Go, PostgreSQL migrations, Vite, React 19, Zustand, `foliate-js`, `@zip.js/zip.js`, `fflate`, `pdfjs-dist`, TypeScript.

---

## File Structure

Backend files:

- Create `internal/migrate/files/0014_readest_reader_state.up.sql`: Adds server-side reader config and expands annotation/booknote persistence.
- Create `internal/migrate/files/0014_readest_reader_state.down.sql`: Rolls back the reader-state migration.
- Modify `internal/store/user_data.go`: Preserve existing `last_cfi` fields and add reader config accessors.
- Modify `internal/store/annotation.go`: Add Readest booknote fields while keeping existing annotation API compatibility.
- Modify `internal/server/user_routes.go`: Expose reader config and booknote-compatible endpoints under existing authenticated user routes.
- Add tests in `internal/store/store_test.go` or focused new files beside the touched stores.
- Add tests in `internal/server/server_test.go` or a focused `internal/server/reader_state_test.go`.

Frontend files:

- Modify `web/package.json`: Remove `epubjs`, add Foliate/Readest runtime dependencies.
- Modify `web/vite.config.ts`: Add aliases needed by the port.
- Create `web/src/reader/silo/SiloReaderService.ts`: Adapter between Readest reader expectations and plugin APIs.
- Create `web/src/reader/silo/types.ts`: Silo-specific reader DTOs and conversion helpers.
- Create `web/src/reader/silo/navigation.ts`: Vite/react-router replacement for the small `next/navigation` surface the reader needs.
- Create `web/src/reader/readest/`: Ported Readest reader modules. Keep paths close to Readest where practical.
- Modify `web/src/pages/Reader.tsx`: Replace the `epubjs` reader with the Readest-derived reader entry.
- Modify `web/src/lib/api.ts`: Add typed reader config/booknote API calls.
- Add focused frontend tests under `web/src/reader/silo/*.test.ts`.

Reference source files:

- `/opt/readest/apps/readest-app/src/app/reader`
- `/opt/readest/apps/readest-app/src/store/readerStore.ts`
- `/opt/readest/apps/readest-app/src/store/bookDataStore.ts`
- `/opt/readest/apps/readest-app/src/libs/document.ts`
- `/opt/readest/apps/readest-app/src/types/book.ts`
- `/opt/readest/apps/readest-app/src/types/view.ts`
- `/opt/readest/packages/foliate-js`

## Constraints

- Server progress/config wins when a book opens.
- The reader must not write page-one or transient preview progress over server progress during initialization.
- Existing API clients that use `last_cfi`, `read_progress`, and simple annotations must continue to work.
- Do not port Tauri APIs. Native-only operations become web no-ops or Silo API calls.
- Do not port Readest cloud auth or full library/import management.
- Keep the port recognizably close to Readest so future upstream comparison remains possible.

---

### Task 1: Backend Reader-State Migration

**Files:**
- Create: `internal/migrate/files/0014_readest_reader_state.up.sql`
- Create: `internal/migrate/files/0014_readest_reader_state.down.sql`

- [ ] **Step 1: Add migration for config and booknote-compatible fields**

Create `internal/migrate/files/0014_readest_reader_state.up.sql`:

```sql
CREATE TABLE reader_config (
  user_id     TEXT NOT NULL,
  book_id     TEXT NOT NULL,
  config_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, book_id)
);

ALTER TABLE annotation
  ADD COLUMN IF NOT EXISTS readest_type TEXT NOT NULL DEFAULT 'annotation',
  ADD COLUMN IF NOT EXISTS xpointer0 TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS xpointer1 TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS page INT,
  ADD COLUMN IF NOT EXISTS style TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE annotation
  DROP CONSTRAINT IF EXISTS annotation_kind_check;

ALTER TABLE annotation
  ADD CONSTRAINT annotation_kind_check
  CHECK (kind IN ('highlight','note','bookmark','excerpt','annotation'));

CREATE INDEX IF NOT EXISTS reader_config_user_updated_idx
  ON reader_config (user_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS annotation_user_book_active_idx
  ON annotation (user_id, book_id, updated_at DESC)
  WHERE deleted_at IS NULL;
```

- [ ] **Step 2: Add rollback migration**

Create `internal/migrate/files/0014_readest_reader_state.down.sql`:

```sql
DROP INDEX IF EXISTS annotation_user_book_active_idx;
DROP INDEX IF EXISTS reader_config_user_updated_idx;

ALTER TABLE annotation
  DROP CONSTRAINT IF EXISTS annotation_kind_check;

ALTER TABLE annotation
  ADD CONSTRAINT annotation_kind_check
  CHECK (kind IN ('highlight','note'));

ALTER TABLE annotation
  DROP COLUMN IF EXISTS metadata_json,
  DROP COLUMN IF EXISTS deleted_at,
  DROP COLUMN IF EXISTS style,
  DROP COLUMN IF EXISTS page,
  DROP COLUMN IF EXISTS xpointer1,
  DROP COLUMN IF EXISTS xpointer0,
  DROP COLUMN IF EXISTS readest_type;

DROP TABLE IF EXISTS reader_config;
```

- [ ] **Step 3: Run backend tests**

Run:

```bash
go test ./...
```

Expected: existing tests pass. If migration ordering tests fail, update migration expectations to include version `0014`.

---

### Task 2: Backend Store API for Reader Config

**Files:**
- Modify: `internal/store/user_data.go`
- Test: `internal/store/store_test.go` or `internal/store/reader_config_test.go`

- [ ] **Step 1: Add store types and methods**

Add this type and methods to `internal/store/user_data.go`:

```go
type ReaderConfig struct {
	UserID     string
	BookID     string
	ConfigJSON []byte
	UpdatedAt  time.Time
}

func (s *Store) UpsertReaderConfig(ctx context.Context, c ReaderConfig) error {
	if c.UserID == "" || c.BookID == "" {
		return fmt.Errorf("user_id and book_id required")
	}
	if len(c.ConfigJSON) == 0 {
		c.ConfigJSON = []byte(`{}`)
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO reader_config (user_id, book_id, config_json, updated_at)
		VALUES ($1, $2, $3::jsonb, now())
		ON CONFLICT (user_id, book_id) DO UPDATE SET
			config_json = EXCLUDED.config_json,
			updated_at = now()
	`, c.UserID, c.BookID, string(c.ConfigJSON))
	if err != nil {
		return fmt.Errorf("upsert reader_config: %w", err)
	}
	return nil
}

func (s *Store) GetReaderConfig(ctx context.Context, userID, bookID string) (ReaderConfig, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT user_id, book_id, config_json, updated_at
		FROM reader_config WHERE user_id = $1 AND book_id = $2
	`, userID, bookID)
	var c ReaderConfig
	if err := row.Scan(&c.UserID, &c.BookID, &c.ConfigJSON, &c.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ReaderConfig{}, ErrNotFound
		}
		return ReaderConfig{}, fmt.Errorf("get reader_config: %w", err)
	}
	return c, nil
}
```

- [ ] **Step 2: Add store tests**

Create `internal/store/reader_config_test.go` with a test matching existing store test setup:

```go
func TestReaderConfigRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	err := s.UpsertReaderConfig(ctx, store.ReaderConfig{
		UserID:     "user-1",
		BookID:     "book-1",
		ConfigJSON: []byte(`{"location":"epubcfi(/6/2)","progress":[3,10]}`),
	})
	require.NoError(t, err)

	got, err := s.GetReaderConfig(ctx, "user-1", "book-1")
	require.NoError(t, err)
	require.JSONEq(t, `{"location":"epubcfi(/6/2)","progress":[3,10]}`, string(got.ConfigJSON))

	err = s.UpsertReaderConfig(ctx, store.ReaderConfig{
		UserID:     "user-1",
		BookID:     "book-1",
		ConfigJSON: []byte(`{"location":"epubcfi(/6/4)","progress":[4,10]}`),
	})
	require.NoError(t, err)

	got, err = s.GetReaderConfig(ctx, "user-1", "book-1")
	require.NoError(t, err)
	require.JSONEq(t, `{"location":"epubcfi(/6/4)","progress":[4,10]}`, string(got.ConfigJSON))
}
```

Adjust imports to match the existing test helper names in the store package.

- [ ] **Step 3: Run focused tests**

Run:

```bash
go test ./internal/store -run TestReaderConfigRoundTrip -count=1
```

Expected: PASS.

---

### Task 3: Backend Readest-Compatible Booknotes

**Files:**
- Modify: `internal/store/annotation.go`
- Modify: `internal/server/user_routes.go`
- Test: `internal/server/reader_state_test.go`

- [ ] **Step 1: Extend annotation store model**

Extend `store.Annotation` in `internal/store/annotation.go`:

```go
type Annotation struct {
	ID           string
	UserID       string
	BookID       string
	CFIRange     string
	Kind         string
	Color        string
	SelectedText string
	NoteText     string
	ReadestType  string
	XPointer0    string
	XPointer1    string
	Page         *int
	Style        string
	MetadataJSON []byte
	DeletedAt    *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
```

Update insert/select SQL to read and write the new columns. Keep `kind`, `cfi_range`, `selected_text`, and `note_text` populated so current clients still work.

- [ ] **Step 2: Add reader config routes**

Add authenticated routes in `internal/server/user_routes.go`:

```text
GET  /api/v1/me/books/{bookID}/reader-config
PUT  /api/v1/me/books/{bookID}/reader-config
```

Response body:

```json
{
  "book_id": "book-1",
  "config": {
    "location": "epubcfi(/6/2)",
    "progress": [3, 10],
    "booknotes": []
  },
  "updated_at": "2026-05-19T10:00:00Z"
}
```

Request body:

```json
{
  "config": {
    "location": "epubcfi(/6/2)",
    "progress": [3, 10],
    "booknotes": []
  }
}
```

On `PUT`, also update `user_data.last_cfi`, `user_data.read_progress`, `user_data.current_page`, and `user_data.last_read_at` when those values are present in the config.

- [ ] **Step 3: Add route tests**

Create a test that:

1. Requests missing config and receives `{ "config": {} }`.
2. Saves config with `location` and `progress`.
3. Reads config back.
4. Verifies `GET /api/v1/me/books/{bookID}` still exposes `last_cfi` through existing user-data behavior.

Run:

```bash
go test ./internal/server -run ReaderConfig -count=1
```

Expected: PASS.

---

### Task 4: Frontend Dependencies and Vite Aliases

**Files:**
- Modify: `web/package.json`
- Modify: `web/vite.config.ts`
- Modify: `web/tsconfig.app.json` or `web/tsconfig.json`

- [ ] **Step 1: Update dependencies**

In `web/package.json`, remove:

```json
"epubjs": "^0.3.93"
```

Add:

```json
"@zip.js/zip.js": "^2.8.16",
"fflate": "^0.8.2",
"foliate-js": "file:../../../readest/packages/foliate-js",
"pdfjs-dist": "^5.4.530",
"zustand": "^5.0.10"
```

- [ ] **Step 2: Install frontend dependencies**

Run:

```bash
npm install
```

Expected: `web/package-lock.json` updates and `npm ls foliate-js` shows the local package.

- [ ] **Step 3: Add aliases**

Ensure `web/vite.config.ts` includes aliases:

```ts
resolve: {
  alias: {
    "@": path.resolve(__dirname, "./src"),
    "@readest": path.resolve(__dirname, "./src/reader/readest"),
    "@pdfjs": path.resolve(__dirname, "./public/vendor/pdfjs"),
  },
},
```

Import `path` from `node:path` and `fileURLToPath` from `node:url` if the file does not already define `__dirname`.

- [ ] **Step 4: Run typecheck**

Run:

```bash
npm run build
```

Expected: build fails only because `Reader.tsx` still imports `epubjs`, or passes if Task 5 has already replaced the import.

---

### Task 5: Silo Reader Service Adapter

**Files:**
- Create: `web/src/reader/silo/types.ts`
- Create: `web/src/reader/silo/SiloReaderService.ts`
- Modify: `web/src/lib/api.ts`
- Test: `web/src/reader/silo/SiloReaderService.test.ts`

- [ ] **Step 1: Add DTO types**

Create `web/src/reader/silo/types.ts`:

```ts
export type ReaderConfigEnvelope = {
  book_id: string;
  config: Record<string, unknown>;
  updated_at?: string;
};

export type ReadestBookNote = {
  id: string;
  type: "bookmark" | "annotation" | "excerpt";
  cfi: string;
  xpointer0?: string;
  xpointer1?: string;
  page?: number;
  text?: string;
  style?: "highlight" | "underline" | "squiggly";
  color?: string;
  note: string;
  createdAt: number;
  updatedAt: number;
  deletedAt?: number | null;
};

export type SiloReaderBook = {
  id: string;
  hash: string;
  format: string;
  title: string;
  author: string;
  files: Array<{ format: string; mime_type: string; size_bytes: number }>;
  primaryLanguage?: string;
};
```

- [ ] **Step 2: Add API calls**

Add to `web/src/lib/api.ts`:

```ts
export const getReaderConfig = (bookID: string) =>
  api.get<ReaderConfigEnvelope>(
    `/api/v1/me/books/${encodeURIComponent(bookID)}/reader-config`,
  );

export const putReaderConfig = (
  bookID: string,
  config: Record<string, unknown>,
) =>
  api.put<ReaderConfigEnvelope>(
    `/api/v1/me/books/${encodeURIComponent(bookID)}/reader-config`,
    { config },
  );
```

- [ ] **Step 3: Add service adapter**

Create `web/src/reader/silo/SiloReaderService.ts`:

```ts
import {
  getBook,
  getReaderConfig,
  mountPath,
  putReaderConfig,
  type EbookDetail,
} from "@/lib/api";

export class SiloReaderService {
  async loadBook(bookID: string): Promise<EbookDetail> {
    return getBook(bookID);
  }

  async loadBookContent(bookID: string, format: string): Promise<File> {
    const response = await fetch(
      `${mountPath()}/api/v1/me/books/${encodeURIComponent(bookID)}/file?format=${encodeURIComponent(format)}`,
      { credentials: "include" },
    );
    if (!response.ok) {
      throw new Error(`Unable to load book file: ${response.status}`);
    }
    const blob = await response.blob();
    return new File([blob], `${bookID}.${format.toLowerCase()}`, {
      type: blob.type || "application/octet-stream",
    });
  }

  async loadBookConfig(bookID: string): Promise<Record<string, unknown>> {
    const envelope = await getReaderConfig(bookID);
    return envelope.config ?? {};
  }

  async saveBookConfig(
    bookID: string,
    config: Record<string, unknown>,
  ): Promise<void> {
    await putReaderConfig(bookID, config);
  }
}
```

- [ ] **Step 4: Add adapter tests**

Mock `fetch` and API calls to verify:

1. `loadBookContent()` returns a `File`.
2. `loadBookConfig()` returns `{}` for missing config envelope.
3. `saveBookConfig()` calls `PUT`.

Run:

```bash
npm run build
```

Expected: TypeScript compiles through the new adapter.

---

### Task 6: Port Foliate Document Loading

**Files:**
- Create: `web/src/reader/readest/libs/document.ts`
- Create: `web/src/reader/readest/types/book.ts`
- Create: `web/src/reader/readest/types/view.ts`
- Copy: required utility files from `/opt/readest/apps/readest-app/src/utils`

- [ ] **Step 1: Copy Readest document and core types**

Copy these files preserving relative import intent:

```bash
mkdir -p web/src/reader/readest/libs web/src/reader/readest/types
cp /opt/readest/apps/readest-app/src/libs/document.ts web/src/reader/readest/libs/document.ts
cp /opt/readest/apps/readest-app/src/types/book.ts web/src/reader/readest/types/book.ts
cp /opt/readest/apps/readest-app/src/types/view.ts web/src/reader/readest/types/view.ts
```

- [ ] **Step 2: Rewrite imports**

In copied files, rewrite imports from `@/libs/document`, `@/types/book`, and `@/types/view` to `@/reader/readest/...`.

Example:

```ts
import { BookMetadata } from "@/reader/readest/libs/document";
```

- [ ] **Step 3: Add minimal missing utilities**

When `document.ts` references Readest utility helpers, copy only the required helper files and preserve their tests if they exist. Do not copy Tauri or cloud service files for this task.

- [ ] **Step 4: Verify format loader compiles**

Run:

```bash
npm run build
```

Expected: any failures are missing local utility imports, not `foliate-js` module resolution.

---

### Task 7: Minimal Foliate Reader Route

**Files:**
- Replace: `web/src/pages/Reader.tsx`
- Create: `web/src/reader/ReadestLiteReader.tsx`

- [ ] **Step 1: Replace `epubjs` with a minimal Foliate shell**

Create `web/src/reader/ReadestLiteReader.tsx`:

```tsx
import { useEffect, useRef, useState } from "react";
import { DocumentLoader, type BookDoc } from "@/reader/readest/libs/document";
import { SiloReaderService } from "@/reader/silo/SiloReaderService";

type Props = {
  bookID: string;
  format: string;
};

export function ReadestLiteReader({ bookID, format }: Props) {
  const containerRef = useRef<HTMLDivElement>(null);
  const viewRef = useRef<any>(null);
  const [error, setError] = useState<string>("");

  useEffect(() => {
    let cancelled = false;
    const service = new SiloReaderService();

    async function open() {
      try {
        const [file, config] = await Promise.all([
          service.loadBookContent(bookID, format),
          service.loadBookConfig(bookID),
        ]);
        const { book } = await new DocumentLoader(file).open();
        if (cancelled) return;

        await import("foliate-js/view.js");
        const view = document.createElement("foliate-view") as any;
        viewRef.current = view;
        containerRef.current?.replaceChildren(view);
        await view.open(book as BookDoc);

        const lastLocation =
          typeof config.location === "string" && config.location.length > 0
            ? config.location
            : undefined;
        if (lastLocation) {
          await view.init({ lastLocation });
        } else {
          await view.goToFraction(0);
        }

        view.addEventListener("relocate", (event: Event) => {
          const detail = (event as CustomEvent).detail;
          const location = detail?.cfi;
          const current = detail?.location?.current ?? 0;
          const total = detail?.location?.total ?? 0;
          if (!location || total <= 0) return;
          void service.saveBookConfig(bookID, {
            ...config,
            location,
            progress: [current + 1, total],
            updatedAt: Date.now(),
          });
        });
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "Unable to open book");
        }
      }
    }

    void open();
    return () => {
      cancelled = true;
      viewRef.current?.close?.();
      viewRef.current?.remove?.();
      viewRef.current = null;
    };
  }, [bookID, format]);

  if (error) {
    return <div className="p-6 text-sm text-destructive">{error}</div>;
  }

  return <div ref={containerRef} className="h-full w-full overflow-hidden" />;
}
```

- [ ] **Step 2: Wire existing route**

Replace the `epubjs` rendering branch in `web/src/pages/Reader.tsx` with `ReadestLiteReader`, keeping existing metadata header and format selector.

- [ ] **Step 3: Verify server-progress-first behavior**

Open a book with an existing `reader_config.config.location`. Confirm the first rendered location is that CFI and no save occurs until Foliate emits a user relocation after initialization. If Foliate emits during initialization, add an `initializedRef` guard that ignores the first relocate event.

Run:

```bash
npm run build
```

Expected: PASS.

---

### Task 8: Port Readest Stores and Full Reader Components

**Files:**
- Copy into `web/src/reader/readest/app/reader`: `/opt/readest/apps/readest-app/src/app/reader`
- Copy into `web/src/reader/readest/store`: required store files
- Copy into `web/src/reader/readest/services`: required reader-only service files
- Copy into `web/src/reader/readest/hooks`: required reader-only shared hooks
- Copy into `web/src/reader/readest/utils`: required reader-only utilities

- [ ] **Step 1: Copy reader subsystem**

Run:

```bash
mkdir -p web/src/reader/readest/app web/src/reader/readest/store web/src/reader/readest/services web/src/reader/readest/hooks web/src/reader/readest/utils
cp -R /opt/readest/apps/readest-app/src/app/reader web/src/reader/readest/app/
cp /opt/readest/apps/readest-app/src/store/readerStore.ts web/src/reader/readest/store/
cp /opt/readest/apps/readest-app/src/store/bookDataStore.ts web/src/reader/readest/store/
cp /opt/readest/apps/readest-app/src/store/settingsStore.ts web/src/reader/readest/store/
cp /opt/readest/apps/readest-app/src/store/sidebarStore.ts web/src/reader/readest/store/
cp /opt/readest/apps/readest-app/src/store/notebookStore.ts web/src/reader/readest/store/
cp /opt/readest/apps/readest-app/src/store/themeStore.ts web/src/reader/readest/store/
cp /opt/readest/apps/readest-app/src/store/customFontStore.ts web/src/reader/readest/store/
cp /opt/readest/apps/readest-app/src/store/parallelViewStore.ts web/src/reader/readest/store/
```

- [ ] **Step 2: Remove native-only entry points**

Replace imports from `@tauri-apps/*` with `web/src/reader/readest/shims/tauri.ts` exports that return web no-ops. For example:

```ts
export const getCurrentWindow = () => ({
  label: "silo-reader",
  close: async () => undefined,
});
```

- [ ] **Step 3: Replace Readest AppService usage**

Create `web/src/reader/readest/context/EnvContext.tsx` that provides the minimal `envConfig.getAppService()` and `appService` shape backed by `SiloReaderService`.

Set booleans:

```ts
isMobile: false,
isMobileApp: false,
isAndroidApp: false,
isIOSApp: false,
isDesktopApp: false,
hasWindow: false,
hasSafeAreaInset: false,
hasScreenBrightness: false,
hasOrientationLock: false,
supportsCanvasContext2DFilter: true,
```

- [ ] **Step 4: Compile and trim missing imports**

Run:

```bash
npm run build
```

For each missing import, either copy the exact Readest reader dependency or replace it with a local shim. Reject imports from Readest cloud auth, Tauri native shell, updater, payment, app store, or full library import features.

---

### Task 9: Progress, Config, and No-Overwrite Guard

**Files:**
- Modify: `web/src/reader/readest/store/bookDataStore.ts`
- Modify: `web/src/reader/readest/store/readerStore.ts`
- Modify: `web/src/reader/readest/app/reader/hooks/useProgressAutoSave.ts`
- Modify: `web/src/reader/silo/SiloReaderService.ts`

- [ ] **Step 1: Load config from server before opening view**

Ensure the Readest init flow calls `SiloReaderService.loadBookConfig(bookID)` before `view.open(bookDoc)` and passes `config.location` to `view.init({ lastLocation })`.

- [ ] **Step 2: Guard initialization relocates**

Add a per-book `hasUserRelocated` or `previewMode` guard so initialization cannot save progress. Existing Readest `previewMode` behavior may be reused.

- [ ] **Step 3: Persist Readest config**

Save the whole Readest-compatible config object through `PUT /reader-config`, including:

```json
{
  "progress": [4, 100],
  "location": "epubcfi(...)",
  "xpointer": "...",
  "booknotes": [],
  "viewSettings": {},
  "updatedAt": 1779177600000
}
```

- [ ] **Step 4: Verify with a manual resume test**

1. Open a book.
2. Navigate to a non-zero location.
3. Reload the browser.
4. Confirm it resumes from the server-saved CFI.
5. Confirm the server does not save page one during reload.

---

### Task 10: Full Annotation, Bookmark, Notebook, Search, and Settings Port

**Files:**
- Modify copied files under `web/src/reader/readest/app/reader/components/annotator`
- Modify copied files under `web/src/reader/readest/app/reader/components/sidebar`
- Modify copied files under `web/src/reader/readest/app/reader/components/notebook`
- Modify copied files under `web/src/reader/readest/app/reader/components/footerbar`
- Modify copied files under `web/src/reader/readest/app/reader/components/tts`

- [ ] **Step 1: Keep Readest `BookNote` model**

Persist annotations, bookmarks, and excerpts in `config.booknotes` and mirror them to the expanded annotation API for list/export compatibility.

- [ ] **Step 2: Enable Foliate overlay drawing**

Keep Readest handlers for:

```ts
onCreateOverlay
onDrawAnnotation
onShowAnnotation
```

Verify `view.addAnnotation(annotation)` is called for active notes after section load.

- [ ] **Step 3: Enable notebook**

Keep Readest notebook tabs for notes and excerpts. Disable AI assistant integrations unless a Silo AI service is explicitly wired.

- [ ] **Step 4: Enable sidebar**

Keep TOC, annotations, bookmarks, and search. Search must call Foliate `view.search(config)` and navigate through `view.goTo(cfi)`.

- [ ] **Step 5: Enable settings**

Persist view settings inside `reader_config.config.viewSettings`. Do not store reader settings only in local browser storage.

- [ ] **Step 6: Run frontend build**

Run:

```bash
npm run build
```

Expected: PASS.

---

### Task 11: Remove Old Reader and Validate Formats

**Files:**
- Modify: `web/package.json`
- Modify: `web/src/pages/Reader.tsx`
- Modify: docs if reader behavior is documented

- [ ] **Step 1: Remove `epubjs` dependency and imports**

Confirm:

```bash
rg "epubjs|ePub\\(" web/src web/package.json
```

Expected: no matches.

- [ ] **Step 2: Validate supported formats**

Use books in these formats:

```text
EPUB
PDF
MOBI
AZW3
CBZ
FB2
FBZ
```

For each format:

1. Open from plugin reader.
2. Confirm content renders.
3. Navigate forward and backward.
4. Save progress.
5. Reload and confirm server resume.

- [ ] **Step 3: Run complete verification**

Run:

```bash
go test ./...
cd web && npm run build
```

Expected: both pass.

---

## Self-Review

- Spec coverage: The plan covers the stable architecture, no Tauri, no `epubjs`, Readest-derived reader behavior, Foliate format compatibility, server-authoritative progress, backend persistence expansion, and current-client compatibility.
- Placeholder scan: The plan contains concrete paths, route shapes, migration SQL, adapter code, and verification commands.
- Type consistency: Backend uses `ReaderConfig`; frontend uses `ReaderConfigEnvelope`, `SiloReaderService`, and Readest-compatible `BookNote` naming.

