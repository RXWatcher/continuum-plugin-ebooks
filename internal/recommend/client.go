// Package recommend implements embedding-based "similar books"
// recommendations for the ebooks plugin. Ported from the audiobooks
// plugin (same Client + Engine shape) with text builder adapted for
// ebook metadata (format, tags, ISBN, no narrators).
package recommend

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

// ClientConfig + Client mirror the audiobooks plugin exactly — same
// provider routing, same retry behaviour, same env-var contract. We
// duplicate rather than import the audiobooks package because the
// plugins are independently buildable and shouldn't depend on each
// other at the Go module level.
type ClientConfig struct {
	BaseURL string
	Model   string
	APIKey  string
}

type Client struct {
	cfg ClientConfig
	hc  *http.Client
}

func NewClient(cfg ClientConfig) *Client {
	return &Client{cfg: cfg, hc: &http.Client{Timeout: 10 * time.Minute}}
}

func (c *Client) Configured() bool {
	if c == nil {
		return false
	}
	return strings.TrimSpace(c.cfg.BaseURL) != "" && strings.TrimSpace(c.cfg.Model) != ""
}

func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if !c.Configured() {
		return nil, errors.New("embedding client not configured")
	}
	if len(texts) == 0 {
		return nil, nil
	}
	if c.isGemini() {
		return c.embedGemini(ctx, texts)
	}
	return c.embedOpenAI(ctx, texts)
}

func (c *Client) isGemini() bool {
	return strings.Contains(c.cfg.BaseURL, "generativelanguage.googleapis.com")
}

func (c *Client) Model() string {
	if c == nil {
		return ""
	}
	return c.cfg.Model
}

type openAIRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type openAIResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

func (c *Client) embedOpenAI(ctx context.Context, texts []string) ([][]float32, error) {
	body, err := json.Marshal(openAIRequest{Model: c.cfg.Model, Input: texts})
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	endpoint := strings.TrimRight(c.cfg.BaseURL, "/") + "/v1/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("embedding endpoint %s: %d %s", endpoint, resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	var out openAIResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 50<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	vecs := make([][]float32, len(texts))
	for _, d := range out.Data {
		if d.Index < 0 || d.Index >= len(vecs) {
			continue
		}
		vecs[d.Index] = d.Embedding
	}
	return vecs, nil
}

type geminiBatchRequest struct {
	Requests []geminiSingle `json:"requests"`
}
type geminiSingle struct {
	Model   string        `json:"model"`
	Content geminiContent `json:"content"`
}
type geminiContent struct{ Parts []geminiPart `json:"parts"` }
type geminiPart struct{ Text string `json:"text"` }
type geminiResponse struct {
	Embeddings []struct{ Values []float32 `json:"values"` } `json:"embeddings"`
}

func (c *Client) embedGemini(ctx context.Context, texts []string) ([][]float32, error) {
	reqs := make([]geminiSingle, len(texts))
	for i, t := range texts {
		reqs[i] = geminiSingle{
			Model:   "models/" + c.cfg.Model,
			Content: geminiContent{Parts: []geminiPart{{Text: t}}},
		}
	}
	body, err := json.Marshal(geminiBatchRequest{Requests: reqs})
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	endpoint := strings.TrimRight(c.cfg.BaseURL, "/") +
		"/v1beta/models/" + c.cfg.Model + ":batchEmbedContents?key=" + c.cfg.APIKey
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("gemini %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	var out geminiResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 50<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	vecs := make([][]float32, len(out.Embeddings))
	for i, e := range out.Embeddings {
		vecs[i] = e.Values
	}
	return vecs, nil
}
