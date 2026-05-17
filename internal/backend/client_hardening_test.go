package backend

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A non-numeric install id (from DB/config, e.g. a synced backend value)
// must never be interpolated into the host-proxy URL path — that would let
// it escape /api/v1/plugins/<id>/ (SSRF / host-API traversal).
func TestGetStream_RejectsNonNumericInstallID(t *testing.T) {
	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(200)
	}))
	defer srv.Close()
	c := NewHostHTTPClient(srv.URL, "tok")

	for _, bad := range []string{"1/../../admin", "../secret", "a/b", "a?b", "a#b", "a b", "", "a%2f", "a@b"} {
		if _, err := c.GetStream(context.Background(), bad, "/api/v1/x", nil); err == nil {
			t.Fatalf("install id %q accepted, want rejected", bad)
		}
		if _, _, err := c.PostJSON(context.Background(), bad, "/api/v1/x", nil); err == nil {
			t.Fatalf("PostJSON install id %q accepted, want rejected", bad)
		}
	}
	if hit {
		t.Fatal("a rejected install id still produced an HTTP request")
	}
	// Both legitimate forms — numeric install id and plugin-id slug — pass.
	for _, ok := range []string{"7", "inst", "continuum.bookwarehouse-ebook"} {
		if _, err := c.GetStream(context.Background(), ok, "/api/v1/x", nil); err != nil {
			t.Fatalf("valid install id %q rejected: %v", ok, err)
		}
	}
}

// A cross-host redirect from the (backend-influenced) proxied response must
// be surfaced, not followed with the host bearer attached.
func TestGetStream_DoesNotFollowRedirect(t *testing.T) {
	var attackerHit bool
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attackerHit = true
	}))
	defer attacker.Close()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, attacker.URL+"/", http.StatusFound)
	}))
	defer proxy.Close()

	c := NewHostHTTPClient(proxy.URL, "tok")
	resp, err := c.GetStream(context.Background(), "1", "/api/v1/x", nil)
	if err != nil {
		t.Fatalf("GetStream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("redirect was followed (status %d); want the 302 surfaced", resp.StatusCode)
	}
	if attackerHit {
		t.Fatal("client followed the redirect to the attacker host")
	}
}

func TestTruncForError_Bounded(t *testing.T) {
	got := truncForError([]byte(strings.Repeat("x", 50000)))
	if len(got) > 512+len("…(truncated)") {
		t.Fatalf("error body not truncated: %d bytes", len(got))
	}
}
