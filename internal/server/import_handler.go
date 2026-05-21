package server

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/auth"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/store"
)

// Restore-from-export for the ebooks plugin. Mirrors the
// audiobooks plugin's /me/import — additive merge with manifest
// validation. Sections supported: smart_collections,
// reading_goals, share_links, ereader_devices.

const maxImportBytes = 20 << 20

func (s *Server) mountImportRoutes(r chi.Router) {
	r.Post("/me/import", s.handleImport)
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	if err := r.ParseMultipartForm(maxImportBytes); err != nil {
		http.Error(w, "invalid multipart: "+err.Error(), http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file part required", http.StatusBadRequest)
		return
	}
	defer file.Close()
	if header.Size > maxImportBytes {
		http.Error(w, "import too large (20 MB max)", http.StatusRequestEntityTooLarge)
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, maxImportBytes+1))
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	zr, err := zip.NewReader(&bytesReadAt{data: data}, int64(len(data)))
	if err != nil {
		http.Error(w, "invalid zip: "+err.Error(), http.StatusBadRequest)
		return
	}
	entries := make(map[string][]byte, len(zr.File))
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(rc, maxImportBytes))
		_ = rc.Close()
		entries[f.Name] = b
	}

	manifestBytes, ok := entries["_manifest.json"]
	if !ok {
		http.Error(w, "missing _manifest.json", http.StatusBadRequest)
		return
	}
	var manifest struct {
		Plugin string `json:"plugin"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		http.Error(w, "invalid manifest: "+err.Error(), http.StatusBadRequest)
		return
	}
	if manifest.Plugin != "continuum-ebooks" {
		http.Error(w, "manifest plugin "+manifest.Plugin+" doesn't match ebooks plugin", http.StatusBadRequest)
		return
	}

	counts := map[string]int{}
	importEbookSection(entries, "smart_collections.json", &counts, "smart_collections",
		func(items []store.SmartCollection) {
			for _, c := range items {
				c.UserID = id.UserID
				if err := s.deps.Store.UpsertSmartCollection(r.Context(), c); err == nil {
					counts["smart_collections"]++
				}
			}
		})
	importEbookSection(entries, "reading_goals.json", &counts, "reading_goals",
		func(items []store.ReadingGoal) {
			for _, g := range items {
				g.UserID = id.UserID
				if err := s.deps.Store.UpsertReadingGoal(r.Context(), g); err == nil {
					counts["reading_goals"]++
				}
			}
		})
	importEbookSection(entries, "share_links.json", &counts, "share_links",
		func(items []store.ShareLink) {
			for _, l := range items {
				l.UserID = id.UserID
				if err := s.deps.Store.CreateShareLink(r.Context(), l); err == nil {
					counts["share_links"]++
				}
			}
		})
	importEbookSection(entries, "ereader_devices.json", &counts, "ereader_devices",
		func(items []store.EreaderDevice) {
			for _, d := range items {
				d.UserID = id.UserID
				if err := s.deps.Store.UpsertEreaderDevice(r.Context(), d); err == nil {
					counts["ereader_devices"]++
				}
			}
		})

	s.audit(r, id.UserID, "import", "personal_data", "", counts)
	writeJSON(w, http.StatusOK, map[string]any{
		"imported_at": time.Now().UTC().Format(time.RFC3339),
		"counts":      counts,
	})
}

func importEbookSection[T any](entries map[string][]byte, name string, counts *map[string]int, label string, handler func([]T)) {
	_ = label
	raw, ok := entries[name]
	if !ok || len(raw) == 0 {
		return
	}
	var items []T
	if err := json.Unmarshal(raw, &items); err != nil {
		return
	}
	handler(items)
}

type bytesReadAt struct{ data []byte }

func (r *bytesReadAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= int64(len(r.data)) {
		return 0, errors.New("read out of range")
	}
	n := copy(p, r.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// _ = chi placeholder so the import stays consistent with sibling
// handler files in this package.
var _ = chi.URLParam
