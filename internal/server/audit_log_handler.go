package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/auth"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/store"
)

// Admin audit log query surface for the ebooks plugin. Same shape
// as the audiobooks plugin's audit log.

func (s *Server) mountAuditLogRoutes(r chi.Router) {
	r.Get("/admin/audit-log", s.handleListAuditEntries)
}

func (s *Server) handleListAuditEntries(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	if !id.IsAdmin {
		http.Error(w, "admin required", http.StatusForbidden)
		return
	}
	f := store.AuditFilters{
		ActorID:    strings.TrimSpace(r.URL.Query().Get("actor_id")),
		Action:     strings.TrimSpace(r.URL.Query().Get("action")),
		EntityType: strings.TrimSpace(r.URL.Query().Get("entity_type")),
		EntityID:   strings.TrimSpace(r.URL.Query().Get("entity_id")),
	}
	if v := r.URL.Query().Get("since"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			f.SinceMs = n
		}
	}
	if v := r.URL.Query().Get("until"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			f.UntilMs = n
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Limit = n
		}
	}
	rows, err := s.deps.Store.ListAuditEntries(r.Context(), f)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

// audit is the best-effort helper admin handlers call to record an
// action. Failures never block the request; we log and swallow.
func (s *Server) audit(r *http.Request, actorID, action, entityType, entityID string, payload any) {
	if s.deps.Store == nil || actorID == "" {
		return
	}
	var payloadJSON json.RawMessage
	if payload != nil {
		if b, err := json.Marshal(payload); err == nil {
			payloadJSON = b
		}
	}
	if len(payloadJSON) == 0 {
		payloadJSON = json.RawMessage("{}")
	}
	entry := store.AuditLogEntry{
		ID:         ulid.Make().String(),
		ActorID:    actorID,
		Action:     action,
		EntityType: entityType,
		EntityID:   entityID,
		IP:         auditClientIP(r),
		UserAgent:  r.UserAgent(),
		Payload:    payloadJSON,
	}
	if err := s.deps.Store.AppendAuditEntry(r.Context(), entry); err != nil {
		slog.Warn("audit append failed", "action", action, "err", err.Error())
	}
}

func auditClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if addr := r.RemoteAddr; addr != "" {
		if i := strings.LastIndex(addr, ":"); i > 0 {
			return addr[:i]
		}
		return addr
	}
	return ""
}
