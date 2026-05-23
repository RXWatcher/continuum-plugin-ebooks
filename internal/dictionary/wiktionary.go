// Package dictionary wraps free dictionary providers used by the
// reader's "Define" selection action. Wiktionary's REST API at
// en.wiktionary.org returns structured definitions per part of
// speech; we flatten to a simple list of {part_of_speech,
// definition, example} so the reader's popover doesn't need to
// know the provider's JSON shape.
package dictionary

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Definition is one entry in a word lookup result. PartOfSpeech is
// "noun" / "verb" / "adjective" etc. Definition is plain text
// (HTML stripped); Example is an optional usage line.
type Definition struct {
	PartOfSpeech string `json:"part_of_speech"`
	Definition   string `json:"definition"`
	Example      string `json:"example,omitempty"`
}

// LookupResult is the full word lookup response.
type LookupResult struct {
	Word     string       `json:"word"`
	Language string       `json:"language"`
	Entries  []Definition `json:"entries"`
}

var client = &http.Client{Timeout: 10 * time.Second}

// Lookup queries en.wiktionary.org for a single word's definitions.
// Returns empty entries when the word isn't in Wiktionary. Errors
// are returned only for transport failures.
//
// lang is the source language code (Wiktionary key — "en" for
// English, "de" for German, etc.); default "en".
func Lookup(ctx context.Context, word, lang string) (LookupResult, error) {
	word = strings.TrimSpace(word)
	if word == "" {
		return LookupResult{}, fmt.Errorf("word required")
	}
	if lang == "" {
		lang = "en"
	}
	// Wiktionary's REST returns one entry per language section
	// keyed on the language code.
	endpoint := "https://" + url.PathEscape(lang) + ".wiktionary.org/api/rest_v1/page/definition/" + url.PathEscape(word)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return LookupResult{}, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("User-Agent", "silo-ebooks/dictionary (+https://siloapp.com)")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return LookupResult{}, fmt.Errorf("wiktionary: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		// Not an error — word just isn't in Wiktionary.
		return LookupResult{Word: word, Language: lang}, nil
	}
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return LookupResult{}, fmt.Errorf("wiktionary %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	// Response shape: {"<lang-name>": [{"partOfSpeech": "Noun",
	// "definitions": [{"definition": "<html>", "examples": ["..."]}]}]}
	var raw map[string][]struct {
		PartOfSpeech string `json:"partOfSpeech"`
		Language     string `json:"language"`
		Definitions  []struct {
			Definition string   `json:"definition"`
			Examples   []string `json:"examples"`
		} `json:"definitions"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&raw); err != nil {
		return LookupResult{}, fmt.Errorf("decode: %w", err)
	}
	out := LookupResult{Word: word, Language: lang}
	for _, sections := range raw {
		for _, section := range sections {
			for _, def := range section.Definitions {
				def.Definition = stripHTML(def.Definition)
				if def.Definition == "" {
					continue
				}
				entry := Definition{
					PartOfSpeech: section.PartOfSpeech,
					Definition:   def.Definition,
				}
				if len(def.Examples) > 0 {
					entry.Example = stripHTML(def.Examples[0])
				}
				out.Entries = append(out.Entries, entry)
				if len(out.Entries) >= 10 {
					return out, nil
				}
			}
		}
	}
	return out, nil
}

// stripHTML is a minimal HTML→text passthrough — Wiktionary
// returns short HTML fragments with link/em/strong tags. The
// reader's popover renders plain text, so we drop everything
// between angle brackets. Not a security-sensitive sanitiser
// (the source is wiktionary.org); a real HTML parser would be
// overkill for these one-sentence definitions.
func stripHTML(s string) string {
	var out strings.Builder
	depth := 0
	for _, r := range s {
		switch r {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				out.WriteRune(r)
			}
		}
	}
	return strings.TrimSpace(out.String())
}
