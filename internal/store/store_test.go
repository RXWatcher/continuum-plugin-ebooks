package store_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/RXWatcher/silo-plugin-ebooks/internal/migrate"
	"github.com/RXWatcher/silo-plugin-ebooks/internal/store"
)

var (
	testSchemaOnce sync.Once
	testSchema     string
)

func schemaName() string {
	testSchemaOnce.Do(func() {
		testSchema = fmt.Sprintf("ebooks_test_%d", os.Getpid())
	})
	return testSchema
}

func testDSN() string {
	if v := os.Getenv("TEST_DATABASE_URL"); v != "" {
		u, err := url.Parse(v)
		if err == nil {
			q := u.Query()
			q.Set("search_path", schemaName())
			u.RawQuery = q.Encode()
			return u.String()
		}
		return v
	}
	return fmt.Sprintf(
		"postgres://silo:silo@localhost:5432/silo?search_path=%s&sslmode=disable",
		schemaName(),
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
	dsn := testDSN()
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
	_, _ = admin.Exec(ctx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schemaName()))
	if _, err := admin.Exec(ctx, fmt.Sprintf("CREATE SCHEMA %s", schemaName())); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if err := migrate.Run(ctx, dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return store.New(pool)
}

func TestMain(m *testing.M) {
	code := m.Run()
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, stripSearchPath(testDSN()))
	if err == nil {
		_, _ = admin.Exec(ctx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schemaName()))
		admin.Close()
	}
	os.Exit(code)
}

