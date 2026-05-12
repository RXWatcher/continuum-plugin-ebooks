// Package streaming implements the portal's two streaming modes:
//
//   - "proxy": the portal forwards bytes from the backend to the client live
//     (no on-disk persistence).
//   - "cache": the portal looks up an on-disk LRU cache of recently-served
//     ebook files. A single-flight Manager deduplicates concurrent downloads
//     of the same cache key; followers wait for the leader to finish and then
//     serve from disk via http.ServeFile.
//
// The Manager owns: filesystem layout under <dir>/<sha[:2]>/<sha>, the
// in-flight map keyed by cache_key, and DB transitions on ebook_file_cache.
package streaming

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/store"
)

// ResolveMode returns the user's effective streaming mode. Currently
// per-config (singleton) — Layer 14 of the spec leaves room for a future
// per-user override but v1 only honours backend_config.default_streaming_mode.
func ResolveMode(cfg store.Config) string {
	if cfg.DefaultStreamingMode != "" {
		return cfg.DefaultStreamingMode
	}
	return "proxy"
}

// ComputeCacheKey hashes the (bookID, format, installID) tuple to a stable
// per-(book,format,backend) cache key. Different backends serving the same
// logical book MUST hash differently because their bytes can differ.
func ComputeCacheKey(bookID, format, installID string) string {
	h := sha256.Sum256([]byte(bookID + "|" + format + "|" + installID))
	return hex.EncodeToString(h[:])
}
