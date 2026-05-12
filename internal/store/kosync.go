package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type KosyncUser struct {
	UserID             string
	KosyncUsername     string
	KosyncPasswordHash string
	CreatedAt          time.Time
}

type KosyncProgress struct {
	UserID     string
	Document   string
	Progress   string
	Device     string
	DeviceID   string
	Percentage float64
	Timestamp  time.Time
}

type KosyncBookLink struct {
	Document string
	BookID   string
	Format   string
	UserID   string
	LinkedAt time.Time
}

func (s *Store) UpsertKosyncUser(ctx context.Context, u KosyncUser) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO kosync_user (user_id, kosync_username, kosync_password_hash)
		VALUES ($1, $2, $3)
		ON CONFLICT (kosync_username) DO UPDATE SET kosync_password_hash = EXCLUDED.kosync_password_hash
	`, u.UserID, u.KosyncUsername, u.KosyncPasswordHash)
	if err != nil {
		return fmt.Errorf("upsert kosync_user: %w", err)
	}
	return nil
}

func (s *Store) GetKosyncUserByUsername(ctx context.Context, username string) (KosyncUser, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT user_id, kosync_username, kosync_password_hash, created_at
		FROM kosync_user WHERE kosync_username = $1
	`, username)
	var u KosyncUser
	if err := row.Scan(&u.UserID, &u.KosyncUsername, &u.KosyncPasswordHash, &u.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return KosyncUser{}, ErrNotFound
		}
		return KosyncUser{}, fmt.Errorf("get kosync_user: %w", err)
	}
	return u, nil
}

func (s *Store) ListKosyncUsers(ctx context.Context) ([]KosyncUser, error) {
	rows, err := s.pool.Query(ctx, `SELECT user_id, kosync_username, kosync_password_hash, created_at FROM kosync_user ORDER BY created_at DESC LIMIT 500`)
	if err != nil {
		return nil, fmt.Errorf("list kosync users: %w", err)
	}
	defer rows.Close()
	var out []KosyncUser
	for rows.Next() {
		var u KosyncUser
		if err := rows.Scan(&u.UserID, &u.KosyncUsername, &u.KosyncPasswordHash, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, u)
	}
	return out, nil
}

func (s *Store) DeleteKosyncUser(ctx context.Context, username string) error {
	u, err := s.GetKosyncUserByUsername(ctx, username)
	if err != nil {
		return err
	}
	_, _ = s.pool.Exec(ctx, `DELETE FROM kosync_progress WHERE user_id = $1`, u.UserID)
	tag, err := s.pool.Exec(ctx, `DELETE FROM kosync_user WHERE kosync_username = $1`, username)
	if err != nil {
		return fmt.Errorf("delete kosync_user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UpsertKosyncProgress(ctx context.Context, p KosyncProgress) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO kosync_progress (user_id, document, progress, percentage, device, device_id, timestamp)
		VALUES ($1, $2, $3, $4, NULLIF($5,''), NULLIF($6,''), now())
		ON CONFLICT (user_id, document) DO UPDATE SET
			progress = EXCLUDED.progress, percentage = EXCLUDED.percentage,
			device = EXCLUDED.device, device_id = EXCLUDED.device_id, timestamp = now()
	`, p.UserID, p.Document, p.Progress, p.Percentage, p.Device, p.DeviceID)
	if err != nil {
		return fmt.Errorf("upsert kosync_progress: %w", err)
	}
	return nil
}

func (s *Store) GetKosyncProgress(ctx context.Context, userID, document string) (KosyncProgress, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT user_id, document, progress, percentage, COALESCE(device,''), COALESCE(device_id,''), timestamp
		FROM kosync_progress WHERE user_id = $1 AND document = $2
	`, userID, document)
	var p KosyncProgress
	if err := row.Scan(&p.UserID, &p.Document, &p.Progress, &p.Percentage, &p.Device, &p.DeviceID, &p.Timestamp); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return KosyncProgress{}, ErrNotFound
		}
		return KosyncProgress{}, fmt.Errorf("get progress: %w", err)
	}
	return p, nil
}

func (s *Store) UpsertKosyncBookLink(ctx context.Context, l KosyncBookLink) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO kosync_book_link (document, book_id, format, user_id) VALUES ($1, $2, $3, $4)
		ON CONFLICT (document, user_id) DO UPDATE SET book_id = EXCLUDED.book_id, format = EXCLUDED.format
	`, l.Document, l.BookID, l.Format, l.UserID)
	if err != nil {
		return fmt.Errorf("upsert kosync_book_link: %w", err)
	}
	return nil
}

func (s *Store) FindKosyncBookLinkByBook(ctx context.Context, userID, bookID string) (KosyncBookLink, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT document, book_id, format, user_id, linked_at
		FROM kosync_book_link WHERE user_id = $1 AND book_id = $2 LIMIT 1
	`, userID, bookID)
	var l KosyncBookLink
	if err := row.Scan(&l.Document, &l.BookID, &l.Format, &l.UserID, &l.LinkedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return KosyncBookLink{}, ErrNotFound
		}
		return KosyncBookLink{}, fmt.Errorf("find kosync_book_link: %w", err)
	}
	return l, nil
}
