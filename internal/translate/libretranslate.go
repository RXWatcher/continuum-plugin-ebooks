// Package translate wraps LibreTranslate-compatible HTTP APIs for
// the reader's "Translate" selection action. LibreTranslate is the
// only major translation service with a free + self-hostable
// option; the API surface is also implemented by Argos Translate,
// libretranslate.de, and several other forks.
//
// Operator configures the upstream via LIBRETRANSLATE_URL +
// LIBRETRANSLATE_API_KEY env vars. Unconfigured deployments
// return a clear "translation not configured" error rather than
// silently degrading.
package translate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Config captures the endpoint + optional API key. LoadFromEnv
// populates from LIBRETRANSLATE_URL / LIBRETRANSLATE_API_KEY.
type Config struct {
	BaseURL string
	APIKey  string
}

func LoadFromEnv() Config {
	return Config{
		BaseURL: strings.TrimSpace(os.Getenv("LIBRETRANSLATE_URL")),
		APIKey:  strings.TrimSpace(os.Getenv("LIBRETRANSLATE_API_KEY")),
	}
}

func (c Config) Configured() bool {
	return c.BaseURL != ""
}

// Result is the translate response: detected source language + the
// translated text. SourceLang is empty when the caller passed an
// explicit `source`.
type Result struct {
	TranslatedText string `json:"translated_text"`
	SourceLang     string `json:"source_lang,omitempty"`
}

var client = &http.Client{Timeout: 20 * time.Second}

// Translate sends `text` to the LibreTranslate /translate endpoint
// asking for translation into `target`. Pass source="auto" (the
// LibreTranslate default) to have the upstream detect the source
// language.
func Translate(ctx context.Context, cfg Config, text, source, target string) (Result, error) {
	if !cfg.Configured() {
		return Result{}, errors.New("translation not configured (set LIBRETRANSLATE_URL)")
	}
	if text == "" {
		return Result{}, errors.New("text required")
	}
	if target == "" {
		return Result{}, errors.New("target language required")
	}
	if source == "" {
		source = "auto"
	}
	body := map[string]any{
		"q":      text,
		"source": source,
		"target": target,
		"format": "text",
	}
	if cfg.APIKey != "" {
		body["api_key"] = cfg.APIKey
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return Result{}, fmt.Errorf("marshal: %w", err)
	}
	endpoint := strings.TrimRight(cfg.BaseURL, "/") + "/translate"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyJSON))
	if err != nil {
		return Result{}, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("libretranslate: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return Result{}, fmt.Errorf("libretranslate %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	var raw struct {
		TranslatedText  string `json:"translatedText"`
		DetectedSource  struct {
			Language   string  `json:"language"`
			Confidence float64 `json:"confidence"`
		} `json:"detectedLanguage"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&raw); err != nil {
		return Result{}, fmt.Errorf("decode: %w", err)
	}
	out := Result{TranslatedText: raw.TranslatedText}
	if source == "auto" {
		out.SourceLang = raw.DetectedSource.Language
	}
	return out, nil
}
