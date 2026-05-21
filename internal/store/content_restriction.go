package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ContentRestriction mirrors the audiobooks plugin shape minus the
// narrators column. Same admin-writes, listener-reads model.
type ContentRestriction struct {
	UserID           string
	BlockedGenres    []string
	BlockedTags      []string
	BlockedAuthors   []string
	BlockedLibraries []int64
	ExplicitBlocked  bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (s *Store) GetContentRestriction(ctx context.Context, userID string) (ContentRestriction, error) {
	if userID == "" {
		return ContentRestriction{}, errors.New("user_id required")
	}
	row := s.pool.QueryRow(ctx, `
		SELECT user_id, blocked_genres, blocked_tags, blocked_authors,
		       blocked_libraries, explicit_blocked, created_at, updated_at
		FROM content_restriction WHERE user_id = $1
	`, userID)
	var r ContentRestriction
	if err := row.Scan(&r.UserID, &r.BlockedGenres, &r.BlockedTags,
		&r.BlockedAuthors, &r.BlockedLibraries, &r.ExplicitBlocked,
		&r.CreatedAt, &r.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ContentRestriction{}, ErrNotFound
		}
		return ContentRestriction{}, fmt.Errorf("get content_restriction: %w", err)
	}
	return r, nil
}

func (s *Store) UpsertContentRestriction(ctx context.Context, r ContentRestriction) error {
	if r.UserID == "" {
		return errors.New("user_id required")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO content_restriction (
			user_id, blocked_genres, blocked_tags, blocked_authors,
			blocked_libraries, explicit_blocked
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id) DO UPDATE SET
			blocked_genres    = EXCLUDED.blocked_genres,
			blocked_tags      = EXCLUDED.blocked_tags,
			blocked_authors   = EXCLUDED.blocked_authors,
			blocked_libraries = EXCLUDED.blocked_libraries,
			explicit_blocked  = EXCLUDED.explicit_blocked,
			updated_at        = now()
	`, r.UserID, r.BlockedGenres, r.BlockedTags, r.BlockedAuthors,
		r.BlockedLibraries, r.ExplicitBlocked)
	if err != nil {
		return fmt.Errorf("upsert content_restriction: %w", err)
	}
	return nil
}

func (s *Store) DeleteContentRestriction(ctx context.Context, userID string) error {
	if userID == "" {
		return errors.New("user_id required")
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM content_restriction WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("delete content_restriction: %w", err)
	}
	return nil
}

func (s *Store) ListContentRestrictions(ctx context.Context) ([]ContentRestriction, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT user_id, blocked_genres, blocked_tags, blocked_authors,
		       blocked_libraries, explicit_blocked, created_at, updated_at
		FROM content_restriction ORDER BY user_id
	`)
	if err != nil {
		return nil, fmt.Errorf("list content_restriction: %w", err)
	}
	defer rows.Close()
	var out []ContentRestriction
	for rows.Next() {
		var r ContentRestriction
		if err := rows.Scan(&r.UserID, &r.BlockedGenres, &r.BlockedTags,
			&r.BlockedAuthors, &r.BlockedLibraries, &r.ExplicitBlocked,
			&r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan content_restriction: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AllowsItem reports whether the given ebook summary passes this
// user's restriction. Genres + tags pass-through from
// EbookDetail-shaped callers; the summary-level filter only checks
// authors + libraries (which is what we have on the wire).
func (r ContentRestriction) AllowsItem(libraryID int64, genres, tags, authors []string, explicit bool) bool {
	if r.UserID == "" {
		return true
	}
	if r.ExplicitBlocked && explicit {
		return false
	}
	for _, id := range r.BlockedLibraries {
		if id == libraryID {
			return false
		}
	}
	if anyMatch(genres, r.BlockedGenres) {
		return false
	}
	if anyMatch(tags, r.BlockedTags) {
		return false
	}
	if anyMatch(authors, r.BlockedAuthors) {
		return false
	}
	return true
}

func anyMatch(haystack, needles []string) bool {
	if len(haystack) == 0 || len(needles) == 0 {
		return false
	}
	hs := make(map[string]struct{}, len(haystack))
	for _, s := range haystack {
		hs[strings.ToLower(strings.TrimSpace(s))] = struct{}{}
	}
	for _, n := range needles {
		if _, ok := hs[strings.ToLower(strings.TrimSpace(n))]; ok {
			return true
		}
	}
	return false
}
