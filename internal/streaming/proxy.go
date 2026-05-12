package streaming

import (
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/backend"
)

// ProxyStream forwards a Range-aware stream from the backend to the client.
// Hop-by-hop headers are stripped; everything else (Content-Type,
// Content-Length, Content-Range, Accept-Ranges, ETag, …) is preserved so the
// portal is transparent for client-side range requests.
func ProxyStream(w http.ResponseWriter, r *http.Request, host *backend.HostHTTPClient, installID, bookID, format string) {
	if installID == "" {
		http.Error(w, "no backend installed", http.StatusServiceUnavailable)
		return
	}
	upstreamPath := fmt.Sprintf("/api/v1/file/%s?format=%s",
		url.PathEscape(bookID), url.QueryEscape(format))
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
