package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type KindleSend struct {
	ID        string
	UserID    string
	BookID    string
	Format    string
	ToAddress string
	Status    string
	ErrorText string
	SentAt    *time.Time
	CreatedAt time.Time
}

func (s *Store) InsertKindleSend(ctx context.Context, k KindleSend) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO kindle_send_log (id, user_id, book_id, format, to_address, status)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, k.ID, k.UserID, k.BookID, k.Format, k.ToAddress, k.Status)
	if err != nil {
		return fmt.Errorf("insert kindle_send: %w", err)
	}
	return nil
}

func (s *Store) GetKindleSend(ctx context.Context, id string) (KindleSend, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, user_id, book_id, format, to_address, status,
		       COALESCE(error_text,''), sent_at, created_at
		FROM kindle_send_log WHERE id = $1
	`, id)
	var k KindleSend
	if err := row.Scan(&k.ID, &k.UserID, &k.BookID, &k.Format, &k.ToAddress, &k.Status,
		&k.ErrorText, &k.SentAt, &k.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return KindleSend{}, ErrNotFound
		}
		return KindleSend{}, fmt.Errorf("get kindle_send: %w", err)
	}
	return k, nil
}

func (s *Store) UpdateKindleSendStatus(ctx context.Context, id, status, errText string, sentAt *time.Time) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE kindle_send_log SET status = $2, error_text = NULLIF($3,''), sent_at = $4
		WHERE id = $1
	`, id, status, errText, sentAt)
	if err != nil {
		return fmt.Errorf("update kindle_send: %w", err)
	}
	return nil
}

func (s *Store) ListKindleSendsByUser(ctx context.Context, userID string, limit int) ([]KindleSend, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.queryKindleSends(ctx, `
		SELECT id, user_id, book_id, format, to_address, status, COALESCE(error_text,''), sent_at, created_at
		FROM kindle_send_log WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2
	`, userID, limit)
}

func (s *Store) ListAllKindleSends(ctx context.Context, limit int) ([]KindleSend, error) {
	if limit <= 0 {
		limit = 100
	}
	return s.queryKindleSends(ctx, `
		SELECT id, user_id, book_id, format, to_address, status, COALESCE(error_text,''), sent_at, created_at
		FROM kindle_send_log ORDER BY created_at DESC LIMIT $1
	`, limit)
}

func (s *Store) ListQueuedKindleSends(ctx context.Context, agedAfter time.Time, limit int) ([]KindleSend, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.queryKindleSends(ctx, `
		SELECT id, user_id, book_id, format, to_address, status, COALESCE(error_text,''), sent_at, created_at
		FROM kindle_send_log WHERE status = 'queued' AND created_at < $1
		ORDER BY created_at LIMIT $2
	`, agedAfter, limit)
}

func (s *Store) queryKindleSends(ctx context.Context, sql string, args ...any) ([]KindleSend, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query kindle sends: %w", err)
	}
	defer rows.Close()
	var out []KindleSend
	for rows.Next() {
		var k KindleSend
		if err := rows.Scan(&k.ID, &k.UserID, &k.BookID, &k.Format, &k.ToAddress, &k.Status,
			&k.ErrorText, &k.SentAt, &k.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, k)
	}
	return out, nil
}

// silence pgx unused-import vs annotation file
var _ = pgx.ErrNoRows
