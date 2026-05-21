package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// EreaderDevice is one registered destination the user can send a
// book to. Vendor is a free-form hint (kindle / kobo / boox /
// generic) so the SPA can render an icon; PreferredFormat lets the
// user pin which file type the send route should pick when a book
// has multiple.
type EreaderDevice struct {
	ID              string    `json:"id"`
	UserID          string    `json:"user_id"`
	Name            string    `json:"name"`
	Email           string    `json:"email"`
	Vendor          string    `json:"vendor"`
	PreferredFormat string    `json:"preferred_format"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (s *Store) UpsertEreaderDevice(ctx context.Context, d EreaderDevice) error {
	if d.ID == "" || d.UserID == "" || d.Name == "" || d.Email == "" {
		return errors.New("id, user_id, name, email required")
	}
	if d.Vendor == "" {
		d.Vendor = "generic"
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO ereader_device (id, user_id, name, email, vendor, preferred_format)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE SET
			name             = EXCLUDED.name,
			email            = EXCLUDED.email,
			vendor           = EXCLUDED.vendor,
			preferred_format = EXCLUDED.preferred_format,
			updated_at       = now()
	`, d.ID, d.UserID, d.Name, d.Email, d.Vendor, d.PreferredFormat)
	if err != nil {
		return fmt.Errorf("upsert ereader_device: %w", err)
	}
	return nil
}

func (s *Store) GetEreaderDevice(ctx context.Context, id, userID string) (EreaderDevice, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, user_id, name, email, vendor, preferred_format, created_at, updated_at
		FROM ereader_device WHERE id = $1 AND user_id = $2
	`, id, userID)
	var d EreaderDevice
	if err := row.Scan(&d.ID, &d.UserID, &d.Name, &d.Email, &d.Vendor,
		&d.PreferredFormat, &d.CreatedAt, &d.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EreaderDevice{}, ErrNotFound
		}
		return EreaderDevice{}, fmt.Errorf("get ereader_device: %w", err)
	}
	return d, nil
}

func (s *Store) ListEreaderDevices(ctx context.Context, userID string) ([]EreaderDevice, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, name, email, vendor, preferred_format, created_at, updated_at
		FROM ereader_device WHERE user_id = $1
		ORDER BY LOWER(name)
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list ereader_device: %w", err)
	}
	defer rows.Close()
	var out []EreaderDevice
	for rows.Next() {
		var d EreaderDevice
		if err := rows.Scan(&d.ID, &d.UserID, &d.Name, &d.Email, &d.Vendor,
			&d.PreferredFormat, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan ereader_device: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) DeleteEreaderDevice(ctx context.Context, id, userID string) error {
	if id == "" || userID == "" {
		return errors.New("id, user_id required")
	}
	_, err := s.pool.Exec(ctx, `
		DELETE FROM ereader_device WHERE id = $1 AND user_id = $2
	`, id, userID)
	if err != nil {
		return fmt.Errorf("delete ereader_device: %w", err)
	}
	return nil
}
