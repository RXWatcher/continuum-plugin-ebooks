package streaming

import (
	"io"
	"net/http"

	"github.com/RXWatcher/silo-plugin-ebooks/internal/backend"
)

// ProxyStream forwards a Range-aware stream from the backend to the client.
// Hop-by-hop headers are stripped; everything else (Content-Type,
// Content-Length, Content-Range, Accept-Ranges, ETag, …) is preserved so the
// portal is transparent for client-side range requests.
//
// upstreamPath is the backend-relative URL the portal hits, with any
// authentication already baked in (typically a signed ?token= produced by
// EbookBackend.SignedFilePath). The host plugin proxy validates the token —
// the portal here is a pure byte forwarder.
func ProxyStream(w http.ResponseWriter, r *http.Request, host *backend.HostHTTPClient, installID, upstreamPath string) {
	if installID == "" {
		http.Error(w, "no backend installed", http.StatusServiceUnavailable)
		return
	}
	headers := map[string]string{}
	if rng := r.Header.Get("Range"); rng != "" {
		headers["Range"] = rng
	}
	if iNM := r.Header.Get("If-None-Match"); iNM != "" {
		headers["If-None-Match"] = iNM
	}
	upstream, err := host.GetStream(r.Context(), installID, upstreamPath, headers)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer upstream.Body.Close()
	for k, v := range upstream.Header {
		if isHopByHop(k) {
			continue
		}
		w.Header()[k] = v
	}
	w.WriteHeader(upstream.StatusCode)
	_, _ = io.Copy(w, upstream.Body)
}

func isHopByHop(name string) bool {
	switch http.CanonicalHeaderKey(name) {
	case "Connection", "Keep-Alive", "Proxy-Authenticate",
		"Proxy-Authorization", "Te", "Trailers", "Transfer-Encoding", "Upgrade":
		return true
	}
	return false
}
