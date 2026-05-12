package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type KoboSession struct {
	ID           string
	UserID       string
	BookID       string
	Format       string
	TransferCode string
	SourcePath   string
	Status       string
	CreatedAt    time.Time
	ExpiresAt    time.Time
	CompletedAt  *time.Time
}

func (s *Store) InsertKoboSession(ctx context.Context, k KoboSession) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO kobo_transfer_session (id, user_id, book_id, format, transfer_code, source_path, status, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, k.ID, k.UserID, k.BookID, k.Format, k.TransferCode, k.SourcePath, k.Status, k.ExpiresAt)
	if err != nil {
		return fmt.Errorf("insert kobo session: %w", err)
	}
	return nil
}

func (s *Store) GetKoboSessionByCode(ctx context.Context, code string) (KoboSession, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, user_id, book_id, format, transfer_code, source_path, status, created_at, expires_at, completed_at
		FROM kobo_transfer_session WHERE transfer_code = $1
	`, code)
	var k KoboSession
	if err := row.Scan(&k.ID, &k.UserID, &k.BookID, &k.Format, &k.TransferCode,
		&k.SourcePath, &k.Status, &k.CreatedAt, &k.ExpiresAt, &k.CompletedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return KoboSession{}, ErrNotFound
		}
		return KoboSession{}, fmt.Errorf("get kobo session: %w", err)
	}
	return k, nil
}

func (s *Store) ListKoboSessionsByUser(ctx context.Context, userID string) ([]KoboSession, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, book_id, format, transfer_code, source_path, status, created_at, expires_at, completed_at
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
		SELECT id, user_id, book_id, format, transfer_code, source_path, status, created_at, expires_at, completed_at
		FROM kobo_transfer_session ORDER BY created_at DESC LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list all kobo sessions: %w", err)
	}
	defer rows.Close()
	return scanKoboSessions(rows)
}

func (s *Store) MarkKoboActive(ctx context.Context, code string) error {
	_, err := s.pool.Exec(ctx, `UPDATE kobo_transfer_session SET status = 'active' WHERE transfer_code = $1 AND status = 'pending'`, code)
	return err
}

func (s *Store) MarkKoboCompleted(ctx context.Context, code string) error {
	_, err := s.pool.Exec(ctx, `UPDATE kobo_transfer_session SET status = 'completed', completed_at = now() WHERE transfer_code = $1`, code)
	return err
}

// ExpireStaleKoboSessions transitions sessions past expires_at into status=expired
// and returns the affected rows (for source_path file cleanup).
func (s *Store) ExpireStaleKoboSessions(ctx context.Context, now time.Time) ([]KoboSession, error) {
	rows, err := s.pool.Query(ctx, `
		UPDATE kobo_transfer_session SET status = 'expired'
		WHERE status IN ('pending','active') AND expires_at < $1
		RETURNING id, user_id, book_id, format, transfer_code, source_path, status, created_at, expires_at, completed_at
	`, now)
	if err != nil {
		return nil, fmt.Errorf("expire stale kobo sessions: %w", err)
	}
	defer rows.Close()
	return scanKoboSessions(rows)
}

func scanKoboSessions(rows pgx.Rows) ([]KoboSession, error) {
	var out []KoboSession
	for rows.Next() {
		var k KoboSession
		if err := rows.Scan(&k.ID, &k.UserID, &k.BookID, &k.Format, &k.TransferCode,
			&k.SourcePath, &k.Status, &k.CreatedAt, &k.ExpiresAt, &k.CompletedAt); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, k)
	}
	return out, nil
}
