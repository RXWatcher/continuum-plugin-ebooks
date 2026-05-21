package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/auth"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/backend"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/kindle"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/store"
)

// User-facing ereader device list + send-ebook-to-device route.
// Mirrors the ABS surface: /api/me/ereader-devices (CRUD) +
// /api/emails/send-ebook-to-device (one-click send). We reuse the
// existing kindle.Sender for SMTP delivery — same transport, just
// targeted at any registered device rather than the legacy single
// kindle_email column.

func (s *Server) mountEreaderRoutes(r chi.Router) {
	r.Get("/me/ereader-devices", s.handleListEreaderDevices)
	r.Post("/me/ereader-devices", s.handleCreateEreaderDevice)
	r.Patch("/me/ereader-devices/{id}", s.handleUpdateEreaderDevice)
	r.Delete("/me/ereader-devices/{id}", s.handleDeleteEreaderDevice)
	r.Post("/emails/send-ebook-to-device", s.handleSendEbookToDevice)
}

type ereaderDeviceBody struct {
	Name            string `json:"name"`
	Email           string `json:"email"`
	Vendor          string `json:"vendor"`
	PreferredFormat string `json:"preferred_format"`
}

func (s *Server) handleListEreaderDevices(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	rows, err := s.deps.Store.ListEreaderDevices(r.Context(), id.UserID)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (s *Server) handleCreateEreaderDevice(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	var body ereaderDeviceBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(body.Name) == "" || strings.TrimSpace(body.Email) == "" {
		http.Error(w, "name and email required", http.StatusBadRequest)
		return
	}
	d := store.EreaderDevice{
		ID:              ulid.Make().String(),
		UserID:          id.UserID,
		Name:            body.Name,
		Email:           body.Email,
		Vendor:          body.Vendor,
		PreferredFormat: body.PreferredFormat,
	}
	if err := s.deps.Store.UpsertEreaderDevice(r.Context(), d); err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

func (s *Server) handleUpdateEreaderDevice(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	deviceID := chi.URLParam(r, "id")
	existing, err := s.deps.Store.GetEreaderDevice(r.Context(), deviceID, id.UserID)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	var body ereaderDeviceBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.Name != "" {
		existing.Name = body.Name
	}
	if body.Email != "" {
		existing.Email = body.Email
	}
	if body.Vendor != "" {
		existing.Vendor = body.Vendor
	}
	existing.PreferredFormat = body.PreferredFormat
	if err := s.deps.Store.UpsertEreaderDevice(r.Context(), existing); err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (s *Server) handleDeleteEreaderDevice(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	if err := s.deps.Store.DeleteEreaderDevice(r.Context(), chi.URLParam(r, "id"), id.UserID); err != nil {
		writeInternal(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleSendEbookToDevice POSTs {device_id, book_id} and triggers
// the SMTP send. Body shape is the ABS-compat form so the official
// mobile client's "Send to e-reader" button works as-is.
//
// Flow:
//   1. Resolve user + verify device ownership.
//   2. Fetch SMTP config from backend_config.kindle_smtp_config.
//   3. Fetch book detail to pick a file (preferred_format wins; fall
//      back to first available).
//   4. Stream the file via host.GetStream into a temp file.
//   5. SMTP send with subject = book title.
//   6. 202 Accepted with the device + book ids.
func (s *Server) handleSendEbookToDevice(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	var body struct {
		DeviceID string `json:"device_id"`
		BookID   string `json:"book_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.DeviceID == "" || body.BookID == "" {
		http.Error(w, "device_id and book_id required", http.StatusBadRequest)
		return
	}

	device, err := s.deps.Store.GetEreaderDevice(r.Context(), body.DeviceID, id.UserID)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}
	if err != nil {
		writeInternal(w, r, err)
		return
	}

	cfg, err := s.deps.Store.GetConfig(r.Context())
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	var smtpCfg kindle.SMTPConfig
	if len(cfg.KindleSMTPConfig) > 0 {
		_ = json.Unmarshal(cfg.KindleSMTPConfig, &smtpCfg)
	}
	if smtpCfg.Host == "" {
		http.Error(w, "SMTP not configured (admin must set kindle_smtp_config)", http.StatusServiceUnavailable)
		return
	}

	bk, err := s.targetBackend(r)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	detail, err := bk.GetBook(r.Context(), body.BookID)
	if err != nil {
		http.Error(w, "book lookup failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	file := pickBookFile(detail, device.PreferredFormat)
	if file == nil {
		http.Error(w, "no suitable file for this device", http.StatusBadRequest)
		return
	}

	// Stream the file into a tempfile. The SMTP client expects a
	// path-on-disk attachment; streaming to disk avoids holding the
	// whole book in memory for big files.
	tmpDir, _ := os.MkdirTemp("", "send-ebook-*")
	defer os.RemoveAll(tmpDir)
	filename := safeFilename(detail.Title, file.Format)
	dst := filepath.Join(tmpDir, filename)
	if err := s.streamBookToFile(r, bk, detail.ID, file.Format, dst); err != nil {
		http.Error(w, "fetch ebook bytes failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	sender := kindle.New(smtpCfg)
	subject := detail.Title
	if subject == "" {
		subject = "Sent from Continuum"
	}
	if err := sender.Send(r.Context(), device.Email, subject, dst, filename); err != nil {
		http.Error(w, "send failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"device_id": device.ID,
		"book_id":   body.BookID,
		"filename":  filename,
	})
}

// pickBookFile chooses one file from the detail. preferred wins
// when present and matched; otherwise first available.
func pickBookFile(detail backend.EbookDetail, preferred string) *backend.EbookFile {
	if len(detail.Files) == 0 {
		return nil
	}
	if preferred != "" {
		for i := range detail.Files {
			if strings.EqualFold(detail.Files[i].Format, preferred) {
				return &detail.Files[i]
			}
		}
	}
	return &detail.Files[0]
}

// streamBookToFile pulls the book bytes via the backend's signed
// /file path into a local tempfile. Uses the host HTTP client so
// the auth + content-range plumbing matches the rest of the server.
func (s *Server) streamBookToFile(r *http.Request, bk *backend.EbookBackend, bookID, format, dst string) error {
	// The backend's FilePath is /api/v1/file/{bookId}. The mediatoken
	// is required since the backend's public route validates it.
	id, _ := auth.FromContext(r.Context())
	cfg, _ := s.deps.Store.GetConfig(r.Context())
	path := bk.SignedFilePath(id.UserID, bookID, cfg.MediaSigningSecret)
	if format != "" {
		// Some backends accept ?format= to pick between formats; ours
		// ignores it, but it doesn't hurt to forward.
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		path += sep + "format=" + url.QueryEscape(format)
	}
	resp, err := s.deps.Host.GetStream(r.Context(), cfg.BackendInstallID(), path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("upstream %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return err
	}
	return nil
}

// safeFilename trims path-unsafe characters from the title and
// appends the format extension. 80-char cap so long titles don't
// produce 250-char attachment filenames.
func safeFilename(title, format string) string {
	if title == "" {
		title = "book"
	}
	clean := strings.NewReplacer(
		`/`, "_", `\`, "_", `:`, "_", `*`, "_",
		`?`, "_", `"`, "_", `<`, "_", `>`, "_", `|`, "_",
	).Replace(title)
	if len(clean) > 80 {
		clean = clean[:80]
	}
	ext := strings.ToLower(strings.TrimPrefix(format, "."))
	if ext == "" {
		ext = "epub"
	}
	return clean + "." + ext
}
