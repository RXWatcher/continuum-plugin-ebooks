package store

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// YearStats is the ebook plugin's year-in-review aggregate. Books-
// focused rather than time-focused (no per-second telemetry on the
// ebook side); we surface count of finished books + distinct active
// days + top books by reading activity.
type YearStats struct {
	Year          int           `json:"year"`
	BooksFinished int           `json:"books_finished"`
	BooksStarted  int           `json:"books_started"`
	DistinctDays  int           `json:"distinct_days"`
	TopBooks      []YearTopBook `json:"top_books"`
}

// YearTopBook is one entry in the "books I spent the most time
// with this year" list. ProgressPct is the final reading_progress
// at year-end; useful for sorting + display.
type YearTopBook struct {
	BookID      string    `json:"book_id"`
	IsFinished  bool      `json:"is_finished"`
	ProgressPct float64   `json:"progress_pct"`
	LastReadAt  time.Time `json:"last_read_at"`
}

// YearStatsForUser computes the year-in-review for one user. Year
// boundaries use the supplied location (UTC by default). Top books
// are picked by last_read_at within the year, capped at 10.
func (s *Store) YearStatsForUser(ctx context.Context, userID string, year int, loc *time.Location) (YearStats, error) {
	if userID == "" {
		return YearStats{}, errors.New("user_id required")
	}
	if loc == nil {
		loc = time.UTC
	}
	yearStart := time.Date(year, time.January, 1, 0, 0, 0, 0, loc)
	yearEnd := yearStart.AddDate(1, 0, 0)
	out := YearStats{Year: year}

	if err := s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE is_finished = TRUE)::int,
			COUNT(*) FILTER (WHERE read_progress > 0)::int,
			COUNT(DISTINCT date_trunc('day', last_read_at AT TIME ZONE $4))::int
		FROM user_data
		WHERE user_id = $1
		  AND last_read_at IS NOT NULL
		  AND last_read_at >= $2 AND last_read_at < $3
	`, userID, yearStart, yearEnd, loc.String()).Scan(
		&out.BooksFinished, &out.BooksStarted, &out.DistinctDays,
	); err != nil {
		return YearStats{}, fmt.Errorf("year stats aggregate: %w", err)
	}

	// Top books — limit 10, most-recently-read first within the
	// year. Some users prefer "most pages read" ordering; we can
	// add that variant once page_count is reliably populated.
	rows, err := s.pool.Query(ctx, `
		SELECT book_id, is_finished, read_progress, last_read_at
		FROM user_data
		WHERE user_id = $1
		  AND last_read_at IS NOT NULL
		  AND last_read_at >= $2 AND last_read_at < $3
		ORDER BY last_read_at DESC
		LIMIT 10
	`, userID, yearStart, yearEnd)
	if err != nil {
		return out, fmt.Errorf("top books: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var b YearTopBook
		var ts *time.Time
		if err := rows.Scan(&b.BookID, &b.IsFinished, &b.ProgressPct, &ts); err != nil {
			return out, fmt.Errorf("scan top book: %w", err)
		}
		if ts != nil {
			b.LastReadAt = *ts
		}
		out.TopBooks = append(out.TopBooks, b)
	}
	return out, rows.Err()
}
