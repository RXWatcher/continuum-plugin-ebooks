package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/time/rate"
)

// TestE4_RateLimiter_PerIPIsolation confirms the per-IP token bucket isolates
// clients: hammering IP A's limiter must not affect IP B's budget. Without
// this isolation a single noisy peer could throttle every other user.
func TestE4_RateLimiter_PerIPIsolation(t *testing.T) {
	// Tight limits so the test runs without time.Sleep.
	lim := newIPLimiter(rate.Limit(1), 2)

	// Burn IP A's burst.
	if !lim.allow("1.2.3.4") || !lim.allow("1.2.3.4") {
		t.Fatal("initial burst should pass for IP A")
	}
	if lim.allow("1.2.3.4") {
		t.Error("3rd req from IP A should be throttled after burst=2")
	}

	// IP B's budget must be untouched.
	if !lim.allow("5.6.7.8") || !lim.allow("5.6.7.8") {
		t.Error("IP B's burst was consumed by IP A activity — limiter is not per-IP")
	}
}

// TestE4_RateLimiter_Middleware429 confirms the chi-style middleware returns
// 429 with a Retry-After header once the limit is exceeded. This is the
// observable contract clients depend on for backoff.
func TestE4_RateLimiter_Middleware429(t *testing.T) {
	lim := newIPLimiter(rate.Limit(1), 1)
	mw := rateLimitMiddleware(lim)
	called := 0
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusOK)
	}))

	mk := func() *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/opds/catalog", nil)
		req.RemoteAddr = "10.0.0.1:54321"
		return req
	}

	// First request: passes.
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, mk())
	if rec1.Code != http.StatusOK {
		t.Fatalf("first request status = %d, want 200", rec1.Code)
	}
	// Second request from same IP: throttled (burst=1).
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, mk())
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want 429", rec2.Code)
	}
	if rec2.Header().Get("Retry-After") == "" {
		t.Error("429 response missing Retry-After header — clients have no backoff signal")
	}
	if called != 1 {
		t.Errorf("inner handler called %d times, want 1 (limiter should short-circuit)", called)
	}
}

// TestE4_ClientIP_PrefersXFFFirstHop documents the trust boundary: when
// X-Forwarded-For is present (set by the trusted reverse proxy), we use the
// FIRST hop only. The rest of the chain is client-supplied and may be forged,
// so a malicious client appending "1.1.1.1, 2.2.2.2" must be keyed on
// "1.1.1.1" (the original peer the reverse proxy observed), not "2.2.2.2".
func TestE4_ClientIP_PrefersXFFFirstHop(t *testing.T) {
	cases := []struct {
		name    string
		remote  string
		xff     string
		wantKey string
	}{
		{"no xff falls back to remote host", "10.0.0.1:54321", "", "10.0.0.1"},
		{"xff single hop wins", "10.0.0.1:54321", "203.0.113.5", "203.0.113.5"},
		{"xff chain → first hop only", "10.0.0.1:54321", "203.0.113.5, 198.51.100.1", "203.0.113.5"},
		{"xff with whitespace trims", "10.0.0.1:54321", "  203.0.113.5  , 1.2.3.4", "203.0.113.5"},
		{"empty xff falls back", "10.0.0.1:54321", " , ", "10.0.0.1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = c.remote
			if c.xff != "" {
				req.Header.Set("X-Forwarded-For", c.xff)
			}
			if got := clientIP(req); got != c.wantKey {
				t.Errorf("clientIP = %q, want %q", got, c.wantKey)
			}
		})
	}
}
