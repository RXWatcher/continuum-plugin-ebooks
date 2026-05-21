package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/RXWatcher/continuum-plugin-ebooks/internal/auth"
	"github.com/RXWatcher/continuum-plugin-ebooks/internal/hardcover"
	"github.com/RXWatcher/continuum-plugin-ebooks/internal/store"
)

// Hardcover.app integration routes — per-user token + push-status
// endpoint. ISBN-based lookup so the SPA doesn't have to manage
// Hardcover edition ids; the lookup happens server-side from the
// book's metadata.

func (s *Server) mountHardcoverRoutes(r chi.Router) {
	r.Get("/me/hardcover/token", s.handleGetHardcoverToken)
	r.Put("/me/hardcover/token", s.handlePutHardcoverToken)
	r.Delete("/me/hardcover/token", s.handleDeleteHardcoverToken)
	r.Post("/me/hardcover/auth-check", s.handleHardcoverAuthCheck)
	r.Post("/me/books/{id}/hardcover/status", s.handleHardcoverPushStatus)
}

func (s *Server) handleGetHardcoverToken(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	tok, err := s.deps.Store.GetHardcoverToken(r.Context(), id.UserID)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusOK, map[string]any{"configured": false})
		return
	}
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"configured": true,
		"masked":     maskToken(tok),
	})
}

func (s *Server) handlePutHardcoverToken(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	tok := strings.TrimSpace(body.Token)
	if tok == "" {
		http.Error(w, "token required", http.StatusBadRequest)
		return
	}
	if err := s.deps.Store.SetHardcoverToken(r.Context(), id.UserID, tok); err != nil {
		writeInternal(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteHardcoverToken(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	if err := s.deps.Store.DeleteHardcoverToken(r.Context(), id.UserID); err != nil {
		writeInternal(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleHardcoverAuthCheck(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	tok, err := s.deps.Store.GetHardcoverToken(r.Context(), id.UserID)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "token not configured", http.StatusBadRequest)
		return
	}
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	client := hardcover.New(tok)
	username, err := client.AuthCheck(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "username": username})
}

// handleHardcoverPushStatus sets the user's Hardcover status on one
// book. Body: {"status": "want_to_read" | "currently_reading" |
// "read"}. The server resolves the book's ISBN via the backend
// then looks up the Hardcover edition id; ISBNless books return
// 400 with a clear message.
func (s *Server) handleHardcoverPushStatus(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	bookID := chi.URLParam(r, "id")

	tok, err := s.deps.Store.GetHardcoverToken(r.Context(), id.UserID)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "Hardcover token not configured", http.StatusBadRequest)
		return
	}
	if err != nil {
		writeInternal(w, r, err)
		return
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	body.Status = strings.TrimSpace(body.Status)
	if body.Status == "" {
		body.Status = "currently_reading"
	}

	bk, err := s.targetBackend(r)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	detail, err := bk.GetBook(r.Context(), bookID)
	if err != nil {
		http.Error(w, "book lookup failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	if detail.ISBN == "" {
		http.Error(w, "book has no ISBN — cannot sync to Hardcover", http.StatusBadRequest)
		return
	}

	client := hardcover.New(tok)
	editionID, err := client.LookupByISBN(r.Context(), detail.ISBN)
	if err != nil {
		http.Error(w, "Hardcover ISBN lookup failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	if editionID == 0 {
		http.Error(w, "no Hardcover edition matches this book's ISBN", http.StatusNotFound)
		return
	}

	userBookID, err := client.PushBookStatus(r.Context(), editionID, body.Status)
	if err != nil {
		http.Error(w, "Hardcover status push failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"edition_id":   editionID,
		"user_book_id": userBookID,
		"status":       body.Status,
	})
}
