// Package backend is the portal's HTTP client for calling ebook backend
// providers via the continuum host plugin proxy.
//
// The host exposes a proxy at:
//
//	GET /api/v1/plugins/<install_id>/api/v1/...
//
// Portal calls always go through that proxy (never directly to the backend
// plugin process). The proxy verifies authentication and forwards the call to
// the target plugin's HttpRoutes.v1 handler.
package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultTimeout = 60 * time.Second

// maxResponseBytes caps response bodies read from upstream backend plugins
// via the host's plugin proxy. Catalog/browse JSON payloads are well under
// this; cover/file fetches use streaming endpoints (separate code path)
// and aren't subject to this cap. The cap defends against memory
// exhaustion if a backend returns a runaway body.
const maxResponseBytes = 10 << 20 // 10 MiB

// HostHTTPClient is a thin HTTP client that knows how to address the
// continuum host plugin proxy.
type HostHTTPClient struct {
	hostBaseURL string // e.g. http://localhost:8090 (set from env or sensible default)
	token       string // service token forwarded as bearer
	hc          *http.Client
}

// NewHostHTTPClient constructs the proxy client. baseURL is the host's HTTP
// API root; token is a service token granted by the host broker.
func NewHostHTTPClient(baseURL, token string) *HostHTTPClient {
	return &HostHTTPClient{
		hostBaseURL: strings.TrimRight(baseURL, "/"),
		token:       token,
		hc:          &http.Client{Timeout: defaultTimeout},
	}
}

func (c *HostHTTPClient) HostBaseURL() string { return c.hostBaseURL }

// Get calls GET on the plugin-proxy path for installID.
func (c *HostHTTPClient) Get(ctx context.Context, installID, pluginPath string) ([]byte, int, error) {
	return c.do(ctx, http.MethodGet, installID, pluginPath, nil, nil)
}

// GetStream returns the response body without buffering (for file streaming).
// Caller MUST close the returned io.ReadCloser. Returns the resp itself so
// the caller can forward headers (Content-Type, Content-Length, etc).
func (c *HostHTTPClient) GetStream(ctx context.Context, installID, pluginPath string, headers map[string]string) (*http.Response, error) {
	url := fmt.Sprintf("%s/api/v1/plugins/%s%s", c.hostBaseURL, installID, pluginPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return c.hc.Do(req)
}

// PostJSON sends a JSON body to the plugin-proxy path.
func (c *HostHTTPClient) PostJSON(ctx context.Context, installID, pluginPath string, body any) ([]byte, int, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, 0, fmt.Errorf("encode: %w", err)
	}
	return c.do(ctx, http.MethodPost, installID, pluginPath, raw, map[string]string{"Content-Type": "application/json"})
}

func (c *HostHTTPClient) do(ctx context.Context, method, installID, pluginPath string, body []byte, headers map[string]string) ([]byte, int, error) {
	url := fmt.Sprintf("%s/api/v1/plugins/%s%s", c.hostBaseURL, installID, pluginPath)
	var bodyReader *strings.Reader
	if body != nil {
		bodyReader = strings.NewReader(string(body))
	}
	var req *http.Request
	var err error
	if bodyReader != nil {
		req, err = http.NewRequestWithContext(ctx, method, url, bodyReader)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, url, nil)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("new request: %w", err)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("do: %w", err)
	}
	defer resp.Body.Close()
	rb, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, 0, fmt.Errorf("read body: %w", err)
	}
	return rb, resp.StatusCode, nil
}
