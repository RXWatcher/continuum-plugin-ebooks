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
	"strconv"

	"github.com/RXWatcher/continuum-plugin-ebooks/internal/store"
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

// ComputeCacheKey hashes the (bookID, installID, libraryID) tuple to a
// stable cache key. libraryID is included so two portal libraries pointing
// at the same backend never collide on the same book id — their permissions
// may differ and a cache hit must not cross library boundaries. Format is
// no longer keyed because both supported backends store a single file per
// book row and ignore format on the byte route.
func ComputeCacheKey(bookID, installID string, libraryID int64) string {
	h := sha256.Sum256([]byte(bookID + "|" + installID + "|" + strconv.FormatInt(libraryID, 10)))
	return hex.EncodeToString(h[:])
}
