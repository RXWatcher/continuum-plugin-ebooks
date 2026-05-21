package recommend

import (
	"fmt"
	"strings"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/backend"
)

const maxOverviewRunes = 1000

// BuildEmbeddingText constructs the canonical embedding-input string
// for one ebook. Mirrors the audiobooks plugin's text builder but
// drops audio-specific fields (narrators) and adds book-specific
// fields (format, tags, ISBN). Lead with semantic content (genres +
// description) so the embedding model doesn't anchor on title words.
func BuildEmbeddingText(d backend.EbookDetail) string {
	var parts []string

	genres := strings.Join(d.Genres, ", ")
	overview := truncateRunes(d.Description, maxOverviewRunes)
	switch {
	case genres != "" && overview != "":
		parts = append(parts, fmt.Sprintf("%s book about %s", genres, overview))
	case genres != "":
		parts = append(parts, genres+" book")
	case overview != "":
		parts = append(parts, "Book. "+overview)
	default:
		parts = append(parts, "Book")
	}

	if d.Year > 0 {
		parts = append(parts, fmt.Sprintf("%s (%d)", d.Title, d.Year))
	} else if d.Title != "" {
		parts = append(parts, d.Title)
	}

	if len(d.Authors) > 0 {
		parts = append(parts, "By "+strings.Join(d.Authors, ", "))
	}

	if d.Series != "" {
		name := d.Series
		if d.SeriesIndex > 0 {
			name = fmt.Sprintf("%s #%g", name, d.SeriesIndex)
		}
		parts = append(parts, "Part of "+name)
	}

	if d.Publisher != "" {
		parts = append(parts, "Published by "+d.Publisher)
	}

	if len(d.Tags) > 0 {
		parts = append(parts, "Tags: "+strings.Join(d.Tags, ", "))
	}

	if len(d.Formats) > 0 {
		parts = append(parts, "Format: "+strings.Join(d.Formats, ", "))
	}

	if d.Language != "" {
		parts = append(parts, "Language: "+d.Language)
	}

	if d.ISBN != "" {
		parts = append(parts, "ISBN: "+d.ISBN)
	}

	return strings.ToValidUTF8(strings.Join(parts, ". "), "")
}

func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	count := 0
	for i := range s {
		if count == n {
			return s[:i]
		}
		count++
	}
	return s
}
