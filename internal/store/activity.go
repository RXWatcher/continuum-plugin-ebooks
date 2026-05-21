package store

import (
	"context"
	"errors"
	"time"
)

// ActivityEvent is one entry in a per-book reading timeline. Same
// shape as the audiobooks plugin's ActivityEvent so a shared SPA
// component can render either plugin's timeline.
type ActivityEvent struct {
	At      time.Time      `json:"at"`
	Kind    string         `json:"kind"`
	Payload map[string]any `json:"payload,omitempty"`
}

// BookActivity merges events for one (user, book) pair across the
// user_data + annotation + share_link tables. Reverse-chronological.
//
// Kinds emitted:
//   progress     — last_read_at touch
//   started      — first time read_progress went above zero (today's
//                  data doesn't preserve that; we emit only when
//                  the row is currently in-progress, not historic)
//   finished     — is_finished flipped true (timestamp = last_read_at)
//   annotation   — highlight / underline / squiggly / note
//   shared       — share_link minted for this book
func (s *Store) BookActivity(ctx context.Context, userID, bookID string) ([]ActivityEvent, error) {
	if userID == "" || bookID == "" {
		return nil, errors.New("user_id, book_id required")
	}
	out := make([]ActivityEvent, 0, 32)

	// user_data → progress + finished + rating events.
	ud, err := s.GetUserData(ctx, userID, bookID)
	if err == nil {
		if ud.LastReadAt != nil {
			out = append(out, ActivityEvent{
				At:   *ud.LastReadAt,
				Kind: "progress",
				Payload: map[string]any{
					"current_page":  ud.CurrentPage,
					"read_progress": ud.ReadProgress,
				},
			})
		}
		if ud.IsFinished && ud.LastReadAt != nil {
			out = append(out, ActivityEvent{
				At:   *ud.LastReadAt,
				Kind: "finished",
				Payload: map[string]any{
					"read_progress": ud.ReadProgress,
				},
			})
		}
		if ud.Rating != nil {
			out = append(out, ActivityEvent{
				At:   ud.UpdatedAt,
				Kind: "rated",
				Payload: map[string]any{
					"rating": *ud.Rating,
				},
			})
		}
	}

	// Annotations — one event per highlight / underline / note.
	if anns, err := s.ListAnnotationsByBook(ctx, userID, bookID); err == nil {
		for _, a := range anns {
			if a.DeletedAt != nil {
				continue
			}
			out = append(out, ActivityEvent{
				At:   a.CreatedAt,
				Kind: "annotation",
				Payload: map[string]any{
					"id":            a.ID,
					"style":         a.Style,
					"color":         a.Color,
					"selected_text": a.SelectedText,
					"note_text":     a.NoteText,
					"page":          a.Page,
				},
			})
		}
	}

	// Share links — one event per outstanding link.
	rows, err := s.pool.Query(ctx, `
		SELECT id, slug, created_at FROM share_link
		WHERE user_id = $1 AND item_id = $2
	`, userID, bookID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id, slug string
			var createdAt time.Time
			if err := rows.Scan(&id, &slug, &createdAt); err != nil {
				continue
			}
			out = append(out, ActivityEvent{
				At:   createdAt,
				Kind: "shared",
				Payload: map[string]any{
					"id":   id,
					"slug": slug,
				},
			})
		}
	}

	sortActivityDesc(out)
	return out, nil
}

func sortActivityDesc(events []ActivityEvent) {
	for i := 1; i < len(events); i++ {
		cur := events[i]
		j := i - 1
		for j >= 0 && events[j].At.Before(cur.At) {
			events[j+1] = events[j]
			j--
		}
		events[j+1] = cur
	}
}
