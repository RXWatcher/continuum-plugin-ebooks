package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// SmartCollection mirrors the audiobooks plugin's row shape — rule-
// based dynamic collection whose membership is computed at query
// time from a JSON DSL. The manual `collection` + `collection_item`
// junction-table flow (migrations 0002) remains for hand-curated
// lists; this is a separate surface.
type SmartCollection struct {
	ID          string
	UserID      string
	ProfileID   string
	Name        string
	Description string
	Color       string
	IsPublic    bool
	IsPinned    bool
	QueryDef    json.RawMessage
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (s *Store) UpsertSmartCollection(ctx context.Context, c SmartCollection) error {
	if c.ID == "" || c.UserID == "" || c.Name == "" {
		return errors.New("id, user_id, name required")
	}
	if len(c.QueryDef) == 0 {
		c.QueryDef = json.RawMessage([]byte("{}"))
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO smart_collection (
			id, user_id, profile_id, name, description, color, is_public, is_pinned, query_def
		) VALUES ($1, $2, $3, $4, NULLIF($5,''), NULLIF($6,''), $7, $8, $9)
		ON CONFLICT (id) DO UPDATE SET
			name        = EXCLUDED.name,
			description = EXCLUDED.description,
			color       = EXCLUDED.color,
			is_public   = EXCLUDED.is_public,
			is_pinned   = EXCLUDED.is_pinned,
			query_def   = EXCLUDED.query_def,
			updated_at  = now()
	`, c.ID, c.UserID, c.ProfileID, c.Name, c.Description, c.Color, c.IsPublic, c.IsPinned, c.QueryDef)
	if err != nil {
		return fmt.Errorf("upsert smart_collection: %w", err)
	}
	return nil
}

func (s *Store) GetSmartCollection(ctx context.Context, id string) (SmartCollection, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, user_id, profile_id, name, COALESCE(description,''), COALESCE(color,''),
		       is_public, is_pinned, query_def, created_at, updated_at
		FROM smart_collection WHERE id = $1
	`, id)
	var c SmartCollection
	if err := row.Scan(&c.ID, &c.UserID, &c.ProfileID, &c.Name, &c.Description, &c.Color,
		&c.IsPublic, &c.IsPinned, &c.QueryDef, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SmartCollection{}, ErrNotFound
		}
		return SmartCollection{}, fmt.Errorf("get smart_collection: %w", err)
	}
	return c, nil
}

func (s *Store) ListSmartCollections(ctx context.Context, userID, profileID string, limit int) ([]SmartCollection, error) {
	if userID == "" {
		return nil, errors.New("user_id required")
	}
	if limit <= 0 {
		limit = 500
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, profile_id, name, COALESCE(description,''), COALESCE(color,''),
		       is_public, is_pinned, query_def, created_at, updated_at
		FROM smart_collection
		WHERE (user_id = $1 AND profile_id = $2) OR is_public = TRUE
		ORDER BY (user_id = $1 AND profile_id = $2) DESC, is_pinned DESC, LOWER(name)
		LIMIT $3
	`, userID, profileID, limit)
	if err != nil {
		return nil, fmt.Errorf("list smart_collections: %w", err)
	}
	defer rows.Close()
	var out []SmartCollection
	for rows.Next() {
		var c SmartCollection
		if err := rows.Scan(&c.ID, &c.UserID, &c.ProfileID, &c.Name, &c.Description, &c.Color,
			&c.IsPublic, &c.IsPinned, &c.QueryDef, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan smart_collection: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) DeleteSmartCollection(ctx context.Context, id, userID, profileID string) error {
	if id == "" || userID == "" {
		return errors.New("id, user_id required")
	}
	_, err := s.pool.Exec(ctx, `
		DELETE FROM smart_collection WHERE id = $1 AND user_id = $2 AND profile_id = $3
	`, id, userID, profileID)
	if err != nil {
		return fmt.Errorf("delete smart_collection: %w", err)
	}
	return nil
}
