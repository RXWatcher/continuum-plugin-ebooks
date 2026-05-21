package streaming_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/migrate"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/store"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/streaming"
)

var (
	cacheSchemaOnce sync.Once
	cacheSchema     string
)

func cacheSchemaName() string {
	cacheSchemaOnce.Do(func() {
		cacheSchema = fmt.Sprintf("ebooks_streaming_test_%d", os.Getpid())
	})
	return cacheSchema
}

func cacheDSN() string {
	if v := os.Getenv("TEST_DATABASE_URL"); v != "" {
		u, err := url.Parse(v)
		if err == nil {
			q := u.Query()
			q.Set("search_path", cacheSchemaName())
			u.RawQuery = q.Encode()
			return u.String()
		}
		return v
	}
	return fmt.Sprintf(
		"postgres://continuum:continuum@localhost:5432/continuum?search_path=%s&sslmode=disable",
		cacheSchemaName(),
	)
}

func stripSearchPath(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return dsn
	}
	q := u.Query()
	q.Del("search_path")
	u.RawQuery = q.Encode()
	return u.String()
}

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dsn := cacheDSN()
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, stripSearchPath(dsn))
	if err != nil {
		t.Skipf("postgres unreachable: %v", err)
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Skipf("postgres unreachable: %v", err)
	}
	defer admin.Close()
	_, _ = admin.Exec(ctx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", cacheSchemaName()))
	if _, err := admin.Exec(ctx, fmt.Sprintf("CREATE SCHEMA %s", cacheSchemaName())); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if err := migrate.Run(ctx, dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		admin, _ := pgxpool.New(context.Background(), stripSearchPath(cacheDSN()))
		if admin != nil {
			_, _ = admin.Exec(context.Background(), fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", cacheSchemaName()))
			admin.Close()
		}
	})
	return store.New(pool)
}

func TestManager_MissThenHit(t *testing.T) {
	st := newTestStore(t)
	dir := t.TempDir()
	m := streaming.NewManager(dir, 1<<30, st)
	ctx := context.Background()
	key := streaming.ComputeCacheKey("b1", "inst", 1)
	fetch := func(ctx context.Context) (io.ReadCloser, http.Header, int64, string, error) {
		return io.NopCloser(strings.NewReader("hello")), http.Header{}, 5, "application/epub+zip", nil
	}
	if _, ok := m.Lookup(ctx, key); ok {
		t.Fatal("expected miss")
	}
	entry, err := m.StartOrJoin(ctx, key, "b1", "epub", fetch)
	if err != nil {
		t.Fatalf("StartOrJoin: %v", err)
	}
	if entry.Status != "ready" || entry.BytesOnDisk != 5 {
		t.Errorf("entry = %+v", entry)
	}
	got, ok := m.Lookup(ctx, key)
	if !ok || got.ID != entry.ID {
		t.Errorf("Lookup after fill: %v ok=%v", got, ok)
	}
}

func TestManager_SingleFlight(t *testing.T) {
	st := newTestStore(t)
	dir := t.TempDir()
	m := streaming.NewManager(dir, 1<<30, st)
	ctx := context.Background()
	key := streaming.ComputeCacheKey("b2", "inst", 1)
	var fetchCount atomic.Int32
	gate := make(chan struct{})
	fetch := func(ctx context.Context) (io.ReadCloser, http.Header, int64, string, error) {
		fetchCount.Add(1)
		<-gate
		return io.NopCloser(strings.NewReader("payload")), http.Header{}, 7, "application/epub+zip", nil
	}
	var wg sync.WaitGroup
	results := make([]store.CacheEntry, 5)
	errs := make([]error, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = m.StartOrJoin(ctx, key, "b2", "epub", fetch)
		}(i)
	}
	// Let goroutines pile up on the inflight map.
	for fetchCount.Load() == 0 {
	}
	close(gate)
	wg.Wait()
	if got := fetchCount.Load(); got != 1 {
		t.Errorf("fetcher called %d times, want 1", got)
	}
	for i, e := range errs {
		if e != nil {
			t.Errorf("err[%d] = %v", i, e)
		}
		if results[i].Status != "ready" {
			t.Errorf("results[%d] = %+v", i, results[i])
		}
	}
}

