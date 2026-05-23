package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/RXWatcher/silo-plugin-ebooks/internal/migrate"
	"github.com/RXWatcher/silo-plugin-ebooks/internal/store"
)

var (
	readerConfigSchemaOnce sync.Once
	readerConfigSchema     string
)

func readerConfigTestSchema() string {
	readerConfigSchemaOnce.Do(func() {
		readerConfigSchema = fmt.Sprintf("ebooks_server_reader_config_test_%d", os.Getpid())
	})
	return readerConfigSchema
}

func readerConfigTestDSN() string {
	if v := os.Getenv("TEST_DATABASE_URL"); v != "" {
		u, err := url.Parse(v)
		if err == nil {
			q := u.Query()
			q.Set("search_path", readerConfigTestSchema())
			u.RawQuery = q.Encode()
			return u.String()
		}
		return v
	}
	return fmt.Sprintf(
		"postgres://silo:silo@localhost:5432/silo?search_path=%s&sslmode=disable",
		readerConfigTestSchema(),
	)
}

func readerConfigStripSearchPath(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return dsn
	}
	q := u.Query()
	q.Del("search_path")
	u.RawQuery = q.Encode()
	return u.String()
}

func newReaderConfigTestServer(t *testing.T) *Server {
	t.Helper()
	dsn := readerConfigTestDSN()
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, readerConfigStripSearchPath(dsn))
	if err != nil {
		t.Skipf("postgres unreachable: %v", err)
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Skipf("postgres unreachable: %v", err)
	}
	defer admin.Close()
	_, _ = admin.Exec(ctx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", readerConfigTestSchema()))
	if _, err := admin.Exec(ctx, fmt.Sprintf("CREATE SCHEMA %s", readerConfigTestSchema())); err != nil {
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
		admin, err := pgxpool.New(context.Background(), readerConfigStripSearchPath(dsn))
		if err == nil {
			_, _ = admin.Exec(context.Background(), fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", readerConfigTestSchema()))
			admin.Close()
		}
	})
	return New(Deps{Store: store.New(pool)})
}

func TestReaderConfigRoutesRoundTripAndMirrorProgress(t *testing.T) {
	srv := newReaderConfigTestServer(t)
	handler := srv.Handler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/books/book-1/reader-config", nil)
	req.Header.Set("X-Silo-User-Id", "user-1")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("missing config status=%d body=%s", rec.Code, rec.Body.String())
	}
	var missing struct {
		BookID string         `json:"book_id"`
		Config map[string]any `json:"config"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &missing); err != nil {
		t.Fatalf("decode missing config: %v", err)
	}
	if missing.BookID != "book-1" || len(missing.Config) != 0 {
		t.Fatalf("missing config response = %+v", missing)
	}

	body := []byte(`{"config":{"location":"epubcfi(/6/4)","progress":[4,10],"booknotes":[]}}`)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/me/books/book-1/reader-config", bytes.NewReader(body))
	req.Header.Set("X-Silo-User-Id", "user-1")
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put config status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/me/books/book-1/reader-config", nil)
	req.Header.Set("X-Silo-User-Id", "user-1")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("saved config status=%d body=%s", rec.Code, rec.Body.String())
	}
	var saved struct {
		Config map[string]any `json:"config"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil {
		t.Fatalf("decode saved config: %v", err)
	}
	if saved.Config["location"] != "epubcfi(/6/4)" {
		t.Fatalf("saved location = %+v", saved.Config)
	}

	ud, err := srv.deps.Store.GetUserData(context.Background(), "user-1", "book-1")
	if err != nil {
		t.Fatalf("GetUserData mirror: %v", err)
	}
	if ud.LastCFI != "epubcfi(/6/4)" || ud.CurrentPage != 4 || math.Abs(ud.ReadProgress-0.4) > 0.00001 {
		t.Fatalf("mirrored user data = %+v", ud)
	}
}

func TestReaderConfigGetFallsBackToUserDataProgress(t *testing.T) {
	srv := newReaderConfigTestServer(t)
	handler := srv.Handler()
	ctx := context.Background()

	if err := srv.deps.Store.UpsertUserData(ctx, store.UserData{
		UserID:       "user-1",
		BookID:       "book-1",
		LastCFI:      "epubcfi(/6/8)",
		CurrentPage:  8,
		ReadProgress: 0.8,
		IsFinished:   false,
		IsFavorite:   true,
		Notes:        "keep me",
	}); err != nil {
		t.Fatalf("seed user data: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/books/book-1/reader-config", nil)
	req.Header.Set("X-Silo-User-Id", "user-1")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get config status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Config map[string]any `json:"config"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if got.Config["location"] != "epubcfi(/6/8)" {
		t.Fatalf("fallback location = %+v", got.Config)
	}
	progress, ok := got.Config["progress"].([]any)
	if !ok || len(progress) != 2 || progress[0].(float64) != 8 || progress[1].(float64) != 10 {
		t.Fatalf("fallback progress = %#v", got.Config["progress"])
	}

	ud, err := srv.deps.Store.GetUserData(ctx, "user-1", "book-1")
	if err != nil {
		t.Fatalf("GetUserData after fallback: %v", err)
	}
	if !ud.IsFavorite || ud.Notes != "keep me" {
		t.Fatalf("fallback read mutated non-progress fields: %+v", ud)
	}
}

func TestReaderConfigIncludesLinkedKosyncProgress(t *testing.T) {
	srv := newReaderConfigTestServer(t)
	handler := srv.Handler()
	ctx := context.Background()

	if err := srv.deps.Store.UpsertKosyncBookLink(ctx, store.KosyncBookLink{
		UserID:   "user-1",
		BookID:   "book-1",
		Document: "doc-1",
		Format:   "epub",
	}); err != nil {
		t.Fatalf("seed kosync link: %v", err)
	}
	if err := srv.deps.Store.UpsertKosyncProgress(ctx, store.KosyncProgress{
		UserID:     "user-1",
		Document:   "doc-1",
		Progress:   "epubcfi(/6/10)",
		Percentage: 0.9,
		Device:     "KOReader",
		DeviceID:   "dev-1",
	}); err != nil {
		t.Fatalf("seed kosync progress: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/books/book-1/reader-config", nil)
	req.Header.Set("X-Silo-User-Id", "user-1")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get config status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Config map[string]any `json:"config"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	external, ok := got.Config["externalProgress"].(map[string]any)
	if !ok {
		t.Fatalf("missing external progress: %+v", got.Config)
	}
	if external["source"] != "kosync" || external["document"] != "doc-1" || external["location"] != "epubcfi(/6/10)" {
		t.Fatalf("external progress = %+v", external)
	}
	if external["canResume"] != true {
		t.Fatalf("external progress should be resumable: %+v", external)
	}
}

func TestKosyncBookLinkRoute(t *testing.T) {
	srv := newReaderConfigTestServer(t)
	handler := srv.Handler()

	body := []byte(`{"document":"doc-route","format":"epub"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/me/books/book-1/kosync-link", bytes.NewReader(body))
	req.Header.Set("X-Silo-User-Id", "user-1")
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("link status=%d body=%s", rec.Code, rec.Body.String())
	}

	link, err := srv.deps.Store.FindKosyncBookLinkByBook(context.Background(), "user-1", "", "book-1")
	if err != nil {
		t.Fatalf("FindKosyncBookLinkByBook: %v", err)
	}
	if link.Document != "doc-route" || link.Format != "epub" {
		t.Fatalf("link = %+v", link)
	}
}
