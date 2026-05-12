package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/store"
)

func (s *Server) mountAdminRoutes(r chi.Router) {
	r.Get("/admin/requests", s.handleAdminListRequests)
	r.Patch("/admin/requests/{id}", s.handleAdminPatchRequest)
	r.Get("/admin/backend", s.handleAdminGetBackend)
	r.Patch("/admin/backend", s.handleAdminPatchBackend)
	r.Get("/admin/cache", s.handleAdminCacheStats)
	r.Get("/admin/cache/largest", s.handleAdminCacheLargest)
	r.Get("/admin/kobo-sessions", s.handleAdminKoboSessions)
	r.Get("/admin/opds-tokens", s.handleAdminOPDSTokens)
	r.Delete("/admin/opds-tokens/{id}", s.handleAdminRevokeOPDSToken)
	r.Get("/admin/kosync-users", s.handleAdminKosyncUsers)
	r.Delete("/admin/kosync-users/{username}", s.handleAdminDeleteKosync)
	r.Get("/admin/kindle-log", s.handleAdminKindleLog)
}

func (s *Server) handleAdminListRequests(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	var rows []store.Request
	if status != "" {
		rows, _ = s.deps.Store.ListRequestsByStatus(r.Context(), status, 200)
	} else {
		rows, _ = s.deps.Store.ListNonTerminal(r.Context(), 200)
	}
	writeJSON(w, 200, map[string]any{"items": rows})
}

func (s *Server) handleAdminPatchRequest(w http.ResponseWriter, r *http.Request) {
	reqID := chi.URLParam(r, "id")
	var body struct {
		Action           string `json:"action"`
		DeniedReason     string `json:"denied_reason"`
		FulfilledBookID  string `json:"fulfilled_book_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	cur, err := s.deps.Store.GetRequest(r.Context(), reqID)
	if err != nil {
		writeErr(w, 404, "not found")
		return
	}
	switch body.Action {
	case "approve":
		_ = s.deps.Store.UpdateRequestStatus(r.Context(), reqID, "submitted", "", "", "", "")
		if s.deps.Ev != nil {
			s.deps.Ev.Publish(r.Context(), "request_submitted", map[string]any{
				"request_id":       cur.ID,
				"target_plugin_id": cur.TargetPluginID,
				"title":            cur.Title,
				"authors":          cur.Authors,
				"isbn":             cur.ISBN,
				"source_id":        cur.SourceID,
				"format_pref":      cur.FormatPref,
				"auto_monitor":     cur.AutoMonitor,
			})
		}
	case "deny":
		_ = s.deps.Store.UpdateRequestStatus(r.Context(), reqID, "denied", "", body.DeniedReason, "", "")
	case "fulfill_manual":
		_ = s.deps.Store.MarkFulfilled(r.Context(), reqID, body.FulfilledBookID)
	default:
		writeErr(w, 400, "unknown action")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleAdminGetBackend(w http.ResponseWriter, r *http.Request) {
	cfg, _ := s.deps.Store.GetConfig(r.Context())
	writeJSON(w, 200, map[string]any{
		"target_backend_plugin_id":  cfg.TargetBackendPluginID,
		"auto_approve_requests":     cfg.AutoApproveRequests,
		"default_streaming_mode":    cfg.DefaultStreamingMode,
		"cache_dir":                 cfg.CacheDir,
		"cache_max_size_gb":         cfg.CacheMaxSizeGB,
		"cache_download_concurrency": cfg.CacheDownloadConcurrency,
		"opds_realm":                cfg.OpdsRealm,
		"kepubify_path":             cfg.KepubifyPath,
	})
}

func (s *Server) handleAdminPatchBackend(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TargetBackendPluginID *string `json:"target_backend_plugin_id"`
		AutoApproveRequests   *bool   `json:"auto_approve_requests"`
		DefaultStreamingMode  *string `json:"default_streaming_mode"`
		CacheMaxSizeGB        *int    `json:"cache_max_size_gb"`
		OpdsRealm             *string `json:"opds_realm"`
		KepubifyPath          *string `json:"kepubify_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	cur, _ := s.deps.Store.GetConfig(r.Context())
	if body.TargetBackendPluginID != nil {
		cur.TargetBackendPluginID = *body.TargetBackendPluginID
	}
	if body.AutoApproveRequests != nil {
		cur.AutoApproveRequests = *body.AutoApproveRequests
	}
	if body.DefaultStreamingMode != nil {
		cur.DefaultStreamingMode = *body.DefaultStreamingMode
	}
	if body.CacheMaxSizeGB != nil {
		cur.CacheMaxSizeGB = *body.CacheMaxSizeGB
	}
	if body.OpdsRealm != nil {
		cur.OpdsRealm = *body.OpdsRealm
	}
	if body.KepubifyPath != nil {
		cur.KepubifyPath = *body.KepubifyPath
	}
	if err := s.deps.Store.UpsertConfig(r.Context(), cur); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleAdminCacheStats(w http.ResponseWriter, r *http.Request) {
	total, _ := s.deps.Store.TotalCacheBytes(r.Context())
	cfg, _ := s.deps.Store.GetConfig(r.Context())
	writeJSON(w, 200, map[string]any{
		"bytes_used":  total,
		"bytes_max":   int64(cfg.CacheMaxSizeGB) * 1024 * 1024 * 1024,
	})
}

func (s *Server) handleAdminCacheLargest(w http.ResponseWriter, r *http.Request) {
	rows, _ := s.deps.Store.ListCacheLargest(r.Context(), 50)
	writeJSON(w, 200, map[string]any{"items": rows})
}

func (s *Server) handleAdminKoboSessions(w http.ResponseWriter, r *http.Request) {
	rows, _ := s.deps.Store.ListAllKoboSessions(r.Context(), 100)
	writeJSON(w, 200, map[string]any{"items": rows})
}

func (s *Server) handleAdminOPDSTokens(w http.ResponseWriter, r *http.Request) {
	rows, _ := s.deps.Store.ListAllOPDSTokens(r.Context())
	writeJSON(w, 200, map[string]any{"items": rows})
}

func (s *Server) handleAdminRevokeOPDSToken(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.deps.Store.AdminRevokeOPDSToken(r.Context(), id); err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	w.WriteHeader(204)
}

func (s *Server) handleAdminKosyncUsers(w http.ResponseWriter, r *http.Request) {
	rows, _ := s.deps.Store.ListKosyncUsers(r.Context())
	writeJSON(w, 200, map[string]any{"items": rows})
}

func (s *Server) handleAdminDeleteKosync(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	if err := s.deps.Store.DeleteKosyncUser(r.Context(), username); err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	w.WriteHeader(204)
}

func (s *Server) handleAdminKindleLog(w http.ResponseWriter, r *http.Request) {
	rows, _ := s.deps.Store.ListAllKindleSends(r.Context(), 200)
	writeJSON(w, 200, map[string]any{"items": rows})
}
