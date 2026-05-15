package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/auth"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/backend"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/store"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/streaming"
)

func (s *Server) mountUserRoutes(r chi.Router) {
	// Identity-scoped reading data
	r.Get("/me/library", s.handleLibrary)
	r.Get("/me/progress", s.handleRecentProgress)
	r.Get("/me/books/{id}", s.handleGetBookUserData)
	r.Post("/me/books/{id}/progress", s.handleUpdateProgress)
	r.Patch("/me/books/{id}", s.handleUpdateBookMeta)
	r.Get("/me/books/{id}/file", s.handleStreamFile)
	r.Get("/me/books/{id}/annotations", s.handleListAnnotations)
	r.Post("/me/books/{id}/annotations", s.handleCreateAnnotation)
	r.Patch("/me/annotations/{annId}", s.handleUpdateAnnotation)
	r.Delete("/me/annotations/{annId}", s.handleDeleteAnnotation)

	// Catalog (proxied to backend)
	r.Get("/libraries", s.handleListLibraries)
	r.Get("/ebooks", s.handleListCatalog)
	r.Get("/ebooks/{id}", s.handleGetBook)
	r.Get("/ebooks/search", s.handleSearchCatalog)

	// Browse facets (proxied to backend; backends without browse degrade to empty)
	r.Get("/browse/authors", s.handleBrowseAuthors)
	r.Get("/browse/series", s.handleBrowseSeries)
	r.Get("/browse/genres", s.handleBrowseGenres)
	r.Get("/request-routing/preview", s.handleRequestRoutingPreview)

	// Requests
	r.Get("/me/requests", s.handleListMyRequests)
	r.Get("/me/requests/{id}", s.handleGetMyRequest)
	r.Post("/me/requests", s.handleCreateRequest)
	r.Delete("/me/requests/{id}", s.handleCancelRequest)

	// Collections
	r.Get("/me/collections", s.handleListMyCollections)
	r.Post("/me/collections", s.handleCreateCollection)
	r.Patch("/me/collections/{id}", s.handleUpdateCollection)
	r.Delete("/me/collections/{id}", s.handleDeleteCollection)
	r.Get("/me/collections/{id}/items", s.handleListCollectionItems)
	r.Post("/me/collections/{id}/items", s.handleAddCollectionItem)
	r.Delete("/me/collections/{id}/items/{bookId}", s.handleRemoveCollectionItem)

	// Kobo / Kindle / OPDS / Kosync user management
	r.Post("/me/books/{id}/send-to-kindle", s.handleSendToKindle)
	r.Post("/me/books/{id}/send-to-kobo", s.handleSendToKobo)
	r.Get("/me/opds-tokens", s.handleListOPDSTokens)
	r.Post("/me/opds-tokens", s.handleCreateOPDSToken)
	r.Delete("/me/opds-tokens/{id}", s.handleRevokeOPDSToken)
	r.Get("/me/kosync", s.handleKosyncStatus)
	r.Post("/me/kosync/register", s.handleKosyncRegister)
	r.Delete("/me/kosync", s.handleKosyncDelete)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": map[string]any{"message": msg}})
}

func (s *Server) targetBackend(r *http.Request) (*backend.EbookBackend, error) {
	lib, err := s.deps.Store.DefaultPortalLibrary(r.Context())
	if err == nil {
		return backend.NewEbookBackend(s.deps.Host, lib.BackendPluginID), nil
	}
	cfg, err := s.deps.Store.GetConfig(r.Context())
	if err != nil {
		return nil, err
	}
	if cfg.TargetBackendPluginID == "" {
		return nil, fmt.Errorf("no backend configured")
	}
	return backend.NewEbookBackend(s.deps.Host, cfg.TargetBackendPluginID), nil
}

func (s *Server) targetLibrary(r *http.Request, libraryID int64) (store.PortalLibrary, error) {
	if libraryID > 0 {
		return s.deps.Store.GetPortalLibrary(r.Context(), libraryID)
	}
	lib, err := s.deps.Store.DefaultPortalLibrary(r.Context())
	if err == nil {
		return lib, nil
	}
	cfg, err := s.deps.Store.GetConfig(r.Context())
	if err != nil || cfg.TargetBackendPluginID == "" {
		return store.PortalLibrary{}, fmt.Errorf("no backend configured")
	}
	return store.PortalLibrary{
		Name:            "Library",
		MediaType:       "book",
		BackendPluginID: cfg.TargetBackendPluginID,
		Enabled:         true,
	}, nil
}

