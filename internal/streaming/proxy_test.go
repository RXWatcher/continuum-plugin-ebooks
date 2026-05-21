package streaming_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/RXWatcher/continuum-plugin-ebooks/internal/backend"
	"github.com/RXWatcher/continuum-plugin-ebooks/internal/streaming"
)

// TestProxyStream_ForwardsBodyAndHeaders spins up a fake host proxy that
// returns a fixed payload + a few representative headers, and asserts the
// portal-side ProxyStream forwards them unmodified to the client (minus
// hop-by-hop).
func TestProxyStream_ForwardsBodyAndHeaders(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v1/plugins/inst/api/v1/file/") {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/epub+zip")
		w.Header().Set("Content-Length", "5")
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Transfer-Encoding", "chunked") // should be stripped
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))
	defer upstream.Close()
	host := backend.NewHostHTTPClient(upstream.URL, "")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/me/books/b1/file?format=epub", nil)
	streaming.ProxyStream(rec, req, host, "inst", "/api/v1/file/b1")
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	if got := rec.Body.String(); got != "hello" {
		t.Errorf("body = %q", got)
	}
	if rec.Header().Get("Content-Type") != "application/epub+zip" {
		t.Errorf("Content-Type missing")
	}
	if rec.Header().Get("Accept-Ranges") != "bytes" {
		t.Errorf("Accept-Ranges missing")
	}
	if rec.Header().Get("Transfer-Encoding") != "" {
		t.Errorf("hop-by-hop leaked")
	}
}

// TestProxyStream_ForwardsRangeHeader verifies Range is plumbed upstream.
func TestProxyStream_ForwardsRangeHeader(t *testing.T) {
	var sawRange string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawRange = r.Header.Get("Range")
		w.Header().Set("Content-Range", "bytes 0-3/100")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("abcd"))
	}))
	defer upstream.Close()
	host := backend.NewHostHTTPClient(upstream.URL, "")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/file", nil)
	req.Header.Set("Range", "bytes=0-3")
	streaming.ProxyStream(rec, req, host, "inst", "/api/v1/file/b1")
	if sawRange != "bytes=0-3" {
		t.Errorf("upstream Range = %q", sawRange)
	}
	if rec.Code != http.StatusPartialContent {
		t.Errorf("code = %d", rec.Code)
	}
	if rec.Header().Get("Content-Range") != "bytes 0-3/100" {
		t.Errorf("Content-Range = %q", rec.Header().Get("Content-Range"))
	}
}

// TestProxyStream_NoInstallID returns 503 before doing any IO.
func TestProxyStream_NoInstallID(t *testing.T) {
	host := backend.NewHostHTTPClient("http://nowhere", "")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	streaming.ProxyStream(rec, req, host, "", "/api/v1/file/b1")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("code = %d", rec.Code)
	}
}
