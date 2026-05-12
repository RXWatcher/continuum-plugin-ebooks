package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type Request struct {
	ID              string
	UserID          string
	Title           string
	Authors         []string
	ISBN            string
	SourceID        string
	FormatPref      string
	Status          string
	TargetPluginID  string
	ExternalID      string
	AutoMonitor     bool
	DeniedReason    string
	FailureReason   string
	FulfilledBookID string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	FulfilledAt     *time.Time
}

func (s *Store) InsertRequest(ctx context.Context, r Request) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO request (id, user_id, title, authors, isbn, source_id, format_pref,
		                    status, target_plugin_id, auto_monitor)
		VALUES ($1, $2, $3, $4, NULLIF($5,''), NULLIF($6,''), NULLIF($7,''),
		        $8, $9, $10)
	`, r.ID, r.UserID, r.Title, r.Authors, r.ISBN, r.SourceID, r.FormatPref,
		r.Status, r.TargetPluginID, r.AutoMonitor)
	if err != nil {
		return fmt.Errorf("insert request: %w", err)
	}
	return nil
}

func (s *Store) GetRequest(ctx context.Context, id string) (Request, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, user_id, title, authors, COALESCE(isbn,''), COALESCE(source_id,''),
		       COALESCE(format_pref,''), status, target_plugin_id, COALESCE(external_id,''),
		       auto_monitor, COALESCE(denied_reason,''), COALESCE(failure_reason,''),
		       COALESCE(fulfilled_book_id,''), created_at, updated_at, fulfilled_at
		FROM request WHERE id = $1
	`, id)
	var r Request
	if err := row.Scan(&r.ID, &r.UserID, &r.Title, &r.Authors, &r.ISBN, &r.SourceID,
		&r.FormatPref, &r.Status, &r.TargetPluginID, &r.ExternalID,
		&r.AutoMonitor, &r.DeniedReason, &r.FailureReason,
		&r.FulfilledBookID, &r.CreatedAt, &r.UpdatedAt, &r.FulfilledAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Request{}, ErrNotFound
		}
		return Request{}, fmt.Errorf("get request: %w", err)
	}
	return r, nil
}

func (s *Store) ListRequestsByUser(ctx context.Context, userID string, limit int) ([]Request, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.queryRequests(ctx, `
		SELECT id, user_id, title, authors, COALESCE(isbn,''), COALESCE(source_id,''),
		       COALESCE(format_pref,''), status, target_plugin_id, COALESCE(external_id,''),
		       auto_monitor, COALESCE(denied_reason,''), COALESCE(failure_reason,''),
		       COALESCE(fulfilled_book_id,''), created_at, updated_at, fulfilled_at
		FROM request WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2
	`, userID, limit)
}

func (s *Store) ListRequestsByStatus(ctx context.Context, status string, limit int) ([]Request, error) {
	if limit <= 0 {
		limit = 100
	}
	return s.queryRequests(ctx, `
		SELECT id, user_id, title, authors, COALESCE(isbn,''), COALESCE(source_id,''),
		       COALESCE(format_pref,''), status, target_plugin_id, COALESCE(external_id,''),
		       auto_monitor, COALESCE(denied_reason,''), COALESCE(failure_reason,''),
		       COALESCE(fulfilled_book_id,''), created_at, updated_at, fulfilled_at
		FROM request WHERE status = $1 ORDER BY created_at DESC LIMIT $2
	`, status, limit)
}

func (s *Store) ListNonTerminal(ctx context.Context, limit int) ([]Request, error) {
	if limit <= 0 {
		limit = 100
	}
	return s.queryRequests(ctx, `
		SELECT id, user_id, title, authors, COALESCE(isbn,''), COALESCE(source_id,''),
		       COALESCE(format_pref,''), status, target_plugin_id, COALESCE(external_id,''),
		       auto_monitor, COALESCE(denied_reason,''), COALESCE(failure_reason,''),
		       COALESCE(fulfilled_book_id,''), created_at, updated_at, fulfilled_at
		FROM request WHERE status NOT IN ('fulfilled','failed','denied','cancelled')
		ORDER BY updated_at ASC LIMIT $1
	`, limit)
}

func (s *Store) UpdateRequestStatus(ctx context.Context, id, status, externalID, denied, failure, fulfilledBookID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE request SET status = $2,
		    external_id       = COALESCE(NULLIF($3,''), external_id),
		    denied_reason     = COALESCE(NULLIF($4,''), denied_reason),
		    failure_reason    = COALESCE(NULLIF($5,''), failure_reason),
		    fulfilled_book_id = COALESCE(NULLIF($6,''), fulfilled_book_id),
		    updated_at        = now(),
		    fulfilled_at      = CASE WHEN $2='fulfilled' AND fulfilled_at IS NULL THEN now() ELSE fulfilled_at END
		WHERE id = $1
	`, id, status, externalID, denied, failure, fulfilledBookID)
	if err != nil {
		return fmt.Errorf("update request: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) MarkFulfilled(ctx context.Context, id, fulfilledBookID string) error {
	return s.UpdateRequestStatus(ctx, id, "fulfilled", "", "", "", fulfilledBookID)
}

func (s *Store) DeleteRequest(ctx context.Context, id, userID string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM request WHERE id = $1 AND user_id = $2 AND status = 'pending'`, id, userID)
	if err != nil {
		return fmt.Errorf("delete request: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) queryRequests(ctx context.Context, sql string, args ...any) ([]Request, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query requests: %w", err)
	}
	defer rows.Close()
	var out []Request
	for rows.Next() {
		var r Request
		if err := rows.Scan(&r.ID, &r.UserID, &r.Title, &r.Authors, &r.ISBN, &r.SourceID,
			&r.FormatPref, &r.Status, &r.TargetPluginID, &r.ExternalID,
			&r.AutoMonitor, &r.DeniedReason, &r.FailureReason,
			&r.FulfilledBookID, &r.CreatedAt, &r.UpdatedAt, &r.FulfilledAt); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, r)
	}
	return out, nil
}
