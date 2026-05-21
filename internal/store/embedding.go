package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

// EbookEmbedding mirrors the audiobooks plugin's AudiobookEmbedding —
// one vector per (book_id, library_id) keyed pair with the metadata
// needed for the canonical_text refresh-check optimisation.
type EbookEmbedding struct {
	BookID        string
	LibraryID     int64
	Embedding     pgvector.Vector
	Model         string
	CanonicalText string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (s *Store) UpsertEbookEmbedding(ctx context.Context, e EbookEmbedding) error {
	if e.BookID == "" || e.LibraryID <= 0 {
		return errors.New("book_id, library_id required")
	}
	if e.Model == "" {
		return errors.New("model required")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO ebook_embedding (book_id, library_id, embedding, model, canonical_text)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (book_id, library_id) DO UPDATE SET
			embedding      = EXCLUDED.embedding,
			model          = EXCLUDED.model,
			canonical_text = EXCLUDED.canonical_text,
			updated_at     = now()
	`, e.BookID, e.LibraryID, e.Embedding, e.Model, e.CanonicalText)
	if err != nil {
		return fmt.Errorf("upsert ebook_embedding: %w", err)
	}
	return nil
}

func (s *Store) GetEbookEmbedding(ctx context.Context, libraryID int64, bookID string) (EbookEmbedding, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT book_id, library_id, embedding, model, canonical_text, created_at, updated_at
		FROM ebook_embedding WHERE library_id = $1 AND book_id = $2
	`, libraryID, bookID)
	var e EbookEmbedding
	if err := row.Scan(&e.BookID, &e.LibraryID, &e.Embedding, &e.Model, &e.CanonicalText, &e.CreatedAt, &e.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EbookEmbedding{}, ErrNotFound
		}
		return EbookEmbedding{}, fmt.Errorf("get ebook_embedding: %w", err)
	}
	return e, nil
}

// SimilarEbook is the vector-search result shape — see audiobooks
// plugin SimilarAudiobook for the equivalent.
type SimilarEbook struct {
	BookID     string
	LibraryID  int64
	Similarity float64
}

func (s *Store) FindSimilarEbooks(ctx context.Context, source pgvector.Vector, excludeBookIDs []string, limit int) ([]SimilarEbook, error) {
	if limit <= 0 {
		limit = 25
	}
	excludeStr := excludeBookIDs
	if excludeStr == nil {
		excludeStr = []string{}
	}
	rows, err := s.pool.Query(ctx, `
		SELECT book_id, library_id, (embedding <=> $1) AS distance
		FROM ebook_embedding
		WHERE NOT (book_id = ANY($2::text[]))
		ORDER BY embedding <=> $1
		LIMIT $3
	`, source, excludeStr, limit)
	if err != nil {
		return nil, fmt.Errorf("find similar: %w", err)
	}
	defer rows.Close()
	out := make([]SimilarEbook, 0, limit)
	for rows.Next() {
		var r SimilarEbook
		var distance float64
		if err := rows.Scan(&r.BookID, &r.LibraryID, &distance); err != nil {
			return nil, fmt.Errorf("scan similar: %w", err)
		}
		r.Similarity = 1.0 - distance/2.0
		out = append(out, r)
	}
	return out, rows.Err()
}

type EbookRecommendationCache struct {
	BookID    string
	LibraryID int64
	RecType   string
	Items     json.RawMessage
	ExpiresAt time.Time
	CreatedAt time.Time
}

func (s *Store) GetEbookRecommendationCache(ctx context.Context, libraryID int64, bookID, recType string) (EbookRecommendationCache, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT book_id, library_id, rec_type, items, expires_at, created_at
		FROM ebook_recommendation_cache
		WHERE library_id = $1 AND book_id = $2 AND rec_type = $3
		  AND expires_at > now()
	`, libraryID, bookID, recType)
	var c EbookRecommendationCache
	if err := row.Scan(&c.BookID, &c.LibraryID, &c.RecType, &c.Items, &c.ExpiresAt, &c.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EbookRecommendationCache{}, ErrNotFound
		}
		return EbookRecommendationCache{}, fmt.Errorf("get rec cache: %w", err)
	}
	return c, nil
}

func (s *Store) SetEbookRecommendationCache(ctx context.Context, libraryID int64, bookID, recType string, items json.RawMessage, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO ebook_recommendation_cache (book_id, library_id, rec_type, items, expires_at)
		VALUES ($1, $2, $3, $4, now() + $5::interval)
		ON CONFLICT (book_id, library_id, rec_type) DO UPDATE SET
			items      = EXCLUDED.items,
			expires_at = EXCLUDED.expires_at,
			created_at = now()
	`, bookID, libraryID, recType, items, fmt.Sprintf("%d seconds", int(ttl.Seconds())))
	if err != nil {
		return fmt.Errorf("set rec cache: %w", err)
	}
	return nil
}

func (s *Store) PurgeExpiredEbookRecommendations(ctx context.Context) (int, error) {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM ebook_recommendation_cache WHERE expires_at <= now()
	`)
	if err != nil {
		return 0, fmt.Errorf("purge expired: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
