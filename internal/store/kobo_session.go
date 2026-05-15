package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type KoboSession struct {
	ID          string
	UserID      string
	BookID      string
	Format      string
	CodeHash    string
	SourcePath  string
	Status      string
	CreatedAt   time.Time
	ExpiresAt   time.Time
	CompletedAt *time.Time
}

func (s *Store) InsertKoboSession(ctx context.Context, k KoboSession) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO kobo_transfer_session (id, user_id, book_id, format, code_hash, source_path, status, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, k.ID, k.UserID, k.BookID, k.Format, k.CodeHash, k.SourcePath, k.Status, k.ExpiresAt)
	if err != nil {
		return fmt.Errorf("insert kobo session: %w", err)
	}
	return nil
}

// ListActiveKoboSessions returns sessions still in pending|active state with
// expires_at > now. The serve-file path iterates these and bcrypt-compares the
// URL-supplied code against each row's CodeHash. The pending/active partial
// index keeps this set small (typically a handful of rows system-wide).
func (s *Store) ListActiveKoboSessions(ctx context.Context, now time.Time) ([]KoboSession, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, book_id, format, code_hash, source_path, status, created_at, expires_at, completed_at
		FROM kobo_transfer_session
		WHERE status IN ('pending','active') AND expires_at > $1
	`, now)
	if err != nil {
		return nil, fmt.Errorf("list active kobo sessions: %w", err)
	}
	defer rows.Close()
	return scanKoboSessions(rows)
}

// MarkKoboActiveByID and MarkKoboCompletedByID replace the previous
// code-keyed variants: since the URL-supplied code is no longer the lookup
// key (the bcrypt hash is), id is now the stable handle.
func (s *Store) MarkKoboActiveByID(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `UPDATE kobo_transfer_session SET status = 'active' WHERE id = $1 AND status = 'pending'`, id)
	return err
}

func (s *Store) MarkKoboCompletedByID(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `UPDATE kobo_transfer_session SET status = 'completed', completed_at = now() WHERE id = $1`, id)
	return err
}

func (s *Store) ListKoboSessionsByUser(ctx context.Context, userID string) ([]KoboSession, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, book_id, format, code_hash, source_path, status, created_at, expires_at, completed_at
		FROM kobo_transfer_session WHERE user_id = $1 ORDER BY created_at DESC LIMIT 50
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list kobo sessions: %w", err)
	}
	defer rows.Close()
	return scanKoboSessions(rows)
}

func (s *Store) ListAllKoboSessions(ctx context.Context, limit int) ([]KoboSession, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, book_id, format, code_hash, source_path, status, created_at, expires_at, completed_at
		FROM kobo_transfer_session ORDER BY created_at DESC LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list all kobo sessions: %w", err)
	}
	defer rows.Close()
	return scanKoboSessions(rows)
}

// ExpireStaleKoboSessions transitions sessions past expires_at into status=expired
// and returns the affected rows (for source_path file cleanup).
func (s *Store) ExpireStaleKoboSessions(ctx context.Context, now time.Time) ([]KoboSession, error) {
	rows, err := s.pool.Query(ctx, `
		UPDATE kobo_transfer_session SET status = 'expired'
		WHERE status IN ('pending','active') AND expires_at < $1
		RETURNING id, user_id, book_id, format, code_hash, source_path, status, created_at, expires_at, completed_at
	`, now)
	if err != nil {
		return nil, fmt.Errorf("expire stale kobo sessions: %w", err)
	}
	defer rows.Close()
	return scanKoboSessions(rows)
}

// ListStaleKoboSessions returns sessions past expires_at without changing
// their status. Used by the reaper to check the in-process refcount registry
// before unlinking the source file; sessions with active readers are skipped
// and reconsidered on the next tick.
func (s *Store) ListStaleKoboSessions(ctx context.Context, now time.Time) ([]KoboSession, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, book_id, format, code_hash, source_path, status, created_at, expires_at, completed_at
		FROM kobo_transfer_session
		WHERE status IN ('pending','active') AND expires_at < $1
	`, now)
	if err != nil {
		return nil, fmt.Errorf("list stale kobo sessions: %w", err)
	}
	defer rows.Close()
	return scanKoboSessions(rows)
}

// ExpireKoboSessionByID transitions a single session to status=expired only
// if it is still in a sweepable state (pending|active). Returns true if the
// row was updated.
func (s *Store) ExpireKoboSessionByID(ctx context.Context, id string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE kobo_transfer_session SET status = 'expired'
		WHERE id = $1 AND status IN ('pending','active')
	`, id)
	if err != nil {
		return false, fmt.Errorf("expire kobo session: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func scanKoboSessions(rows pgx.Rows) ([]KoboSession, error) {
	var out []KoboSession
	for rows.Next() {
		var k KoboSession
		if err := rows.Scan(&k.ID, &k.UserID, &k.BookID, &k.Format, &k.CodeHash,
			&k.SourcePath, &k.Status, &k.CreatedAt, &k.ExpiresAt, &k.CompletedAt); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, k)
	}
	return out, nil
}
