package backend

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"

	"github.com/RXWatcher/continuum-plugin-ebooks/internal/mediatoken"
)

// Capabilities mirrors the backend /capabilities response shape.
type Capabilities struct {
	Formats                []string `json:"formats"`
	Features               []string `json:"features"`
	MaxConcurrentDownloads int      `json:"max_concurrent_downloads"`
	SupportsRangeRequests  bool     `json:"supports_range_requests"`
}

// EbookSummary mirrors the ebook_backend.v1 contract response.
type EbookSummary struct {
	ID          string   `json:"id"`
	LibraryID   int64    `json:"library_id,omitempty"`
	LibraryName string   `json:"library_name,omitempty"`
	MediaType   string   `json:"media_type,omitempty"`
	Title       string   `json:"title"`
	Authors     []string `json:"authors,omitempty"`
	Series      string   `json:"series,omitempty"`
	SeriesIndex float64  `json:"series_index,omitempty"`
	Year        int      `json:"year,omitempty"`
	Language    string   `json:"language,omitempty"`
	CoverURL    string   `json:"cover_url,omitempty"`
	HasCover    bool     `json:"has_cover"`
	Rating      float64  `json:"rating,omitempty"`
	Formats     []string `json:"formats"`
}

type EbookFile struct {
	Format    string `json:"format"`
	SizeBytes int64  `json:"size_bytes"`
	MimeType  string `json:"mime_type"`
	URL       string `json:"url,omitempty"`
}

type EbookDetail struct {
	EbookSummary
	Description string      `json:"description,omitempty"`
	ISBN        string      `json:"isbn,omitempty"`
	Publisher   string      `json:"publisher,omitempty"`
	Genres      []string    `json:"genres,omitempty"`
	Tags        []string    `json:"tags,omitempty"`
	Files       []EbookFile `json:"files"`
	UpstreamID  string      `json:"upstream_id,omitempty"`
}

type LibraryInfo struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Path          string `json:"path,omitempty"`
	MediaType     string `json:"media_type"`
	Enabled       bool   `json:"enabled"`
	LastScannedAt string `json:"last_scanned_at,omitempty"`
}

type PageEnvelope[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
	Total      int    `json:"total,omitempty"`
}

// EbookBackend is the typed facade over HostHTTPClient for one installed
// ebook_backend.v1 plugin. Construct one per request/handler using the
// installID resolved from backend_config.
type EbookBackend struct {
	host      *HostHTTPClient
	installID string
}

func NewEbookBackend(host *HostHTTPClient, installID string) *EbookBackend {
	return &EbookBackend{host: host, installID: installID}
}

func (b *EbookBackend) GetCapabilities(ctx context.Context) (Capabilities, error) {
	var c Capabilities
	if _, err := b.host.GetJSON(ctx, b.installID, "/api/v1/capabilities", &c); err != nil {
		return Capabilities{}, err
	}
	return c, nil
}

// CatalogQuery captures all optional inputs to ListCatalog. Filter fields
// (Author/Series/Genre/Tag) pass through to the backend's /catalog endpoint.
// Genre must be the upstream slug returned by the selected provider.
type CatalogQuery struct {
	Cursor    string
	Sort      string
	Order     string
	Limit     int
	LibraryID int64
	Author    string
	Series    string
	Genre     string
	Tag       string
}

func (b *EbookBackend) ListCatalog(ctx context.Context, p CatalogQuery) (PageEnvelope[EbookSummary], error) {
	q := url.Values{}
	if p.Cursor != "" {
		q.Set("cursor", p.Cursor)
	}
	if p.Sort != "" {
		q.Set("sort", p.Sort)
	}
	if p.Order != "" {
		q.Set("order", p.Order)
	}
	if p.Limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", p.Limit))
	}
	if p.LibraryID > 0 {
		q.Set("library_id", fmt.Sprintf("%d", p.LibraryID))
	}
	if p.Author != "" {
		q.Set("author", p.Author)
	}
	if p.Series != "" {
		q.Set("series", p.Series)
	}
	if p.Genre != "" {
		q.Set("genre", p.Genre)
	}
	if p.Tag != "" {
		q.Set("tag", p.Tag)
	}
	var env PageEnvelope[EbookSummary]
	if _, err := b.host.GetJSON(ctx, b.installID, "/api/v1/catalog?"+q.Encode(), &env); err != nil {
		return PageEnvelope[EbookSummary]{}, err
	}
	return env, nil
}

func (b *EbookBackend) ListLibraries(ctx context.Context) ([]LibraryInfo, error) {
	var env struct {
		Items []LibraryInfo `json:"items"`
	}
	if _, err := b.host.GetJSON(ctx, b.installID, "/api/v1/catalog/libraries", &env); err != nil {
		return nil, err
	}
	return env.Items, nil
}

func (b *EbookBackend) Search(ctx context.Context, query string) (PageEnvelope[EbookSummary], error) {
	q := url.Values{}
	q.Set("q", query)
	var env PageEnvelope[EbookSummary]
	if _, err := b.host.GetJSON(ctx, b.installID, "/api/v1/catalog/search?"+q.Encode(), &env); err != nil {
		return PageEnvelope[EbookSummary]{}, err
	}
	return env, nil
}

