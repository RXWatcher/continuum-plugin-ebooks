package server

import (
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/auth"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/enrich"
)

// Metadata enrichment surface — calls into OpenLibrary + Google
// Books in parallel and merges the candidate lists. Mirrors the
// audiobooks plugin's enrich endpoint.

func (s *Server) mountEnrichRoutes(r chi.Router) {
	r.Get("/admin/enrich/search", s.handleEnrichSearch)
}

func (s *Server) handleEnrichSearch(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	if !id.IsAdmin {
		http.Error(w, "admin required", http.StatusForbidden)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		http.Error(w, "q required", http.StatusBadRequest)
		return
	}
	limit := 10
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 25 {
			limit = n
		}
	}

	var (
		wg           sync.WaitGroup
		mu           sync.Mutex
		matches      []enrich.Match
		olErr, gbErr string
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		results, err := enrich.SearchOpenLibrary(r.Context(), query, limit)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			olErr = err.Error()
		} else {
			matches = append(matches, results...)
		}
	}()
	go func() {
		defer wg.Done()
		results, err := enrich.SearchGoogleBooks(r.Context(), query, limit)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			gbErr = err.Error()
		} else {
			matches = append(matches, results...)
		}
	}()
	wg.Wait()

	resp := map[string]any{"matches": matches}
	if olErr != "" {
		resp["openlibrary_error"] = olErr
	}
	if gbErr != "" {
		resp["google_books_error"] = gbErr
	}
	writeJSON(w, http.StatusOK, resp)
}
