package server

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
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
	toAddr, err := validateKindleAddress(body.ToAddress)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	entry := store.KindleSend{
		ID: ulid.Make().String(), UserID: id.UserID, BookID: bookRef,
		Format: body.Format, ToAddress: toAddr, Status: "queued",
	}
	if err := s.deps.Store.InsertKindleSend(r.Context(), entry); err != nil {
		writeInternal(w, r, err)
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
		writeBadGateway(w, r, err)
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
		writeInternal(w, r, err)
		return
	}
	_, err = io.Copy(f, resp.Body)
	_ = f.Close()
	if err != nil {
		_ = os.Remove(tmpEpub)
		writeInternal(w, r, err)
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
	// 10 symbols over a 31-char alphabet ≈ 8e14 keyspace: the public
	// /kobo/{code} endpoint is brute-forceable, and the old 4-char code
	// (~9e5) was trivially enumerable within the 30-minute session window.
	code, err := randCode(10)
	if err != nil {
		_ = os.Remove(kepubPath) // converted file is orphaned otherwise (no DB row → reaper never sees it)
		writeInternal(w, r, err)
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
		writeInternal(w, r, err)
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
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{
		"transfer_code": code,
		"transfer_url":  "/kobo/" + code,
		"expires_at":    session.ExpiresAt,
	})
}

// kindleDomains is the allowlist of Amazon Send-to-Kindle ingestion domains.
// Restricting recipients here turns the endpoint from an authenticated
// open mail/attachment relay into a Kindle-only delivery path.
var kindleDomains = map[string]bool{
	"kindle.com":      true,
	"kindle.cn":       true,
	"free.kindle.com": true,
}

// validateKindleAddress parses raw, rejects anything that isn't a single
// bare address at an Amazon Kindle domain, and rejects CR/LF (header
// injection into the SMTP "To" header). Returns the canonical address.
func validateKindleAddress(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("to_address required")
	}
	if strings.ContainsAny(raw, "\r\n") {
		return "", errors.New("invalid to_address")
	}
	addr, err := mail.ParseAddress(raw)
	if err != nil {
		return "", errors.New("invalid to_address")
	}
	// Reject display-name form ("Name <a@b>") and anything ParseAddress
	// normalized away — we want exactly the bare address the user typed.
	if addr.Name != "" || addr.Address != raw {
		return "", errors.New("invalid to_address")
	}
	at := strings.LastIndex(addr.Address, "@")
	if at < 0 {
		return "", errors.New("invalid to_address")
	}
	if !kindleDomains[strings.ToLower(addr.Address[at+1:])] {
		return "", errors.New("to_address must be an @kindle.com address")
	}
	return addr.Address, nil
}

func randCode(n int) (string, error) {
	const alphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789" // 31, no ambiguous 0/O/1/I/L
	// Rejection sampling: bytes >= the largest multiple of len(alphabet) that
	// fits in a byte are discarded so every symbol is equiprobable (the old
	// b % len(alphabet) skewed toward the first 256%31 symbols).
	const limit = 256 - (256 % len(alphabet))
	out := make([]byte, 0, n)
	buf := make([]byte, n)
	for len(out) < n {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		for _, b := range buf {
			if int(b) >= limit {
				continue
			}
			out = append(out, alphabet[int(b)%len(alphabet)])
			if len(out) == n {
				break
			}
		}
	}
	return string(out), nil
}