// Smoke test exercising every store wrapper in one pass to verify
// migrations + table layouts agree with the Go signatures.
func TestStore_Smoke(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// backend_config — auto-inserts default singleton
	cfg, err := s.GetConfig(ctx)
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if len(cfg.KosyncSecret) == 0 {
		t.Errorf("kosync_secret should be auto-populated")
	}
	cfg.TargetBackendPluginID = "silo.ebook-library-source"
	if err := s.UpsertConfig(ctx, cfg); err != nil {
		t.Fatalf("UpsertConfig: %v", err)
	}

	// user_data
	if err := s.UpsertUserData(ctx, store.UserData{
		UserID: "u1", BookID: "b1", ReadProgress: 50, IsFinished: false, IsFavorite: true,
	}); err != nil {
		t.Fatalf("UpsertUserData: %v", err)
	}
	ud, err := s.GetUserData(ctx, "u1", "b1")
	if err != nil {
		t.Fatalf("GetUserData: %v", err)
	}
	if !ud.IsFavorite {
		t.Errorf("favorite: %+v", ud)
	}

	// annotation
	if err := s.InsertAnnotation(ctx, store.Annotation{
		ID: "a1", UserID: "u1", BookID: "b1", CFIRange: "epubcfi(/6/4!/4/2)",
		Kind: "highlight", Color: "yellow", SelectedText: "foo",
	}); err != nil {
		t.Fatalf("InsertAnnotation: %v", err)
	}
	ans, _ := s.ListAnnotationsByBook(ctx, "u1", "b1")
	if len(ans) != 1 {
		t.Errorf("ann count = %d", len(ans))
	}

	// request
	if err := s.InsertRequest(ctx, store.Request{
		ID: "r1", UserID: "u1", Title: "X", Authors: []string{"A"},
		Status: "pending", TargetPluginID: "silo.ebook-library-source",
	}); err != nil {
		t.Fatalf("InsertRequest: %v", err)
	}
	if err := s.UpdateRequestStatus(ctx, "r1", "submitted", "ext-1", "", "", ""); err != nil {
		t.Fatalf("UpdateRequestStatus: %v", err)
	}
	reqRow, _ := s.GetRequest(ctx, "r1")
	if reqRow.Status != "submitted" || reqRow.ExternalID != "ext-1" {
		t.Errorf("req: %+v", reqRow)
	}

	// collection
	if err := s.CreateCollection(ctx, store.Collection{ID: "c1", UserID: "u1", Name: "TBR"}); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	if err := s.AddItem(ctx, "c1", "b1", 0); err != nil {
		t.Fatalf("AddItem: %v", err)
	}
	items, _ := s.ListItems(ctx, "c1")
	if len(items) != 1 {
		t.Errorf("items = %v", items)
	}

	// kobo_transfer_session — CodeHash stores the bcrypt hash of the URL code;
	// this fixture uses a dummy hash since the smoke path doesn't exercise
	// CompareHashAndPassword (covered in TestKoboSession_BcryptMatch).
	if err := s.InsertKoboSession(ctx, store.KoboSession{
		ID: "k1", UserID: "u1", BookID: "b1", Format: "kepub",
		CodeHash: "$2a$10$abcdefghijklmnopqrstuv", SourcePath: "/tmp/x.kepub", Status: "pending",
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("InsertKoboSession: %v", err)
	}

	// ebook_file_cache
	if err := s.InsertCacheEntry(ctx, store.CacheEntry{
		ID: "fc1", CacheKey: "sha256-a", BookID: "b1", Format: "epub",
		MimeType: "application/epub+zip", ContentLength: 1024, Status: "ready",
		RelativePath: "aa/bb.epub",
	}); err != nil {
		t.Fatalf("InsertCacheEntry: %v", err)
	}
	_ = s.UpdateCacheStatus(ctx, "fc1", "ready", "", 1024)
	total, _ := s.TotalCacheBytes(ctx)
	if total != 1024 {
		t.Errorf("total = %d", total)
	}

	// kosync
	if err := s.UpsertKosyncUser(ctx, store.KosyncUser{
		UserID: "u1", KosyncUsername: "alice", KosyncPasswordHash: "h",
	}); err != nil {
		t.Fatalf("UpsertKosyncUser: %v", err)
	}
	if err := s.UpsertKosyncProgress(ctx, store.KosyncProgress{
		UserID: "u1", Document: "md5-x", Progress: "/6/4!/4/2", Percentage: 0.47,
	}); err != nil {
		t.Fatalf("UpsertKosyncProgress: %v", err)
	}

	// kindle_send_log
	if err := s.InsertKindleSend(ctx, store.KindleSend{
		ID: "ks1", UserID: "u1", BookID: "b1", Format: "epub",
		ToAddress: "alice@kindle.com", Status: "queued",
	}); err != nil {
		t.Fatalf("InsertKindleSend: %v", err)
	}
}

// TestE3_KosyncProgress_DeviceIDBinding verifies the (user_id, document,
// device_id) primary key isolates per-device progress so two devices on the
// same document for the same user never clobber each other. Before this fix
// the PK was (user_id, document) and an upsert from device B would silently
// overwrite device A's last-known position.
//
// The test also confirms the cross-user case: even with identical
// (document, device_id), one user's row cannot overwrite another's — the
// authenticated-session user_id is part of the PK.
func TestE3_KosyncProgress_DeviceIDBinding(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	mk := func(user, doc, dev, progress string, pct float64) store.KosyncProgress {
		return store.KosyncProgress{
			UserID: user, Document: doc, DeviceID: dev, Progress: progress, Percentage: pct,
		}
	}

	// Same user, same document, different devices → two coexisting rows.
	if err := s.UpsertKosyncProgress(ctx, mk("alice", "doc-x", "kobo-1", "p:10", 0.10)); err != nil {
		t.Fatalf("upsert dev1: %v", err)
	}
	if err := s.UpsertKosyncProgress(ctx, mk("alice", "doc-x", "kindle-1", "p:50", 0.50)); err != nil {
		t.Fatalf("upsert dev2: %v", err)
	}
	// Different user, same document, same device_id → must coexist (user_id is
	// part of the PK). This is the security-critical case from finding E3:
	// before the fix, the PK was (user_id, document) and user_id was server-
	// authenticated; the new PK does not loosen that — device_id widens the
	// upsert key, never narrows it.
	if err := s.UpsertKosyncProgress(ctx, mk("bob", "doc-x", "kindle-1", "p:90", 0.90)); err != nil {
		t.Fatalf("upsert cross-user: %v", err)
	}

	// Re-upserting alice/doc-x/kindle-1 must overwrite ONLY that tuple.
	if err := s.UpsertKosyncProgress(ctx, mk("alice", "doc-x", "kindle-1", "p:55", 0.55)); err != nil {
		t.Fatalf("upsert dev2 again: %v", err)
	}

	// Read-back returns the latest row for (user, doc) — kindle-1 had the
	// most recent write so its 0.55 should win.
	got, err := s.GetKosyncProgress(ctx, "alice", "", "doc-x")
	if err != nil {
		t.Fatalf("GetKosyncProgress alice: %v", err)
	}
	if got.Percentage < 0.54 || got.Percentage > 0.56 {
		t.Errorf("expected latest write (0.55) to win, got %+v", got)
	}
	if got.DeviceID != "kindle-1" {
		t.Errorf("latest write was on kindle-1, got DeviceID=%q", got.DeviceID)
	}

	// Bob's row for the same (document, device_id) must be untouched.
	gotBob, err := s.GetKosyncProgress(ctx, "bob", "", "doc-x")
	if err != nil {
		t.Fatalf("GetKosyncProgress bob: %v", err)
	}
	if gotBob.Percentage < 0.89 || gotBob.Percentage > 0.91 {
		t.Errorf("bob's progress was clobbered by alice's upsert: %+v", gotBob)
	}
}
