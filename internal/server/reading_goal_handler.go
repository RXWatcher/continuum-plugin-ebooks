package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/RXWatcher/continuum-plugin-ebooks/internal/auth"
	"github.com/RXWatcher/continuum-plugin-ebooks/internal/store"
)

// Per-user yearly reading goals for the ebook plugin. Books-count
// only — there's no "hours read" measure on the ebook side (no
// timed playback like the audiobook player).

func (s *Server) mountReadingGoalRoutes(r chi.Router) {
	r.Get("/me/goals", s.handleListGoals)
	r.Put("/me/goals/{year}/{kind}", s.handlePutGoal)
	r.Delete("/me/goals/{year}/{kind}", s.handleDeleteGoal)
	r.Get("/me/goals/progress", s.handleGoalProgress)
}

func (s *Server) handleListGoals(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	year := 0
	if v := r.URL.Query().Get("year"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			year = n
		}
	}
	rows, err := s.deps.Store.ListReadingGoals(r.Context(), id.UserID, year)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (s *Server) handlePutGoal(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	year, err := strconv.Atoi(chi.URLParam(r, "year"))
	if err != nil {
		http.Error(w, "invalid year", http.StatusBadRequest)
		return
	}
	kind := chi.URLParam(r, "kind")
	var body struct {
		Target int `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	g := store.ReadingGoal{
		UserID: id.UserID,
		Year:   year,
		Kind:   kind,
		Target: body.Target,
	}
	if err := s.deps.Store.UpsertReadingGoal(r.Context(), g); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) handleDeleteGoal(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	year, err := strconv.Atoi(chi.URLParam(r, "year"))
	if err != nil {
		http.Error(w, "invalid year", http.StatusBadRequest)
		return
	}
	if err := s.deps.Store.DeleteReadingGoal(r.Context(), id.UserID, year, chi.URLParam(r, "kind")); err != nil {
		writeInternal(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGoalProgress(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	year := time.Now().UTC().Year()
	if v := r.URL.Query().Get("year"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 2000 {
			year = n
		}
	}
	prog, err := s.deps.Store.GoalProgressForUser(r.Context(), id.UserID, year, time.UTC)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"year":  year,
		"goals": prog,
	})
}