func (b *EbookBackend) GetBook(ctx context.Context, bookID string) (EbookDetail, error) {
	var d EbookDetail
	if _, err := b.host.GetJSON(ctx, b.installID, "/api/v1/catalog/"+url.PathEscape(bookID), &d); err != nil {
		return EbookDetail{}, err
	}
	return d, nil
}

// FilePath returns the backend-relative file route. Both supported ebook
// backends store a single file per book and ignore any ?format= query, so
// the URL doesn't carry format — callers that need the format string for
// display read it from EbookFile.Format on the catalog response.
func (b *EbookBackend) FilePath(bookID string) string {
	return "/api/v1/file/" + url.PathEscape(bookID)
}

// SignedFilePath returns FilePath with a signed media token appended as
// ?token=. Use this for portal server-side fetches via host.GetStream; the
// backend's public file route verifies the token. Returns the unsigned path
// (which the backend will reject) when secret or userID is empty so the
// caller surfaces "secret not configured" instead of a silent auth bypass.
func (b *EbookBackend) SignedFilePath(userID, bookID, secret string) string {
	base := b.FilePath(bookID)
	if secret == "" || userID == "" {
		return base
	}
	tok, err := mediatoken.Mint(secret, userID, bookID, mediatoken.FileFileIdx)
	if err != nil {
		slog.Warn("mint file token failed", "book_id", bookID, "err", err)
		return base
	}
	return base + "?token=" + url.QueryEscape(tok)
}

func (b *EbookBackend) CoverPath(bookID, size string) string {
	return fmt.Sprintf("/api/v1/cover/%s/%s", url.PathEscape(bookID), url.PathEscape(size))
}

// SignedCoverPath returns CoverPath with a signed media token appended for
// portal server-side cover fetches (mostly used by Kobo/Kindle integrations).
func (b *EbookBackend) SignedCoverPath(userID, bookID, size, secret string) string {
	base := b.CoverPath(bookID, size)
	if secret == "" || userID == "" {
		return base
	}
	tok, err := mediatoken.Mint(secret, userID, bookID, mediatoken.CoverFileIdx)
	if err != nil {
		slog.Warn("mint cover token failed", "book_id", bookID, "err", err)
		return base
	}
	return base + "?token=" + url.QueryEscape(tok)
}

// FacetItem mirrors the upstream Author/Series/Genre shape returned by
// /api/v1/browse/<kind> endpoints ({id, name, count}). Count is optional;
// upstream providers omit it when zero.
type FacetItem struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count,omitempty"`
}

// BrowseAuthors proxies GET /api/v1/browse/authors on the configured backend.
// If the backend doesn't support browse (e.g. ebookdb returns 404) we
// gracefully degrade to an empty envelope so the UI can show a "no facets"
// state instead of a hard error.
func (b *EbookBackend) BrowseAuthors(ctx context.Context, cursor string, limit int, libraryID int64) (PageEnvelope[FacetItem], error) {
	return b.browseFacet(ctx, "authors", cursor, limit, libraryID)
}

// BrowseSeries proxies GET /api/v1/browse/series. See BrowseAuthors for the
// graceful-degrade behaviour on backends that don't expose browse.
func (b *EbookBackend) BrowseSeries(ctx context.Context, cursor string, limit int, libraryID int64) (PageEnvelope[FacetItem], error) {
	return b.browseFacet(ctx, "series", cursor, limit, libraryID)
}

// BrowseGenres proxies GET /api/v1/browse/genres. See BrowseAuthors for the
// graceful-degrade behaviour on backends that don't expose browse.
func (b *EbookBackend) BrowseGenres(ctx context.Context, cursor string, limit int, libraryID int64) (PageEnvelope[FacetItem], error) {
	return b.browseFacet(ctx, "genres", cursor, limit, libraryID)
}

func (b *EbookBackend) browseFacet(ctx context.Context, kind, cursor string, limit int, libraryID int64) (PageEnvelope[FacetItem], error) {
	q := url.Values{}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	if limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	if libraryID > 0 {
		q.Set("library_id", fmt.Sprintf("%d", libraryID))
	}
	var env PageEnvelope[FacetItem]
	code, err := b.host.GetJSON(ctx, b.installID, "/api/v1/browse/"+kind+"?"+q.Encode(), &env)
	// ebookdb intentionally has no browse endpoints — treat 404 as "no facets
	// available" rather than a hard error so the portal can render an empty
	// state.
	if code == 404 {
		return PageEnvelope[FacetItem]{Items: []FacetItem{}}, nil
	}
	if err != nil {
		return PageEnvelope[FacetItem]{}, err
	}
	return env, nil
}

// RequestSnapshot returns a snapshot of the upstream request state.
type RequestSnapshot struct {
	ExternalID string `json:"external_id"`
	Status     string `json:"status"`
}

func (b *EbookBackend) GetRequestSnapshot(ctx context.Context, externalID string) (RequestSnapshot, error) {
	var snap RequestSnapshot
	if _, err := b.host.GetJSON(ctx, b.installID, "/api/v1/requests/"+url.PathEscape(externalID), &snap); err != nil {
		return RequestSnapshot{}, err
	}
	return snap, nil
}
