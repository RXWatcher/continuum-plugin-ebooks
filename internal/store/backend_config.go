package store

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type Config struct {
	TargetBackendPluginID    string
	TargetBackendInstallID   string
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
	StandaloneHTTPListen     string
	UpdatedAt                time.Time
}

func (c Config) BackendInstallID() string {
	if c.TargetBackendInstallID != "" {
		return c.TargetBackendInstallID
	}
	if isNumericID(c.TargetBackendPluginID) {
		return c.TargetBackendPluginID
	}
	return ""
}

func (c Config) BackendPluginID() string {
	if isNumericID(c.TargetBackendPluginID) {
		return ""
	}
	return c.TargetBackendPluginID
}

// BackendTarget returns the identifier used to address the configured backend
// through the host: the installation id when set (preferred since the 0013
// installation-id migration), otherwise a legacy plugin id. Empty means no
// backend is configured. Every backend-targeting call site must resolve
// through this so config done via target_backend_installation_id is honored.
func (c Config) BackendTarget() string {
	if id := c.BackendInstallID(); id != "" {
		return id
	}
	return c.BackendPluginID()
}

// HasBackend reports whether a usable backend target is configured.
func (c Config) HasBackend() bool { return c.BackendTarget() != "" }

func defaultConfigShape() Config {
	return Config{
		DefaultStreamingMode:     "proxy",
		CacheMaxSizeGB:           10,
		CacheDownloadConcurrency: 4,
		PathRemappings:           []byte("[]"),
		OpdsRealm:                "Continuum Library",
		KindleSMTPConfig:         []byte("{}"),
		KepubifyPath:             "/usr/local/bin/kepubify",
	}
}

func isNumericID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// GetConfig returns the singleton backend_config row. If none exists, it
// inserts a sensible default (with a fresh random kosync_secret) and returns
// the new row.
func (s *Store) GetConfig(ctx context.Context) (Config, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT target_backend_plugin_id, target_backend_installation_id,
		       auto_approve_requests, default_streaming_mode,
		       COALESCE(cache_dir,''), cache_max_size_gb, cache_download_concurrency,
		       path_remappings, kosync_secret, opds_realm, kindle_smtp_config,
		       kepubify_path, standalone_http_listen, updated_at
		FROM backend_config WHERE id = 1
	`)
	var c Config
	if err := row.Scan(&c.TargetBackendPluginID, &c.TargetBackendInstallID,
		&c.AutoApproveRequests, &c.DefaultStreamingMode,
		&c.CacheDir, &c.CacheMaxSizeGB, &c.CacheDownloadConcurrency,
		&c.PathRemappings, &c.KosyncSecret, &c.OpdsRealm, &c.KindleSMTPConfig,
		&c.KepubifyPath, &c.StandaloneHTTPListen, &c.UpdatedAt); err != nil {
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
	c = configWithDefaults(c)
	if len(c.KosyncSecret) == 0 {
		existing, err := s.GetConfig(ctx)
		if err == nil {
			c.KosyncSecret = existing.KosyncSecret
		}
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO backend_config (id, target_backend_plugin_id, target_backend_installation_id, auto_approve_requests,
			default_streaming_mode, cache_dir, cache_max_size_gb, cache_download_concurrency,
			path_remappings, kosync_secret, opds_realm, kindle_smtp_config, kepubify_path, standalone_http_listen, updated_at)
		VALUES (1, $1, $2, $3, $4, NULLIF($5,''), $6, $7, $8, $9, $10, $11, $12, $13, now())
		ON CONFLICT (id) DO UPDATE SET
			target_backend_plugin_id   = EXCLUDED.target_backend_plugin_id,
			target_backend_installation_id = EXCLUDED.target_backend_installation_id,
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
			standalone_http_listen     = EXCLUDED.standalone_http_listen,
			updated_at                 = now()
	`, c.TargetBackendPluginID, c.TargetBackendInstallID, c.AutoApproveRequests, c.DefaultStreamingMode,
		c.CacheDir, c.CacheMaxSizeGB, c.CacheDownloadConcurrency,
		c.PathRemappings, c.KosyncSecret, c.OpdsRealm, c.KindleSMTPConfig, c.KepubifyPath, c.StandaloneHTTPListen)
	if err != nil {
		return fmt.Errorf("upsert config: %w", err)
	}
	return nil
}

func configWithDefaults(c Config) Config {
	if len(c.KosyncSecret) == 0 {
		c.KosyncSecret = nil
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
	return c
}

func (s *Store) ImportLegacyConfig(ctx context.Context, legacy Config) (bool, error) {
	current, err := s.GetConfig(ctx)
	if err != nil {
		return false, err
	}
	if !configIsDefault(current) {
		return false, nil
	}
	next := configWithDefaults(legacy)
	next.KosyncSecret = current.KosyncSecret
	if reflect.DeepEqual(configComparable(next), configComparable(current)) {
		return false, nil
	}
	if err := s.UpsertConfig(ctx, next); err != nil {
		return false, err
	}
	return true, nil
}

func configIsDefault(c Config) bool {
	return reflect.DeepEqual(configComparable(configWithDefaults(c)), configComparable(defaultConfigShape()))
}

func configComparable(c Config) Config {
	c.KosyncSecret = nil
	c.UpdatedAt = time.Time{}
	return c
}
