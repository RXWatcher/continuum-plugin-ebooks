package store

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type Config struct {
	TargetBackendPluginID    string
	AutoApproveRequests      bool
	DefaultStreamingMode     string
	CacheDir                 string
	CacheMaxSizeGB           int
	CacheDownloadConcurrency int
	PathRemappings           []byte
	KosyncSecret             []byte
	OpdsRealm                string
	KindleSMTPConfig         []byte
	KepubifyPath             string
	UpdatedAt                time.Time
}

// GetConfig returns the singleton backend_config row. If none exists, it
// inserts a sensible default (with a fresh random kosync_secret) and returns
// the new row.
func (s *Store) GetConfig(ctx context.Context) (Config, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT target_backend_plugin_id, auto_approve_requests, default_streaming_mode,
		       COALESCE(cache_dir,''), cache_max_size_gb, cache_download_concurrency,
		       path_remappings, kosync_secret, opds_realm, kindle_smtp_config,
		       kepubify_path, updated_at
		FROM backend_config WHERE id = 1
	`)
	var c Config
	if err := row.Scan(&c.TargetBackendPluginID, &c.AutoApproveRequests, &c.DefaultStreamingMode,
		&c.CacheDir, &c.CacheMaxSizeGB, &c.CacheDownloadConcurrency,
		&c.PathRemappings, &c.KosyncSecret, &c.OpdsRealm, &c.KindleSMTPConfig,
		&c.KepubifyPath, &c.UpdatedAt); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return Config{}, fmt.Errorf("get config: %w", err)
		}
		// Insert default
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return Config{}, fmt.Errorf("random: %w", err)
		}
		if _, err := s.pool.Exec(ctx, `
			INSERT INTO backend_config (id, kosync_secret) VALUES (1, $1)
			ON CONFLICT (id) DO NOTHING
		`, secret); err != nil {
			return Config{}, fmt.Errorf("insert default: %w", err)
		}
		return s.GetConfig(ctx)
	}
	return c, nil
}

// UpsertConfig replaces the singleton row.
func (s *Store) UpsertConfig(ctx context.Context, c Config) error {
	if len(c.KosyncSecret) == 0 {
		existing, err := s.GetConfig(ctx)
		if err == nil {
			c.KosyncSecret = existing.KosyncSecret
		}
	}
	if c.DefaultStreamingMode == "" {
		c.DefaultStreamingMode = "proxy"
	}
	if c.OpdsRealm == "" {
		c.OpdsRealm = "Continuum Library"
	}
	if c.KepubifyPath == "" {
		c.KepubifyPath = "/usr/local/bin/kepubify"
	}
	if len(c.PathRemappings) == 0 {
		c.PathRemappings = []byte("[]")
	}
	if len(c.KindleSMTPConfig) == 0 {
		c.KindleSMTPConfig = []byte("{}")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO backend_config (id, target_backend_plugin_id, auto_approve_requests,
			default_streaming_mode, cache_dir, cache_max_size_gb, cache_download_concurrency,
			path_remappings, kosync_secret, opds_realm, kindle_smtp_config, kepubify_path, updated_at)
		VALUES (1, $1, $2, $3, NULLIF($4,''), $5, $6, $7, $8, $9, $10, $11, now())
		ON CONFLICT (id) DO UPDATE SET
			target_backend_plugin_id   = EXCLUDED.target_backend_plugin_id,
			auto_approve_requests      = EXCLUDED.auto_approve_requests,
			default_streaming_mode     = EXCLUDED.default_streaming_mode,
			cache_dir                  = EXCLUDED.cache_dir,
			cache_max_size_gb          = EXCLUDED.cache_max_size_gb,
			cache_download_concurrency = EXCLUDED.cache_download_concurrency,
			path_remappings            = EXCLUDED.path_remappings,
			kosync_secret              = EXCLUDED.kosync_secret,
			opds_realm                 = EXCLUDED.opds_realm,
			kindle_smtp_config         = EXCLUDED.kindle_smtp_config,
			kepubify_path              = EXCLUDED.kepubify_path,
			updated_at                 = now()
	`, c.TargetBackendPluginID, c.AutoApproveRequests, c.DefaultStreamingMode,
		c.CacheDir, c.CacheMaxSizeGB, c.CacheDownloadConcurrency,
		c.PathRemappings, c.KosyncSecret, c.OpdsRealm, c.KindleSMTPConfig, c.KepubifyPath)
	if err != nil {
		return fmt.Errorf("upsert config: %w", err)
	}
	return nil
}
