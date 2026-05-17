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
	MediaType       string
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
		INSERT INTO request (id, user_id, title, authors, isbn, source_id, format_pref, media_type,
		                    status, target_plugin_id, auto_monitor)
		VALUES ($1, $2, $3, $4, NULLIF($5,''), NULLIF($6,''), NULLIF($7,''),
		        COALESCE(NULLIF($8,''), 'book'), $9, $10, $11)
	`, r.ID, r.UserID, r.Title, r.Authors, r.ISBN, r.SourceID, r.FormatPref,
		r.MediaType, r.Status, r.TargetPluginID, r.AutoMonitor)
	if err != nil {
		return fmt.Errorf("insert request: %w", err)
	}
	return nil
}

func (s *Store) GetRequest(ctx context.Context, id string) (Request, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, user_id, title, authors, COALESCE(isbn,''), COALESCE(source_id,''),
		       COALESCE(format_pref,''), COALESCE(media_type,'book'), status, target_plugin_id, COALESCE(external_id,''),
		       auto_monitor, COALESCE(denied_reason,''), COALESCE(failure_reason,''),
		       COALESCE(fulfilled_book_id,''), created_at, updated_at, fulfilled_at
		FROM request WHERE id = $1
	`, id)
	var r Request
	if err := row.Scan(&r.ID, &r.UserID, &r.Title, &r.Authors, &r.ISBN, &r.SourceID,
		&r.FormatPref, &r.MediaType, &r.Status, &r.TargetPluginID, &r.ExternalID,
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
		       COALESCE(format_pref,''), COALESCE(media_type,'book'), status, target_plugin_id, COALESCE(external_id,''),
		       auto_monitor, COALESCE(denied_reason,''), COALESCE(failure_reason,''),
		       COALESCE(fulfilled_book_id,''), created_at, updated_at, fulfilled_at
		FROM request WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2
	`, userID, limit)
}

func (s *Store) GetRequestForUser(ctx context.Context, id, userID string) (Request, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, user_id, title, authors, COALESCE(isbn,''), COALESCE(source_id,''),
		       COALESCE(format_pref,''), COALESCE(media_type,'book'), status, target_plugin_id, COALESCE(external_id,''),
		       auto_monitor, COALESCE(denied_reason,''), COALESCE(failure_reason,''),
		       COALESCE(fulfilled_book_id,''), created_at, updated_at, fulfilled_at
		FROM request WHERE id = $1 AND user_id = $2
	`, id, userID)
	var r Request
	if err := row.Scan(&r.ID, &r.UserID, &r.Title, &r.Authors, &r.ISBN, &r.SourceID,
		&r.FormatPref, &r.MediaType, &r.Status, &r.TargetPluginID, &r.ExternalID,
		&r.AutoMonitor, &r.DeniedReason, &r.FailureReason,
		&r.FulfilledBookID, &r.CreatedAt, &r.UpdatedAt, &r.FulfilledAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Request{}, ErrNotFound
		}
		return Request{}, fmt.Errorf("get request for user: %w", err)
	}
	return r, nil
}

func (s *Store) ListRequestsByStatus(ctx context.Context, status string, limit int) ([]Request, error) {
	if limit <= 0 {
		limit = 100
	}
	return s.queryRequests(ctx, `
		SELECT id, user_id, title, authors, COALESCE(isbn,''), COALESCE(source_id,''),
		       COALESCE(format_pref,''), COALESCE(media_type,'book'), status, target_plugin_id, COALESCE(external_id,''),
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
		       COALESCE(format_pref,''), COALESCE(media_type,'book'), status, target_plugin_id, COALESCE(external_id,''),
		       auto_monitor, COALESCE(denied_reason,''), COALESCE(failure_reason,''),
		       COALESCE(fulfilled_book_id,''), created_at, updated_at, fulfilled_at
		FROM request WHERE status NOT IN ('fulfilled','failed','denied','cancelled')
		ORDER BY updated_at ASC, id ASC LIMIT $1
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

// AdvanceRequestStatus is the event-driven counterpart of
// UpdateRequestStatus: it refuses to move a request OUT of a terminal state
// (fulfilled/failed/denied/cancelled). Backend events are unordered and
// at-least-once, so a delayed/duplicate "downloading" must not resurrect a
// already-fulfilled request. Returns:
//   - nil            : status advanced, or row already terminal (benign no-op)
//   - ErrNotFound    : no such request (caller should retry — the submit row
//     may not be visible yet — or treat as a foreign event)
//   - other error    : real DB failure (caller should nack for redelivery)
func (s *Store) AdvanceRequestStatus(ctx context.Context, id, status, externalID, failure, fulfilledBookID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE request SET status = $2,
		    external_id       = COALESCE(NULLIF($3,''), external_id),
		    failure_reason    = COALESCE(NULLIF($4,''), failure_reason),
		    fulfilled_book_id = COALESCE(NULLIF($5,''), fulfilled_book_id),
		    updated_at        = now(),
		    fulfilled_at      = CASE WHEN $2='fulfilled' AND fulfilled_at IS NULL THEN now() ELSE fulfilled_at END
		WHERE id = $1
		  AND status NOT IN ('fulfilled','failed','denied','cancelled')
	`, id, status, externalID, failure, fulfilledBookID)
	if err != nil {
		return fmt.Errorf("advance request: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	// 0 rows: either the request doesn't exist, or it's already terminal
	// (guard blocked the regression). Distinguish so the consumer can retry
	// the former but ack the latter.
	var dummy string
	if err := s.pool.QueryRow(ctx, `SELECT id FROM request WHERE id = $1`, id).Scan(&dummy); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("advance request lookup: %w", err)
	}
	return nil // exists and terminal — nothing to do
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
			&r.FormatPref, &r.MediaType, &r.Status, &r.TargetPluginID, &r.ExternalID,
			&r.AutoMonitor, &r.DeniedReason, &r.FailureReason,
			&r.FulfilledBookID, &r.CreatedAt, &r.UpdatedAt, &r.FulfilledAt); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, r)
	}
	return out, nil
}
