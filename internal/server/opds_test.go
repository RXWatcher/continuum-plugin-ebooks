package server

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/RXWatcher/continuum-plugin-ebooks/internal/backend"
	"github.com/RXWatcher/continuum-plugin-ebooks/internal/store"
)

// TestE1_OPDSCatalogEmitsNextLink confirms the catalog feed builder emits a
// <link rel="next"/> exactly when the backend page envelope carries a
// NextCursor. This is the regression guard against the silent-truncation bug:
// before the fix, catalogs >50 books dropped the tail with no signal to the
// OPDS client. The "next" link's Href must round-trip the cursor and limit
// so a paginating client can fetch the subsequent page.
func TestE1_OPDSCatalogEmitsNextLink(t *testing.T) {
	env := backend.PageEnvelope[backend.EbookSummary]{
		Items: []backend.EbookSummary{
			{ID: "b1", Title: "First", Authors: []string{"A"}, Formats: []string{"epub"}},
			{ID: "b2", Title: "Second", Formats: []string{"epub"}},
		},
		NextCursor: "cur-abc123",
	}
	feed := buildOPDSCatalogFeed(env, "Library", 20, time.Unix(1700000000, 0))

	var nextHref string
	for _, lk := range feed.Links {
		if lk.Rel == "next" {
			nextHref = lk.Href
		}
	}
	if nextHref == "" {
		t.Fatal(`no rel="next" link emitted even though NextCursor was set`)
	}
	if !strings.Contains(nextHref, "cursor=cur-abc123") {
		t.Errorf("next href missing cursor: %s", nextHref)
	}
	if !strings.Contains(nextHref, "limit=20") {
		t.Errorf("next href missing limit: %s", nextHref)
	}
}

// TestE1_OPDSCatalogNoNextLinkWhenComplete confirms the converse: when the
// backend signals the page is the last (NextCursor==""), we MUST NOT emit a
// rel="next" link. Some OPDS readers loop until they stop seeing the link, so
// emitting a spurious next would cause infinite-pagination bugs client-side.
func TestE1_OPDSCatalogNoNextLinkWhenComplete(t *testing.T) {
	env := backend.PageEnvelope[backend.EbookSummary]{
		Items:      []backend.EbookSummary{{ID: "b1", Title: "Only"}},
		NextCursor: "",
	}
	feed := buildOPDSCatalogFeed(env, "Library", 50, time.Unix(1700000000, 0))
	for _, lk := range feed.Links {
		if lk.Rel == "next" {
			t.Errorf("rel=\"next\" emitted on final page: %+v", lk)
		}
	}
}

// TestE1_OpdsCatalogLimit_Clamps verifies ?limit= parsing matches the catalog
// API: blank → 50, valid → as-given, oversize → 200. This is the cap that
// keeps catalog browsing from being a memory-pressure vector.
func TestE1_OpdsCatalogLimit_Clamps(t *testing.T) {
	cases := []struct {
		raw  string
		want int
	}{
		{"", 50},
		{"20", 20},
		{"500", 200},
		{"abc", 50},
		{"-5", 50},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/opds/catalog?limit="+c.raw, nil)
		if got := opdsCatalogLimit(req); got != c.want {
			t.Errorf("limit=%q → %d, want %d", c.raw, got, c.want)
		}
	}
}

// TestE2_KoboBcryptMatch is the core security test for finding E2: the
// stored DB column is the bcrypt hash of the URL code, never the plaintext.
// findKoboSessionForCode must successfully match a session by comparing the
// URL-supplied plaintext against the stored hash, and MUST reject mismatches.
func TestE2_KoboBcryptMatch(t *testing.T) {
	code := "ABCD"
	hash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.MinCost) // MinCost keeps the test fast
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	sessions := []store.KoboSession{
		{ID: "other1", CodeHash: mustHash(t, "WXYZ")},
		{ID: "match", CodeHash: string(hash)},
		{ID: "other2", CodeHash: mustHash(t, "QRST")},
	}

	got, ok := findKoboSessionForCode(sessions, code)
	if !ok {
		t.Fatal("expected match for correct code")
	}
	if got.ID != "match" {
		t.Errorf("matched wrong session: %s", got.ID)
	}

	// Wrong code → no match. This is the regression guard against an
	// accidental "compare plaintext" reintroduction.
	if _, ok := findKoboSessionForCode(sessions, "ZZZZ"); ok {
		t.Error("matched a non-existent code; hash comparison is broken")
	}

	// Empty session list MUST not match anything (defense against the empty
	// active-list edge case yielding a zero-value match).
	if _, ok := findKoboSessionForCode(nil, code); ok {
		t.Error("matched against empty session list")
	}
}

