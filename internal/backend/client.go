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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginsdk/runtimehost"
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
	runtimeHost *runtimehost.Client
}

// NewHostHTTPClient constructs the proxy client. baseURL is the host's HTTP
// API root; token is a service token granted by the host broker.
func NewHostHTTPClient(baseURL, token string) *HostHTTPClient {
	return &HostHTTPClient{
		hostBaseURL: strings.TrimRight(baseURL, "/"),
		token:       token,
		hc: &http.Client{
			Timeout: defaultTimeout,
			// The host plugin-proxy returns the response directly; we never
			// expect to *follow* a redirect. A backend plugin can influence
			// the proxied response, so following a 3xx would let it point us
			// (with the host bearer attached) at an arbitrary/internal host.
			// Surface the 3xx as the response instead (callers that treat 302
			// as success still see it).
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// validInstallID reports whether s is a safe path segment for the
// /api/v1/plugins/<id><path> host-proxy URL. A backend target is either a
// numeric install id ("7") or a plugin-id slug
// ("continuum.bookwarehouse-ebook"), both of which are
// [A-Za-z0-9._-]. Rejecting anything else stops a DB/config-sourced value
// like "1/../../admin" or "a%2f.." from collapsing/escaping the proxy path
// (SSRF / host-API traversal).
func validInstallID(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == '-':
		default:
			return false
		}
	}
	return true
}

// truncForError caps an upstream body inlined into an error string (it
// propagates into logs); the raw body can be up to maxResponseBytes.
func truncForError(b []byte) string {
	const limit = 512
	if len(b) <= limit {
		return string(b)
	}
	return string(b[:limit]) + "…(truncated)"
}

func (c *HostHTTPClient) WithRuntimeHost(host *runtimehost.Client) *HostHTTPClient {
	c.runtimeHost = host
	return c
}

func (c *HostHTTPClient) HostBaseURL() string { return c.hostBaseURL }

// Get calls GET on the plugin-proxy path for installID.
func (c *HostHTTPClient) Get(ctx context.Context, installID, pluginPath string) ([]byte, int, error) {
	return c.do(ctx, http.MethodGet, installID, pluginPath, nil, nil)
}

// GetJSON issues a GET against the plugin-proxy path and decodes JSON into
// out. When RuntimeHost is connected this uses the SDK JSON helper; otherwise
// it falls back to the host HTTP proxy.
func (c *HostHTTPClient) GetJSON(ctx context.Context, installID, pluginPath string, out any) (int, error) {
	if c.runtimeHost != nil {
		if id, err := strconv.Atoi(installID); err == nil && id > 0 {
			path, query := splitPluginPath(pluginPath)
			headers := map[string]string{}
			if c.token != "" {
				headers["Authorization"] = "Bearer " + c.token
			}
			err := c.runtimeHost.CallPluginJSON(ctx, runtimehost.CallPluginJSONRequest{
				InstallationID:   id,
				Path:             path,
				Headers:          headers,
				Query:            query,
				Response:         out,
				MaxResponseBytes: maxResponseBytes,
			})
			var statusErr *runtimehost.HTTPStatusError
			if errors.As(err, &statusErr) {
				return statusErr.StatusCode, fmt.Errorf("upstream %d: %s", statusErr.StatusCode, truncForError(statusErr.Body))
			}
			if err != nil {
				return 0, err
			}
			return http.StatusOK, nil
		}
	}
	body, code, err := c.Get(ctx, installID, pluginPath)
	if err != nil {
		return 0, err
	}
	if code < 200 || code >= 300 {
		return code, fmt.Errorf("upstream %d: %s", code, truncForError(body))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return code, fmt.Errorf("decode: %w", err)
	}
	return code, nil
}

// GetStream returns the response body without buffering (for file streaming).
// Caller MUST close the returned io.ReadCloser. Returns the resp itself so
// the caller can forward headers (Content-Type, Content-Length, etc).
func (c *HostHTTPClient) GetStream(ctx context.Context, installID, pluginPath string, headers map[string]string) (*http.Response, error) {
	if !validInstallID(installID) {
		return nil, fmt.Errorf("invalid install id")
	}
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
	if c.runtimeHost != nil {
		if id, err := strconv.Atoi(installID); err == nil && id > 0 {
			path, query := splitPluginPath(pluginPath)
			resp, err := c.runtimeHost.CallPluginHTTP(ctx, runtimehost.CallPluginHTTPRequest{
				InstallationID: id,
				Method:         method,
				Path:           path,
				Headers:        headers,
				Body:           body,
				Query:          query,
			})
			if err != nil {
				return nil, 0, err
			}
			if len(resp.Body) > maxResponseBytes {
				return nil, resp.StatusCode, fmt.Errorf("response exceeds %d bytes", maxResponseBytes)
			}
			return resp.Body, resp.StatusCode, nil
		}
	}
	if !validInstallID(installID) {
		return nil, 0, fmt.Errorf("invalid install id")
	}
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

func splitPluginPath(pathAndQuery string) (string, map[string]any) {
	u, err := url.Parse(pathAndQuery)
	if err != nil || u.RawQuery == "" {
		return pathAndQuery, nil
	}
	query := make(map[string]any)
	for key, values := range u.Query() {
		if len(values) == 1 {
			query[key] = values[0]
			continue
		}
		items := make([]any, 0, len(values))
		for _, value := range values {
			items = append(items, value)
		}
		query[key] = items
	}
	return u.Path, query
}
