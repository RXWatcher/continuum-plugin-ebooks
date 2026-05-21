package server

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/auth"
)

// Personal data export — one-click ZIP of everything the plugin
// stores for the requesting user. Same shape as the audiobooks
// plugin's export.
//
// Ebook coverage:
//   user_data.json            — progress + finished flag + ratings + notes
//   annotations.json          — highlights / underlines / notes
//   collections.json          — manual collections + item lists
//   smart_collections.json    — smart collection definitions
//   reading_goals.json        — yearly targets
//   share_links.json          — owner-side share metadata
//   ereader_devices.json      — registered Kindle/Kobo destinations

func (s *Server) mountExportRoutes(r chi.Router) {
	r.Get("/me/export", s.handleExport)
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	filename := fmt.Sprintf("continuum-ebooks-export-%s-%s.zip",
		id.UserID, time.Now().UTC().Format("20060102"))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")

	zw := zip.NewWriter(w)
	defer zw.Close()

	errs := map[string]string{}
	add := func(name string, data any, err error) {
		if err != nil {
			errs[name] = err.Error()
			return
		}
		if werr := writeExportSection(zw, name, data); werr != nil {
			errs[name] = werr.Error()
		}
	}

	ctx := r.Context()
	progress, err := s.deps.Store.ListRecentByUser(ctx, id.UserID, 100000)
	add("user_data.json", progress, err)

	annotations, err := s.deps.Store.ListAnnotationsByUser(ctx, id.UserID, 100000)
	add("annotations.json", annotations, err)

	collections, err := s.deps.Store.ListCollectionsByUser(ctx, id.UserID)
	add("collections.json", collections, err)

	smart, err := s.deps.Store.ListSmartCollections(ctx, id.UserID, 1000)
	add("smart_collections.json", smart, err)

	goals, err := s.deps.Store.ListReadingGoals(ctx, id.UserID, 0)
	add("reading_goals.json", goals, err)

	shares, err := s.deps.Store.ListShareLinks(ctx, id.UserID)
	add("share_links.json", shares, err)

	devices, err := s.deps.Store.ListEreaderDevices(ctx, id.UserID)
	add("ereader_devices.json", devices, err)

	if len(errs) > 0 {
		_ = writeExportSection(zw, "_errors.json", errs)
	}
	_ = writeExportSection(zw, "_manifest.json", map[string]any{
		"plugin":      "continuum-ebooks",
		"user_id":     id.UserID,
		"exported_at": time.Now().UTC().Format(time.RFC3339),
		"sections": []string{
			"user_data", "annotations", "collections", "smart_collections",
			"reading_goals", "share_links", "ereader_devices",
		},
	})
}

func writeExportSection(zw *zip.Writer, name string, data any) error {
	f, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("create %s: %w", name, err)
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		return fmt.Errorf("encode %s: %w", name, err)
	}
	return nil
}

// _ = chi placeholder so the import stays even before we add path
// params to the export surface.
var _ = chi.URLParam
