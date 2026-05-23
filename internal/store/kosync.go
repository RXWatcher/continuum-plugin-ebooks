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
	ProfileID          string
	KosyncUsername     string
	KosyncPasswordHash string
	CreatedAt          time.Time
}

type KosyncProgress struct {
	UserID     string
	ProfileID  string
	Document   string
	Progress   string
	Device     string
	DeviceID   string
	Percentage float64
	Timestamp  time.Time
}

type KosyncBookLink struct {
	Document  string
	BookID    string
	Format    string
	UserID    string
	ProfileID string
	LinkedAt  time.Time
}

func (s *Store) UpsertKosyncUser(ctx context.Context, u KosyncUser) error {
	// The DO UPDATE only fires when the conflicting row is owned by the SAME
	// silo user, so re-registering / rotating a password works but a
	// different user cannot hijack an existing username (the conflict then
	// updates zero rows). kosync_username stays globally unique because
	// auth lookup is by username.
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO kosync_user (user_id, profile_id, kosync_username, kosync_password_hash)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (kosync_username) DO UPDATE
			SET kosync_password_hash = EXCLUDED.kosync_password_hash
			WHERE kosync_user.user_id = EXCLUDED.user_id
	`, u.UserID, u.ProfileID, u.KosyncUsername, u.KosyncPasswordHash)
	if err != nil {
		return fmt.Errorf("upsert kosync_user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrKosyncUsernameTaken
	}
	return nil
}

// CreateKosyncUserStrict inserts a new standalone kosync account and NEVER
// updates an existing one. It is used by the public, unauthenticated KOReader
// /kosync/users/create route: there is no silo identity to authorize a
// password change there, so a conflicting username must be rejected rather
// than have its password silently overwritten by whoever POSTs next
// (account takeover). Password rotation is only possible via the
// authenticated /api/v1/me/kosync/register path (UpsertKosyncUser).
func (s *Store) CreateKosyncUserStrict(ctx context.Context, u KosyncUser) error {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO kosync_user (user_id, profile_id, kosync_username, kosync_password_hash)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (kosync_username) DO NOTHING
	`, u.UserID, u.ProfileID, u.KosyncUsername, u.KosyncPasswordHash)
	if err != nil {
		return fmt.Errorf("create kosync_user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrKosyncUsernameTaken
	}
	return nil
}

func (s *Store) GetKosyncUserByUsername(ctx context.Context, username string) (KosyncUser, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT user_id, profile_id, kosync_username, kosync_password_hash, created_at
		FROM kosync_user WHERE kosync_username = $1
	`, username)
	var u KosyncUser
	if err := row.Scan(&u.UserID, &u.ProfileID, &u.KosyncUsername, &u.KosyncPasswordHash, &u.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return KosyncUser{}, ErrNotFound
		}
		return KosyncUser{}, fmt.Errorf("get kosync_user: %w", err)
	}
	return u, nil
}

func (s *Store) ListKosyncUsers(ctx context.Context) ([]KosyncUser, error) {
	rows, err := s.pool.Query(ctx, `SELECT user_id, profile_id, kosync_username, kosync_password_hash, created_at FROM kosync_user ORDER BY created_at DESC LIMIT 500`)
	if err != nil {
		return nil, fmt.Errorf("list kosync users: %w", err)
	}
	defer rows.Close()
	var out []KosyncUser
	for rows.Next() {
		var u KosyncUser
		if err := rows.Scan(&u.UserID, &u.ProfileID, &u.KosyncUsername, &u.KosyncPasswordHash, &u.CreatedAt); err != nil {
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
	// One transaction so we never orphan kosync_progress/kosync_book_link
	// rows (keyed by user_id) by deleting the user but failing the rest, or
	// vice versa. The progress delete error is no longer discarded.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("delete kosync_user: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM kosync_progress WHERE user_id = $1`, u.UserID); err != nil {
		return fmt.Errorf("delete kosync_progress: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM kosync_book_link WHERE user_id = $1`, u.UserID); err != nil {
		return fmt.Errorf("delete kosync_book_link: %w", err)
	}
	tag, err := tx.Exec(ctx, `DELETE FROM kosync_user WHERE kosync_username = $1`, username)
	if err != nil {
		return fmt.Errorf("delete kosync_user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return tx.Commit(ctx)
}

// UpsertKosyncProgress writes progress scoped to (user_id, profile_id, document, device_id).
// device_id collapses to ” when the client omits it so the legacy "no device"
// case still has a stable upsert key. UserID is the authenticated session's
// identity — callers MUST NOT take it from the request body or any
// client-supplied field, otherwise a malicious client can overwrite another
// user's progress.
func (s *Store) UpsertKosyncProgress(ctx context.Context, p KosyncProgress) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO kosync_progress (user_id, profile_id, document, progress, percentage, device, device_id, timestamp)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6,''), $7, now())
		ON CONFLICT (user_id, profile_id, document, device_id) DO UPDATE SET
			progress = EXCLUDED.progress, percentage = EXCLUDED.percentage,
			device = EXCLUDED.device, device_id = EXCLUDED.device_id, timestamp = now()
	`, p.UserID, p.ProfileID, p.Document, p.Progress, p.Percentage, p.Device, p.DeviceID)
	if err != nil {
		return fmt.Errorf("upsert kosync_progress: %w", err)
	}
	return nil
}

// GetKosyncProgress returns the most-recent progress row for (user_id,
// profile_id, document). KOReader's contract is "latest wins on read".
func (s *Store) GetKosyncProgress(ctx context.Context, userID, profileID, document string) (KosyncProgress, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT user_id, profile_id, document, progress, percentage, COALESCE(device,''), device_id, timestamp
		FROM kosync_progress WHERE user_id = $1 AND profile_id = $2 AND document = $3
		ORDER BY timestamp DESC, device_id DESC LIMIT 1
	`, userID, profileID, document)
	var p KosyncProgress
	if err := row.Scan(&p.UserID, &p.ProfileID, &p.Document, &p.Progress, &p.Percentage, &p.Device, &p.DeviceID, &p.Timestamp); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return KosyncProgress{}, ErrNotFound
		}
		return KosyncProgress{}, fmt.Errorf("get progress: %w", err)
	}
	return p, nil
}

func (s *Store) UpsertKosyncBookLink(ctx context.Context, l KosyncBookLink) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO kosync_book_link (document, book_id, format, user_id, profile_id) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (document, user_id, profile_id) DO UPDATE SET book_id = EXCLUDED.book_id, format = EXCLUDED.format
	`, l.Document, l.BookID, l.Format, l.UserID, l.ProfileID)
	if err != nil {
		return fmt.Errorf("upsert kosync_book_link: %w", err)
	}
	return nil
}

func (s *Store) FindKosyncBookLinkByBook(ctx context.Context, userID, profileID, bookID string) (KosyncBookLink, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT document, book_id, format, user_id, profile_id, linked_at
		FROM kosync_book_link WHERE user_id = $1 AND profile_id = $2 AND book_id = $3 LIMIT 1
	`, userID, profileID, bookID)
	var l KosyncBookLink
	if err := row.Scan(&l.Document, &l.BookID, &l.Format, &l.UserID, &l.ProfileID, &l.LinkedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return KosyncBookLink{}, ErrNotFound
		}
		return KosyncBookLink{}, fmt.Errorf("find kosync_book_link: %w", err)
	}
	return l, nil
}
