package server

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/auth"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/store"
)

// Time-limited share links for ebooks. Symmetric port of the
// audiobooks plugin's surface — owner mints a slug + optional TTL +
// optional use cap; recipients hit /share/{slug} without auth.

func (s *Server) mountShareLinkRoutes(r chi.Router) {
	r.Get("/me/share-links", s.handleListShareLinks)
	r.Post("/me/share-links", s.handleCreateShareLink)
	r.Delete("/me/share-links/{id}", s.handleDeleteShareLink)
}

// MountPublicShare registers /share/{slug} OUTSIDE the auth group.
func (s *Server) MountPublicShare(r chi.Router) {
	r.Get("/share/{slug}", s.handleResolveShareLink)
}

type shareLinkBody struct {
	ItemID   string `json:"item_id"`
	TTLHours int    `json:"ttl_hours"`
	MaxUses  int    `json:"max_uses"`
}

func (s *Server) handleListShareLinks(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	rows, err := s.deps.Store.ListShareLinks(r.Context(), id.UserID)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (s *Server) handleCreateShareLink(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	var body shareLinkBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.ItemID == "" {
		http.Error(w, "item_id required", http.StatusBadRequest)
		return
	}
	slug, err := mintShareSlug()
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	link := store.ShareLink{
		ID:      ulid.Make().String(),
		UserID:  id.UserID,
		Slug:    slug,
		ItemID:  body.ItemID,
		MaxUses: body.MaxUses,
	}
	if body.TTLHours > 0 {
		exp := time.Now().Add(time.Duration(body.TTLHours) * time.Hour)
		link.ExpiresAt = &exp
	}
	if err := s.deps.Store.CreateShareLink(r.Context(), link); err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, link)
}

func (s *Server) handleDeleteShareLink(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	if err := s.deps.Store.DeleteShareLink(r.Context(), chi.URLParam(r, "id"), id.UserID); err != nil {
		writeInternal(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleResolveShareLink(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	link, err := s.deps.Store.GetActiveShareLinkBySlug(r.Context(), slug)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "share link not found or expired", http.StatusNotFound)
		return
	}
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	_ = s.deps.Store.IncrementShareUse(r.Context(), link.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"slug":       link.Slug,
		"item_id":    link.ItemID,
		"expires_at": link.ExpiresAt,
		"created_at": link.CreatedAt,
	})
}

func mintShareSlug() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
