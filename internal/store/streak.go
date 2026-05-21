package store

import (
	"context"
	"fmt"
	"time"
)

// Streak is the user's reading-streak summary. Mirrors the audiobooks
// plugin's Streak shape so a SPA shared component can render either
// without branching on plugin.
type Streak struct {
	Current        int       `json:"current"`
	Longest        int       `json:"longest"`
	LastActiveDate time.Time `json:"last_active_date"`
}

// StreakForUser computes the user's reading streak from
// user_data.last_read_at distinct days. The current streak counts
// backwards from today (or yesterday — the same 1-day grace as the
// audiobooks plugin so users don't lose a streak by going to bed
// early); longest is the max consecutive run ever recorded across
// the user's history.
func (s *Store) StreakForUser(ctx context.Context, userID string, loc *time.Location) (Streak, error) {
	if userID == "" {
		return Streak{}, fmt.Errorf("user_id required")
	}
	if loc == nil {
		loc = time.UTC
	}
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT date_trunc('day', last_read_at AT TIME ZONE $2)::date AS day
		FROM user_data
		WHERE user_id = $1 AND last_read_at IS NOT NULL
		ORDER BY day DESC
		LIMIT 365
	`, userID, loc.String())
	if err != nil {
		return Streak{}, fmt.Errorf("streak query: %w", err)
	}
	defer rows.Close()
	var days []time.Time
	for rows.Next() {
		var d time.Time
		if err := rows.Scan(&d); err != nil {
			return Streak{}, fmt.Errorf("scan: %w", err)
		}
		days = append(days, d)
	}
	if err := rows.Err(); err != nil {
		return Streak{}, fmt.Errorf("streak rows: %w", err)
	}
	if len(days) == 0 {
		return Streak{}, nil
	}

	today := time.Now().In(loc).Format("2006-01-02")
	yesterday := time.Now().In(loc).AddDate(0, 0, -1).Format("2006-01-02")
	mostRecent := days[0].Format("2006-01-02")

	current := 0
	if mostRecent == today || mostRecent == yesterday {
		current = 1
		for i := 1; i < len(days); i++ {
			diff := days[i-1].Sub(days[i]).Hours() / 24
			if diff > 1.5 {
				break
			}
			current++
		}
	}

	longest := 0
	run := 0
	for i := range days {
		if i == 0 {
			run = 1
			longest = 1
			continue
		}
		diff := days[i-1].Sub(days[i]).Hours() / 24
		if diff <= 1.5 {
			run++
		} else {
			run = 1
		}
		if run > longest {
			longest = run
		}
	}

	return Streak{
		Current:        current,
		Longest:        longest,
		LastActiveDate: days[0],
	}, nil
}
