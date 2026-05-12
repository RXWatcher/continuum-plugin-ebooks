package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type Annotation struct {
	ID           string
	UserID       string
	BookID       string
	CFIRange     string
	Kind         string // highlight | note
	Color        string
	SelectedText string
	NoteText     string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (s *Store) InsertAnnotation(ctx context.Context, a Annotation) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO annotation (id, user_id, book_id, cfi_range, kind, color, selected_text, note_text)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6,''), $7, $8)
	`, a.ID, a.UserID, a.BookID, a.CFIRange, a.Kind, a.Color, a.SelectedText, a.NoteText)
	if err != nil {
		return fmt.Errorf("insert annotation: %w", err)
	}
	return nil
}

func (s *Store) UpdateAnnotation(ctx context.Context, id, userID, color, noteText string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE annotation SET color = NULLIF($3,''), note_text = $4, updated_at = now()
		WHERE id = $1 AND user_id = $2
	`, id, userID, color, noteText)
	if err != nil {
		return fmt.Errorf("update annotation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteAnnotation(ctx context.Context, id, userID string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM annotation WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete annotation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListAnnotationsByBook(ctx context.Context, userID, bookID string) ([]Annotation, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, book_id, cfi_range, kind, COALESCE(color,''), selected_text, note_text, created_at, updated_at
		FROM annotation WHERE user_id = $1 AND book_id = $2 ORDER BY created_at
	`, userID, bookID)
	if err != nil {
		return nil, fmt.Errorf("list annotations: %w", err)
	}
	defer rows.Close()
	return scanAnnotations(rows)
}

func (s *Store) ListAnnotationsByUser(ctx context.Context, userID string, limit int) ([]Annotation, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, book_id, cfi_range, kind, COALESCE(color,''), selected_text, note_text, created_at, updated_at
		FROM annotation WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list annotations: %w", err)
	}
	defer rows.Close()
	return scanAnnotations(rows)
}

func scanAnnotations(rows pgx.Rows) ([]Annotation, error) {
	var out []Annotation
	for rows.Next() {
		var a Annotation
		if err := rows.Scan(&a.ID, &a.UserID, &a.BookID, &a.CFIRange, &a.Kind,
			&a.Color, &a.SelectedText, &a.NoteText, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, a)
	}
	return out, nil
}

// Test that ErrNotFound matches pgx.ErrNoRows at the helper layer.
var _ = errors.Is
