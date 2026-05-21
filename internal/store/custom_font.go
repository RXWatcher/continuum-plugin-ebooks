package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// CustomFont mirrors the custom_font row. Data is the raw font
// bytes; reading List* returns rows WITHOUT data so a listing
// query doesn't pull megabytes per row.
type CustomFont struct {
	ID        string
	UserID    string
	Name      string
	MIME      string
	SizeBytes int
	Data      []byte
	CreatedAt time.Time
}

// InsertCustomFont stores one uploaded font. Caller validated the
// MIME + size before calling.
func (s *Store) InsertCustomFont(ctx context.Context, f CustomFont) error {
	if f.ID == "" || f.UserID == "" || f.Name == "" || len(f.Data) == 0 {
		return errors.New("id, user_id, name, data required")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO custom_font (id, user_id, name, mime, size_bytes, data)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, f.ID, f.UserID, f.Name, f.MIME, f.SizeBytes, f.Data)
	if err != nil {
		return fmt.Errorf("insert custom_font: %w", err)
	}
	return nil
}

// ListCustomFonts returns metadata-only rows (no data). The reader's
// font picker calls this; the actual bytes load from
// /me/fonts/{id}/data when the user selects one.
func (s *Store) ListCustomFonts(ctx context.Context, userID string) ([]CustomFont, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, name, mime, size_bytes, created_at
		FROM custom_font WHERE user_id = $1
		ORDER BY LOWER(name)
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list custom_font: %w", err)
	}
	defer rows.Close()
	var out []CustomFont
	for rows.Next() {
		var f CustomFont
		if err := rows.Scan(&f.ID, &f.UserID, &f.Name, &f.MIME, &f.SizeBytes, &f.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan custom_font: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// GetCustomFontData returns the raw font bytes for the data route.
// userID gate so users can't fetch each other's fonts.
func (s *Store) GetCustomFontData(ctx context.Context, id, userID string) (CustomFont, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, user_id, name, mime, size_bytes, data, created_at
		FROM custom_font WHERE id = $1 AND user_id = $2
	`, id, userID)
	var f CustomFont
	if err := row.Scan(&f.ID, &f.UserID, &f.Name, &f.MIME, &f.SizeBytes, &f.Data, &f.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CustomFont{}, ErrNotFound
		}
		return CustomFont{}, fmt.Errorf("get custom_font: %w", err)
	}
	return f, nil
}

func (s *Store) DeleteCustomFont(ctx context.Context, id, userID string) error {
	if id == "" || userID == "" {
		return errors.New("id, user_id required")
	}
	_, err := s.pool.Exec(ctx, `
		DELETE FROM custom_font WHERE id = $1 AND user_id = $2
	`, id, userID)
	if err != nil {
		return fmt.Errorf("delete custom_font: %w", err)
	}
	return nil
}
