package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type CacheEntry struct {
	ID             string
	CacheKey       string
	BookID         string
	Format         string
	MimeType       string
	Status         string
	ErrorMessage   string
	RelativePath   string
	ContentLength  int64
	BytesOnDisk    int64
	LastAccessedAt time.Time
	CreatedAt      time.Time
}

func (s *Store) InsertCacheEntry(ctx context.Context, e CacheEntry) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO ebook_file_cache (id, cache_key, book_id, format, mime_type, content_length, status, relative_path)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (cache_key) DO NOTHING
	`, e.ID, e.CacheKey, e.BookID, e.Format, e.MimeType, e.ContentLength, e.Status, e.RelativePath)
	if err != nil {
		return fmt.Errorf("insert cache entry: %w", err)
	}
	return nil
}

func (s *Store) GetCacheByCacheKey(ctx context.Context, key string) (CacheEntry, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, cache_key, book_id, format, mime_type, content_length, status,
		       COALESCE(error_message,''), relative_path, bytes_on_disk, last_accessed_at, created_at
		FROM ebook_file_cache WHERE cache_key = $1
	`, key)
	var e CacheEntry
	if err := row.Scan(&e.ID, &e.CacheKey, &e.BookID, &e.Format, &e.MimeType,
		&e.ContentLength, &e.Status, &e.ErrorMessage, &e.RelativePath,
		&e.BytesOnDisk, &e.LastAccessedAt, &e.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CacheEntry{}, ErrNotFound
		}
		return CacheEntry{}, fmt.Errorf("get cache: %w", err)
	}
	return e, nil
}

func (s *Store) UpdateCacheStatus(ctx context.Context, id, status, errMsg string, bytesOnDisk int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE ebook_file_cache SET status = $2,
		    error_message = NULLIF($3,''), bytes_on_disk = $4
		WHERE id = $1
	`, id, status, errMsg, bytesOnDisk)
	if err != nil {
		return fmt.Errorf("update cache status: %w", err)
	}
	return nil
}

func (s *Store) TouchCache(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `UPDATE ebook_file_cache SET last_accessed_at = now() WHERE id = $1`, id)
	return err
}

func (s *Store) ListCacheLRU(ctx context.Context, limit int) ([]CacheEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, cache_key, book_id, format, mime_type, content_length, status,
		       COALESCE(error_message,''), relative_path, bytes_on_disk, last_accessed_at, created_at
		FROM ebook_file_cache WHERE status = 'ready' ORDER BY last_accessed_at ASC LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list LRU: %w", err)
	}
	defer rows.Close()
	return scanCacheEntries(rows)
}

func (s *Store) DeleteCacheEntry(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM ebook_file_cache WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete cache: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) TotalCacheBytes(ctx context.Context) (int64, error) {
	row := s.pool.QueryRow(ctx, `SELECT COALESCE(SUM(bytes_on_disk),0) FROM ebook_file_cache WHERE status='ready'`)
	var total int64
	if err := row.Scan(&total); err != nil {
		return 0, fmt.Errorf("total bytes: %w", err)
	}
	return total, nil
}

func (s *Store) ListCacheLargest(ctx context.Context, limit int) ([]CacheEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, cache_key, book_id, format, mime_type, content_length, status,
		       COALESCE(error_message,''), relative_path, bytes_on_disk, last_accessed_at, created_at
		FROM ebook_file_cache WHERE status='ready' ORDER BY bytes_on_disk DESC LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list largest: %w", err)
	}
	defer rows.Close()
	return scanCacheEntries(rows)
}

func scanCacheEntries(rows pgx.Rows) ([]CacheEntry, error) {
	var out []CacheEntry
	for rows.Next() {
		var e CacheEntry
		if err := rows.Scan(&e.ID, &e.CacheKey, &e.BookID, &e.Format, &e.MimeType,
			&e.ContentLength, &e.Status, &e.ErrorMessage, &e.RelativePath,
			&e.BytesOnDisk, &e.LastAccessedAt, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, e)
	}
	return out, nil
}
