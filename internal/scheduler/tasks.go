package scheduler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-hclog"

	"github.com/RXWatcher/continuum-plugin-ebooks/internal/backend"
	"github.com/RXWatcher/continuum-plugin-ebooks/internal/event"
	"github.com/RXWatcher/continuum-plugin-ebooks/internal/kindle"
	"github.com/RXWatcher/continuum-plugin-ebooks/internal/koboref"
	"github.com/RXWatcher/continuum-plugin-ebooks/internal/libsync"
	"github.com/RXWatcher/continuum-plugin-ebooks/internal/store"
	"github.com/RXWatcher/continuum-plugin-ebooks/internal/streaming"
)

func decodeBookRef(ref string) (int64, string, bool) {
	left, right, ok := strings.Cut(ref, ":")
	if !ok {
		return 0, ref, false
	}
	id, err := strconv.ParseInt(left, 10, 64)
	if err != nil || id <= 0 {
		return 0, ref, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(right)
	if err != nil {
		return 0, ref, false
	}
	return id, string(raw), true
}

// Tasks holds the shared dependencies all scheduled tasks need.
type Tasks struct {
	Store        *store.Store
	Host         *backend.HostHTTPClient
	Ev           *event.Publisher
	Log          hclog.Logger
	CacheManager *streaming.Manager
	// CacheDir is the on-disk root for cached ebook files.
	CacheDir string
	// KoboRefs is the in-process registry of active Kobo session readers.
	// KoboSessionReaper consults it before unlinking a session source file.
	// If nil, the reaper falls back to the legacy unconditional unlink.
	KoboRefs *koboref.Registry
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
	if !cfg.HasBackend() {
		return nil
	}
	rows, err := t.Store.ListNonTerminal(ctx, 100)
	if err != nil {
		return fmt.Errorf("list non-terminal: %w", err)
	}
	for _, r := range rows {
		if r.ExternalID == "" {
			continue
		}
		// Only poll requests aged > 30s to avoid racing the consumer.
		if time.Since(r.UpdatedAt) < 30*time.Second {
			continue
		}
		targetPluginID := r.TargetPluginID
		if targetPluginID == "" {
			targetPluginID = cfg.BackendTarget()
		}
		bk := backend.NewEbookBackend(t.Host, targetPluginID)
		snap, err := bk.GetRequestSnapshot(ctx, r.ExternalID)
		if err != nil {
			t.Log.Debug("reconciler: snapshot error", "request_id", r.ID, "err", err)
			continue
		}
		if snap.Status == "" || snap.Status == r.Status {
			continue
		}
		// AdvanceRequestStatus (not UpdateRequestStatus) so a backend
		// snapshot polled here can't resurrect a request that a
		// request_fulfilled/denied consumer event terminalised in the race
		// window between ListNonTerminal and this write — it is
		// terminal-guarded and a no-op on an already-terminal row.
		_ = t.Store.AdvanceRequestStatus(ctx, r.ID, snap.Status, r.ExternalID, "", "")
	}
	return nil
}

// CacheEvictor LRU-evicts ebook_file_cache rows down to 95% of the configured
// max size. Removes the on-disk file referenced by relative_path. Delegates
// to streaming.Manager.EvictTo when available so the refcount-aware skip of
// in-flight serves is honored (preventing the read/delete race that produced
// mid-transfer 410/EOF errors). The legacy inline loop is retained only for
// the unusual case where no manager is wired.
func (t *Tasks) CacheEvictor(ctx context.Context) error {
	cfg, err := t.Store.GetConfig(ctx)
	if err != nil {
		return err
	}
	maxBytes := int64(cfg.CacheMaxSizeGB) * 1024 * 1024 * 1024
	target := int64(float64(maxBytes) * 0.95)
	if t.CacheManager != nil {
		_, err := t.CacheManager.EvictTo(ctx, target)
		return err
	}
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
// files. Sessions with an active in-process reader (per t.KoboRefs) are
// skipped — both the status update and the unlink are deferred to the next
// tick. This prevents the read/delete race where handleKoboServeFile is
// mid-io.Copy when the reaper runs.
//
// Also sweeps stray kepub temp files left in the cache dir from
// failed/interrupted previous runs (older than 6h). The threshold is
// deliberately wide: an in-progress kepubify conversion may have a file on
// disk with no DB row yet (so the refcount registry can't see it), and a
// 6h sweep window makes that race statistically impossible while still
// cleaning up after genuinely-abandoned conversions.
func (t *Tasks) KoboSessionReaper(ctx context.Context) error {
	now := time.Now()
	// Only reap sessions that expired more than koboReapGrace ago. A transfer
	// that started just before expiry can still be mid-io.Copy; the grace
	// window (well beyond the 120s server WriteTimeout) keeps its file alive
	// until the copy finishes — defense in depth that holds even when
	// KoboRefs is nil and the in-process refcount can't see the reader.
	const koboReapGrace = 5 * time.Minute
	cutoff := now.Add(-koboReapGrace)
	if t.KoboRefs != nil {
		candidates, err := t.Store.ListStaleKoboSessions(ctx, cutoff)
		if err != nil {
			return err
		}
		for _, k := range candidates {
			if t.KoboRefs.InUse(k.ID) {
				continue
			}
			ok, err := t.Store.ExpireKoboSessionByID(ctx, k.ID)
			if err != nil || !ok {
				continue
			}
			if k.SourcePath != "" {
				_ = os.Remove(k.SourcePath)
			}
		}
	} else {
		expired, err := t.Store.ExpireStaleKoboSessions(ctx, cutoff)
		if err != nil {
			return err
		}
		for _, k := range expired {
			if k.SourcePath != "" {
				_ = os.Remove(k.SourcePath)
			}
		}
	}
	// Sweep stray kepub temp files older than 6h from the cache dir.
	if t.CacheDir != "" {
		cutoff := now.Add(-6 * time.Hour)
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
// The returned cleanup func MUST be called on completion. For (1)/(2) it
// releases the in-process refcount that prevents EvictTo from unlinking the
// file while the SMTP attach is still reading it. For (3) it removes the
// temp file.
func (t *Tasks) fetchKindleSource(ctx context.Context, cfg store.Config, k store.KindleSend) (string, func(), error) {
	noop := func() {}
	backendID := cfg.BackendTarget()
	libraryID, backendBookID, scoped := decodeBookRef(k.BookID)
	if scoped {
		lib, err := t.Store.GetPortalLibrary(ctx, libraryID)
		if err != nil {
			return "", noop, fmt.Errorf("library not configured")
		}
		backendID = lib.BackendPluginID
	}
	if backendID == "" {
		return "", noop, fmt.Errorf("no backend configured")
	}
	bk := backend.NewEbookBackend(t.Host, backendID)
	// Both fetch paths must carry the signed media token — the backend's
	// /api/v1/file/* route is public + token-gated.
	signedPath := bk.SignedFilePath(k.UserID, backendBookID, cfg.MediaSigningSecret)
	if t.CacheManager != nil {
		key := streaming.ComputeCacheKey(backendBookID, backendID, libraryID)
		if e, ok := t.CacheManager.Lookup(ctx, key); ok {
			release := t.CacheManager.Acquire(e.ID)
			return t.CacheManager.PathFor(e), release, nil
		}
		fetch := func(ctx context.Context) (io.ReadCloser, http.Header, int64, string, error) {
			resp, err := t.Host.GetStream(ctx, backendID, signedPath, nil)
			if err != nil {
				return nil, nil, 0, "", err
			}
			return resp.Body, resp.Header, resp.ContentLength, resp.Header.Get("Content-Type"), nil
		}
		entry, err := t.CacheManager.StartOrJoin(ctx, key, backendBookID, "", fetch)
		if err != nil {
			return "", noop, err
		}
		release := t.CacheManager.Acquire(entry.ID)
		return t.CacheManager.PathFor(entry), release, nil
	}
	// Fallback: stream to a temp file.
	resp, err := t.Host.GetStream(ctx, backendID, signedPath, nil)
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

// PortalLibrarySync mirrors the configured target backend's libraries into
// portal presentation shelves. No backend configured -> no-op. The
// libsync.Sync guard makes a briefly-unavailable backend a safe (logged)
// no-op rather than a destructive prune.
func (t *Tasks) PortalLibrarySync(ctx context.Context) error {
	cfg, err := t.Store.GetConfig(ctx)
	if err != nil {
		return fmt.Errorf("get config: %w", err)
	}
	if !cfg.HasBackend() {
		return nil
	}
	target := cfg.BackendTarget()
	if _, err := libsync.Sync(ctx, t.Store,
		backend.NewEbookBackend(t.Host, target), target); err != nil {
		if isPluginHTTPUnsupported(err) {
			t.Log.Info("portal_library_sync skipped: backend does not expose plugin HTTP", "backend", target)
			return nil
		}
		t.Log.Warn("portal_library_sync", "err", err)
		return err
	}
	return nil
}

func isPluginHTTPUnsupported(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "code = Unimplemented") &&
		strings.Contains(msg, "CallPluginHTTP")
}

// PurgeExpired runs the once-per-period cleanup pass. Drops share
// link rows past their expiry + recommendation_cache rows past
// their TTL. Both stores are idempotent on re-run.
func (t *Tasks) PurgeExpired(ctx context.Context) error {
	if t.Store == nil {
		return nil
	}
	if n, err := t.Store.PurgeExpiredShareLinks(ctx); err == nil && n > 0 {
		t.Log.Info("purged expired share_links", "count", n)
	} else if err != nil {
		t.Log.Warn("purge share_links", "err", err.Error())
	}
	if n, err := t.Store.PurgeExpiredEbookRecommendations(ctx); err == nil && n > 0 {
		t.Log.Info("purged expired ebook_recommendation_cache", "count", n)
	} else if err != nil {
		t.Log.Warn("purge ebook_recommendation_cache", "err", err.Error())
	}
	return nil
}
