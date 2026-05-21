package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/auth"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/store"
)

// Per-library settings for the ebooks plugin. Mirrors the
// audiobooks plugin's surface.

func (s *Server) mountLibrarySettingsRoutes(r chi.Router) {
	r.Get("/libraries/{id}/settings", s.handleGetLibrarySettings)
	r.Put("/admin/libraries/{id}/settings", s.handlePutLibrarySettings)
}

func (s *Server) handleGetLibrarySettings(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	ls, err := s.deps.Store.GetLibrarySettings(r.Context(), id)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ls)
}

func (s *Server) handlePutLibrarySettings(w http.ResponseWriter, r *http.Request) {
	ident, _ := auth.FromContext(r.Context())
	if !ident.IsAdmin {
		http.Error(w, "admin required", http.StatusForbidden)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	var body store.LibrarySettings
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if err := s.deps.Store.SetLibrarySettings(r.Context(), id, body); err != nil {
		writeInternal(w, r, err)
		return
	}
	s.audit(r, ident.UserID, "update_library_settings", "portal_library",
		strconv.FormatInt(id, 10), body)
	writeJSON(w, http.StatusOK, body)
}
