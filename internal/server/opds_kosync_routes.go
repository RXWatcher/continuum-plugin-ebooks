package server

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/RXWatcher/silo-plugin-ebooks/internal/auth"
	"github.com/RXWatcher/silo-plugin-ebooks/internal/backend"
	"github.com/RXWatcher/silo-plugin-ebooks/internal/store"
)

// -- OPDS feeds (public route; basic-auth handled here) ------------------

func (s *Server) mountOPDS(r chi.Router) {
	r.Get("/", s.handleOPDSRoot)
	r.Get("/catalog", s.handleOPDSCatalog)
	r.Get("/search", s.handleOPDSSearch)
	r.Get("/book/{id}", s.handleOPDSBookEntry)
	r.Get("/book/{id}/download/{format}", s.handleOPDSDownload)
	r.Get("/collections", s.handleOPDSCollections)
	r.Get("/collection/{id}", s.handleOPDSCollection)
}

type opdsFeed struct {
	XMLName   xml.Name    `xml:"feed"`
	XMLNS     string      `xml:"xmlns,attr"`
	XMLNSOPDS string      `xml:"xmlns:opds,attr,omitempty"`
	ID        string      `xml:"id"`
	Title     string      `xml:"title"`
	Updated   string      `xml:"updated"`
	Links     []opdsLink  `xml:"link"`
	Entries   []opdsEntry `xml:"entry"`
}

type opdsEntry struct {
	ID      string     `xml:"id"`
	Title   string     `xml:"title"`
	Updated string     `xml:"updated"`
	Authors []opdsAuth `xml:"author"`
	Summary string     `xml:"summary,omitempty"`
	Links   []opdsLink `xml:"link"`
}

type opdsAuth struct {
	Name string `xml:"name"`
}

type opdsLink struct {
	Rel  string `xml:"rel,attr,omitempty"`
	Type string `xml:"type,attr,omitempty"`
	Href string `xml:"href,attr"`
}

func (s *Server) handleOPDSRoot(w http.ResponseWriter, r *http.Request) {
	feed := opdsFeed{
		XMLNS:   "http://www.w3.org/2005/Atom",
		ID:      "tag:silo:ebooks:opds",
		Title:   "Silo Library",
		Updated: time.Now().UTC().Format(time.RFC3339),
		Links: []opdsLink{
			{Rel: "self", Type: "application/atom+xml;profile=opds-catalog", Href: "/opds/"},
			{Rel: "start", Type: "application/atom+xml;profile=opds-catalog", Href: "/opds/"},
			{Rel: "http://opds-spec.org/sort/new", Type: "application/atom+xml;profile=opds-catalog;kind=acquisition", Href: "/opds/catalog"},
			{Rel: "search", Type: "application/opensearchdescription+xml", Href: "/opds/search"},
		{Rel: "subsection", Type: "application/atom+xml;profile=opds-catalog", Href: "/opds/collections"},
		},
	}
	writeOPDS(w, r, feed)
}

