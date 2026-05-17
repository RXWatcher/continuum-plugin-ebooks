package server

import (
	cryptoRand "crypto/rand"
	"crypto/sha1"
	"encoding/base64"
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
	"github.com/oklog/ulid/v2"
	"golang.org/x/crypto/bcrypt"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/auth"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/backend"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/store"
)

// -- OPDS feeds (public route; basic-auth handled here) ------------------

func (s *Server) mountOPDS(r chi.Router) {
	r.Get("/", s.handleOPDSRoot)
	r.Get("/catalog", s.handleOPDSCatalog)
	r.Get("/search", s.handleOPDSSearch)
	r.Get("/book/{id}", s.handleOPDSBookEntry)
	r.Get("/book/{id}/download/{format}", s.handleOPDSDownload)
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
		ID:      "tag:continuum:ebooks:opds",
		Title:   "Continuum Library",
		Updated: time.Now().UTC().Format(time.RFC3339),
		Links: []opdsLink{
			{Rel: "self", Type: "application/atom+xml;profile=opds-catalog", Href: "/opds/"},
			{Rel: "start", Type: "application/atom+xml;profile=opds-catalog", Href: "/opds/"},
			{Rel: "http://opds-spec.org/sort/new", Type: "application/atom+xml;profile=opds-catalog;kind=acquisition", Href: "/opds/catalog"},
			{Rel: "search", Type: "application/opensearchdescription+xml", Href: "/opds/search"},
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
		ID:      "tag:continuum:ebooks:opds:catalog",
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
			ID: "tag:continuum:ebooks:book:" + b.ID, Title: b.Title,
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
	if !s.opdsAuth(r) {
		s.opdsChallenge(w, r)
		return
	}
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
		writeErr(w, 502, err.Error())
		return
	}
	feed := buildOPDSCatalogFeed(env, cfg.OpdsRealm, limit, time.Now())
	writeOPDS(w, r, feed)
}

