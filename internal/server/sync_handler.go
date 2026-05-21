package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/auth"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/hlc"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/store"
)

// chi is imported for the route mount even though no handler uses
// chi.URLParam in this file — keeps mountSyncRoutes consistent
// with the rest of the package.
var _ = chi.NewRouter

// Replica sync surface for annotations. Foundation for cross-device
// sync — clients pull changes since their last cursor and push
// batches of local changes back. Row-level LWW: a newer HLC wins
// over an older one when both reference the same annotation_id.
//
// Field-level LWW (each annotation field has its own HLC) is a
// follow-up; the current contract treats the annotation row as the
// merge unit which is correct for the common case (one user edits
// a highlight on phone, the desktop pulls and overwrites cleanly).

func (s *Server) mountSyncRoutes(r chi.Router) {
	r.Get("/me/sync/annotations", s.handlePullAnnotationChanges)
	r.Post("/me/sync/annotations", s.handlePushAnnotationChanges)
}

// handlePullAnnotationChanges — GET /me/sync/annotations?since=<hlc>&limit=N
// Returns up to `limit` changes for the calling user with hlc >
// since. The empty since returns the whole history (subject to
// limit). Response shape: {changes: [...], next_cursor: hlc}.
//
// next_cursor is the hlc of the last row in this page; the client
// passes it back as `since` on the next request to resume.
func (s *Server) handlePullAnnotationChanges(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	since := r.URL.Query().Get("since")
	limit := 500
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	changes, err := s.deps.Store.PullAnnotationChanges(r.Context(), id.UserID, since, limit)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	next := since
	if len(changes) > 0 {
		next = changes[len(changes)-1].HLC
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"changes":     changes,
		"next_cursor": next,
	})
}

type pushChangeBody struct {
	AnnotationID string          `json:"annotation_id"`
	Op           string          `json:"op"`
	Payload      json.RawMessage `json:"payload"`
	// hlc is optional — when empty the server mints one (the
	// typical case for SPA pushes). Clients that have their own
	// HLC clock (mobile app holding the line across an offline
	// gap) supply their pre-stamped value.
	HLC string `json:"hlc"`
}

// handlePushAnnotationChanges — POST /me/sync/annotations
// Body: {changes: [{annotation_id, op, payload, hlc?}, ...]}
//
// Each change appends to the log; client-supplied hlc is preferred
// when present (preserves cross-replica ordering); when absent the
// server mints one and Observe()s the local clock against any
// peer timestamps the client also referenced.
//
// Response: {applied: N, cursor: hlc} — cursor is the HLC after
// the push that the client can use for its next pull.
func (s *Server) handlePushAnnotationChanges(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	var body struct {
		Changes []pushChangeBody `json:"changes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	applied := 0
	var lastHLC string
	for _, ch := range body.Changes {
		if ch.AnnotationID == "" || ch.Op == "" {
			continue
		}
		var stamp hlc.Timestamp
		if ch.HLC != "" {
			parsed, err := hlc.Parse(ch.HLC)
			if err != nil {
				continue
			}
			stamp = parsed
			s.syncClock().Observe(stamp)
		} else {
			stamp = s.syncClock().Now()
		}
		if err := s.deps.Store.AppendAnnotationChange(r.Context(), store.AnnotationChange{
			HLC:          stamp.String(),
			UserID:       id.UserID,
			AnnotationID: ch.AnnotationID,
			Op:           ch.Op,
			Payload:      ch.Payload,
			OriginNode:   stamp.NodeID,
		}); err != nil {
			writeInternal(w, r, err)
			return
		}
		applied++
		lastHLC = stamp.String()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"applied": applied,
		"cursor":  lastHLC,
	})
}

// syncClock returns the process-shared HLC clock. The plugin owns
// one clock per install — keyed by InstallID — so every change
// originating on this replica carries the same node-id. Lazily
// constructed on first use so tests that don't touch sync don't
// pay the construction cost.
func (s *Server) syncClock() *hlc.Clock {
	s.syncClockOnce.Do(func() {
		// nodeID is stable across the process lifetime; in the
		// future an env var or backend_config column can override
		// for multi-replica deployments. For single-replica installs
		// "ebook-replica" is fine — clients only need uniqueness
		// vs their OWN device's clock.
		s.clockCached = hlc.New("ebook-replica")
	})
	return s.clockCached
}
