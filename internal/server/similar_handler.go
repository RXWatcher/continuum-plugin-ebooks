package server

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/RXWatcher/continuum-plugin-ebooks/internal/auth"
	"github.com/RXWatcher/continuum-plugin-ebooks/internal/store"
)

// Recommender is the narrow surface the server uses for embedding-
// driven similar-items recommendations. Implemented by
// recommend.Engine; surfaced as an interface so the server package
// can be tested with a fake.
type Recommender interface {
	Similar(ctx context.Context, libraryID int64, bookID string, limit int) ([]store.SimilarEbook, error)
}

// handleSimilarBooks — GET /me/books/{id}/similar?limit=N
// Returns the cached embedding-derived top-K similar ebooks for the
// source book. Same behaviour as the audiobooks plugin's
// /api/items/{id}/similar: when the recommender isn't configured
// (no EMBEDDING_BASE_URL) the route returns an empty items array
// with 200 rather than 404, so the SPA can render an empty shelf.
func (s *Server) handleSimilarBooks(w http.ResponseWriter, r *http.Request) {
	ident, _ := auth.FromContext(r.Context())
	bookID := chi.URLParam(r, "id")
	if bookID == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	libraryID, err := s.resolveBookLibrary(r, bookID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	limit := 10
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 50 {
			limit = n
		}
	}
	rec := s.deps.Recommender
	if rec == nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}, "total": 0})
		return
	}
	candidates, err := rec.Similar(r.Context(), libraryID, bookID, limit)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}, "total": 0})
		return
	}
	bk, err := s.targetBackend(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	items := make([]any, 0, len(candidates))
	for _, c := range candidates {
		detail, err := bk.GetBook(r.Context(), c.BookID)
		if err != nil {
			continue
		}
		items = append(items, detail.EbookSummary)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":    items,
		"total":    len(items),
		"sortBy":   "relevance",
		"sortDesc": true,
		"userId":   ident.UserID,
	})
}

// resolveBookLibrary picks the library this book lives in. With the
// current single-backend model the answer is whichever portal_library
// has the matching backend_plugin_id — we look up via the configured
// backend rather than an explicit (book, library) index.
//
// Returns 0 when no library matches, which the caller will treat as
// "no embedding scope" and return empty results.
func (s *Server) resolveBookLibrary(r *http.Request, bookID string) (int64, error) {
	libs, err := s.deps.Store.ListPortalLibraries(r.Context(), true)
	if err != nil {
		return 0, err
	}
	if len(libs) == 0 {
		return 0, nil
	}
	// Single-library fast path: most deployments only have one ebook
	// library wired up. If exactly one exists, return it directly
	// rather than calling the backend for resolution.
	if len(libs) == 1 {
		return libs[0].ID, nil
	}
	// Multi-library case: look up which library the book lives in via
	// the embedding row itself. Free lookup since embeddings are
	// keyed by (book_id, library_id).
	for _, lib := range libs {
		if _, err := s.deps.Store.GetEbookEmbedding(r.Context(), lib.ID, bookID); err == nil {
			return lib.ID, nil
		}
	}
	// Fall back to the first library — covers the cold-start case
	// where the book isn't embedded yet.
	return libs[0].ID, nil
}