// TestE2_KoboCodeHashIsNotPlaintext is a belt-and-suspenders check: bcrypt's
// hash output for a given plaintext is non-deterministic (random salt). This
// confirms the store column never contains anything that looks like the URL
// code, so a DB dump leak doesn't immediately reveal credentials.
func TestE2_KoboCodeHashIsNotPlaintext(t *testing.T) {
	code := "ABCD"
	h := mustHash(t, code)
	if strings.Contains(h, code) {
		t.Errorf("hash %q contains the plaintext code; defeats the point of bcrypt", h)
	}
	// Bcrypt's standard prefix; this assertion locks the format so a future
	// change away from bcrypt would have to update the comment in
	// 0005_kobo_code_hash.up.sql too.
	if !strings.HasPrefix(h, "$2a$") && !strings.HasPrefix(h, "$2b$") {
		t.Errorf("hash %q is not bcrypt-prefixed", h)
	}
}

func mustHash(t *testing.T, code string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	return string(h)
}

// TestE5_OPDSFeedETagStable verifies the same feed yields a byte-stable ETag.
// This is what makes If-None-Match round-trips actually save bandwidth — if
// the ETag rotated per-request (e.g. tied to time.Now), every client request
// would 200 with full payload.
func TestE5_OPDSFeedETagStable(t *testing.T) {
	feed := opdsFeed{
		Entries: []opdsEntry{
			{ID: "book:1", Updated: "2025-01-01T00:00:00Z"},
			{ID: "book:2", Updated: "2025-01-02T00:00:00Z"},
		},
		Links: []opdsLink{
			{Rel: "self", Href: "/opds/catalog"},
		},
	}
	t1 := opdsFeedETag(feed)
	t2 := opdsFeedETag(feed)
	if t1 != t2 {
		t.Errorf("etag drifted between calls on identical feed: %q vs %q", t1, t2)
	}
	if !strings.HasPrefix(t1, `W/"`) {
		t.Errorf("expected weak etag, got %q", t1)
	}
}

// TestE5_OPDSFeedETagChangesOnContent verifies the ETag actually changes when
// feed content changes — otherwise a stale cache key would shadow real
// updates and the client would never see new books appear.
func TestE5_OPDSFeedETagChangesOnContent(t *testing.T) {
	base := opdsFeed{Entries: []opdsEntry{{ID: "book:1", Updated: "x"}}}
	mutated := opdsFeed{Entries: []opdsEntry{{ID: "book:2", Updated: "x"}}}
	if opdsFeedETag(base) == opdsFeedETag(mutated) {
		t.Error("etag did not change when entry IDs changed; clients would miss updates")
	}
}

// TestE5_OPDSConditional304 is the round-trip test: a client resending the
// ETag in If-None-Match MUST get 304 and no body. This is the actual
// bandwidth-saving check; without it the prior two tests would be moot if
// writeOPDS forgot to honour the header.
func TestE5_OPDSConditional304(t *testing.T) {
	feed := opdsFeed{
		XMLNS: "http://www.w3.org/2005/Atom",
		ID:    "tag:test",
		Entries: []opdsEntry{
			{ID: "book:1", Title: "A", Updated: "2025-01-01T00:00:00Z"},
		},
		Links: []opdsLink{{Rel: "self", Href: "/opds/catalog"}},
	}
	// First request: no If-None-Match, expect 200 + ETag.
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/opds/catalog", nil)
	writeOPDS(rec1, req1, feed)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first response code = %d, want 200", rec1.Code)
	}
	etag := rec1.Header().Get("ETag")
	if etag == "" {
		t.Fatal("first response missing ETag")
	}
	if cc := rec1.Header().Get("Cache-Control"); !strings.Contains(cc, "max-age=60") {
		t.Errorf("Cache-Control = %q, want it to include max-age=60", cc)
	}

	// Second request: client sends If-None-Match with the ETag — expect 304
	// and an EMPTY body (RFC 7232 §4.1).
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/opds/catalog", nil)
	req2.Header.Set("If-None-Match", etag)
	writeOPDS(rec2, req2, feed)
	if rec2.Code != http.StatusNotModified {
		t.Fatalf("304 expected on matching If-None-Match, got %d", rec2.Code)
	}
	if rec2.Body.Len() != 0 {
		t.Errorf("304 body must be empty, got %d bytes", rec2.Body.Len())
	}

	// Sanity: the 200 body must actually be parseable as XML (so the etag
	// short-circuit doesn't accidentally leak into the normal path).
	if !strings.Contains(rec1.Body.String(), "<feed") {
		t.Errorf("200 response body doesn't look like an opds feed: %q", rec1.Body.String())
	}
	var decoded opdsFeed
	if err := xml.Unmarshal(rec1.Body.Bytes(), &decoded); err != nil {
		t.Errorf("200 body did not decode as opdsFeed: %v", err)
	}
}