func backendLibraryID(lib store.PortalLibrary) int64 {
	if lib.BackendLibraryID == nil {
		return 0
	}
	return *lib.BackendLibraryID
}

func wrapCatalogItems(env backend.PageEnvelope[backend.EbookSummary], lib store.PortalLibrary) backend.PageEnvelope[backend.EbookSummary] {
	for i := range env.Items {
		env.Items[i].ID = encodeBookRef(lib.ID, env.Items[i].ID)
		env.Items[i].LibraryID = lib.ID
		env.Items[i].LibraryName = lib.Name
		env.Items[i].MediaType = lib.MediaType
	}
	return env
}

// -- library/progress/annotations -----------------------------------------

func (s *Server) handleLibrary(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	status := r.URL.Query().Get("status")
	rows, err := s.deps.Store.ListByUser(r.Context(), id.UserID, status, 100)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"items": rows})
}

func (s *Server) handleRecentProgress(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	rows, err := s.deps.Store.ListRecentByUser(r.Context(), id.UserID, 20)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"items": rows})
}

func (s *Server) handleGetBookUserData(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	bookID := chi.URLParam(r, "id")
	row, err := s.deps.Store.GetUserData(r.Context(), id.UserID, bookID)
	if err != nil {
		if err == store.ErrNotFound {
			writeJSON(w, 200, store.UserData{UserID: id.UserID, BookID: bookID})
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, row)
}

func (s *Server) handleUpdateProgress(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	bookID := chi.URLParam(r, "id")
	var body struct {
		LastCFI      string  `json:"last_cfi"`
		CurrentPage  int     `json:"current_page"`
		ReadProgress float64 `json:"read_progress"`
		IsFinished   bool    `json:"is_finished"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	now := time.Now()
	if err := s.deps.Store.UpsertUserData(r.Context(), store.UserData{
		UserID: id.UserID, BookID: bookID,
		LastCFI: body.LastCFI, CurrentPage: body.CurrentPage,
		ReadProgress: body.ReadProgress, IsFinished: body.IsFinished,
		LastReadAt: &now,
	}); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleUpdateBookMeta(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	bookID := chi.URLParam(r, "id")
	var body struct {
		IsFavorite *bool   `json:"is_favorite"`
		Rating     *int    `json:"rating"`
		Notes      *string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	cur, _ := s.deps.Store.GetUserData(r.Context(), id.UserID, bookID)
	cur.UserID = id.UserID
	cur.BookID = bookID
	if body.IsFavorite != nil {
		cur.IsFavorite = *body.IsFavorite
	}
	if body.Rating != nil {
		cur.Rating = body.Rating
	}
	if body.Notes != nil {
		cur.Notes = *body.Notes
	}
	if err := s.deps.Store.UpsertUserData(r.Context(), cur); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// handleStreamFile dispatches to the configured streaming mode. "proxy"
// pipes bytes live; "cache" looks up (and on miss, single-flight downloads)
// the file into the LRU-managed cache.
func (s *Server) handleStreamFile(w http.ResponseWriter, r *http.Request) {
	bookRef := chi.URLParam(r, "id")
	libraryID, bookID, _ := decodeBookRef(bookRef)
	lib, err := s.targetLibrary(r, libraryID)
	if err != nil {
		writeErr(w, 412, err.Error())
		return
	}
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "epub"
	}
	cfg, err := s.deps.Store.GetConfig(r.Context())
	if err != nil {
		writeErr(w, 412, "no backend configured")
		return
	}
	mode := streaming.ResolveMode(cfg)
	if mode == "cache" && s.deps.CacheManager != nil {
		s.cacheStream(w, r, lib.BackendPluginID, bookID, format)
		return
	}
	streaming.ProxyStream(w, r, s.deps.Host, lib.BackendPluginID, bookID, format)
}

// cacheStream implements the cache-mode handler. Cache hit → http.ServeFile;
// miss → single-flight download via streaming.Manager, then serve.
//
// The serve path acquires an in-process refcount on the entry for the duration
// of the response so the eviction sweeper can't unlink the file mid-io.Copy.
// Refcount only blocks NEW evictions — if a past sweep already deleted the
// file, http.ServeFile returns a normal 404.
func (s *Server) cacheStream(w http.ResponseWriter, r *http.Request, installID, bookID, format string) {
	key := streaming.ComputeCacheKey(bookID, format, installID)
	m := s.deps.CacheManager
	if e, ok := m.Lookup(r.Context(), key); ok {
		release := m.Acquire(e.ID)
		defer release()
		_ = m.Touch(r.Context(), e.ID)
		serveCacheFile(w, r, m.PathFor(e), e.MimeType)
		return
	}
	bk := backend.NewEbookBackend(s.deps.Host, installID)
	fetch := func(ctx context.Context) (io.ReadCloser, http.Header, int64, string, error) {
		upstream, err := s.deps.Host.GetStream(ctx, installID, bk.FilePath(bookID, format), nil)
		if err != nil {
			return nil, nil, 0, "", err
		}
		return upstream.Body, upstream.Header, upstream.ContentLength, upstream.Header.Get("Content-Type"), nil
	}
	entry, err := m.StartOrJoin(r.Context(), key, bookID, format, fetch)
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	release := m.Acquire(entry.ID)
	defer release()
	_ = m.Touch(r.Context(), entry.ID)
	serveCacheFile(w, r, m.PathFor(entry), entry.MimeType)
}

func serveCacheFile(w http.ResponseWriter, r *http.Request, path, mime string) {
	if mime != "" {
		w.Header().Set("Content-Type", mime)
	}
	http.ServeFile(w, r, path)
}

func (s *Server) handleListAnnotations(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	bookID := chi.URLParam(r, "id")
	anns, _ := s.deps.Store.ListAnnotationsByBook(r.Context(), id.UserID, bookID)
	writeJSON(w, 200, map[string]any{"items": anns})
}

func (s *Server) handleCreateAnnotation(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	bookID := chi.URLParam(r, "id")
	var body struct {
		CFIRange     string `json:"cfi_range"`
		Kind         string `json:"kind"`
		Color        string `json:"color"`
		SelectedText string `json:"selected_text"`
		NoteText     string `json:"note_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ann := store.Annotation{
		ID: ulid.Make().String(), UserID: id.UserID, BookID: bookID,
		CFIRange: body.CFIRange, Kind: body.Kind, Color: body.Color,
		SelectedText: body.SelectedText, NoteText: body.NoteText,
	}
	if err := s.deps.Store.InsertAnnotation(r.Context(), ann); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, ann)
}

