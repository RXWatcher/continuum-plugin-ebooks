package store

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// NotificationPref is one (user, category, delivery) toggle.
// Defaults to enabled when no row exists — the IsEnabled helper
// implements that "opt-out" semantic.
type NotificationPref struct {
	UserID    string    `json:"user_id"`
	Category  string    `json:"category"`
	Delivery  string    `json:"delivery"` // "inapp" | "email" | "push"
	Enabled   bool      `json:"enabled"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SupportedCategories is the canonical list the SPA renders the
// preferences UI from. Server-side validation rejects categories
// outside this list so a typo doesn't quietly persist a
// preference no notification dispatcher will ever read.
var SupportedCategories = []string{
	"new_book",          // a new audiobook landed
	"reading_reminder",
	"request_fulfilled", // a previously-requested audiobook arrived
	"backup_complete",   // admin-scope; ignored when not admin
	"share_used",        // someone opened a share link the user created
}

var SupportedDeliveries = []string{"inapp", "email", "push"}

func isSupportedCategory(c string) bool {
	for _, x := range SupportedCategories {
		if x == c {
			return true
		}
	}
	return false
}

func isSupportedDelivery(d string) bool {
	for _, x := range SupportedDeliveries {
		if x == d {
			return true
		}
	}
	return false
}

func (s *Store) UpsertNotificationPref(ctx context.Context, p NotificationPref) error {
	if p.UserID == "" || p.Category == "" || p.Delivery == "" {
		return errors.New("user_id, category, delivery required")
	}
	if !isSupportedCategory(p.Category) {
		return fmt.Errorf("unsupported category %q", p.Category)
	}
	if !isSupportedDelivery(p.Delivery) {
		return fmt.Errorf("unsupported delivery %q", p.Delivery)
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO notification_pref (user_id, category, delivery, enabled)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, category, delivery) DO UPDATE SET
			enabled    = EXCLUDED.enabled,
			updated_at = now()
	`, p.UserID, p.Category, p.Delivery, p.Enabled)
	if err != nil {
		return fmt.Errorf("upsert notification_pref: %w", err)
	}
	return nil
}

func (s *Store) ListNotificationPrefs(ctx context.Context, userID string) ([]NotificationPref, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT user_id, category, delivery, enabled, updated_at
		FROM notification_pref WHERE user_id = $1
		ORDER BY category, delivery
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list notification_pref: %w", err)
	}
	defer rows.Close()
	var out []NotificationPref
	for rows.Next() {
		var p NotificationPref
		if err := rows.Scan(&p.UserID, &p.Category, &p.Delivery, &p.Enabled, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan notification_pref: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// IsNotificationEnabled is the dispatcher-side check. Missing
// rows default to true (opt-out semantics — new categories enable
// by default).
func (s *Store) IsNotificationEnabled(ctx context.Context, userID, category, delivery string) (bool, error) {
	if userID == "" {
		return false, errors.New("user_id required")
	}
	var enabled bool
	err := s.pool.QueryRow(ctx, `
		SELECT enabled FROM notification_pref
		WHERE user_id = $1 AND category = $2 AND delivery = $3
	`, userID, category, delivery).Scan(&enabled)
	if err != nil {
		// pgx.ErrNoRows here is "default to enabled".
		return true, nil
	}
	return enabled, nil
}
