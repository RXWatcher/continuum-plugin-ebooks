package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/auth"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/store"
)

// Per-user notification preferences for the ebooks plugin. Mirrors
// the audiobooks shape with adapted categories (reading_reminder
// instead of new_episode).

func (s *Server) mountNotificationPrefRoutes(r chi.Router) {
	r.Get("/me/notification-prefs", s.handleListNotificationPrefs)
	r.Get("/me/notification-prefs/catalog", s.handleNotificationCatalog)
	r.Put("/me/notification-prefs", s.handlePutNotificationPref)
}

func (s *Server) handleListNotificationPrefs(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	rows, err := s.deps.Store.ListNotificationPrefs(r.Context(), id.UserID)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (s *Server) handleNotificationCatalog(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"categories": store.SupportedCategories,
		"deliveries": store.SupportedDeliveries,
	})
}

func (s *Server) handlePutNotificationPref(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	var body struct {
		Category string `json:"category"`
		Delivery string `json:"delivery"`
		Enabled  bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if err := s.deps.Store.UpsertNotificationPref(r.Context(), store.NotificationPref{
		UserID:   id.UserID,
		Category: body.Category,
		Delivery: body.Delivery,
		Enabled:  body.Enabled,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// chi import kept for future routes with path params.
var _ = chi.URLParam