func (s *Server) handleUpdateAnnotation(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	annID := chi.URLParam(r, "annId")
	var body struct {
		Color    string `json:"color"`
		NoteText string `json:"note_text"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if err := s.deps.Store.UpdateAnnotation(r.Context(), annID, id.UserID, body.Color, body.NoteText); err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleDeleteAnnotation(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	annID := chi.URLParam(r, "annId")
	if err := s.deps.Store.DeleteAnnotation(r.Context(), annID, id.UserID); err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	w.WriteHeader(204)
}

// -- catalog (proxied) ---------------------------------------------------

func (s *Server) handleListCatalog(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil {
			limit = n
		}
	}
	queryFor := func(lib store.PortalLibrary, limit int) backend.CatalogQuery {
		return backend.CatalogQuery{
			Cursor:    r.URL.Query().Get("cursor"),
			Sort:      r.URL.Query().Get("sort"),
			Order:     r.URL.Query().Get("order"),
			Limit:     limit,
			LibraryID: backendLibraryID(lib),
			Author:    r.URL.Query().Get("author"),
			Series:    r.URL.Query().Get("series"),
			Genre:     r.URL.Query().Get("genre"),
			Tag:       r.URL.Query().Get("tag"),
		}
	}
	if libraryID := queryLibraryID(r); libraryID > 0 {
		lib, err := s.targetLibrary(r, libraryID)
		if err != nil {
			writeErr(w, 404, err.Error())
			return
		}
		env, err := backend.NewEbookBackend(s.deps.Host, lib.BackendPluginID).ListCatalog(r.Context(), queryFor(lib, limit))
		if err != nil {
			writeErr(w, 502, err.Error())
			return
		}
		writeJSON(w, 200, wrapCatalogItems(env, lib))
		return
	}
	libs, err := s.deps.Store.ListPortalLibraries(r.Context(), true)
	if err != nil || len(libs) == 0 {
		lib, libErr := s.targetLibrary(r, 0)
		if libErr != nil {
			writeErr(w, 412, libErr.Error())
			return
		}
		libs = []store.PortalLibrary{lib}
	}
	perLibraryLimit := limit
	if len(libs) > 1 && perLibraryLimit > 20 {
		perLibraryLimit = 20
	}
	combined := backend.PageEnvelope[backend.EbookSummary]{Items: []backend.EbookSummary{}}
	for _, lib := range libs {
		env, err := backend.NewEbookBackend(s.deps.Host, lib.BackendPluginID).ListCatalog(r.Context(), queryFor(lib, perLibraryLimit))
		if err != nil {
			continue
		}
		env = wrapCatalogItems(env, lib)
		combined.Items = append(combined.Items, env.Items...)
		combined.Total += env.Total
	}
	if len(combined.Items) > limit {
		combined.Items = combined.Items[:limit]
	}
	writeJSON(w, 200, combined)
}

func (s *Server) handleGetBook(w http.ResponseWriter, r *http.Request) {
	libraryID, backendID, _ := decodeBookRef(chi.URLParam(r, "id"))
	lib, err := s.targetLibrary(r, libraryID)
	if err != nil {
		writeErr(w, 412, err.Error())
		return
	}
	d, err := backend.NewEbookBackend(s.deps.Host, lib.BackendPluginID).GetBook(r.Context(), backendID)
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	d.ID = encodeBookRef(lib.ID, d.ID)
	d.LibraryID = lib.ID
	d.LibraryName = lib.Name
	d.MediaType = lib.MediaType
	writeJSON(w, 200, d)
}

func (s *Server) handleSearchCatalog(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if libraryID := queryLibraryID(r); libraryID > 0 {
		lib, err := s.targetLibrary(r, libraryID)
		if err != nil {
			writeErr(w, 404, err.Error())
			return
		}
		env, err := backend.NewEbookBackend(s.deps.Host, lib.BackendPluginID).Search(r.Context(), query)
		if err != nil {
			writeErr(w, 502, err.Error())
			return
		}
		writeJSON(w, 200, wrapCatalogItems(env, lib))
		return
	}
	libs, err := s.deps.Store.ListPortalLibraries(r.Context(), true)
	if err != nil || len(libs) == 0 {
		lib, libErr := s.targetLibrary(r, 0)
		if libErr != nil {
			writeErr(w, 412, libErr.Error())
			return
		}
		libs = []store.PortalLibrary{lib}
	}
	combined := backend.PageEnvelope[backend.EbookSummary]{Items: []backend.EbookSummary{}}
	for _, lib := range libs {
		env, err := backend.NewEbookBackend(s.deps.Host, lib.BackendPluginID).Search(r.Context(), query)
		if err != nil {
			continue
		}
		env = wrapCatalogItems(env, lib)
		combined.Items = append(combined.Items, env.Items...)
		combined.Total += env.Total
	}
	writeJSON(w, 200, combined)
}

// browseQueryLimit reads ?limit= and clamps it to [1,200], defaulting to 50.
// Matches handleListCatalog's behaviour.
func browseQueryLimit(r *http.Request) int {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 200 {
		limit = 200
	}
	return limit
}

func queryLibraryID(r *http.Request) int64 {
	raw := r.URL.Query().Get("library_id")
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func (s *Server) handleListLibraries(w http.ResponseWriter, r *http.Request) {
	items, err := s.deps.Store.ListPortalLibraries(r.Context(), true)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (s *Server) handleBrowseAuthors(w http.ResponseWriter, r *http.Request) {
	lib, err := s.targetLibrary(r, queryLibraryID(r))
	if err != nil {
		writeErr(w, 412, err.Error())
		return
	}
	env, err := backend.NewEbookBackend(s.deps.Host, lib.BackendPluginID).BrowseAuthors(r.Context(), r.URL.Query().Get("cursor"), browseQueryLimit(r), backendLibraryID(lib))
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, env)
}

func (s *Server) handleBrowseSeries(w http.ResponseWriter, r *http.Request) {
	lib, err := s.targetLibrary(r, queryLibraryID(r))
	if err != nil {
		writeErr(w, 412, err.Error())
		return
	}
	env, err := backend.NewEbookBackend(s.deps.Host, lib.BackendPluginID).BrowseSeries(r.Context(), r.URL.Query().Get("cursor"), browseQueryLimit(r), backendLibraryID(lib))
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, env)
}

func (s *Server) handleBrowseGenres(w http.ResponseWriter, r *http.Request) {
	lib, err := s.targetLibrary(r, queryLibraryID(r))
	if err != nil {
		writeErr(w, 412, err.Error())
		return
	}
	env, err := backend.NewEbookBackend(s.deps.Host, lib.BackendPluginID).BrowseGenres(r.Context(), r.URL.Query().Get("cursor"), browseQueryLimit(r), backendLibraryID(lib))
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, env)
}

// -- requests ------------------------------------------------------------

func (s *Server) handleListMyRequests(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	rs, _ := s.deps.Store.ListRequestsByUser(r.Context(), id.UserID, 50)
	writeJSON(w, 200, map[string]any{"items": rs})
}

func (s *Server) handleGetMyRequest(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	req, err := s.deps.Store.GetRequestForUser(r.Context(), chi.URLParam(r, "id"), id.UserID)
	if err != nil {
		writeErr(w, 404, "not found")
		return
	}
	writeJSON(w, 200, req)
}

func (s *Server) handleCreateRequest(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	var body struct {
		Title          string   `json:"title"`
		Authors        []string `json:"authors"`
		ISBN           string   `json:"isbn"`
		SourceID       string   `json:"source_id"`
		FormatPref     string   `json:"format_pref"`
		MediaType      string   `json:"media_type"`
		AutoMonitor    bool     `json:"auto_monitor"`
		TargetPluginID string   `json:"target_plugin_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if body.Title == "" {
		writeErr(w, 400, "title required")
		return
	}
	cfg, _ := s.deps.Store.GetConfig(r.Context())
	if body.MediaType == "" {
		body.MediaType = "book"
	}
	body.MediaType = normalizeMediaType(body.MediaType)
	targetPluginID := body.TargetPluginID
	if targetPluginID == "" {
		if rule, err := s.deps.Store.ResolveRequestRoutingRule(r.Context(), body.MediaType); err == nil {
			targetPluginID = rule.TargetPluginID
			if body.FormatPref == "" {
				body.FormatPref = rule.FormatPref
			}
			if !body.AutoMonitor {
				body.AutoMonitor = rule.AutoMonitor
			}
		}
	}
	if targetPluginID == "" {
		targetPluginID = cfg.TargetBackendPluginID
	}
	if targetPluginID == "" {
		writeErr(w, 412, "no download provider configured")
		return
	}
	reqRow := store.Request{
		ID: ulid.Make().String(), UserID: id.UserID, Title: body.Title,
		Authors: body.Authors, ISBN: body.ISBN, SourceID: body.SourceID,
		FormatPref: body.FormatPref, MediaType: body.MediaType, Status: "pending",
		TargetPluginID: targetPluginID, AutoMonitor: body.AutoMonitor,
	}
	if err := s.deps.Store.InsertRequest(r.Context(), reqRow); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// If auto-approve is on, immediately submit to backend via broadcast event.
	if cfg.AutoApproveRequests {
		_ = s.deps.Store.UpdateRequestStatus(r.Context(), reqRow.ID, "submitted", "", "", "", "")
		if s.deps.Ev != nil {
			s.deps.Ev.Publish(r.Context(), "request_submitted", map[string]any{
				"request_id":       reqRow.ID,
				"target_plugin_id": reqRow.TargetPluginID,
				"title":            reqRow.Title,
				"authors":          reqRow.Authors,
				"isbn":             reqRow.ISBN,
				"source_id":        reqRow.SourceID,
				"format_pref":      reqRow.FormatPref,
				"media_type":       reqRow.MediaType,
				"auto_monitor":     reqRow.AutoMonitor,
			})
		}
	}
	writeJSON(w, 201, reqRow)
}

