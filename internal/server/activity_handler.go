package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/RXWatcher/silo-plugin-ebooks/internal/auth"
)

// Per-book activity timeline for the ebook reader's detail page.
// Mirrors the audiobooks plugin's surface; payload kinds adapted
// for the ebook domain (annotations + finished + rated; no
// session_opened/closed since the ebook reader is page-based
// rather than session-based).

func (s *Server) mountActivityRoutes(r chi.Router) {
	r.Get("/me/books/{id}/activity", s.handleBookActivity)
}

func (s *Server) handleBookActivity(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	bookID := chi.URLParam(r, "id")
	events, err := s.deps.Store.BookActivity(r.Context(), id.UserID, bookID)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}