// opdsCatalogLimit reads ?limit= and clamps it to [1,200], defaulting to 50.
// Matches the user-facing /api/v1/ebooks limit semantics.
func opdsCatalogLimit(r *http.Request) int {
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

// buildOPDSCatalogFeed assembles the catalog feed from a backend page
// envelope. Extracted so tests can exercise pagination-link emission without
// needing a real Store or backend wired up.
func buildOPDSCatalogFeed(env backend.PageEnvelope[backend.EbookSummary], realm string, limit int, now time.Time) opdsFeed {
	feed := opdsFeed{
		XMLNS:   "http://www.w3.org/2005/Atom",
		ID:      "tag:silo:ebooks:opds:catalog",
		Title:   realm + " — Catalog",
		Updated: now.UTC().Format(time.RFC3339),
		Links: []opdsLink{
			{Rel: "self", Type: "application/atom+xml;profile=opds-catalog;kind=acquisition", Href: "/opds/catalog"},
		},
	}
	// Emit rel="next" when the backend signals more pages. The catalog feed
	// would otherwise silently truncate at `limit`; cursor-based pagination
	// matches the backend's PageEnvelope.NextCursor shape, so we just forward
	// the opaque cursor token along with the requested limit.
	if env.NextCursor != "" {
		feed.Links = append(feed.Links, opdsLink{
			Rel:  "next",
			Type: "application/atom+xml;profile=opds-catalog;kind=acquisition",
			Href: fmt.Sprintf("/opds/catalog?cursor=%s&limit=%d", url.QueryEscape(env.NextCursor), limit),
		})
	}
	for _, b := range env.Items {
		entry := opdsEntry{
			ID: "tag:silo:ebooks:book:" + b.ID, Title: b.Title,
			Updated: now.UTC().Format(time.RFC3339),
		}
		for _, a := range b.Authors {
			entry.Authors = append(entry.Authors, opdsAuth{Name: a})
		}
		for _, f := range b.Formats {
			entry.Links = append(entry.Links, opdsLink{
				Rel: "http://opds-spec.org/acquisition", Type: formatMime(f),
				Href: fmt.Sprintf("/opds/book/%s/download/%s", b.ID, f),
			})
		}
		feed.Entries = append(feed.Entries, entry)
	}
	return feed
}

func (s *Server) handleOPDSCatalog(w http.ResponseWriter, r *http.Request) {
	userID, _, autherr := s.opdsAuth(r)
	if autherr != nil {
		writeErr(w, http.StatusBadGateway, "auth service unavailable")
		return
	}
	if userID == "" {
		s.opdsChallenge(w, r)
		return
	}
	_ = userID
	cfg, _ := s.deps.Store.GetConfig(r.Context())
	if !cfg.HasBackend() {
		writeErr(w, 412, "no backend")
		return
	}
	bk := backend.NewEbookBackend(s.deps.Host, cfg.BackendTarget())
	limit := opdsCatalogLimit(r)
	cursor := r.URL.Query().Get("cursor")
	env, err := bk.ListCatalog(r.Context(), backend.CatalogQuery{
		Sort: "added", Order: "desc", Limit: limit, Cursor: cursor,
	})
	if err != nil {
		writeBadGateway(w, r, err)
		return
	}
	feed := buildOPDSCatalogFeed(env, cfg.OpdsRealm, limit, time.Now())
	writeOPDS(w, r, feed)
}

func (s *Server) handleOPDSSearch(w http.ResponseWriter, r *http.Request) {
	userID, _, autherr := s.opdsAuth(r)
	if autherr != nil {
		writeErr(w, http.StatusBadGateway, "auth service unavailable")
		return
	}
	if userID == "" {
		s.opdsChallenge(w, r)
		return
	}
	_ = userID
	q := r.URL.Query().Get("q")
	if q == "" {
		// Return OpenSearch description.
		w.Header().Set("Content-Type", "application/opensearchdescription+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<OpenSearchDescription xmlns="http://a9.com/-/spec/opensearch/1.1/">
  <ShortName>Silo Library</ShortName>
  <Description>Search Silo's ebook library</Description>
  <Url template="/opds/search?q={searchTerms}" type="application/atom+xml"/>
</OpenSearchDescription>`))
		return
	}
	cfg, _ := s.deps.Store.GetConfig(r.Context())
	if !cfg.HasBackend() {
		writeErr(w, 412, "no backend")
		return
	}
	bk := backend.NewEbookBackend(s.deps.Host, cfg.BackendTarget())
	env, err := bk.Search(r.Context(), q)
	if err != nil {
		writeBadGateway(w, r, err)
		return
	}
	feed := opdsFeed{
		XMLNS:   "http://www.w3.org/2005/Atom",
		ID:      "tag:silo:ebooks:opds:search",
		Title:   "Search: " + q,
		Updated: time.Now().UTC().Format(time.RFC3339),
	}
	for _, b := range env.Items {
		entry := opdsEntry{
			ID: "tag:silo:ebooks:book:" + b.ID, Title: b.Title,
			Updated: time.Now().UTC().Format(time.RFC3339),
		}
		for _, a := range b.Authors {
			entry.Authors = append(entry.Authors, opdsAuth{Name: a})
		}
		for _, f := range b.Formats {
			entry.Links = append(entry.Links, opdsLink{
				Rel: "http://opds-spec.org/acquisition", Type: formatMime(f),
				Href: fmt.Sprintf("/opds/book/%s/download/%s", b.ID, f),
			})
		}
		feed.Entries = append(feed.Entries, entry)
	}
	writeOPDS(w, r, feed)
}

// buildOPDSCollectionsFeed renders the authenticated profile's collections as
// an OPDS navigation feed; each entry links to that collection's acquisition
// feed at /opds/collection/{id}.
func buildOPDSCollectionsFeed(cols []store.Collection, now time.Time) opdsFeed {
	feed := opdsFeed{
		XMLNS:   "http://www.w3.org/2005/Atom",
		ID:      "tag:silo:ebooks:opds:collections",
		Title:   "My Collections",
		Updated: now.UTC().Format(time.RFC3339),
		Links: []opdsLink{
			{Rel: "self", Type: "application/atom+xml;profile=opds-catalog", Href: "/opds/collections"},
		},
	}
	for _, c := range cols {
		feed.Entries = append(feed.Entries, opdsEntry{
			ID:      "tag:silo:ebooks:collection:" + c.ID,
			Title:   c.Name,
			Updated: now.UTC().Format(time.RFC3339),
			Links: []opdsLink{{
				Rel:  "subsection",
				Type: "application/atom+xml;profile=opds-catalog;kind=acquisition",
				Href: "/opds/collection/" + c.ID,
			}},
		})
	}
	return feed
}

func (s *Server) handleOPDSCollections(w http.ResponseWriter, r *http.Request) {
	userID, profileID, autherr := s.opdsAuth(r)
	if autherr != nil {
		writeErr(w, http.StatusBadGateway, "auth service unavailable")
		return
	}
	if userID == "" {
		s.opdsChallenge(w, r)
		return
	}
	cols, err := s.deps.Store.ListCollectionsByProfile(r.Context(), userID, profileID)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeOPDS(w, r, buildOPDSCollectionsFeed(cols, time.Now()))
}

func (s *Server) handleOPDSCollection(w http.ResponseWriter, r *http.Request) {
	userID, profileID, autherr := s.opdsAuth(r)
	if autherr != nil {
		writeErr(w, http.StatusBadGateway, "auth service unavailable")
		return
	}
	if userID == "" {
		s.opdsChallenge(w, r)
		return
	}
	collectionID := chi.URLParam(r, "id")
	items, err := s.deps.Store.ListItemsForUser(r.Context(), userID, profileID, collectionID)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	cfg, _ := s.deps.Store.GetConfig(r.Context())
	if !cfg.HasBackend() {
		writeErr(w, 412, "no backend")
		return
	}
	bk := backend.NewEbookBackend(s.deps.Host, cfg.BackendTarget())
	now := time.Now()
	feed := opdsFeed{
		XMLNS:   "http://www.w3.org/2005/Atom",
		ID:      "tag:silo:ebooks:opds:collection:" + collectionID,
		Title:   "Collection",
		Updated: now.UTC().Format(time.RFC3339),
		Links: []opdsLink{
			{Rel: "self", Type: "application/atom+xml;profile=opds-catalog;kind=acquisition", Href: "/opds/collection/" + collectionID},
		},
	}
	for _, item := range items {
		d, err := bk.GetBook(r.Context(), item.BookID)
		if err != nil {
			writeBadGateway(w, r, err)
			return
		}
		entry := opdsEntry{
			ID:      "tag:silo:ebooks:book:" + d.ID,
			Title:   d.Title,
			Summary: d.Description,
			Updated: now.UTC().Format(time.RFC3339),
		}
		for _, a := range d.Authors {
			entry.Authors = append(entry.Authors, opdsAuth{Name: a})
		}
		for _, f := range d.Files {
			entry.Links = append(entry.Links, opdsLink{
				Rel:  "http://opds-spec.org/acquisition",
				Type: f.MimeType,
				Href: fmt.Sprintf("/opds/book/%s/download/%s", d.ID, f.Format),
			})
		}
		feed.Entries = append(feed.Entries, entry)
	}
	writeOPDS(w, r, feed)
}

func (s *Server) handleOPDSBookEntry(w http.ResponseWriter, r *http.Request) {
	userID, _, autherr := s.opdsAuth(r)
	if autherr != nil {
		writeErr(w, http.StatusBadGateway, "auth service unavailable")
		return
	}
	if userID == "" {
		s.opdsChallenge(w, r)
		return
	}
	_ = userID
	cfg, _ := s.deps.Store.GetConfig(r.Context())
	if !cfg.HasBackend() {
		writeErr(w, 412, "no backend")
		return
	}
	bk := backend.NewEbookBackend(s.deps.Host, cfg.BackendTarget())
	d, err := bk.GetBook(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeBadGateway(w, r, err)
		return
	}
	entry := opdsEntry{
		ID: "tag:silo:ebooks:book:" + d.ID, Title: d.Title, Summary: d.Description,
		Updated: time.Now().UTC().Format(time.RFC3339),
	}
	for _, a := range d.Authors {
		entry.Authors = append(entry.Authors, opdsAuth{Name: a})
	}
	for _, f := range d.Files {
		entry.Links = append(entry.Links, opdsLink{
			Rel: "http://opds-spec.org/acquisition", Type: f.MimeType,
			Href: fmt.Sprintf("/opds/book/%s/download/%s", d.ID, f.Format),
		})
	}
	w.Header().Set("Content-Type", "application/atom+xml;profile=opds-catalog;kind=acquisition")
	_ = xml.NewEncoder(w).Encode(entry)
}

func (s *Server) handleOPDSDownload(w http.ResponseWriter, r *http.Request) {
	userID, _, autherr := s.opdsAuth(r)
	if autherr != nil {
		writeErr(w, http.StatusBadGateway, "auth service unavailable")
		return
	}
	if userID == "" {
		s.opdsChallenge(w, r)
		return
	}
	cfg, _ := s.deps.Store.GetConfig(r.Context())
	if !cfg.HasBackend() {
		writeErr(w, 412, "no backend")
		return
	}
	// The OPDS feed embeds portal-encoded book refs ("libID:b64bookid") so a
	// multi-library deployment can route the download to the right backend.
	// Plain ids (no colon) fall back to the configured default backend, which
	// preserves single-library deployments unchanged.
	rawRef := chi.URLParam(r, "id")
	libraryID, bookID, _ := decodeBookRef(rawRef)
	installID := cfg.BackendTarget()
	if libraryID > 0 {
		if lib, err := s.deps.Store.GetPortalLibrary(r.Context(), libraryID); err == nil && lib.BackendPluginID != "" {
			installID = lib.BackendPluginID
		}
	}
	bk := backend.NewEbookBackend(s.deps.Host, installID)
	// Mint a signed media token before hitting the backend — the backend's
	// /api/v1/file/* route is public + token-gated, an unsigned fetch 401s.
	resp, err := s.deps.Host.GetStream(r.Context(), installID, bk.SignedFilePath(userID, bookID, cfg.MediaSigningSecret), nil)
	if err != nil {
		writeBadGateway(w, r, err)
		return
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	for _, h := range []string{"Content-Type", "Content-Length"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// opdsFeedETag derives a weak ETag from the feed's entry identifiers and the
// link Hrefs (so the next-cursor link is part of the cache key — clients that
// pinned page 1 must invalidate when a new book pushes the head). We use SHA-1
// truncated to 16 hex chars; the value is wrapped W/"..." since the underlying
// XML is not byte-stable (timestamps in Updated change per request).
func opdsFeedETag(f opdsFeed) string {
	h := sha1.New()
	for _, e := range f.Entries {
		_, _ = io.WriteString(h, e.ID)
		_, _ = io.WriteString(h, "|")
		_, _ = io.WriteString(h, e.Updated)
		_, _ = io.WriteString(h, "\n")
	}
	for _, lk := range f.Links {
		_, _ = io.WriteString(h, lk.Rel)
		_, _ = io.WriteString(h, "=")
		_, _ = io.WriteString(h, lk.Href)
		_, _ = io.WriteString(h, "\n")
	}
	return fmt.Sprintf(`W/"%s"`, hex.EncodeToString(h.Sum(nil))[:16])
}

// writeOPDS serialises an OPDS feed with HTTP caching headers. Clients that
// resend a matching If-None-Match get 304; otherwise we emit a weak ETag and a
// 60-second private Cache-Control so polling OPDS clients don't re-download
// the full feed every refresh.
func writeOPDS(w http.ResponseWriter, r *http.Request, f opdsFeed) {
	etag := opdsFeedETag(f)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, max-age=60")
	if match := r.Header.Get("If-None-Match"); match != "" && match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/atom+xml;profile=opds-catalog;kind=acquisition")
	_, _ = w.Write([]byte(xml.Header))
	_ = xml.NewEncoder(w).Encode(f)
}

func formatMime(f string) string {
	switch strings.ToLower(f) {
	case "epub":
		return "application/epub+zip"
	case "pdf":
		return "application/pdf"
	case "mobi":
		return "application/x-mobipocket-ebook"
	case "azw", "azw3":
		return "application/vnd.amazon.ebook"
	}
	return "application/octet-stream"
}

// opdsAuth validates OPDS Basic-Auth and returns the resolved (userID,
// profileID). Both are "" when the request is unauthorized. profileID is ""
// for the primary profile. The error return distinguishes a bad credential
// (nil error, empty ids) from an auth-service failure (non-nil error).
func (s *Server) opdsAuth(r *http.Request) (string, string, error) {
	user, pass, ok := r.BasicAuth()
	if !ok || user == "" || pass == "" {
		return "", "", nil
	}
	if s.deps.Credentials == nil {
		return "", "", errors.New("credential validator not configured")
	}
	userID, profileID, err := s.deps.Credentials.ValidateProfileCredential(r.Context(), user, pass)
	if err != nil {
		if status.Code(err) == codes.Unauthenticated {
			return "", "", nil // bad credential — not a service error
		}
		return "", "", err // transport / Unimplemented — service error
	}
	return userID, profileID, nil
}

func (s *Server) opdsChallenge(w http.ResponseWriter, r *http.Request) {
	cfg, _ := s.deps.Store.GetConfig(r.Context())
	realm := cfg.OpdsRealm
	if realm == "" {
		realm = "Silo Library"
	}
	w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Basic realm=%q`, realm))
	http.Error(w, "auth required", http.StatusUnauthorized)
}

// -- kosync routes --------------------------------------------------------

func (s *Server) mountKosync(r chi.Router) {
	r.Post("/users/auth", s.handleKosyncAuth)
	r.Get("/syncs/progress/{document}", s.handleKosyncGetProgress)
	r.Put("/syncs/progress", s.handleKosyncPutProgress)
}

func (s *Server) handleKosyncCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if body.Username == "" || body.Password == "" {
		writeErr(w, 400, "username/password required")
		return
	}
	id, _ := auth.FromContext(r.Context())
	if id.UserID == "" {
		writeErr(w, 401, "authentication required")
		return
	}

	// KOReader hashes password client-side as sha1(password) → we then bcrypt.
	pwsha1 := sha1.Sum([]byte(body.Password))
	pwhex := hex.EncodeToString(pwsha1[:])
	hash, err := bcrypt.GenerateFromPassword([]byte(pwhex), bcrypt.DefaultCost)
	if err != nil {
		slog.Error("kosync bcrypt", "err", err)
		writeErr(w, 500, "internal error")
		return
	}

	// Compute the kosync username: bare userName for the primary profile,
	// userName#profileName for named profiles. Profile names come from the
	// host-injected headers so the SPA can pass them through on registration.
	userName := r.Header.Get("X-Silo-User-Name")
	if userName == "" {
		userName = body.Username
	}
	profileName := r.Header.Get("X-Silo-Profile-Name")
	kosyncUsername := userName
	if id.ProfileID != "" && profileName != "" {
		kosyncUsername = userName + "#" + profileName
	}

	// Authenticated registration: the silo user owns the account and may
	// rotate their own password (owner-scoped DO UPDATE).
	if err := s.deps.Store.UpsertKosyncUser(r.Context(), store.KosyncUser{
		UserID:             id.UserID,
		ProfileID:          id.ProfileID,
		KosyncUsername:     kosyncUsername,
		KosyncPasswordHash: string(hash),
	}); err != nil {
		if errors.Is(err, store.ErrKosyncUsernameTaken) {
			writeErr(w, 409, "kosync username already taken")
			return
		}
		slog.Error("kosync upsert", "err", err)
		writeErr(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, map[string]any{"username": kosyncUsername})
}

func (s *Server) kosyncAuthHeader(r *http.Request) (store.KosyncUser, error) {
	username := r.Header.Get("x-auth-user")
	key := r.Header.Get("x-auth-key")
	if username == "" || key == "" {
		return store.KosyncUser{}, errors.New("missing auth headers")
	}
	u, err := s.deps.Store.GetKosyncUserByUsername(r.Context(), username)
	if err != nil {
		return store.KosyncUser{}, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.KosyncPasswordHash), []byte(key)); err != nil {
		return store.KosyncUser{}, err
	}
	return u, nil
}

