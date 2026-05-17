package server

import (
	"encoding/base64"
	"encoding/json"
)

// The multi-library catalog fans one portal request across every enabled
// library, each backed by an independent backend with its own opaque cursor.
// A single shared ?cursor= cannot paginate that, so the combined response
// carries a *composite* cursor: the set of (portal library id -> backend
// cursor) for libraries that still have more pages. Libraries absent from the
// map are exhausted and are skipped on the next page, so every book is
// reachable with no duplication or loss.

// encodeCatalogCursor serializes the per-library backend cursors into one
// opaque token. Returns "" when nothing has more pages (so the caller emits
// no next_cursor and clients stop paginating).
func encodeCatalogCursor(perLibrary map[int64]string) string {
	if len(perLibrary) == 0 {
		return ""
	}
	b, err := json.Marshal(perLibrary)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// decodeCatalogCursor parses a token produced by encodeCatalogCursor. The
// bool is false when raw is empty or not a valid composite token (e.g. a
// stray single-library backend cursor) — callers treat that as "first page,
// query all libraries" rather than crashing.
func decodeCatalogCursor(raw string) (map[int64]string, bool) {
	if raw == "" {
		return nil, false
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, false
	}
	var m map[int64]string
	if err := json.Unmarshal(b, &m); err != nil || len(m) == 0 {
		return nil, false
	}
	return m, true
}
