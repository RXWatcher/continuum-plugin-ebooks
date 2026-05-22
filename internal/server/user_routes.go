package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"

	"github.com/RXWatcher/continuum-plugin-ebooks/internal/auth"
	"github.com/RXWatcher/continuum-plugin-ebooks/internal/backend"
	"github.com/RXWatcher/continuum-plugin-ebooks/internal/mediatoken"
	"github.com/RXWatcher/continuum-plugin-ebooks/internal/store"
	"github.com/RXWatcher/continuum-plugin-ebooks/internal/streaming"
)

func (s *Server) mountUserRoutes(r chi.Router) {
	// Identity-scoped reading data
	r.Get("/me/library", s.handleLibrary)
	r.Get("/me/progress", s.handleRecentProgress)
	r.Get("/me/streak", s.handleGetStreak)
	r.Get("/me/books/{id}", s.handleGetBookUserData)
	r.Get("/me/books/{id}/reader-config", s.handleGetReaderConfig)
	r.Put("/me/books/{id}/reader-config", s.handlePutReaderConfig)
	r.Post("/me/books/{id}/kosync-link", s.handleLinkKosyncBook)
	r.Post("/me/books/{id}/progress", s.handleUpdateProgress)
	r.Patch("/me/books/{id}", s.handleUpdateBookMeta)
	r.Get("/me/books/{id}/file", s.handleStreamFile)
	r.Get("/me/books/{id}/annotations", s.handleListAnnotations)
	r.Post("/me/books/{id}/annotations", s.handleCreateAnnotation)
	r.Patch("/me/annotations/{annId}", s.handleUpdateAnnotation)
	r.Delete("/me/annotations/{annId}", s.handleDeleteAnnotation)

	// Embedding-driven similar-items recommendation.
	r.Get("/me/books/{id}/similar", s.handleSimilarBooks)

	// Rule-based Smart Collections.
	s.mountSmartCollectionRoutes(r)

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
	r.Get("/me/kosync", s.handleKosyncStatus)
	r.Post("/me/kosync/register", s.handleKosyncRegister)
	r.Delete("/me/kosync", s.handleKosyncDelete)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func nonNilSlice[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

func writeItems[T any](w http.ResponseWriter, code int, items []T) {
	writeJSON(w, code, map[string]any{"items": nonNilSlice(items)})
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": map[string]any{"message": msg}})
}

// writeInternal handles an unexpected store/internal error. The underlying
// error can carry SQL text, schema names, the DSN host, or internal paths,
// so it is logged server-side (with method+path) and only an opaque 500 is
// returned — important because /opds, /kosync and /kobo are public routes.
func writeInternal(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("ebooks-portal internal error",
		"method", r.Method, "path", r.URL.Path, "err", err)
	writeErr(w, http.StatusInternalServerError, "internal error")
}

// writeBadGateway handles an upstream/backend failure. The wrapped error can
// embed the full upstream response body and host-proxy URL; log it, return a
// generic message.
func writeBadGateway(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("ebooks-portal backend error",
		"method", r.Method, "path", r.URL.Path, "err", err)
	writeErr(w, http.StatusBadGateway, "backend unavailable")
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
	if !cfg.HasBackend() {
		return nil, fmt.Errorf("no backend configured")
	}
	return backend.NewEbookBackend(s.deps.Host, cfg.BackendTarget()), nil
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
	if err != nil || !cfg.HasBackend() {
		return store.PortalLibrary{}, fmt.Errorf("no backend configured")
	}
	return store.PortalLibrary{
		Name:            "Library",
		MediaType:       "book",
		BackendPluginID: cfg.BackendTarget(),
		Enabled:         true,
	}, nil
}

func backendLibraryID(lib store.PortalLibrary) int64 {
	if lib.BackendLibraryID == nil {
		return 0
	}
	return *lib.BackendLibraryID
}

// libResult is one portal library's outcome when fanning a catalog/search
// request across every enabled library.
type libResult struct {
	lib store.PortalLibrary
	env backend.PageEnvelope[backend.EbookSummary]
	err error
}

// combineCatalogResults merges per-library backend results into one envelope.
//
// User catalog pages should stay usable when one or more configured backends
// are unavailable. Partial failures still return the libraries that worked;
// an all-failed fanout degrades to an empty envelope so the portal shell can
// render instead of crashing behind a 502.
//
// limit <= 0 disables truncation (search path); limit > 0 caps the merged
// list (list path).
func combineCatalogResults(results []libResult, limit int, userID, secret string) (backend.PageEnvelope[backend.EbookSummary], error) {
	combined := backend.PageEnvelope[backend.EbookSummary]{Items: []backend.EbookSummary{}}
	for _, res := range results {
		if res.err != nil {
			continue
		}
		env := wrapCatalogItems(res.env, res.lib, userID, secret)
		combined.Items = append(combined.Items, env.Items...)
		combined.Total += env.Total
	}
	if limit > 0 && len(combined.Items) > limit {
		combined.Items = combined.Items[:limit]
	}
	return combined, nil
}

func wrapCatalogItems(env backend.PageEnvelope[backend.EbookSummary], lib store.PortalLibrary, userID, secret string) backend.PageEnvelope[backend.EbookSummary] {
	for i := range env.Items {
		backendBookID := env.Items[i].ID
		env.Items[i].CoverURL = signedCoverURL(env.Items[i].CoverURL, lib.BackendPluginID, userID, backendBookID, secret)
		env.Items[i].ID = encodeBookRef(lib.ID, env.Items[i].ID)
		env.Items[i].LibraryID = lib.ID
		env.Items[i].LibraryName = lib.Name
		env.Items[i].MediaType = lib.MediaType
	}
	return env
}

// signedCoverURL turns a backend-relative cover URL into a host plugin proxy
// URL with a short-TTL signed media token in ?token=. Browsers can't send
// Authorization headers on <img>-tag requests, so the token rides in the URL.
func signedCoverURL(raw, installID, userID, backendBookID, secret string) string {
	if raw == "" || installID == "" {
		return raw
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	out := raw
	if !strings.HasPrefix(out, "/api/v1/plugins/") {
		if !strings.HasPrefix(out, "/api/v1/") {
			if !strings.HasPrefix(out, "/") {
				out = "/" + out
			}
			out = "/api/v1" + out
		}
		out = "/api/v1/plugins/" + url.PathEscape(installID) + out
	}
	if secret == "" || userID == "" || backendBookID == "" {
		return out
	}
	token, err := mediatoken.Mint(secret, userID, backendBookID, mediatoken.CoverFileIdx)
	if err != nil {
		slog.Warn("mint cover token failed", "book_id", backendBookID, "err", err)
		return out
	}
	sep := "?"
	if strings.Contains(out, "?") {
		sep = "&"
	}
	return out + sep + "token=" + url.QueryEscape(token)
}

// signedFileURL builds the file URL the SPA puts in <a href> or passes to
// the reader. Both supported ebook backends store a single file per book
// and ignore any ?format= query, so the URL doesn't carry format — callers
// that need the format string for display read it from EbookFile.Format on
// the catalog response.
func signedFileURL(installID, userID, backendBookID, secret string) string {
	if installID == "" || backendBookID == "" {
		return ""
	}
	out := "/api/v1/plugins/" + url.PathEscape(installID) +
		"/api/v1/file/" + url.PathEscape(backendBookID)
	if secret == "" || userID == "" {
		return out
	}
	token, err := mediatoken.Mint(secret, userID, backendBookID, mediatoken.FileFileIdx)
	if err != nil {
		slog.Warn("mint file token failed", "book_id", backendBookID, "err", err)
		return out
	}
	return out + "?token=" + url.QueryEscape(token)
}

// mediaSigningContext returns (userID, secret) for the request. Empty values
// mean callers will return URLs without tokens — the backend then rejects
// with a clear 401 the operator can debug.
func (s *Server) mediaSigningContext(r *http.Request) (string, string) {
	id, _ := auth.FromContext(r.Context())
	cfg, err := s.deps.Store.GetConfig(r.Context())
	if err != nil {
		return id.UserID, ""
	}
	return id.UserID, cfg.MediaSigningSecret
}

// -- library/progress/annotations -----------------------------------------

func (s *Server) handleLibrary(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	status := r.URL.Query().Get("status")
	rows, err := s.deps.Store.ListByUser(r.Context(), id.UserID, status, 100)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeItems(w, 200, rows)
}

func (s *Server) handleRecentProgress(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	rows, err := s.deps.Store.ListRecentByUser(r.Context(), id.UserID, 20)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeItems(w, 200, rows)
}

// handleGetStreak — GET /me/streak
// Returns {current, longest, last_active_date} computed from
// user_data.last_read_at distinct dates. Mirrors the audiobooks
// plugin's /me/streak shape so a shared SPA component can render
// either side.
func (s *Server) handleGetStreak(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	streak, err := s.deps.Store.StreakForUser(r.Context(), id.UserID, time.UTC)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, 200, streak)
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
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, 200, row)
}

func (s *Server) handleGetReaderConfig(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	bookID := chi.URLParam(r, "id")
	row, err := s.deps.Store.GetReaderConfig(r.Context(), id.UserID, bookID)
	if err != nil {
		if err == store.ErrNotFound {
			if userData, dataErr := s.deps.Store.GetUserData(r.Context(), id.UserID, bookID); dataErr == nil {
				config := readerConfigFromUserData(userData)
				s.addExternalReaderProgress(r.Context(), id.UserID, bookID, config)
				writeJSON(w, 200, map[string]any{
					"book_id": bookID,
					"config":  config,
				})
				return
			}
			config := map[string]any{}
			s.addExternalReaderProgress(r.Context(), id.UserID, bookID, config)
			writeJSON(w, 200, map[string]any{
				"book_id": bookID,
				"config":  config,
			})
			return
		}
		writeInternal(w, r, err)
		return
	}
	var config map[string]any
	if err := json.Unmarshal(row.ConfigJSON, &config); err != nil {
		writeInternal(w, r, err)
		return
	}
	s.addExternalReaderProgress(r.Context(), id.UserID, bookID, config)
	writeJSON(w, 200, map[string]any{
		"book_id":    bookID,
		"config":     config,
		"updated_at": row.UpdatedAt,
	})
}

func readerConfigFromUserData(row store.UserData) map[string]any {
	config := map[string]any{}
	if row.LastCFI != "" {
		config["location"] = row.LastCFI
	}
	if row.CurrentPage > 0 && row.ReadProgress > 0 {
		total := math.Round(float64(row.CurrentPage) / row.ReadProgress)
		if total > 0 {
			config["progress"] = []float64{float64(row.CurrentPage), total}
		}
	} else if row.ReadProgress > 0 {
		config["progress"] = []float64{row.ReadProgress * 100, 100}
	}
	return config
}

func (s *Server) addExternalReaderProgress(ctx context.Context, userID, bookID string, config map[string]any) {
	link, err := s.deps.Store.FindKosyncBookLinkByBook(ctx, userID, bookID)
	if err != nil {
		return
	}
	progress, err := s.deps.Store.GetKosyncProgress(ctx, userID, link.Document)
	if err != nil {
		return
	}
	external := map[string]any{
		"source":     "kosync",
		"document":   progress.Document,
		"progress":   progress.Progress,
		"percentage": progress.Percentage,
		"device":     progress.Device,
		"device_id":  progress.DeviceID,
		"timestamp":  progress.Timestamp,
		"canResume":  false,
	}
	if strings.HasPrefix(progress.Progress, "epubcfi(") {
		external["location"] = progress.Progress
		external["canResume"] = true
	}
	config["externalProgress"] = external
}

func (s *Server) handlePutReaderConfig(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	bookID := chi.URLParam(r, "id")
	var body struct {
		Config json.RawMessage `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if len(body.Config) == 0 {
		body.Config = json.RawMessage(`{}`)
	}
	if !json.Valid(body.Config) {
		writeErr(w, 400, "config must be valid JSON")
		return
	}
	if err := s.deps.Store.UpsertReaderConfig(r.Context(), store.ReaderConfig{
		UserID:     id.UserID,
		BookID:     bookID,
		ConfigJSON: body.Config,
	}); err != nil {
		writeInternal(w, r, err)
		return
	}
	if err := s.mirrorReaderConfigProgress(r.Context(), id.UserID, bookID, body.Config); err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{
		"book_id": bookID,
		"config":  json.RawMessage(body.Config),
	})
}

func (s *Server) handleLinkKosyncBook(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	bookID := chi.URLParam(r, "id")
	var body struct {
		Document string `json:"document"`
		Format   string `json:"format"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	body.Document = strings.TrimSpace(body.Document)
	body.Format = strings.ToLower(strings.TrimSpace(body.Format))
	if body.Document == "" {
		writeErr(w, 400, "document required")
		return
	}
	if body.Format == "" {
		body.Format = "epub"
	}
	if err := s.deps.Store.UpsertKosyncBookLink(r.Context(), store.KosyncBookLink{
		UserID:   id.UserID,
		BookID:   bookID,
		Document: body.Document,
		Format:   body.Format,
	}); err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{
		"book_id":  bookID,
		"document": body.Document,
		"format":   body.Format,
	})
}

func (s *Server) mirrorReaderConfigProgress(ctx context.Context, userID, bookID string, raw json.RawMessage) error {
	var cfg struct {
		Location string    `json:"location"`
		Progress []float64 `json:"progress"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}
	if cfg.Location == "" && len(cfg.Progress) == 0 {
		return nil
	}
	cur, _ := s.deps.Store.GetUserData(ctx, userID, bookID)
	cur.UserID = userID
	cur.BookID = bookID
	if cfg.Location != "" {
		cur.LastCFI = cfg.Location
	}
	if len(cfg.Progress) >= 2 {
		current := int(cfg.Progress[0])
		total := cfg.Progress[1]
		cur.CurrentPage = current
		if total > 0 {
			cur.ReadProgress = cfg.Progress[0] / total
			cur.IsFinished = cur.ReadProgress >= 0.98
		}
	}
	now := time.Now()
	cur.LastReadAt = &now
	return s.deps.Store.UpsertUserData(ctx, cur)
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
	// Read-modify-write: a progress ping carries only progress fields, so we
	// must preserve is_favorite/notes/rating (the upsert overwrites
	// is_favorite/notes from EXCLUDED). Rebuilding UserData from scratch here
	// silently wiped the user's favorite flag and notes every time they
	// opened a book. Mirrors handleUpdateBookMeta's load-then-patch.
	cur, _ := s.deps.Store.GetUserData(r.Context(), id.UserID, bookID)
	cur.UserID = id.UserID
	cur.BookID = bookID
	cur.LastCFI = body.LastCFI
	cur.CurrentPage = body.CurrentPage
	cur.ReadProgress = body.ReadProgress
	cur.IsFinished = body.IsFinished
	cur.LastReadAt = &now
	if err := s.deps.Store.UpsertUserData(r.Context(), cur); err != nil {
		writeInternal(w, r, err)
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
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// handleStreamFile dispatches to the configured streaming mode. "proxy"
// pipes bytes live; "cache" looks up (and on miss, single-flight downloads)
// the file into the LRU-managed cache. Either mode mints a signed media
// token before hitting the backend — the backend's file route is public +
// token-gated, so an unsigned server-side fetch would 401.
func (s *Server) handleStreamFile(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	bookRef := chi.URLParam(r, "id")
	libraryID, bookID, _ := decodeBookRef(bookRef)
	lib, err := s.targetLibrary(r, libraryID)
	if err != nil {
		writeErr(w, 412, err.Error())
		return
	}
	cfg, err := s.deps.Store.GetConfig(r.Context())
	if err != nil {
		writeErr(w, 412, "no backend configured")
		return
	}
	bk := backend.NewEbookBackend(s.deps.Host, lib.BackendPluginID)
	signedPath := bk.SignedFilePath(id.UserID, bookID, cfg.MediaSigningSecret)
	mode := streaming.ResolveMode(cfg)
	if mode == "cache" {
		if s.deps.CacheManager != nil {
			s.cacheStream(w, r, lib.BackendPluginID, lib.ID, bookID, signedPath)
			return
		}
		slog.Warn("ebooks: cache streaming mode requested but cache manager not initialized; falling back to proxy",
			"library_id", lib.ID, "book_id", bookID)
	}
	streaming.ProxyStream(w, r, s.deps.Host, lib.BackendPluginID, signedPath)
}

// cacheStream implements the cache-mode handler. Cache hit → http.ServeFile;
// miss → single-flight download via streaming.Manager, then serve.
//
// The serve path acquires an in-process refcount on the entry for the duration
// of the response so the eviction sweeper can't unlink the file mid-io.Copy.
// Refcount only blocks NEW evictions — if a past sweep already deleted the
// file, http.ServeFile returns a normal 404.
//
// upstreamPath must already carry a signed ?token= — the backend's file
// route is public + token-gated, an unsigned fetch would cache a 401 body.
func (s *Server) cacheStream(w http.ResponseWriter, r *http.Request, installID string, libraryID int64, bookID, upstreamPath string) {
	// libraryID is part of the cache key so two portal libraries pointing at
	// the same backend can't collide on book id — without it, switching
	// libraries would serve cross-library cached bytes until eviction.
	key := streaming.ComputeCacheKey(bookID, installID, libraryID)
	m := s.deps.CacheManager
	if e, ok := m.Lookup(r.Context(), key); ok {
		release := m.Acquire(e.ID)
		defer release()
		_ = m.Touch(r.Context(), e.ID)
		serveCacheFile(w, r, m.PathFor(e), e.MimeType)
		return
	}
	fetch := func(ctx context.Context) (io.ReadCloser, http.Header, int64, string, error) {
		upstream, err := s.deps.Host.GetStream(ctx, installID, upstreamPath, nil)
		if err != nil {
			return nil, nil, 0, "", err
		}
		// Never cache a non-success response: a 404/500/redirect/HTML error
		// page would otherwise be written to disk, marked "ready", and served
		// as the book to every subsequent reader until LRU eviction.
		if upstream.StatusCode < 200 || upstream.StatusCode >= 300 {
			_ = upstream.Body.Close()
			return nil, nil, 0, "", fmt.Errorf("backend status %d", upstream.StatusCode)
		}
		return upstream.Body, upstream.Header, upstream.ContentLength, upstream.Header.Get("Content-Type"), nil
	}
	entry, err := m.StartOrJoin(r.Context(), key, bookID, "", fetch)
	if err != nil {
		writeBadGateway(w, r, err)
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
	writeItems(w, 200, anns)
}

func (s *Server) handleCreateAnnotation(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	bookID := chi.URLParam(r, "id")
	var body struct {
		CFIRange     string          `json:"cfi_range"`
		Kind         string          `json:"kind"`
		Color        string          `json:"color"`
		SelectedText string          `json:"selected_text"`
		NoteText     string          `json:"note_text"`
		ReadestType  string          `json:"readest_type"`
		XPointer0    string          `json:"xpointer0"`
		XPointer1    string          `json:"xpointer1"`
		Page         *int            `json:"page"`
		Style        string          `json:"style"`
		MetadataJSON json.RawMessage `json:"metadata_json"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if len(body.MetadataJSON) == 0 {
		body.MetadataJSON = json.RawMessage(`{}`)
	}
	ann := store.Annotation{
		ID: ulid.Make().String(), UserID: id.UserID, BookID: bookID,
		CFIRange: body.CFIRange, Kind: body.Kind, Color: body.Color,
		SelectedText: body.SelectedText, NoteText: body.NoteText,
		ReadestType: body.ReadestType, XPointer0: body.XPointer0, XPointer1: body.XPointer1,
		Page: body.Page, Style: body.Style, MetadataJSON: body.MetadataJSON,
	}
	if err := s.deps.Store.InsertAnnotation(r.Context(), ann); err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, 201, ann)
}

func (s *Server) handleUpdateAnnotation(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	annID := chi.URLParam(r, "annId")
	var body struct {
		CFIRange     string `json:"cfi_range"`
		Color        string `json:"color"`
		SelectedText string `json:"selected_text"`
		NoteText     string `json:"note_text"`
		Style        string `json:"style"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if err := s.deps.Store.UpdateAnnotation(r.Context(), annID, id.UserID, store.Annotation{
		CFIRange:     body.CFIRange,
		Color:        body.Color,
		SelectedText: body.SelectedText,
		NoteText:     body.NoteText,
		Style:        body.Style,
	}); err != nil {
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
			slog.Warn("ebooks-portal catalog backend unavailable",
				"method", r.Method, "path", r.URL.Path, "err", err)
			writeJSON(w, 200, backend.PageEnvelope[backend.EbookSummary]{Items: []backend.EbookSummary{}})
			return
		}
		userID, secret := s.mediaSigningContext(r)
		writeJSON(w, 200, wrapCatalogItems(env, lib, userID, secret))
		return
	}
	libs, err := s.deps.Store.ListPortalLibraries(r.Context(), true)
	if err != nil || len(libs) == 0 {
		lib, libErr := s.targetLibrary(r, 0)
		if libErr != nil {
			writeJSON(w, 200, backend.PageEnvelope[backend.EbookSummary]{Items: []backend.EbookSummary{}})
			return
		}
		libs = []store.PortalLibrary{lib}
	}
	perLibraryLimit := limit
	if len(libs) > 1 && perLibraryLimit > 20 {
		perLibraryLimit = 20
	}
	// Composite pagination: on the first page (no/!valid cursor) query every
	// library; afterwards only the libraries that still had pages, each
	// resumed from its own backend cursor. This keeps every book reachable.
	cursors, paging := decodeCatalogCursor(r.URL.Query().Get("cursor"))
	results := make([]libResult, 0, len(libs))
	nextCursors := map[int64]string{}
	for _, lib := range libs {
		backendCursor := ""
		if paging {
			c, ok := cursors[lib.ID]
			if !ok {
				continue // exhausted on an earlier page
			}
			backendCursor = c
		}
		q := queryFor(lib, perLibraryLimit)
		q.Cursor = backendCursor
		env, err := backend.NewEbookBackend(s.deps.Host, lib.BackendPluginID).ListCatalog(r.Context(), q)
		results = append(results, libResult{lib: lib, env: env, err: err})
		if err == nil && env.NextCursor != "" {
			nextCursors[lib.ID] = env.NextCursor
		}
	}
	combinedUserID, combinedSecret := s.mediaSigningContext(r)
	combined, err := combineCatalogResults(results, 0, combinedUserID, combinedSecret)
	if err != nil {
		writeBadGateway(w, r, err)
		return
	}
	combined.NextCursor = encodeCatalogCursor(nextCursors)
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
		writeBadGateway(w, r, err)
		return
	}
	userID, secret := s.mediaSigningContext(r)
	d.CoverURL = signedCoverURL(d.CoverURL, lib.BackendPluginID, userID, backendID, secret)
	for i := range d.Files {
		d.Files[i].URL = signedFileURL(lib.BackendPluginID, userID, backendID, secret)
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
			slog.Warn("ebooks-portal search backend unavailable",
				"method", r.Method, "path", r.URL.Path, "err", err)
			writeJSON(w, 200, backend.PageEnvelope[backend.EbookSummary]{Items: []backend.EbookSummary{}})
			return
		}
		userID, secret := s.mediaSigningContext(r)
		writeJSON(w, 200, wrapCatalogItems(env, lib, userID, secret))
		return
	}
	libs, err := s.deps.Store.ListPortalLibraries(r.Context(), true)
	if err != nil || len(libs) == 0 {
		lib, libErr := s.targetLibrary(r, 0)
		if libErr != nil {
			writeJSON(w, 200, backend.PageEnvelope[backend.EbookSummary]{Items: []backend.EbookSummary{}})
			return
		}
		libs = []store.PortalLibrary{lib}
	}
	results := make([]libResult, 0, len(libs))
	for _, lib := range libs {
		env, err := backend.NewEbookBackend(s.deps.Host, lib.BackendPluginID).Search(r.Context(), query)
		results = append(results, libResult{lib: lib, env: env, err: err})
	}
	combinedUserID, combinedSecret := s.mediaSigningContext(r)
	combined, err := combineCatalogResults(results, 0, combinedUserID, combinedSecret)
	if err != nil {
		writeBadGateway(w, r, err)
		return
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
		writeInternal(w, r, err)
		return
	}
	writeItems(w, 200, items)
}

func (s *Server) handleBrowseAuthors(w http.ResponseWriter, r *http.Request) {
	lib, err := s.targetLibrary(r, queryLibraryID(r))
	if err != nil {
		writeErr(w, 412, err.Error())
		return
	}
	env, err := backend.NewEbookBackend(s.deps.Host, lib.BackendPluginID).BrowseAuthors(r.Context(), r.URL.Query().Get("cursor"), browseQueryLimit(r), backendLibraryID(lib))
	if err != nil {
		slog.Warn("ebooks-portal authors backend unavailable",
			"method", r.Method, "path", r.URL.Path, "err", err)
		writeJSON(w, 200, backend.PageEnvelope[backend.FacetItem]{Items: []backend.FacetItem{}})
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
		slog.Warn("ebooks-portal series backend unavailable",
			"method", r.Method, "path", r.URL.Path, "err", err)
		writeJSON(w, 200, backend.PageEnvelope[backend.FacetItem]{Items: []backend.FacetItem{}})
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
		slog.Warn("ebooks-portal genres backend unavailable",
			"method", r.Method, "path", r.URL.Path, "err", err)
		writeJSON(w, 200, backend.PageEnvelope[backend.FacetItem]{Items: []backend.FacetItem{}})
		return
	}
	writeJSON(w, 200, env)
}

// -- requests ------------------------------------------------------------

func (s *Server) handleListMyRequests(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	rs, _ := s.deps.Store.ListRequestsByUser(r.Context(), id.UserID, 50)
	writeItems(w, 200, rs)
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
	targetPluginID := strings.TrimSpace(body.TargetPluginID)
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
		targetPluginID = cfg.BackendTarget()
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
		writeInternal(w, r, err)
		return
	}
	// If auto-approve is on, immediately submit to backend.
	if cfg.AutoApproveRequests {
		if err := s.submitRequest(r.Context(), reqRow); err != nil {
			writeInternal(w, r, err)
			return
		}
	}
	writeJSON(w, 201, reqRow)
}

func publishRequestSubmitted(ctx context.Context, pub EventPublisher, req store.Request) {
	if pub == nil {
		return
	}
	target := strings.TrimSpace(req.TargetPluginID)
	payload := map[string]any{
		"request_id":   req.ID,
		"requestId":    req.ID,
		"title":        req.Title,
		"authors":      req.Authors,
		"isbn":         req.ISBN,
		"source_id":    req.SourceID,
		"format_pref":  req.FormatPref,
		"media_type":   req.MediaType,
		"auto_monitor": req.AutoMonitor,
	}
	if target != "" {
		payload["target_plugin_id"] = target
		payload["target_provider_plugin_id"] = target
		if targeted, ok := pub.(TargetedEventPublisher); ok {
			targeted.PublishTo(ctx, target, "request_submitted", payload)
			return
		}
	}
	pub.Publish(ctx, "request_submitted", payload)
}

func (s *Server) handleRequestRoutingPreview(w http.ResponseWriter, r *http.Request) {
	mediaType := normalizeMediaType(r.URL.Query().Get("media_type"))
	cfg, _ := s.deps.Store.GetConfig(r.Context())
	targetPluginID := cfg.BackendTarget()
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
	cs, _ := s.deps.Store.ListCollectionsByProfile(r.Context(), id.UserID, id.ProfileID)
	// Use writeItems (nonNilSlice) so nil → []. encoding/json marshals nil
	// slices as `null`, which crashes SPA code like `data?.items.length`
	// (optional chaining short-circuits on data being null, but null.length
	// is still a thrown TypeError).
	writeItems(w, 200, cs)
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
		ID: ulid.Make().String(), UserID: id.UserID, ProfileID: id.ProfileID, Name: body.Name,
		Color: body.Color, IsPublic: body.IsPublic, IsPinned: body.IsPinned,
	}
	if err := s.deps.Store.CreateCollection(r.Context(), c); err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, 201, c)
}

func (s *Server) handleDeleteCollection(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	cid := chi.URLParam(r, "id")
	if err := s.deps.Store.DeleteCollection(r.Context(), cid, id.UserID, id.ProfileID); err != nil {
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
		ProfileID:   id.ProfileID,
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
	items, err := s.deps.Store.ListItemsForUser(r.Context(), id.UserID, id.ProfileID, cid)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeItems(w, 200, items)
}

func (s *Server) handleAddCollectionItem(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	cid := chi.URLParam(r, "id")
	var body struct {
		BookID   string `json:"book_id"`
		Position int    `json:"position"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if err := s.deps.Store.AddItemForUser(r.Context(), id.UserID, id.ProfileID, cid, body.BookID, body.Position); err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{"ok": true})
}

func (s *Server) handleRemoveCollectionItem(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	cid := chi.URLParam(r, "id")
	bid := chi.URLParam(r, "bookId")
	if err := s.deps.Store.RemoveItemForUser(r.Context(), id.UserID, id.ProfileID, cid, bid); err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	w.WriteHeader(204)
}