func (s *Server) handleRequestRoutingPreview(w http.ResponseWriter, r *http.Request) {
	mediaType := normalizeMediaType(r.URL.Query().Get("media_type"))
	cfg, _ := s.deps.Store.GetConfig(r.Context())
	targetPluginID := cfg.TargetBackendPluginID
	formatPref := ""
	autoMonitor := false
	source := "default"
	if rule, err := s.deps.Store.ResolveRequestRoutingRule(r.Context(), mediaType); err == nil {
		targetPluginID = rule.TargetPluginID
		formatPref = rule.FormatPref
		autoMonitor = rule.AutoMonitor
		source = "rule"
	}
	writeJSON(w, 200, map[string]any{
		"media_type":       mediaType,
		"target_plugin_id": targetPluginID,
		"format_pref":      formatPref,
		"auto_monitor":     autoMonitor,
		"source":           source,
	})
}

func normalizeMediaType(mediaType string) string {
	switch mediaType {
	case "comics":
		return "comic"
	case "documents":
		return "document"
	case "magazines":
		return "magazine"
	case "mangas":
		return "manga"
	case "":
		return "book"
	default:
		return mediaType
	}
}

func (s *Server) handleCancelRequest(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	reqID := chi.URLParam(r, "id")
	if err := s.deps.Store.DeleteRequest(r.Context(), reqID, id.UserID); err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	w.WriteHeader(204)
}

