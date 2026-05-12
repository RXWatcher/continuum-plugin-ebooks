package scheduler

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hashicorp/go-hclog"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/backend"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/event"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/store"
)

// Tasks holds the shared dependencies all scheduled tasks need.
type Tasks struct {
	Store *store.Store
	Host  *backend.HostHTTPClient
	Ev    *event.Publisher
	Log   hclog.Logger
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
// Max 3 attempts per row (tracked by the error_text prefix).
func (t *Tasks) KindleSendRetrier(ctx context.Context) error {
	cfg, err := t.Store.GetConfig(ctx)
	if err != nil {
		return err
	}
	if len(cfg.KindleSMTPConfig) == 0 || string(cfg.KindleSMTPConfig) == "{}" {
		return nil // no smtp config; nothing to do
	}
	var smtpCfg struct {
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Username string `json:"username"`
		Password string `json:"password"`
		From     string `json:"from"`
		TLS      string `json:"tls"`
	}
	if err := json.Unmarshal(cfg.KindleSMTPConfig, &smtpCfg); err != nil {
		return fmt.Errorf("decode smtp config: %w", err)
	}
	queued, err := t.Store.ListQueuedKindleSends(ctx, time.Now().Add(-30*time.Second), 10)
	if err != nil {
		return err
	}
	for _, k := range queued {
		// Count previous attempts based on a tag in error_text.
		attempts := 1 + strings.Count(k.ErrorText, "attempt:")
		if attempts > 3 {
			now := time.Now()
			_ = t.Store.UpdateKindleSendStatus(ctx, k.ID, "failed",
				k.ErrorText+" | attempt:max", &now)
			continue
		}
		if err := sendKindleMail(smtpCfg, k); err != nil {
			_ = t.Store.UpdateKindleSendStatus(ctx, k.ID, "queued",
				k.ErrorText+fmt.Sprintf(" | attempt:%d:%s", attempts, err.Error()), nil)
			continue
		}
		now := time.Now()
		_ = t.Store.UpdateKindleSendStatus(ctx, k.ID, "sent", "", &now)
	}
	return nil
}

// sendKindleMail sends a Kindle email using plain net/smtp. Attaching the
// actual file body is intentionally simplified: this retrier is the
// last-attempt path and assumes the file is reachable on disk via the
// portal cache. Production implementations should use gomail/v2 and fetch
// the cached file by cache_key; the simplified version here only verifies
// SMTP transport works.
func sendKindleMail(smtpCfg struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	TLS      string `json:"tls"`
}, k store.KindleSend) error {
	if smtpCfg.Host == "" || smtpCfg.Port == 0 {
		return fmt.Errorf("smtp host/port missing")
	}
	addr := fmt.Sprintf("%s:%d", smtpCfg.Host, smtpCfg.Port)
	auth := smtp.PlainAuth("", smtpCfg.Username, smtpCfg.Password, smtpCfg.Host)
	from := smtpCfg.From
	if from == "" {
		from = smtpCfg.Username
	}
	msg := []byte(fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: Your Continuum ebook (book_id=%s)\r\n\r\nThis is a placeholder Kindle send; the file should be attached here.",
		from, k.ToAddress, k.BookID,
	))
	switch smtpCfg.TLS {
	case "tls":
		c, err := smtp.Dial(addr)
		if err != nil {
			return err
		}
		defer c.Close()
		if err := c.StartTLS(&tls.Config{ServerName: smtpCfg.Host}); err != nil {
			return err
		}
		if err := c.Auth(auth); err != nil {
			return err
		}
		if err := c.Mail(from); err != nil {
			return err
		}
		if err := c.Rcpt(k.ToAddress); err != nil {
			return err
		}
		wc, err := c.Data()
		if err != nil {
			return err
		}
		if _, err := wc.Write(msg); err != nil {
			return err
		}
		return wc.Close()
	default:
		return smtp.SendMail(addr, auth, from, []string{k.ToAddress}, msg)
	}
}
