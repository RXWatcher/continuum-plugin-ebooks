package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hashicorp/go-hclog"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/backend"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/event"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/kindle"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/store"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/streaming"
)

// Tasks holds the shared dependencies all scheduled tasks need.
type Tasks struct {
	Store        *store.Store
	Host         *backend.HostHTTPClient
	Ev           *event.Publisher
	Log          hclog.Logger
	CacheManager *streaming.Manager
	// CacheDir is the on-disk root for cached ebook files.
	CacheDir string
}

// RequestReconciler polls the active backend for non-terminal request status.
// Non-terminal requests are those with an external_id whose status is not yet
// fulfilled/failed/denied/cancelled. Calls /api/v1/requests/{external_id} on
// the backend.
func (t *Tasks) RequestReconciler(ctx context.Context) error {
	cfg, err := t.Store.GetConfig(ctx)
	if err != nil {
		return fmt.Errorf("get config: %w", err)
	}
	if cfg.TargetBackendPluginID == "" {
		return nil
	}
	rows, err := t.Store.ListNonTerminal(ctx, 100)
	if err != nil {
		return fmt.Errorf("list non-terminal: %w", err)
	}
	bk := backend.NewEbookBackend(t.Host, cfg.TargetBackendPluginID)
	for _, r := range rows {
		if r.ExternalID == "" {
			continue
		}
		// Only poll requests aged > 30s to avoid racing the consumer.
		if time.Since(r.UpdatedAt) < 30*time.Second {
			continue
		}
		snap, err := bk.GetRequestSnapshot(ctx, r.ExternalID)
		if err != nil {
			t.Log.Debug("reconciler: snapshot error", "request_id", r.ID, "err", err)
			continue
		}
		if snap.Status == "" || snap.Status == r.Status {
			continue
		}
		_ = t.Store.UpdateRequestStatus(ctx, r.ID, snap.Status, r.ExternalID, "", "", "")
	}
	return nil
}

// CacheEvictor LRU-evicts ebook_file_cache rows down to 95% of the configured
// max size. Removes the on-disk file referenced by relative_path.
func (t *Tasks) CacheEvictor(ctx context.Context) error {
	cfg, err := t.Store.GetConfig(ctx)
	if err != nil {
		return err
	}
	maxBytes := int64(cfg.CacheMaxSizeGB) * 1024 * 1024 * 1024
	target := int64(float64(maxBytes) * 0.95)
	total, err := t.Store.TotalCacheBytes(ctx)
	if err != nil {
		return err
	}
	if total <= target {
		return nil
	}
	entries, err := t.Store.ListCacheLRU(ctx, 500)
	if err != nil {
		return err
	}
	cacheDir := t.CacheDir
	if cacheDir == "" {
		cacheDir = cfg.CacheDir
	}
	for _, e := range entries {
		if total <= target {
			break
		}
		full := filepath.Join(cacheDir, e.RelativePath)
		_ = os.Remove(full)
		if err := t.Store.DeleteCacheEntry(ctx, e.ID); err == nil {
			total -= e.BytesOnDisk
		}
	}
	return nil
}

