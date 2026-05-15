package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
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
	body, code, err := b.host.Get(ctx, b.installID, "/api/v1/capabilities")
	if err != nil {
		return Capabilities{}, err
	}
	if code != 200 {
		return Capabilities{}, fmt.Errorf("backend /capabilities returned %d: %s", code, string(body))
	}
	var c Capabilities
	if err := json.Unmarshal(body, &c); err != nil {
		return Capabilities{}, fmt.Errorf("decode: %w", err)
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
	body, code, err := b.host.Get(ctx, b.installID, "/api/v1/catalog?"+q.Encode())
	if err != nil {
		return PageEnvelope[EbookSummary]{}, err
	}
	if code != 200 {
		return PageEnvelope[EbookSummary]{}, fmt.Errorf("upstream %d: %s", code, string(body))
	}
	var env PageEnvelope[EbookSummary]
	if err := json.Unmarshal(body, &env); err != nil {
		return PageEnvelope[EbookSummary]{}, fmt.Errorf("decode: %w", err)
	}
	return env, nil
}

func (b *EbookBackend) ListLibraries(ctx context.Context) ([]LibraryInfo, error) {
	body, code, err := b.host.Get(ctx, b.installID, "/api/v1/catalog/libraries")
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("upstream %d: %s", code, string(body))
	}
	var env struct {
		Items []LibraryInfo `json:"items"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return env.Items, nil
}

func (b *EbookBackend) Search(ctx context.Context, query string) (PageEnvelope[EbookSummary], error) {
	q := url.Values{}
	q.Set("q", query)
	body, code, err := b.host.Get(ctx, b.installID, "/api/v1/catalog/search?"+q.Encode())
	if err != nil {
		return PageEnvelope[EbookSummary]{}, err
	}
	if code != 200 {
		return PageEnvelope[EbookSummary]{}, fmt.Errorf("upstream %d: %s", code, string(body))
	}
	var env PageEnvelope[EbookSummary]
	if err := json.Unmarshal(body, &env); err != nil {
		return PageEnvelope[EbookSummary]{}, fmt.Errorf("decode: %w", err)
	}
	return env, nil
}

func (b *EbookBackend) GetBook(ctx context.Context, bookID string) (EbookDetail, error) {
	body, code, err := b.host.Get(ctx, b.installID, "/api/v1/catalog/"+url.PathEscape(bookID))
	if err != nil {
		return EbookDetail{}, err
	}
	if code != 200 {
		return EbookDetail{}, fmt.Errorf("upstream %d: %s", code, string(body))
	}
	var d EbookDetail
	if err := json.Unmarshal(body, &d); err != nil {
		return EbookDetail{}, fmt.Errorf("decode: %w", err)
	}
	return d, nil
}

// FileURL constructs the host-proxy URL for the file fetch endpoint (the
// portal's streaming layer hits this).
func (b *EbookBackend) FilePath(bookID, format string) string {
	return fmt.Sprintf("/api/v1/file/%s?format=%s", url.PathEscape(bookID), url.QueryEscape(format))
}

func (b *EbookBackend) CoverPath(bookID, size string) string {
	return fmt.Sprintf("/api/v1/cover/%s/%s", url.PathEscape(bookID), url.PathEscape(size))
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
	body, code, err := b.host.Get(ctx, b.installID, "/api/v1/browse/"+kind+"?"+q.Encode())
	if err != nil {
		return PageEnvelope[FacetItem]{}, err
	}
	// ebookdb intentionally has no browse endpoints — treat 404 as "no facets
	// available" rather than a hard error so the portal can render an empty
	// state.
	if code == 404 {
		return PageEnvelope[FacetItem]{Items: []FacetItem{}}, nil
	}
	if code != 200 {
		return PageEnvelope[FacetItem]{}, fmt.Errorf("upstream %d: %s", code, string(body))
	}
	var env PageEnvelope[FacetItem]
	if err := json.Unmarshal(body, &env); err != nil {
		return PageEnvelope[FacetItem]{}, fmt.Errorf("decode: %w", err)
	}
	return env, nil
}

// RequestSnapshot returns a snapshot of the upstream request state.
type RequestSnapshot struct {
	ExternalID string `json:"external_id"`
	Status     string `json:"status"`
}

func (b *EbookBackend) GetRequestSnapshot(ctx context.Context, externalID string) (RequestSnapshot, error) {
	body, code, err := b.host.Get(ctx, b.installID, "/api/v1/requests/"+url.PathEscape(externalID))
	if err != nil {
		return RequestSnapshot{}, err
	}
	if code != 200 {
		return RequestSnapshot{}, fmt.Errorf("upstream %d: %s", code, string(body))
	}
	var snap RequestSnapshot
	if err := json.Unmarshal(body, &snap); err != nil {
		return RequestSnapshot{}, fmt.Errorf("decode: %w", err)
	}
	return snap, nil
}
