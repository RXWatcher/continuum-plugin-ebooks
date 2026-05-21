package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type Annotation struct {
	ID           string          `json:"id"`
	UserID       string          `json:"user_id"`
	BookID       string          `json:"book_id"`
	CFIRange     string          `json:"cfi_range"`
	Kind         string          `json:"kind"` // highlight | note | bookmark | excerpt | annotation
	Color        string          `json:"color,omitempty"`
	SelectedText string          `json:"selected_text"`
	NoteText     string          `json:"note_text"`
	ReadestType  string          `json:"readest_type"`
	XPointer0    string          `json:"xpointer0"`
	XPointer1    string          `json:"xpointer1"`
	Page         *int            `json:"page,omitempty"`
	Style        string          `json:"style"`
	MetadataJSON json.RawMessage `json:"metadata_json"`
	DeletedAt    *time.Time      `json:"deleted_at,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

func (s *Store) InsertAnnotation(ctx context.Context, a Annotation) error {
	if a.ReadestType == "" {
		a.ReadestType = "annotation"
	}
	if len(a.MetadataJSON) == 0 {
		a.MetadataJSON = []byte(`{}`)
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO annotation (
			id, user_id, book_id, cfi_range, kind, color, selected_text, note_text,
			readest_type, xpointer0, xpointer1, page, style, metadata_json, deleted_at
		)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6,''), $7, $8, $9, $10, $11, $12, $13, $14::jsonb, $15)
	`, a.ID, a.UserID, a.BookID, a.CFIRange, a.Kind, a.Color, a.SelectedText, a.NoteText,
		a.ReadestType, a.XPointer0, a.XPointer1, a.Page, a.Style, string(a.MetadataJSON), a.DeletedAt)
	if err != nil {
		return fmt.Errorf("insert annotation: %w", err)
	}
	return nil
}

func (s *Store) UpdateAnnotation(ctx context.Context, id, userID string, a Annotation) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE annotation SET
			color = COALESCE(NULLIF($3,''), color),
			note_text = $4,
			cfi_range = COALESCE(NULLIF($5,''), cfi_range),
			selected_text = COALESCE(NULLIF($6,''), selected_text),
			style = COALESCE(NULLIF($7,''), style),
			updated_at = now()
		WHERE id = $1 AND user_id = $2
	`, id, userID, a.Color, a.NoteText, a.CFIRange, a.SelectedText, a.Style)
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
		SELECT id, user_id, book_id, cfi_range, kind, COALESCE(color,''), selected_text, note_text,
		       readest_type, xpointer0, xpointer1, page, style, metadata_json, deleted_at,
		       created_at, updated_at
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
		SELECT id, user_id, book_id, cfi_range, kind, COALESCE(color,''), selected_text, note_text,
		       readest_type, xpointer0, xpointer1, page, style, metadata_json, deleted_at,
		       created_at, updated_at
		FROM annotation WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list annotations: %w", err)
	}
	defer rows.Close()
	return scanAnnotations(rows)
}

// NotebookFilters is the filter envelope for SearchAnnotations.
// All fields optional; an empty filter returns the same list as
// ListAnnotationsByUser.
type NotebookFilters struct {
	Color    string    // exact match on annotation.color
	Style    string    // "highlight" | "underline" | "squiggly"
	Query    string    // ILIKE substring on selected_text OR note_text
	SinceMs  int64     // unix ms; only rows with created_at >= since
	UntilMs  int64     // unix ms; only rows with created_at <= until
	BookID   string    // restrict to one book
	Limit    int
}

// SearchAnnotations is the notebook-view query: every annotation
// for the user, optionally narrowed by colour / style / date range
// / text search / book. Excludes tombstoned rows.
//
// Filter clauses are AND'd together. Query matches selected_text
// OR note_text (case-insensitive substring) so a search for "war"
// surfaces both highlighted-passages-containing-war and
// notes-about-war.
func (s *Store) SearchAnnotations(ctx context.Context, userID string, f NotebookFilters) ([]Annotation, error) {
	if userID == "" {
		return nil, fmt.Errorf("user_id required")
	}
	limit := f.Limit
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	q := `
		SELECT id, user_id, book_id, cfi_range, kind, COALESCE(color,''), selected_text, note_text,
		       readest_type, xpointer0, xpointer1, page, style, metadata_json, deleted_at,
		       created_at, updated_at
		FROM annotation
		WHERE user_id = $1 AND deleted_at IS NULL`
	args := []any{userID}
	addClause := func(clause string, v any) {
		args = append(args, v)
		q += fmt.Sprintf(" AND %s $%d", clause, len(args))
	}
	if f.Color != "" {
		addClause("color =", f.Color)
	}
	if f.Style != "" {
		addClause("style =", f.Style)
	}
	if f.BookID != "" {
		addClause("book_id =", f.BookID)
	}
	if f.SinceMs > 0 {
		addClause("created_at >=", time.UnixMilli(f.SinceMs))
	}
	if f.UntilMs > 0 {
		addClause("created_at <=", time.UnixMilli(f.UntilMs))
	}
	if f.Query != "" {
		args = append(args, "%"+f.Query+"%")
		idx := len(args)
		q += fmt.Sprintf(" AND (selected_text ILIKE $%d OR note_text ILIKE $%d)", idx, idx)
	}
	args = append(args, limit)
	q += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", len(args))
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("search annotations: %w", err)
	}
	defer rows.Close()
	return scanAnnotations(rows)
}

func scanAnnotations(rows pgx.Rows) ([]Annotation, error) {
	var out []Annotation
	for rows.Next() {
		var a Annotation
		if err := rows.Scan(&a.ID, &a.UserID, &a.BookID, &a.CFIRange, &a.Kind,
			&a.Color, &a.SelectedText, &a.NoteText,
			&a.ReadestType, &a.XPointer0, &a.XPointer1, &a.Page, &a.Style, &a.MetadataJSON, &a.DeletedAt,
			&a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, a)
	}
	return out, nil
}

// Test that ErrNotFound matches pgx.ErrNoRows at the helper layer.
var _ = errors.Is
