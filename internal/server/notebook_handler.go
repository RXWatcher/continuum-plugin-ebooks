package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/auth"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/store"
)

// Notebook view — aggregated cross-book annotation list with
// filters. The reader-side annotation panel covers per-book; this
// is the "all my highlights / notes" surface backed by a single
// query.
//
// Filters:
//   color    — exact match (#FF8800 etc.)
//   style    — highlight | underline | squiggly
//   q        — substring search across selected_text + note_text
//   since    — unix ms lower bound on created_at
//   until    — unix ms upper bound
//   book_id  — restrict to one book
//   limit    — page size cap (default 500, max 2000)

func (s *Server) mountNotebookRoutes(r chi.Router) {
	r.Get("/me/notebook", s.handleNotebook)
}

func (s *Server) handleNotebook(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	f := store.NotebookFilters{
		Color:  strings.TrimSpace(r.URL.Query().Get("color")),
		Style:  strings.TrimSpace(r.URL.Query().Get("style")),
		Query:  strings.TrimSpace(r.URL.Query().Get("q")),
		BookID: strings.TrimSpace(r.URL.Query().Get("book_id")),
	}
	if v := r.URL.Query().Get("since"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			f.SinceMs = n
		}
	}
	if v := r.URL.Query().Get("until"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			f.UntilMs = n
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Limit = n
		}
	}
	rows, err := s.deps.Store.SearchAnnotations(r.Context(), id.UserID, f)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	// Group response by book_id so the SPA's notebook view can
	// render "Book Title (N notes)" headers without re-grouping.
	groups := make(map[string][]store.Annotation, 16)
	order := make([]string, 0, 16)
	for _, a := range rows {
		if _, seen := groups[a.BookID]; !seen {
			order = append(order, a.BookID)
		}
		groups[a.BookID] = append(groups[a.BookID], a)
	}
	out := make([]map[string]any, 0, len(order))
	for _, bid := range order {
		out = append(out, map[string]any{
			"book_id":     bid,
			"count":       len(groups[bid]),
			"annotations": groups[bid],
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"groups": out,
		"total":  len(rows),
	})
}

// _ = chi placeholder so the import sticks even when other
// notebook routes get added that don't take URL params.
var _ = chi.URLParam