func (s *Server) handleOPDSSearch(w http.ResponseWriter, r *http.Request) {
	if !s.opdsAuth(r) {
		s.opdsChallenge(w, r)
		return
	}
	q := r.URL.Query().Get("q")
	if q == "" {
		// Return OpenSearch description.
		w.Header().Set("Content-Type", "application/opensearchdescription+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<OpenSearchDescription xmlns="http://a9.com/-/spec/opensearch/1.1/">
  <ShortName>Continuum Library</ShortName>
  <Description>Search Continuum's ebook library</Description>
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
		writeErr(w, 502, err.Error())
		return
	}
	feed := opdsFeed{
		XMLNS:   "http://www.w3.org/2005/Atom",
		ID:      "tag:continuum:ebooks:opds:search",
		Title:   "Search: " + q,
		Updated: time.Now().UTC().Format(time.RFC3339),
	}
	for _, b := range env.Items {
		entry := opdsEntry{
			ID: "tag:continuum:ebooks:book:" + b.ID, Title: b.Title,
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

func (s *Server) handleOPDSBookEntry(w http.ResponseWriter, r *http.Request) {
	if !s.opdsAuth(r) {
		s.opdsChallenge(w, r)
		return
	}
	cfg, _ := s.deps.Store.GetConfig(r.Context())
	if !cfg.HasBackend() {
		writeErr(w, 412, "no backend")
		return
	}
	bk := backend.NewEbookBackend(s.deps.Host, cfg.BackendTarget())
	d, err := bk.GetBook(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	entry := opdsEntry{
		ID: "tag:continuum:ebooks:book:" + d.ID, Title: d.Title, Summary: d.Description,
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
	if !s.opdsAuth(r) {
		s.opdsChallenge(w, r)
		return
	}
	cfg, _ := s.deps.Store.GetConfig(r.Context())
	if !cfg.HasBackend() {
		writeErr(w, 412, "no backend")
		return
	}
	bk := backend.NewEbookBackend(s.deps.Host, cfg.BackendTarget())
	bookID := chi.URLParam(r, "id")
	format := chi.URLParam(r, "format")
	resp, err := s.deps.Host.GetStream(r.Context(), cfg.BackendTarget(), bk.FilePath(bookID, format), nil)
	if err != nil {
		writeErr(w, 502, err.Error())
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

func (s *Server) opdsAuth(r *http.Request) bool {
	user, pass, ok := r.BasicAuth()
	if !ok || user == "" || pass == "" {
		return false
	}
	t, err := s.deps.Store.GetOPDSTokenByJTI(r.Context(), pass)
	if err != nil {
		return false
	}
	if t.UserID != user {
		return false
	}
	if err := bcrypt.CompareHashAndPassword([]byte(t.TokenHash), []byte(pass)); err != nil {
		return false
	}
	_ = s.deps.Store.TouchOPDSToken(r.Context(), pass)
	return true
}

func (s *Server) opdsChallenge(w http.ResponseWriter, r *http.Request) {
	cfg, _ := s.deps.Store.GetConfig(r.Context())
	realm := cfg.OpdsRealm
	if realm == "" {
		realm = "Continuum Library"
	}
	w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Basic realm=%q`, realm))
	http.Error(w, "auth required", http.StatusUnauthorized)
}

// OPDS token user endpoints
func (s *Server) handleListOPDSTokens(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	ts, _ := s.deps.Store.ListOPDSTokensByUser(r.Context(), id.UserID)
	out := make([]map[string]any, 0, len(ts))
	for _, t := range ts {
		out = append(out, map[string]any{
			"id":           t.ID,
			"label":        t.Label,
			"last_used_at": t.LastUsedAt,
			"created_at":   t.CreatedAt,
			"revoked":      t.RevokedAt != nil,
		})
	}
	writeJSON(w, 200, map[string]any{"items": out})
}

func (s *Server) handleCreateOPDSToken(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	var body struct {
		Label string `json:"label"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	// Random JTI shown to user once; hash stored.
	buf := make([]byte, 24)
	if _, err := io.ReadFull(cryptoRand.Reader, buf); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	jti := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(buf)
	hash, err := bcrypt.GenerateFromPassword([]byte(jti), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	t := store.OPDSToken{
		ID: ulid.Make().String(), UserID: id.UserID, JTI: jti, TokenHash: string(hash), Label: body.Label,
	}
	if err := s.deps.Store.InsertOPDSToken(r.Context(), t); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{"id": t.ID, "label": t.Label, "jti_shown_once": jti})
}

func (s *Server) handleRevokeOPDSToken(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	tid := chi.URLParam(r, "id")
	if err := s.deps.Store.RevokeOPDSToken(r.Context(), tid, id.UserID); err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	w.WriteHeader(204)
}

// -- kosync routes --------------------------------------------------------

func (s *Server) mountKosync(r chi.Router) {
	r.Post("/users/create", s.handleKosyncCreate)
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
	// KOReader hashes password client-side as sha1(password) → we then bcrypt.
	pwsha1 := sha1.Sum([]byte(body.Password))
	pwhex := hex.EncodeToString(pwsha1[:])
	hash, err := bcrypt.GenerateFromPassword([]byte(pwhex), bcrypt.DefaultCost)
	if err != nil {
		slog.Error("kosync bcrypt", "err", err)
		writeErr(w, 500, "internal error")
		return
	}

	// This handler serves BOTH the public KOReader /kosync/users/create
	// (access:"public" — the host injects NO identity, so id.UserID == "")
	// and the authenticated /api/v1/me/kosync/register. When unauthenticated,
	// the kosync account is standalone and MUST be keyed by its globally
	// unique username, not the empty continuum id — otherwise every
	// KOReader-registered user collapses to user_id="" and (a) shares/clobbers
	// every other user's reading progress and (b) the owner-scoped upsert
	// lets anyone overwrite an existing account's password (takeover).
	if id.UserID == "" {
		owner := "kosync:" + body.Username
		if err := s.deps.Store.CreateKosyncUserStrict(r.Context(), store.KosyncUser{
			UserID:             owner,
			KosyncUsername:     body.Username,
			KosyncPasswordHash: string(hash),
		}); err != nil {
			if errors.Is(err, store.ErrKosyncUsernameTaken) {
				writeErr(w, 409, "kosync username already taken")
				return
			}
			slog.Error("kosync create", "err", err)
			writeErr(w, 500, "internal error")
			return
		}
		writeJSON(w, 200, map[string]any{"username": body.Username})
		return
	}

	// Authenticated path: the continuum user owns the account and may rotate
	// their own password (owner-scoped DO UPDATE).
	if err := s.deps.Store.UpsertKosyncUser(r.Context(), store.KosyncUser{
		UserID:             id.UserID,
		KosyncUsername:     body.Username,
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
	writeJSON(w, 200, map[string]any{"username": body.Username})
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
	p, err := s.deps.Store.GetKosyncProgress(r.Context(), u.UserID, doc)
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
		UserID: u.UserID, Document: body.Document, Progress: body.Progress,
		Percentage: body.Percentage, Device: body.Device, DeviceID: body.DeviceID,
	}); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"document": body.Document})
}

// User-facing kosync management (under /api/v1/me/kosync)
func (s *Server) handleKosyncStatus(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	users, _ := s.deps.Store.ListKosyncUsers(r.Context())
	for _, u := range users {
		if u.UserID == id.UserID {
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
		if u.UserID == id.UserID {
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
		writeErr(w, 500, err.Error())
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
