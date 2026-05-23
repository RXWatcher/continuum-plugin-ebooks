package recommend

import (
	"strings"
	"testing"

	"github.com/RXWatcher/silo-plugin-ebooks/internal/backend"
)

// TestBuildEmbeddingText_LeadsWithSemanticContent pins the same
// design contract as the audiobooks version: genres + description
// come first so the embedding doesn't anchor on title words.
func TestBuildEmbeddingText_LeadsWithSemanticContent(t *testing.T) {
	d := backend.EbookDetail{
		EbookSummary: backend.EbookSummary{
			Title:    "Way of Kings",
			Year:     2010,
			Authors:  []string{"Brandon Sanderson"},
			Series:   "Stormlight Archive",
			SeriesIndex: 1,
			Language: "en",
			Formats:  []string{"epub", "pdf"},
			Rating:   4.7,
		},
		Description: "Roshar is a world of stone and storms, populated by humans and other peoples.",
		Genres:      []string{"Fantasy", "Epic"},
		Tags:        []string{"magic", "war", "betrayal"},
		Publisher:   "Tor Books",
		ISBN:        "9780765326355",
	}
	out := BuildEmbeddingText(d)
	if !strings.HasPrefix(out, "Fantasy, Epic book about Roshar") {
		t.Errorf("lead does not start with genres+description: %q", out)
	}
	if !strings.Contains(out, "Way of Kings (2010)") {
		t.Errorf("title+year missing: %q", out)
	}
	if !strings.Contains(out, "By Brandon Sanderson") {
		t.Errorf("author missing: %q", out)
	}
	if !strings.Contains(out, "Stormlight Archive #1") {
		t.Errorf("series with sequence missing: %q", out)
	}
	if !strings.Contains(out, "Format: epub, pdf") {
		t.Errorf("format missing: %q", out)
	}
	if !strings.Contains(out, "Language: en") {
		t.Errorf("language missing: %q", out)
	}
	if !strings.Contains(out, "Tags: magic, war, betrayal") {
		t.Errorf("tags missing: %q", out)
	}
	if !strings.Contains(out, "ISBN: 9780765326355") {
		t.Errorf("ISBN missing: %q", out)
	}
}

// TestBuildEmbeddingText_DegradesGracefully — when most metadata is
// missing, the text still produces a deterministic output.
func TestBuildEmbeddingText_DegradesGracefully(t *testing.T) {
	d := backend.EbookDetail{
		EbookSummary: backend.EbookSummary{Title: "Unknown Book"},
	}
	out := BuildEmbeddingText(d)
	if !strings.Contains(out, "Unknown Book") {
		t.Errorf("title should appear: %q", out)
	}
	if !strings.HasPrefix(out, "Book") {
		t.Errorf("lead should fall back to 'Book': %q", out)
	}
}

// TestBuildEmbeddingText_DeterministicAcrossCalls — calling twice
// with the same input produces the same string. The canonical_text
// lock-in optimization depends on this.
func TestBuildEmbeddingText_DeterministicAcrossCalls(t *testing.T) {
	d := backend.EbookDetail{
		EbookSummary: backend.EbookSummary{
			Title: "X", Year: 2020, Authors: []string{"A", "B"},
		},
		Description: "A short tale.",
		Genres:      []string{"Mystery"},
	}
	a := BuildEmbeddingText(d)
	b := BuildEmbeddingText(d)
	if a != b {
		t.Errorf("non-deterministic output:\nA: %q\nB: %q", a, b)
	}
}