// KoboSessionReaper marks expired kobo sessions and removes their disk
// files. Also sweeps stray kepub temp files left in the cache dir from
// failed/interrupted previous runs (older than 1h).
func (t *Tasks) KoboSessionReaper(ctx context.Context) error {
	expired, err := t.Store.ExpireStaleKoboSessions(ctx, time.Now())
	if err != nil {
		return err
	}
	for _, k := range expired {
		if k.SourcePath != "" {
			_ = os.Remove(k.SourcePath)
		}
	}
	// Sweep stray .kepub.epub files older than 1h from the cache dir.
	if t.CacheDir != "" {
		cutoff := time.Now().Add(-1 * time.Hour)
		_ = filepath.WalkDir(t.CacheDir, func(p string, d osDirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			name := d.Name()
			if !strings.HasPrefix(name, "kobo-") {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			if info.ModTime().Before(cutoff) {
				_ = os.Remove(p)
			}
			return nil
		})
	}
	return nil
}

// osDirEntry is an alias to keep the import surface minimal.
type osDirEntry = fs.DirEntry

// OPDSTokenPruner deletes OPDS tokens revoked > 30 days ago.
func (t *Tasks) OPDSTokenPruner(ctx context.Context) error {
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	_, err := t.Store.DeleteOPDSTokensRevokedBefore(ctx, cutoff)
	return err
}

// KindleSendRetrier retries Kindle sends that have been in 'queued' status > 30s.
// Max 3 attempts per row (tracked by the error_text prefix). For each row it
// fetches the EPUB via the streaming layer (cache hit or single-flight
// download), then emails it as an attachment using internal/kindle.Sender.
func (t *Tasks) KindleSendRetrier(ctx context.Context) error {
	cfg, err := t.Store.GetConfig(ctx)
	if err != nil {
		return err
	}
	if len(cfg.KindleSMTPConfig) == 0 || string(cfg.KindleSMTPConfig) == "{}" {
		return nil
	}
	var smtpCfg kindle.SMTPConfig
	if err := json.Unmarshal(cfg.KindleSMTPConfig, &smtpCfg); err != nil {
		return fmt.Errorf("decode smtp config: %w", err)
	}
	sender := kindle.New(smtpCfg)
	queued, err := t.Store.ListQueuedKindleSends(ctx, time.Now().Add(-30*time.Second), 10)
	if err != nil {
		return err
	}
	for _, k := range queued {
		attempts := 1 + strings.Count(k.ErrorText, "attempt:")
		if attempts > 3 {
			now := time.Now()
			_ = t.Store.UpdateKindleSendStatus(ctx, k.ID, "failed",
				k.ErrorText+" | attempt:max", &now)
			continue
		}
		path, cleanup, err := t.fetchKindleSource(ctx, cfg, k)
		if err != nil {
			_ = t.Store.UpdateKindleSendStatus(ctx, k.ID, "queued",
				k.ErrorText+fmt.Sprintf(" | attempt:%d:fetch:%s", attempts, err.Error()), nil)
			continue
		}
		subject := "Your Continuum ebook"
		attachName := fmt.Sprintf("%s.%s", k.BookID, k.Format)
		if err := sender.Send(ctx, k.ToAddress, subject, path, attachName); err != nil {
			_ = t.Store.UpdateKindleSendStatus(ctx, k.ID, "queued",
				k.ErrorText+fmt.Sprintf(" | attempt:%d:send:%s", attempts, err.Error()), nil)
			cleanup()
			continue
		}
		cleanup()
		now := time.Now()
		_ = t.Store.UpdateKindleSendStatus(ctx, k.ID, "sent", "", &now)
	}
	return nil
}

// fetchKindleSource returns a local on-disk path to the ebook bytes for one
// kindle_send_log row. Preference order:
//  1. cache hit via streaming.Manager (no extra IO; returns cached file path)
//  2. cache fill via single-flight (one network round-trip)
//  3. transient temp file as a last resort if no manager is wired
//
// The returned cleanup func MUST be called on completion. It is a no-op for
// (1)/(2) but removes the temp file in case (3).
func (t *Tasks) fetchKindleSource(ctx context.Context, cfg store.Config, k store.KindleSend) (string, func(), error) {
	noop := func() {}
	if cfg.TargetBackendPluginID == "" {
		return "", noop, fmt.Errorf("no backend configured")
	}
	bk := backend.NewEbookBackend(t.Host, cfg.TargetBackendPluginID)
	if t.CacheManager != nil {
		key := streaming.ComputeCacheKey(k.BookID, k.Format, cfg.TargetBackendPluginID)
		if e, ok := t.CacheManager.Lookup(ctx, key); ok {
			return t.CacheManager.PathFor(e), noop, nil
		}
		fetch := func(ctx context.Context) (io.ReadCloser, http.Header, int64, string, error) {
			resp, err := t.Host.GetStream(ctx, cfg.TargetBackendPluginID, bk.FilePath(k.BookID, k.Format), nil)
			if err != nil {
				return nil, nil, 0, "", err
			}
			return resp.Body, resp.Header, resp.ContentLength, resp.Header.Get("Content-Type"), nil
		}
		entry, err := t.CacheManager.StartOrJoin(ctx, key, k.BookID, k.Format, fetch)
		if err != nil {
			return "", noop, err
		}
		return t.CacheManager.PathFor(entry), noop, nil
	}
	// Fallback: stream to a temp file.
	resp, err := t.Host.GetStream(ctx, cfg.TargetBackendPluginID, bk.FilePath(k.BookID, k.Format), nil)
	if err != nil {
		return "", noop, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", noop, fmt.Errorf("upstream %d", resp.StatusCode)
	}
	dir := t.CacheDir
	if dir == "" {
		dir = os.TempDir()
	}
	tmp, err := os.CreateTemp(dir, "kindle-*.epub")
	if err != nil {
		return "", noop, err
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return "", noop, err
	}
	_ = tmp.Close()
	return tmp.Name(), func() { _ = os.Remove(tmp.Name()) }, nil
}
