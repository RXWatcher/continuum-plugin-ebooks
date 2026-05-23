package backend_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/RXWatcher/silo-plugin-ebooks/internal/backend"
)

// fakeHost stands up an httptest.Server that mimics the silo host's
// plugin-proxy ("GET /api/v1/plugins/<id>/<plugin-path>"). The browse tests
// configure the handler to return either the happy-path payload, a 404 (to
// exercise the ebookdb graceful-degrade path), or a 500.
func fakeHost(t *testing.T, status int, body string, capturePath *string) (*backend.HostHTTPClient, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if capturePath != nil {
			*capturePath = r.URL.RequestURI()
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	return backend.NewHostHTTPClient(srv.URL, ""), srv.Close
}

func TestBrowseAuthors_HappyPath(t *testing.T) {
	var gotPath string
	body := `{"items":[{"id":"a1","name":"Asimov","count":42}],"next_cursor":"c2","total":100}`
	host, stop := fakeHost(t, 200, body, &gotPath)
	defer stop()

	bk := backend.NewEbookBackend(host, "inst")
	env, err := bk.BrowseAuthors(context.Background(), "c1", 25, 0)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(gotPath, "/api/v1/plugins/inst/api/v1/browse/authors") {
		t.Errorf("upstream path = %s", gotPath)
	}
	if !strings.Contains(gotPath, "cursor=c1") || !strings.Contains(gotPath, "limit=25") {
		t.Errorf("query params missing: %s", gotPath)
	}
	if len(env.Items) != 1 || env.Items[0].ID != "a1" || env.Items[0].Name != "Asimov" || env.Items[0].Count != 42 {
		t.Errorf("items = %+v", env.Items)
	}
	if env.NextCursor != "c2" || env.Total != 100 {
		t.Errorf("envelope = %+v", env)
	}
}

// TestBrowse_404Degrade verifies the ebookdb case: a backend that doesn't
// expose /browse/* returns 404; we want a clean empty envelope, NOT an error.
func TestBrowse_404Degrade(t *testing.T) {
	host, stop := fakeHost(t, 404, `{"error":"not found"}`, nil)
	defer stop()

	bk := backend.NewEbookBackend(host, "inst")
	for _, fn := range []func(context.Context, string, int, int64) (backend.PageEnvelope[backend.FacetItem], error){
		bk.BrowseAuthors, bk.BrowseSeries, bk.BrowseGenres,
	} {
		env, err := fn(context.Background(), "", 50, 0)
		if err != nil {
			t.Fatalf("404 should degrade, got err: %v", err)
		}
		if len(env.Items) != 0 {
			t.Errorf("expected empty items, got %+v", env.Items)
		}
	}
}

func TestBrowse_UpstreamError(t *testing.T) {
	host, stop := fakeHost(t, 500, "kaboom", nil)
	defer stop()

	bk := backend.NewEbookBackend(host, "inst")
	_, err := bk.BrowseSeries(context.Background(), "", 50, 0)
	if err == nil {
		t.Fatal("expected error from 500")
	}
	if !strings.Contains(err.Error(), "upstream 500") {
		t.Errorf("error didn't mention upstream status: %v", err)
	}
}

func TestListCatalog_PassesFilterParams(t *testing.T) {
	var gotPath string
	body := `{"items":[]}`
	host, stop := fakeHost(t, 200, body, &gotPath)
	defer stop()

	bk := backend.NewEbookBackend(host, "inst")
	_, err := bk.ListCatalog(context.Background(), backend.CatalogQuery{
		Cursor:    "c1",
		Limit:     25,
		LibraryID: 7,
		Author:    "Andy Weir",
		Series:    "Bobiverse",
		Genre:     "sci-fi",
		Tag:       "favorite",
	})
	if err != nil {
		t.Fatalf("ListCatalog: %v", err)
	}
	if !strings.Contains(gotPath, "/api/v1/plugins/inst/api/v1/catalog") {
		t.Errorf("upstream path = %s", gotPath)
	}
	// url.Values encodes alphabetically with '+' for spaces.
	for _, want := range []string{"author=Andy+Weir", "series=Bobiverse", "genre=sci-fi", "tag=favorite", "cursor=c1", "limit=25", "library_id=7"} {
		if !strings.Contains(gotPath, want) {
			t.Errorf("upstream query missing %q in %s", want, gotPath)
		}
	}
}

func TestBrowseGenres_HappyPath(t *testing.T) {
	body := `{"items":[{"id":"g1","name":"Sci-Fi"},{"id":"g2","name":"Mystery","count":5}]}`
	host, stop := fakeHost(t, 200, body, nil)
	defer stop()

	bk := backend.NewEbookBackend(host, "inst")
	env, err := bk.BrowseGenres(context.Background(), "", 0, 0)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(env.Items) != 2 {
		t.Fatalf("items = %+v", env.Items)
	}
	if env.Items[0].Count != 0 || env.Items[1].Count != 5 {
		t.Errorf("count handling wrong: %+v", env.Items)
	}
}