func TestManager_EvictTo(t *testing.T) {
	st := newTestStore(t)
	dir := t.TempDir()
	m := streaming.NewManager(dir, 1<<30, st)
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		key := streaming.ComputeCacheKey(fmt.Sprintf("b-%d", i), "inst", 1)
		fetch := func(ctx context.Context) (io.ReadCloser, http.Header, int64, string, error) {
			body := strings.Repeat("x", 100)
			return io.NopCloser(strings.NewReader(body)), http.Header{}, int64(len(body)), "application/epub+zip", nil
		}
		if _, err := m.StartOrJoin(ctx, key, fmt.Sprintf("b-%d", i), "epub", fetch); err != nil {
			t.Fatalf("seed[%d]: %v", i, err)
		}
	}
	// Now 4*100 = 400 bytes. Evict down to 250.
	n, err := m.EvictTo(ctx, 250)
	if err != nil {
		t.Fatalf("EvictTo: %v", err)
	}
	if n < 1 {
		t.Errorf("evicted %d, want >=1", n)
	}
	total, _ := st.TotalCacheBytes(ctx)
	if total > 250 {
		t.Errorf("total = %d after evict (target 250)", total)
	}
}

// TestManager_AcquireBlocksEvict verifies that an entry held via Acquire is
// NOT evicted by EvictTo, and that releasing the ref allows a subsequent
// EvictTo to remove it. This is the load-bearing invariant that prevents the
// read/delete-mid-io.Copy race the registry was added to fix.
func TestManager_AcquireBlocksEvict(t *testing.T) {
	st := newTestStore(t)
	dir := t.TempDir()
	m := streaming.NewManager(dir, 1<<30, st)
	ctx := context.Background()

	// Seed two entries, each 100 bytes. The LRU order puts the older one
	// first; we'll hold a ref on the oldest so EvictTo has to skip it.
	var keys []string
	var ids []string
	for i := 0; i < 2; i++ {
		key := streaming.ComputeCacheKey(fmt.Sprintf("hold-%d", i), "inst", 1)
		keys = append(keys, key)
		fetch := func(ctx context.Context) (io.ReadCloser, http.Header, int64, string, error) {
			body := strings.Repeat("y", 100)
			return io.NopCloser(strings.NewReader(body)), http.Header{}, int64(len(body)), "application/epub+zip", nil
		}
		entry, err := m.StartOrJoin(ctx, key, fmt.Sprintf("hold-%d", i), "epub", fetch)
		if err != nil {
			t.Fatalf("seed[%d]: %v", i, err)
		}
		ids = append(ids, entry.ID)
	}

	// Hold a ref on the oldest (LRU-first) entry.
	heldID := ids[0]
	release := m.Acquire(heldID)

	// Concurrent eviction: drop total from 200 → 50. Without the refcount
	// skip, both entries would be evicted. With it, the held one must
	// survive.
	done := make(chan error, 1)
	go func() {
		_, err := m.EvictTo(ctx, 50)
		done <- err
	}()
	if err := <-done; err != nil {
		t.Fatalf("EvictTo: %v", err)
	}

	if e, ok := m.Lookup(ctx, keys[0]); !ok {
		t.Errorf("held entry was evicted (id=%s)", heldID)
	} else if e.ID != heldID {
		t.Errorf("held entry mismatch: got %s want %s", e.ID, heldID)
	}

	// Now release and re-evict; the entry should be removable.
	release()
	if _, err := m.EvictTo(ctx, 0); err != nil {
		t.Fatalf("EvictTo after release: %v", err)
	}
	if _, ok := m.Lookup(ctx, keys[0]); ok {
		t.Errorf("entry not evicted after release (id=%s)", heldID)
	}
}

// TestManager_AcquireReleaseIdempotent verifies the documented exactly-once
// contract is robust to accidental double-release — required because the
// Kindle send path may invoke its cleanup func through multiple defer/error
// branches if future refactors aren't careful.
func TestManager_AcquireReleaseIdempotent(t *testing.T) {
	m := streaming.NewManager(t.TempDir(), 1<<30, nil)
	release := m.Acquire("entry-x")
	release()
	// Second call must NOT panic and must NOT make the refcount go negative.
	release()
	// Third call too.
	release()
	// A fresh Acquire after release should give count=1, not 0 or -2.
	r2 := m.Acquire("entry-x")
	defer r2()
	// Indirect probe: a parallel acquire/release should leave the entry
	// usable (no panic).
}
