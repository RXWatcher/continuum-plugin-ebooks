package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type UserData struct {
	UserID       string
	BookID       string
	LastCFI      string
	CurrentPage  int
	ReadProgress float64
	IsFinished   bool
	IsFavorite   bool
	Rating       *int
	Notes        string
	LastReadAt   *time.Time
	UpdatedAt    time.Time
}

func (s *Store) UpsertUserData(ctx context.Context, d UserData) error {
	if d.UserID == "" || d.BookID == "" {
		return fmt.Errorf("user_id and book_id required")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO user_data (user_id, book_id, last_cfi, current_page, read_progress,
			is_finished, is_favorite, rating, notes, last_read_at, updated_at)
		VALUES ($1, $2, NULLIF($3,''), $4, $5, $6, $7, $8, $9, $10, now())
		ON CONFLICT (user_id, book_id) DO UPDATE SET
			last_cfi      = COALESCE(EXCLUDED.last_cfi, user_data.last_cfi),
			current_page  = EXCLUDED.current_page,
			read_progress = EXCLUDED.read_progress,
			is_finished   = EXCLUDED.is_finished,
			is_favorite   = EXCLUDED.is_favorite,
			rating        = COALESCE(EXCLUDED.rating, user_data.rating),
			notes         = EXCLUDED.notes,
			last_read_at  = COALESCE(EXCLUDED.last_read_at, user_data.last_read_at),
			updated_at    = now()
	`, d.UserID, d.BookID, d.LastCFI, d.CurrentPage, d.ReadProgress,
		d.IsFinished, d.IsFavorite, d.Rating, d.Notes, d.LastReadAt)
	if err != nil {
		return fmt.Errorf("upsert user_data: %w", err)
	}
	return nil
}

func (s *Store) GetUserData(ctx context.Context, userID, bookID string) (UserData, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT user_id, book_id, COALESCE(last_cfi,''), COALESCE(current_page,0),
		       read_progress, is_finished, is_favorite, rating,
		       notes, last_read_at, updated_at
		FROM user_data WHERE user_id = $1 AND book_id = $2
	`, userID, bookID)
	var d UserData
	if err := row.Scan(&d.UserID, &d.BookID, &d.LastCFI, &d.CurrentPage,
		&d.ReadProgress, &d.IsFinished, &d.IsFavorite, &d.Rating,
		&d.Notes, &d.LastReadAt, &d.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UserData{}, ErrNotFound
		}
		return UserData{}, fmt.Errorf("get user_data: %w", err)
	}
	return d, nil
}

// ListRecentByUser returns the user's most recently read books.
func (s *Store) ListRecentByUser(ctx context.Context, userID string, limit int) ([]UserData, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx, `
		SELECT user_id, book_id, COALESCE(last_cfi,''), COALESCE(current_page,0),
		       read_progress, is_finished, is_favorite, rating,
		       notes, last_read_at, updated_at
		FROM user_data WHERE user_id = $1 AND last_read_at IS NOT NULL
		ORDER BY last_read_at DESC LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent: %w", err)
	}
	defer rows.Close()
	return scanUserData(rows)
}

// ListByUser returns user_data rows filtered by status (reading|finished|favorite|"").
func (s *Store) ListByUser(ctx context.Context, userID, status string, limit int) ([]UserData, error) {
	if limit <= 0 {
		limit = 100
	}
	var where string
	switch status {
	case "reading":
		where = "AND NOT is_finished AND read_progress > 0"
	case "finished":
		where = "AND is_finished"
	case "favorite":
		where = "AND is_favorite"
	}
	query := fmt.Sprintf(`
		SELECT user_id, book_id, COALESCE(last_cfi,''), COALESCE(current_page,0),
		       read_progress, is_finished, is_favorite, rating,
		       notes, last_read_at, updated_at
		FROM user_data WHERE user_id = $1 %s
		ORDER BY last_read_at DESC NULLS LAST LIMIT $2
	`, where)
	rows, err := s.pool.Query(ctx, query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list by user: %w", err)
	}
	defer rows.Close()
	return scanUserData(rows)
}

func scanUserData(rows pgx.Rows) ([]UserData, error) {
	var out []UserData
	for rows.Next() {
		var d UserData
		if err := rows.Scan(&d.UserID, &d.BookID, &d.LastCFI, &d.CurrentPage,
			&d.ReadProgress, &d.IsFinished, &d.IsFavorite, &d.Rating,
			&d.Notes, &d.LastReadAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, d)
	}
	return out, nil
}
