package streaming

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/RXWatcher/continuum-plugin-ebooks/internal/store"
)

// Manager implements cache-mode streaming. It owns:
//   - the filesystem layout under <dir>/<sha[:2]>/<sha>
//   - an in-memory single-flight map keyed by cache_key
//   - DB transitions on ebook_file_cache (pending → ready/failed)
//   - an in-process reference count of cache entries currently being read,
//     used by EvictTo to skip in-flight serves and avoid the
//     read/delete-mid-io.Copy race that produced mid-transfer 410/EOF errors.
//
// Lookup hits return immediately. A miss promotes the calling goroutine to
// leader for that key: the leader streams the upstream body to a temp file,
// renames it into place, and marks the row "ready". Followers block on the
// leader's done channel and then serve from the same on-disk path.
type Manager struct {
	dir      string
	maxBytes int64
	store    *store.Store
	inflight sync.Map // cache_key -> *download

	refMu sync.Mutex
	refs  map[string]int // entry.ID → active reader count
}

// Fetcher returns the upstream body. content-length/mime-type are advisory
// (they are stored in DB so admin UIs can show them).
type Fetcher func(ctx context.Context) (body io.ReadCloser, headers http.Header, contentLength int64, mimeType string, err error)

type download struct {
	done  chan struct{}
	err   error
	entry store.CacheEntry
}

// NewManager constructs a Manager rooted at dir with the given LRU max-size.
// maxBytes ≤ 0 means "evictor turned off" (used in tests).
func NewManager(dir string, maxBytes int64, st *store.Store) *Manager {
	return &Manager{
		dir:      dir,
		maxBytes: maxBytes,
		store:    st,
		refs:     make(map[string]int),
	}
}

// Acquire increments the active-reader count for entry id and returns a
// release closure. Callers MUST defer the release exactly once. The returned
// closure is idempotent across repeated calls — only the first call decrements.
// EvictTo skips entries whose count is > 0.
func (m *Manager) Acquire(id string) (release func()) {
	m.refMu.Lock()
	m.refs[id]++
	m.refMu.Unlock()
	var released bool
	return func() {
		m.refMu.Lock()
		defer m.refMu.Unlock()
		if released {
			return
		}
		released = true
		m.refs[id]--
		if m.refs[id] <= 0 {
			delete(m.refs, id)
		}
	}
}

// inUse reports whether id currently has any active readers. Caller must
// hold m.refMu.
func (m *Manager) inUse(id string) bool {
	return m.refs[id] > 0
}

// Dir returns the on-disk root.
func (m *Manager) Dir() string { return m.dir }

// PathFor returns the full on-disk path for an entry.
func (m *Manager) PathFor(e store.CacheEntry) string {
	return filepath.Join(m.dir, e.RelativePath)
}

// Lookup returns the entry for cacheKey if it's ready in the DB.
func (m *Manager) Lookup(ctx context.Context, cacheKey string) (store.CacheEntry, bool) {
	e, err := m.store.GetCacheByCacheKey(ctx, cacheKey)
	if err != nil || e.Status != "ready" {
		return store.CacheEntry{}, false
	}
	return e, true
}

// Touch updates last_accessed_at to support LRU eviction.
func (m *Manager) Touch(ctx context.Context, id string) error {
	return m.store.TouchCache(ctx, id)
}

// StartOrJoin is the single-flight entrypoint: if no download is active for
// cacheKey, the caller becomes leader and fetches; otherwise the caller
// blocks until the leader finishes and returns the leader's entry/err.
//
// On success the on-disk file exists at PathFor(entry) and the row is
// marked status='ready' with bytes_on_disk populated.
func (m *Manager) StartOrJoin(ctx context.Context, cacheKey, bookID, format string, fetch Fetcher) (store.CacheEntry, error) {
	// Fast path: already ready.
	if e, ok := m.Lookup(ctx, cacheKey); ok {
		return e, nil
	}
	// Slow path: become leader or join existing.
	d := &download{done: make(chan struct{})}
	actual, loaded := m.inflight.LoadOrStore(cacheKey, d)
	if loaded {
		// Follower: wait for leader.
		existing := actual.(*download)
		select {
		case <-ctx.Done():
			return store.CacheEntry{}, ctx.Err()
		case <-existing.done:
			return existing.entry, existing.err
		}
	}
	// Leader. Decouple the shared fill from this one request's context: if the
	// client that happened to win leadership disconnects mid-download, the
	// blocked followers (still connected) must not all get a canceled-copy
	// error. The fill keeps its own deadline so a hung upstream still fails.
	defer m.inflight.Delete(cacheKey)
	defer close(d.done)
	leaderCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Minute)
	defer cancel()
	d.entry, d.err = m.fetchAsLeader(leaderCtx, cacheKey, bookID, format, fetch)
	return d.entry, d.err
}

