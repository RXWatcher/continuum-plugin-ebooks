package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/auth"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/backend"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/store"
)

// Content-restriction admin surface for the ebooks plugin. Same
// shape as the audiobooks plugin minus narrators.

func (s *Server) mountContentRestrictionRoutes(r chi.Router) {
	r.Get("/admin/content-restrictions", s.handleListContentRestrictions)
	r.Get("/admin/content-restrictions/{userId}", s.handleGetContentRestriction)
	r.Put("/admin/content-restrictions/{userId}", s.handlePutContentRestriction)
	r.Delete("/admin/content-restrictions/{userId}", s.handleDeleteContentRestriction)
	r.Get("/me/content-restriction", s.handleGetMyContentRestriction)
}

type contentRestrictionBody struct {
	BlockedGenres    []string `json:"blocked_genres"`
	BlockedTags      []string `json:"blocked_tags"`
	BlockedAuthors   []string `json:"blocked_authors"`
	BlockedLibraries []int64  `json:"blocked_libraries"`
	ExplicitBlocked  bool     `json:"explicit_blocked"`
}

func (s *Server) handleListContentRestrictions(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	if !id.IsAdmin {
		http.Error(w, "admin required", http.StatusForbidden)
		return
	}
	rows, err := s.deps.Store.ListContentRestrictions(r.Context())
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (s *Server) handleGetContentRestriction(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	if !id.IsAdmin {
		http.Error(w, "admin required", http.StatusForbidden)
		return
	}
	userID := chi.URLParam(r, "userId")
	row, err := s.deps.Store.GetContentRestriction(r.Context(), userID)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusOK, store.ContentRestriction{UserID: userID})
		return
	}
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (s *Server) handlePutContentRestriction(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	if !id.IsAdmin {
		http.Error(w, "admin required", http.StatusForbidden)
		return
	}
	userID := chi.URLParam(r, "userId")
	var body contentRestrictionBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	row := store.ContentRestriction{
		UserID:           userID,
		BlockedGenres:    body.BlockedGenres,
		BlockedTags:      body.BlockedTags,
		BlockedAuthors:   body.BlockedAuthors,
		BlockedLibraries: body.BlockedLibraries,
		ExplicitBlocked:  body.ExplicitBlocked,
	}
	if err := s.deps.Store.UpsertContentRestriction(r.Context(), row); err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (s *Server) handleDeleteContentRestriction(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	if !id.IsAdmin {
		http.Error(w, "admin required", http.StatusForbidden)
		return
	}
	userID := chi.URLParam(r, "userId")
	if err := s.deps.Store.DeleteContentRestriction(r.Context(), userID); err != nil {
		writeInternal(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetMyContentRestriction(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	row, err := s.deps.Store.GetContentRestriction(r.Context(), id.UserID)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusOK, store.ContentRestriction{UserID: id.UserID})
		return
	}
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

// ApplyContentRestriction filters out ebook summaries blocked by the
// user's content_restriction row (if any). Fail-open on error — a
// transient store outage shouldn't black out the catalog.
func (s *Server) ApplyContentRestriction(r *http.Request, userID string, libraryID int64, items []backend.EbookSummary) []backend.EbookSummary {
	if s.deps.Store == nil || userID == "" {
		return items
	}
	restriction, err := s.deps.Store.GetContentRestriction(r.Context(), userID)
	if err != nil {
		return items
	}
	if restriction.UserID == "" {
		return items
	}
	out := items[:0]
	for _, item := range items {
		if restriction.AllowsItem(libraryID, nil, nil, item.Authors, false) {
			out = append(out, item)
		}
	}
	return out
}