// -- collections ---------------------------------------------------------

func (s *Server) handleListMyCollections(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	cs, _ := s.deps.Store.ListCollectionsByUser(r.Context(), id.UserID)
	writeJSON(w, 200, map[string]any{"items": cs})
}

func (s *Server) handleCreateCollection(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	var body struct {
		Name     string `json:"name"`
		Color    string `json:"color"`
		IsPublic bool   `json:"is_public"`
		IsPinned bool   `json:"is_pinned"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	c := store.Collection{
		ID: ulid.Make().String(), UserID: id.UserID, Name: body.Name,
		Color: body.Color, IsPublic: body.IsPublic, IsPinned: body.IsPinned,
	}
	if err := s.deps.Store.CreateCollection(r.Context(), c); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, c)
}

func (s *Server) handleDeleteCollection(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	cid := chi.URLParam(r, "id")
	if err := s.deps.Store.DeleteCollection(r.Context(), cid, id.UserID); err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	w.WriteHeader(204)
}

func (s *Server) handleUpdateCollection(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	cid := chi.URLParam(r, "id")
	var body struct {
		Name        string `json:"name"`
		Color       string `json:"color"`
		IsPublic    bool   `json:"is_public"`
		IsPinned    bool   `json:"is_pinned"`
		CoverBookID string `json:"cover_book_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	c := store.Collection{
		ID:          cid,
		UserID:      id.UserID,
		Name:        body.Name,
		Color:       body.Color,
		IsPublic:    body.IsPublic,
		IsPinned:    body.IsPinned,
		CoverBookID: body.CoverBookID,
	}
	if err := s.deps.Store.UpdateCollection(r.Context(), c); err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	writeJSON(w, 200, c)
}

func (s *Server) handleListCollectionItems(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	cid := chi.URLParam(r, "id")
	items, err := s.deps.Store.ListItemsForUser(r.Context(), id.UserID, cid)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (s *Server) handleAddCollectionItem(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	cid := chi.URLParam(r, "id")
	var body struct {
		BookID   string `json:"book_id"`
		Position int    `json:"position"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if err := s.deps.Store.AddItemForUser(r.Context(), id.UserID, cid, body.BookID, body.Position); err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{"ok": true})
}

func (s *Server) handleRemoveCollectionItem(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	cid := chi.URLParam(r, "id")
	bid := chi.URLParam(r, "bookId")
	if err := s.deps.Store.RemoveItemForUser(r.Context(), id.UserID, cid, bid); err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	w.WriteHeader(204)
}