// fetchAsLeader does the actual download. Errors are recorded in DB so the
// next caller can decide whether to retry.
func (m *Manager) fetchAsLeader(ctx context.Context, cacheKey, bookID, format string, fetch Fetcher) (store.CacheEntry, error) {
	body, _, contentLength, mimeType, err := fetch(ctx)
	if err != nil {
		return store.CacheEntry{}, fmt.Errorf("fetch: %w", err)
	}
	if body == nil {
		return store.CacheEntry{}, errors.New("nil body from upstream")
	}
	defer body.Close()

	rel := filepath.Join(cacheKey[:2], cacheKey)
	full := filepath.Join(m.dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return store.CacheEntry{}, fmt.Errorf("mkdir: %w", err)
	}
	tmp := full + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return store.CacheEntry{}, fmt.Errorf("create tmp: %w", err)
	}

	entry := store.CacheEntry{
		ID:            ulid.Make().String(),
		CacheKey:      cacheKey,
		BookID:        bookID,
		Format:        format,
		MimeType:      mimeType,
		ContentLength: contentLength,
		Status:        "pending",
		RelativePath:  rel,
	}
	if err := m.store.InsertCacheEntry(ctx, entry); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return store.CacheEntry{}, fmt.Errorf("insert cache row: %w", err)
	}

	n, copyErr := io.Copy(f, body)
	closeErr := f.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(tmp)
		msg := combineErr(copyErr, closeErr).Error()
		_ = m.store.UpdateCacheStatus(ctx, entry.ID, "failed", msg, 0)
		return store.CacheEntry{}, fmt.Errorf("copy: %s", msg)
	}
	if err := os.Rename(tmp, full); err != nil {
		_ = os.Remove(tmp)
		_ = m.store.UpdateCacheStatus(ctx, entry.ID, "failed", err.Error(), 0)
		return store.CacheEntry{}, fmt.Errorf("rename: %w", err)
	}
	if err := m.store.UpdateCacheStatus(ctx, entry.ID, "ready", "", n); err != nil {
		return store.CacheEntry{}, fmt.Errorf("mark ready: %w", err)
	}
	entry.Status = "ready"
	entry.BytesOnDisk = n
	entry.LastAccessedAt = time.Now()
	return entry, nil
}

func combineErr(a, b error) error {
	switch {
	case a != nil && b != nil:
		return fmt.Errorf("%v; %v", a, b)
	case a != nil:
		return a
	case b != nil:
		return b
	}
	return nil
}

// EvictTo deletes least-recently-used ready rows (and their on-disk files)
// until the total bytes_on_disk drops at or below targetBytes. Entries with a
// non-zero in-process reader refcount are skipped — they'll be reconsidered
// on the next sweep once readers release. Returns the number of entries
// evicted.
func (m *Manager) EvictTo(ctx context.Context, targetBytes int64) (int, error) {
	total, err := m.store.TotalCacheBytes(ctx)
	if err != nil {
		return 0, err
	}
	if total <= targetBytes {
		return 0, nil
	}
	entries, err := m.store.ListCacheLRU(ctx, 500)
	if err != nil {
		return 0, err
	}
	evicted := 0
	for _, e := range entries {
		if total <= targetBytes {
			break
		}
		m.refMu.Lock()
		busy := m.inUse(e.ID)
		m.refMu.Unlock()
		if busy {
			continue
		}
		// If the unlink fails for anything other than "already gone", keep the
		// DB row so a later sweep retries — deleting the row here would orphan
		// the on-disk file forever (nothing else references its sha-named path)
		// and make TotalCacheBytes under-report real disk usage.
		if err := os.Remove(m.PathFor(e)); err != nil && !os.IsNotExist(err) {
			continue
		}
		if err := m.store.DeleteCacheEntry(ctx, e.ID); err == nil {
			total -= e.BytesOnDisk
			evicted++
		}
	}
	return evicted, nil
}
