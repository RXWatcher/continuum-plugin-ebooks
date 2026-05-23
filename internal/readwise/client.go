// Package readwise wraps the Readwise.io v2 API for pushing
// annotations from the ebook plugin. The API contract is simple
// enough that we don't import a third-party SDK — a Bearer auth
// POST + 50-highlight batches covers the export flow.
package readwise

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	apiBase   = "https://readwise.io/api/v2"
	batchSize = 50 // Readwise's published batch cap; larger requests 400.
)

// Highlight is the wire shape Readwise accepts on POST /highlights.
// Text + title are required; the rest map our annotation metadata
// onto Readwise's optional fields.
type Highlight struct {
	Text          string `json:"text"`
	Title         string `json:"title"`
	Author        string `json:"author,omitempty"`
	ImageURL      string `json:"image_url,omitempty"`
	SourceURL     string `json:"source_url,omitempty"`
	SourceType    string `json:"source_type,omitempty"` // e.g. "silo-ebooks"
	Category      string `json:"category,omitempty"`    // "books" / "articles" / ...
	Note          string `json:"note,omitempty"`
	Location      int    `json:"location,omitempty"`
	LocationType  string `json:"location_type,omitempty"` // "page" / "order" / "time_offset"
	HighlightedAt string `json:"highlighted_at,omitempty"`
}

type Client struct {
	hc    *http.Client
	token string
}

func New(token string) *Client {
	return &Client{
		hc:    &http.Client{Timeout: 30 * time.Second},
		token: strings.TrimSpace(token),
	}
}

// Push uploads `highlights` in 50-row batches. Returns the count of
// highlights Readwise accepted (across all batches) and the first
// error encountered. On partial failure we still return the count
// so the caller can show "exported N of M" rather than just an
// error.
func (c *Client) Push(ctx context.Context, highlights []Highlight) (int, error) {
	if c.token == "" {
		return 0, errors.New("readwise token not configured")
	}
	if len(highlights) == 0 {
		return 0, nil
	}
	pushed := 0
	for i := 0; i < len(highlights); i += batchSize {
		end := i + batchSize
		if end > len(highlights) {
			end = len(highlights)
		}
		batch := highlights[i:end]
		if err := c.pushBatch(ctx, batch); err != nil {
			return pushed, fmt.Errorf("batch %d-%d: %w", i, end, err)
		}
		pushed += len(batch)
	}
	return pushed, nil
}

func (c *Client) pushBatch(ctx context.Context, batch []Highlight) error {
	body, err := json.Marshal(map[string]any{"highlights": batch})
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		apiBase+"/highlights/", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Authorization", "Token "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("do: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		// Read up to 512 bytes of the error body for the
		// surfaced message — Readwise responds with JSON
		// {"detail": "..."} on most failures.
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("readwise %d: %s", resp.StatusCode,
			strings.TrimSpace(string(snippet)))
	}
	return nil
}

// AuthCheck pings GET /auth to verify the token is live. Returns
// nil on 204 (Readwise's success code for this endpoint).
func (c *Client) AuthCheck(ctx context.Context) error {
	if c.token == "" {
		return errors.New("readwise token not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/auth/", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Token "+c.token)
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("readwise auth: %d %s", resp.StatusCode,
			strings.TrimSpace(string(snippet)))
	}
	return nil
}
