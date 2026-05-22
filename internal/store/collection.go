package store

import (
	"context"
	"fmt"
	"time"
)

type Collection struct {
	ID          string
	UserID      string
	ProfileID   string
	Name        string
	Color       string
	IsPublic    bool
	IsPinned    bool
	CoverBookID string
	CreatedAt   time.Time
}

type CollectionItem struct {
	CollectionID string
	BookID       string
	Position     int
	AddedAt      time.Time
}

func (s *Store) CreateCollection(ctx context.Context, c Collection) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO collection (id, user_id, profile_id, name, color, is_public, is_pinned, cover_book_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		c.ID, c.UserID, c.ProfileID, c.Name, c.Color, c.IsPublic, c.IsPinned, c.CoverBookID)
	return err
}

func (s *Store) DeleteCollection(ctx context.Context, id, userID, profileID string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM collection WHERE id = $1 AND user_id = $2 AND profile_id = $3`, id, userID, profileID)
	if err != nil {
		return fmt.Errorf("delete collection: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UpdateCollection(ctx context.Context, c Collection) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE collection
		SET name = $4,
		    color = NULLIF($5,''),
		    is_public = $6,
		    is_pinned = $7,
		    cover_book_id = NULLIF($8,'')
		WHERE id = $1 AND user_id = $2 AND profile_id = $3
	`, c.ID, c.UserID, c.ProfileID, c.Name, c.Color, c.IsPublic, c.IsPinned, c.CoverBookID)
	if err != nil {
		return fmt.Errorf("update collection: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListCollectionsByProfile(ctx context.Context, userID, profileID string) ([]Collection, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, profile_id, name, COALESCE(color,''), is_public, is_pinned,
		       COALESCE(cover_book_id,''), created_at
		FROM collection WHERE user_id = $1 AND profile_id = $2 ORDER BY is_pinned DESC, name
	`, userID, profileID)
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}
	defer rows.Close()
	var out []Collection
	for rows.Next() {
		var c Collection
		if err := rows.Scan(&c.ID, &c.UserID, &c.ProfileID, &c.Name, &c.Color, &c.IsPublic, &c.IsPinned,
			&c.CoverBookID, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, c)
	}
	return out, nil
}

func (s *Store) ListPublicCollections(ctx context.Context, limit int) ([]Collection, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, name, COALESCE(color,''), is_public, is_pinned,
		       COALESCE(cover_book_id,''), created_at
		FROM collection WHERE is_public ORDER BY created_at DESC LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list public collections: %w", err)
	}
	defer rows.Close()
	var out []Collection
	for rows.Next() {
		var c Collection
		if err := rows.Scan(&c.ID, &c.UserID, &c.Name, &c.Color, &c.IsPublic, &c.IsPinned,
			&c.CoverBookID, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, c)
	}
	return out, nil
}

func (s *Store) AddItem(ctx context.Context, collID, bookID string, position int) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO collection_item (collection_id, book_id, position) VALUES ($1, $2, $3)
		ON CONFLICT (collection_id, book_id) DO UPDATE SET position = EXCLUDED.position
	`, collID, bookID, position)
	if err != nil {
		return fmt.Errorf("add collection item: %w", err)
	}
	return nil
}

func (s *Store) AddItemForUser(ctx context.Context, userID, profileID, collID, bookID string, position int) error {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO collection_item (collection_id, book_id, position)
		SELECT id, $4, $5 FROM collection WHERE id = $3 AND user_id = $1 AND profile_id = $2
		ON CONFLICT (collection_id, book_id) DO UPDATE SET position = EXCLUDED.position
	`, userID, profileID, collID, bookID, position)
	if err != nil {
		return fmt.Errorf("add collection item: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) RemoveItem(ctx context.Context, collID, bookID string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM collection_item WHERE collection_id = $1 AND book_id = $2`, collID, bookID)
	if err != nil {
		return fmt.Errorf("remove collection item: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) RemoveItemForUser(ctx context.Context, userID, profileID, collID, bookID string) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM collection_item ci
		USING collection c
		WHERE ci.collection_id = c.id
		  AND c.id = $3
		  AND c.user_id = $1
		  AND c.profile_id = $2
		  AND ci.book_id = $4
	`, userID, profileID, collID, bookID)
	if err != nil {
		return fmt.Errorf("remove collection item: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListItems(ctx context.Context, collID string) ([]CollectionItem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT collection_id, book_id, position, added_at FROM collection_item
		WHERE collection_id = $1 ORDER BY position
	`, collID)
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}
	defer rows.Close()
	var out []CollectionItem
	for rows.Next() {
		var i CollectionItem
		if err := rows.Scan(&i.CollectionID, &i.BookID, &i.Position, &i.AddedAt); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, i)
	}
	return out, nil
}

func (s *Store) ListItemsForUser(ctx context.Context, userID, profileID, collID string) ([]CollectionItem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT ci.collection_id, ci.book_id, ci.position, ci.added_at
		FROM collection_item ci
		JOIN collection c ON c.id = ci.collection_id
		WHERE c.id = $3 AND c.user_id = $1 AND c.profile_id = $2
		ORDER BY ci.position
	`, userID, profileID, collID)
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}
	defer rows.Close()
	var out []CollectionItem
	for rows.Next() {
		var i CollectionItem
		if err := rows.Scan(&i.CollectionID, &i.BookID, &i.Position, &i.AddedAt); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, i)
	}
	return out, nil
}
