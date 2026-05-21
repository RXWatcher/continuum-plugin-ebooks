package server

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/auth"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/store"
)

// Per-user custom fonts for the ebook reader. Users upload a TTF /
// WOFF / WOFF2 / OTF blob via multipart POST; the reader's font
// picker fetches the metadata list and lazily loads each blob via
// /me/fonts/{id}/data when the user selects it.
//
// Size cap: 5 MB. Average reader font is under 500 KB; the cap
// guards against accidentally uploading a 50-MB icon set as a
// "font."

const maxFontUploadBytes = 5 << 20

var allowedFontMIMEs = map[string]bool{
	"font/ttf":                    true,
	"font/otf":                    true,
	"font/woff":                   true,
	"font/woff2":                  true,
	"application/font-woff":       true,
	"application/x-font-ttf":      true,
	"application/octet-stream":    true, // some browsers send this for fonts; whitelist gates by extension below too
}

var allowedFontExts = map[string]bool{
	".ttf":   true,
	".otf":   true,
	".woff":  true,
	".woff2": true,
}

func (s *Server) mountCustomFontRoutes(r chi.Router) {
	r.Get("/me/fonts", s.handleListCustomFonts)
	r.Post("/me/fonts", s.handleUploadCustomFont)
	r.Get("/me/fonts/{id}/data", s.handleGetCustomFontData)
	r.Delete("/me/fonts/{id}", s.handleDeleteCustomFont)
}

func (s *Server) handleListCustomFonts(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	rows, err := s.deps.Store.ListCustomFonts(r.Context(), id.UserID)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, f := range rows {
		out = append(out, map[string]any{
			"id":         f.ID,
			"name":       f.Name,
			"mime":       f.MIME,
			"size_bytes": f.SizeBytes,
			"created_at": f.CreatedAt.UnixMilli(),
			// Relative to the plugin's API root (/api/v1). The SPA
			// prepends apiBase() before injecting into @font-face so
			// the URL routes through the host proxy correctly.
			"url": "/me/fonts/" + f.ID + "/data",
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// handleUploadCustomFont accepts multipart/form-data with a "file"
// part. Form fields: name (font-family the CSS @font-face emits;
// defaults to the filename minus extension).
func (s *Server) handleUploadCustomFont(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	if err := r.ParseMultipartForm(maxFontUploadBytes); err != nil {
		http.Error(w, "invalid multipart: "+err.Error(), http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file part required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	ext := strings.ToLower(extOf(header.Filename))
	if !allowedFontExts[ext] {
		http.Error(w, "unsupported font extension; need .ttf/.otf/.woff/.woff2", http.StatusUnsupportedMediaType)
		return
	}
	mime := header.Header.Get("Content-Type")
	if mime != "" && !allowedFontMIMEs[mime] {
		http.Error(w, "unsupported font mime "+mime, http.StatusUnsupportedMediaType)
		return
	}
	if header.Size > maxFontUploadBytes {
		http.Error(w, "font too large (5 MB max)", http.StatusRequestEntityTooLarge)
		return
	}

	data, err := io.ReadAll(io.LimitReader(file, maxFontUploadBytes+1))
	if err != nil {
		http.Error(w, "read failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if len(data) > maxFontUploadBytes {
		http.Error(w, "font too large (5 MB max)", http.StatusRequestEntityTooLarge)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		name = strings.TrimSuffix(header.Filename, ext)
	}
	if name == "" {
		name = "Custom"
	}
	if mime == "" {
		mime = mimeForExt(ext)
	}

	font := store.CustomFont{
		ID:        ulid.Make().String(),
		UserID:    id.UserID,
		Name:      name,
		MIME:      mime,
		SizeBytes: len(data),
		Data:      data,
	}
	if err := s.deps.Store.InsertCustomFont(r.Context(), font); err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         font.ID,
		"name":       font.Name,
		"mime":       font.MIME,
		"size_bytes": font.SizeBytes,
		"url":        "/me/fonts/" + font.ID + "/data",
	})
}

// handleGetCustomFontData streams the raw font bytes with the
// stored MIME + a long Cache-Control. The CSS @font-face the
// reader emits points at this URL; browsers cache it aggressively
// once they've fetched it once.
func (s *Server) handleGetCustomFontData(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	f, err := s.deps.Store.GetCustomFontData(r.Context(), chi.URLParam(r, "id"), id.UserID)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	w.Header().Set("Content-Type", f.MIME)
	w.Header().Set("Cache-Control", "private, max-age=86400, immutable")
	w.Header().Set("Content-Length", intToStr(len(f.Data)))
	_, _ = w.Write(f.Data)
}

func (s *Server) handleDeleteCustomFont(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	if err := s.deps.Store.DeleteCustomFont(r.Context(), chi.URLParam(r, "id"), id.UserID); err != nil {
		writeInternal(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// extOf returns the lowercased extension including the leading
// dot, or empty when there's none.
func extOf(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return strings.ToLower(name[i:])
	}
	return ""
}

// mimeForExt picks a default Content-Type when the upload didn't
// supply one (some old browsers omit it for less-common extensions).
func mimeForExt(ext string) string {
	switch ext {
	case ".ttf":
		return "font/ttf"
	case ".otf":
		return "font/otf"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	}
	return "application/octet-stream"
}

// intToStr keeps the file lean — Content-Length needs a string, but
// adding strconv just for this is a yak shave. itoa-ish loop.
func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// (Compile-time guard against accidental unused-time-import warnings
// if the file's first revision drops the timestamp formatter; safer
// to keep until the final shape stabilises.)
var _ = time.Now
