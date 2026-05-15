package server

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"
	"golang.org/x/crypto/bcrypt"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/auth"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/backend"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/store"
)

// handleSendToKindle queues a Kindle send; the kindle_send_retrier scheduled
// task picks it up. We accept the request synchronously and return 202.
func (s *Server) handleSendToKindle(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	bookRef := chi.URLParam(r, "id")
	var body struct {
		Format    string `json:"format"`
		ToAddress string `json:"to_address"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if body.Format == "" {
		body.Format = "epub"
	}
	if body.ToAddress == "" || !strings.Contains(body.ToAddress, "@") {
		writeErr(w, 400, "to_address required")
		return
	}
	entry := store.KindleSend{
		ID: ulid.Make().String(), UserID: id.UserID, BookID: bookRef,
		Format: body.Format, ToAddress: body.ToAddress, Status: "queued",
	}
	if err := s.deps.Store.InsertKindleSend(r.Context(), entry); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 202, entry)
}

// handleSendToKobo fetches the EPUB, converts it via kepubify, stores a
// short-lived transfer session, and returns the transfer URL + 4-char code.
func (s *Server) handleSendToKobo(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	bookRef := chi.URLParam(r, "id")
	libraryID, bookID, _ := decodeBookRef(bookRef)
	lib, err := s.targetLibrary(r, libraryID)
	if err != nil {
		writeErr(w, 412, err.Error())
		return
	}
	cfg, err := s.deps.Store.GetConfig(r.Context())
	if err != nil {
		writeErr(w, 412, "no backend configured")
		return
	}
	bk := backend.NewEbookBackend(s.deps.Host, lib.BackendPluginID)
	resp, err := s.deps.Host.GetStream(r.Context(), lib.BackendPluginID, bk.FilePath(bookID, "epub"), nil)
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 302 {
		writeErr(w, 502, fmt.Sprintf("backend %d", resp.StatusCode))
		return
	}
	dir := cfg.CacheDir
	if dir == "" {
		dir = "/tmp"
	}
	tmpEpub := filepath.Join(dir, fmt.Sprintf("kobo-%s.epub", ulid.Make().String()))
	f, err := os.Create(tmpEpub)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_, err = io.Copy(f, resp.Body)
	_ = f.Close()
	if err != nil {
		_ = os.Remove(tmpEpub)
		writeErr(w, 500, err.Error())
		return
	}
	// Convert via kepubify.
	kepubPath := strings.TrimSuffix(tmpEpub, ".epub") + ".kepub.epub"
	kepubify := cfg.KepubifyPath
	if kepubify == "" {
		kepubify = "/usr/local/bin/kepubify"
	}
	cmd := exec.CommandContext(r.Context(), kepubify, "-o", kepubPath, tmpEpub)
	if err := cmd.Run(); err != nil {
		_ = os.Remove(tmpEpub)
		writeErr(w, 500, fmt.Sprintf("kepubify failed: %v", err))
		return
	}
	_ = os.Remove(tmpEpub)
	code, err := randCode(4)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// Bcrypt-hash the URL-supplied code. The plaintext code is shown to the
	// user once (returned in this response) and never persisted; the DB only
	// stores the hash. Cost is the bcrypt default — at serve-file time the
	// reaper-bounded active-session set is small, so the linear scan of
	// CompareHashAndPassword across pending rows is cheap.
	codeHash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
	if err != nil {
		_ = os.Remove(kepubPath)
		writeErr(w, 500, err.Error())
		return
	}
	session := store.KoboSession{
		ID:         ulid.Make().String(),
		UserID:     id.UserID,
		BookID:     bookRef,
		Format:     "kepub",
		CodeHash:   string(codeHash),
		SourcePath: kepubPath,
		Status:     "pending",
		ExpiresAt:  time.Now().Add(30 * time.Minute),
	}
	if err := s.deps.Store.InsertKoboSession(r.Context(), session); err != nil {
		_ = os.Remove(kepubPath)
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"transfer_code": code,
		"transfer_url":  "/kobo/" + code,
		"expires_at":    session.ExpiresAt,
	})
}

func randCode(n int) (string, error) {
	const alphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i, b := range buf {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(out), nil
}
