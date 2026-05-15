package server

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// rateLimitConfig captures the per-IP token-bucket parameters applied to the
// public route subtree (/opds/*, /kosync/*, /kobo/*) and to the authenticated
// send-to-kindle path. These endpoints accept basic-auth or unauthenticated
// traffic and would otherwise be brute-forceable.
//
// Trust model for the client IP key:
//   - r.RemoteAddr is always the immediate TCP peer (the host's reverse proxy
//     in production, or the client directly in standalone-listener mode).
//   - If X-Forwarded-For is set, we trust ONLY the first hop and use it as the
//     key. This is correct when the immediate peer is a trusted reverse proxy
//     (the Continuum host) that appends the original client IP. It is NOT
//     correct in deployments where untrusted clients can supply
//     X-Forwarded-For themselves — operators in such deployments should strip
//     the header at the edge.
//   - The key falls back to r.RemoteAddr when no XFF header is present.
//
// Values: 30 req/s sustained, burst 60 — generous enough that legit OPDS
// catalog browsing (one feed pull + a handful of cover fetches) never trips
// the limiter, but tight enough to make password-spray attacks against the
// kosync /users/auth endpoint waste hours per IP.
const (
	rateLimitRPS   = 30
	rateLimitBurst = 60

	// ipLimiterIdle is how long a per-IP limiter stays alive after its last
	// request. Trades memory for accurate burst tracking — set short enough
	// that one-shot scanners don't pin memory, long enough that a real user's
	// browsing session keeps the same bucket.
	ipLimiterIdle = 10 * time.Minute

	// ipLimiterGCInterval is how often the janitor sweeps idle entries.
	ipLimiterGCInterval = 5 * time.Minute
)

type ipLimiterEntry struct {
	limiter *rate.Limiter
	last    time.Time
}

// ipLimiter is a process-local per-IP rate limiter. Each unique key gets its
// own token bucket; idle buckets are reaped to keep memory bounded.
type ipLimiter struct {
	mu      sync.Mutex
	buckets map[string]*ipLimiterEntry
	rps     rate.Limit
	burst   int
}

func newIPLimiter(rps rate.Limit, burst int) *ipLimiter {
	l := &ipLimiter{
		buckets: make(map[string]*ipLimiterEntry),
		rps:     rps,
		burst:   burst,
	}
	go l.janitor()
	return l
}

// allow returns true if the given key is within budget. It allocates a new
// bucket on first sight and refreshes the entry's last-seen time on every hit.
func (l *ipLimiter) allow(key string) bool {
	l.mu.Lock()
	e, ok := l.buckets[key]
	if !ok {
		e = &ipLimiterEntry{limiter: rate.NewLimiter(l.rps, l.burst)}
		l.buckets[key] = e
	}
	e.last = time.Now()
	lim := e.limiter
	l.mu.Unlock()
	return lim.Allow()
}

func (l *ipLimiter) janitor() {
	ticker := time.NewTicker(ipLimiterGCInterval)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-ipLimiterIdle)
		l.mu.Lock()
		for k, e := range l.buckets {
			if e.last.Before(cutoff) {
				delete(l.buckets, k)
			}
		}
		l.mu.Unlock()
	}
}

// clientIP returns the rate-limit key for a request. See rateLimitConfig's
// docstring for the trust model behind the X-Forwarded-For handling.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Trust the first hop only — the rest of the chain is client-supplied
		// and may be forged.
		if i := strings.IndexByte(xff, ','); i >= 0 {
			xff = xff[:i]
		}
		if v := strings.TrimSpace(xff); v != "" {
			return v
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// rateLimitMiddleware enforces the per-IP token-bucket on every request that
// passes through it. Use it on the public route subtrees where the absence of
// authenticated identity makes brute-force or scraping attacks cheap.
func rateLimitMiddleware(l *ipLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !l.allow(clientIP(r)) {
				w.Header().Set("Retry-After", "1")
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
