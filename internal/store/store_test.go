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

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/migrate"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/store"
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
		"postgres://continuum:continuum@localhost:5432/continuum?search_path=%s&sslmode=disable",
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
	cfg.TargetBackendPluginID = "continuum.bookwarehouse-ebook"
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
		Status: "pending", TargetPluginID: "continuum.bookwarehouse-ebook",
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

	// kobo_transfer_session
	if err := s.InsertKoboSession(ctx, store.KoboSession{
		ID: "k1", UserID: "u1", BookID: "b1", Format: "kepub",
		TransferCode: "ABCD", SourcePath: "/tmp/x.kepub", Status: "pending",
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

	// opds_token
	if err := s.InsertOPDSToken(ctx, store.OPDSToken{
		ID: "ot1", UserID: "u1", JTI: "jti-1", TokenHash: "hash", Label: "phone",
	}); err != nil {
		t.Fatalf("InsertOPDSToken: %v", err)
	}
	if _, err := s.GetOPDSTokenByJTI(ctx, "jti-1"); err != nil {
		t.Errorf("GetOPDSTokenByJTI: %v", err)
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
