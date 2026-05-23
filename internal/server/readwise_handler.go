package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/RXWatcher/silo-plugin-ebooks/internal/auth"
	"github.com/RXWatcher/silo-plugin-ebooks/internal/readwise"
	"github.com/RXWatcher/silo-plugin-ebooks/internal/store"
)

// Readwise integration routes — per-user token storage + export
// endpoint that pushes the user's annotations on one book (or all
// books) to readwise.io.

func (s *Server) mountReadwiseRoutes(r chi.Router) {
	r.Get("/me/readwise/token", s.handleGetReadwiseToken)
	r.Put("/me/readwise/token", s.handlePutReadwiseToken)
	r.Delete("/me/readwise/token", s.handleDeleteReadwiseToken)
	r.Post("/me/readwise/auth-check", s.handleReadwiseAuthCheck)
	r.Post("/me/books/{id}/readwise/export", s.handleReadwiseExportBook)
}

// handleGetReadwiseToken returns {configured: bool, masked: "...xyz"}
// — never the full token, so reading the route doesn't leak the
// secret to JS that shouldn't have it.
func (s *Server) handleGetReadwiseToken(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	tok, err := s.deps.Store.GetReadwiseToken(r.Context(), id.UserID)
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

func (s *Server) handlePutReadwiseToken(w http.ResponseWriter, r *http.Request) {
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
	if err := s.deps.Store.SetReadwiseToken(r.Context(), id.UserID, tok); err != nil {
		writeInternal(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteReadwiseToken(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	if err := s.deps.Store.DeleteReadwiseToken(r.Context(), id.UserID); err != nil {
		writeInternal(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleReadwiseAuthCheck verifies the stored token. Returns 200
// when Readwise accepts the token, 401 when rejected, 502 on
// network failure.
func (s *Server) handleReadwiseAuthCheck(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	tok, err := s.deps.Store.GetReadwiseToken(r.Context(), id.UserID)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "token not configured", http.StatusBadRequest)
		return
	}
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	client := readwise.New(tok)
	if err := client.AuthCheck(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleReadwiseExportBook pushes every annotation on one book to
// the user's Readwise account. The body contains no fields; the
// path identifies the book and the auth token identifies the user.
// Returns {pushed: N, total: M} so the SPA can show "exported N of
// M highlights" rather than a binary success/failure.
func (s *Server) handleReadwiseExportBook(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	bookID := chi.URLParam(r, "id")

	tok, err := s.deps.Store.GetReadwiseToken(r.Context(), id.UserID)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "Readwise token not configured", http.StatusBadRequest)
		return
	}
	if err != nil {
		writeInternal(w, r, err)
		return
	}

	annotations, err := s.deps.Store.ListAnnotationsByBook(r.Context(), id.UserID, bookID)
	if err != nil {
		writeInternal(w, r, err)
		return
	}

	// Title + authors enrich the Readwise highlight; fetch the
	// detail once and reuse across the batch.
	bk, err := s.targetBackend(r)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	detail, err := bk.GetBook(r.Context(), bookID)
	if err != nil {
		http.Error(w, "failed to fetch book metadata: "+err.Error(), http.StatusBadGateway)
		return
	}
	title := detail.Title
	author := strings.Join(detail.Authors, ", ")

	highlights := make([]readwise.Highlight, 0, len(annotations))
	for _, a := range annotations {
		if a.DeletedAt != nil {
			continue
		}
		text := a.SelectedText
		if text == "" {
			// Readwise rejects highlights without text — skip
			// bookmark-only annotations.
			continue
		}
		h := readwise.Highlight{
			Text:       text,
			Title:      title,
			Author:     author,
			SourceType: "silo-ebooks",
			Category:   "books",
			Note:       a.NoteText,
		}
		if a.Page != nil && *a.Page > 0 {
			h.Location = *a.Page
			h.LocationType = "page"
		}
		if !a.CreatedAt.IsZero() {
			h.HighlightedAt = a.CreatedAt.UTC().Format("2006-01-02T15:04:05Z")
		}
		highlights = append(highlights, h)
	}

	client := readwise.New(tok)
	pushed, err := client.Push(r.Context(), highlights)
	if err != nil {
		// Partial success: return both the count and the error so
		// the SPA can surface "exported 35 of 40, then failed".
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"pushed": pushed,
			"total":  len(highlights),
			"error":  err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"pushed": pushed,
		"total":  len(highlights),
	})
}

// maskToken returns "********xyz" — last 3 chars of the token,
// prefixed with 8 asterisks. Just enough for the user to identify
// which token they pasted without exposing the secret.
func maskToken(tok string) string {
	if len(tok) <= 3 {
		return strings.Repeat("*", 8)
	}
	return strings.Repeat("*", 8) + tok[len(tok)-3:]
}