func (s *Server) handleKosyncAuth(w http.ResponseWriter, r *http.Request) {
	if _, err := s.kosyncAuthHeader(r); err != nil {
		writeErr(w, 401, "unauthorized")
		return
	}
	writeJSON(w, 200, map[string]any{"authorized": "OK"})
}

func (s *Server) handleKosyncGetProgress(w http.ResponseWriter, r *http.Request) {
	u, err := s.kosyncAuthHeader(r)
	if err != nil {
		writeErr(w, 401, "unauthorized")
		return
	}
	doc := chi.URLParam(r, "document")
	p, err := s.deps.Store.GetKosyncProgress(r.Context(), u.UserID, u.ProfileID, doc)
	if err != nil {
		writeJSON(w, 200, nil)
		return
	}
	writeJSON(w, 200, map[string]any{
		"document": p.Document, "progress": p.Progress,
		"percentage": p.Percentage, "device": p.Device, "device_id": p.DeviceID,
		"timestamp": p.Timestamp,
	})
}

func (s *Server) handleKosyncPutProgress(w http.ResponseWriter, r *http.Request) {
	u, err := s.kosyncAuthHeader(r)
	if err != nil {
		writeErr(w, 401, "unauthorized")
		return
	}
	var body struct {
		Document   string  `json:"document"`
		Progress   string  `json:"progress"`
		Percentage float64 `json:"percentage"`
		Device     string  `json:"device"`
		DeviceID   string  `json:"device_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	// UserID is taken from the authenticated kosync session (u.UserID), never
	// from the request body — otherwise a malicious client could clobber any
	// other user's progress by lying about who they are. DeviceID is
	// client-supplied but bound to the authenticated user via the
	// (user_id, document, device_id) primary key, so a malicious client can
	// only collide with rows owned by their own user.
	if err := s.deps.Store.UpsertKosyncProgress(r.Context(), store.KosyncProgress{
		UserID: u.UserID, ProfileID: u.ProfileID, Document: body.Document, Progress: body.Progress,
		Percentage: body.Percentage, Device: body.Device, DeviceID: body.DeviceID,
	}); err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"document": body.Document})
}

// User-facing kosync management (under /api/v1/me/kosync)
func (s *Server) handleKosyncStatus(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	users, _ := s.deps.Store.ListKosyncUsers(r.Context())
	for _, u := range users {
		if u.UserID == id.UserID && u.ProfileID == id.ProfileID {
			writeJSON(w, 200, map[string]any{
				"registered": true, "kosync_username": u.KosyncUsername,
			})
			return
		}
	}
	writeJSON(w, 200, map[string]any{"registered": false})
}

func (s *Server) handleKosyncRegister(w http.ResponseWriter, r *http.Request) {
	s.handleKosyncCreate(w, r)
}

func (s *Server) handleKosyncDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	users, _ := s.deps.Store.ListKosyncUsers(r.Context())
	for _, u := range users {
		if u.UserID == id.UserID && u.ProfileID == id.ProfileID {
			_ = s.deps.Store.DeleteKosyncUser(r.Context(), u.KosyncUsername)
			w.WriteHeader(204)
			return
		}
	}
	w.WriteHeader(404)
}

// -- Kobo browser-served transfer ----------------------------------------

func (s *Server) mountKobo(r chi.Router) {
	r.Get("/{code}", s.handleKoboServeFile)
}

// findKoboSessionForCode walks an active-session list and returns the first
// row whose stored CodeHash bcrypt-matches the URL-supplied code. Extracted
// so tests can exercise the hash-comparison loop without a real DB.
func findKoboSessionForCode(sessions []store.KoboSession, code string) (store.KoboSession, bool) {
	for _, candidate := range sessions {
		if bcrypt.CompareHashAndPassword([]byte(candidate.CodeHash), []byte(code)) == nil {
			return candidate, true
		}
	}
	return store.KoboSession{}, false
}

func (s *Server) handleKoboServeFile(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	// Scan pending/active sessions and bcrypt-compare each row's hash against
	// the URL-supplied code. The pending/active partial index keeps this set
	// small in practice (sessions expire after 30m), and bcrypt's
	// constant-time compare deflates timing-distinguisher attacks across rows.
	sessions, err := s.deps.Store.ListActiveKoboSessions(r.Context(), time.Now())
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	sess, matched := findKoboSessionForCode(sessions, code)
	if !matched {
		writeErr(w, 404, "not found")
		return
	}
	if sess.Status == "completed" || sess.Status == "expired" || time.Now().After(sess.ExpiresAt) {
		writeErr(w, 410, "session expired")
		return
	}
	// Hold an in-process refcount on this session for the duration of the
	// transfer so KoboSessionReaper can't unlink sess.SourcePath while we're
	// mid-io.Copy. Refcount only blocks NEW evictions — if a past reap
	// already removed the file, os.Open returns 410 below.
	if s.deps.KoboRefs != nil {
		release := s.deps.KoboRefs.Acquire(sess.ID)
		defer release()
	}
	_ = s.deps.Store.MarkKoboActiveByID(r.Context(), sess.ID)
	f, err := os.Open(sess.SourcePath)
	if err != nil {
		writeErr(w, 410, "file gone")
		return
	}
	defer f.Close()
	stat, _ := f.Stat()
	w.Header().Set("Content-Type", "application/epub+zip")
	if stat != nil {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
	}
	w.Header().Set("Content-Disposition", "attachment; filename=\"book.kepub.epub\"")
	_, _ = io.Copy(w, f)
	_ = s.deps.Store.MarkKoboCompletedByID(r.Context(), sess.ID)
}
