package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/auth"
)

// Year-in-review stats — books finished + distinct active days +
// top books. Mirrors the audiobooks plugin's /me/stats/year/{year}
// surface (which is time-based; this is books-based since the
// ebook plugin has no playback telemetry).

func (s *Server) mountYearStatsRoutes(r chi.Router) {
	r.Get("/me/stats/year/{year}", s.handleYearStats)
}

func (s *Server) handleYearStats(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	year, err := strconv.Atoi(chi.URLParam(r, "year"))
	if err != nil || year < 2000 || year > 2100 {
		http.Error(w, "invalid year", http.StatusBadRequest)
		return
	}
	stats, err := s.deps.Store.YearStatsForUser(r.Context(), id.UserID, year, time.UTC)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}
