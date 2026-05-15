package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/backend"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/store"
)

func (s *Server) mountAdminRoutes(r chi.Router) {
	r.Get("/admin/requests", s.handleAdminListRequests)
	r.Patch("/admin/requests/{id}", s.handleAdminPatchRequest)
	r.Post("/admin/requests/bulk", s.handleAdminBulkRequests)
	r.Get("/admin/backend", s.handleAdminGetBackend)
	r.Patch("/admin/backend", s.handleAdminPatchBackend)
	r.Get("/admin/providers/{id}/health", s.handleAdminProviderHealth)
	r.Get("/admin/providers/{id}/test-search", s.handleAdminProviderTestSearch)
	r.Get("/admin/routing-rules", s.handleAdminListRoutingRules)
	r.Put("/admin/routing-rules", s.handleAdminReplaceRoutingRules)
	r.Get("/admin/libraries", s.handleAdminListLibraries)
	r.Put("/admin/libraries", s.handleAdminReplaceLibraries)
	r.Get("/admin/backend-libraries", s.handleAdminBackendLibraries)
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
		var err error
		rows, err = s.deps.Store.ListRequestsByStatus(r.Context(), status, 200)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	} else {
		var err error
		rows, err = s.deps.Store.ListNonTerminal(r.Context(), 200)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	writeJSON(w, 200, map[string]any{"items": rows})
}

func (s *Server) handleAdminProviderTestSearch(w http.ResponseWriter, r *http.Request) {
	providerID := chi.URLParam(r, "id")
	query := r.URL.Query().Get("q")
	if query == "" {
		query = "test"
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	env, err := backend.NewEbookBackend(s.deps.Host, providerID).Search(ctx, query)
	if err != nil {
		writeJSON(w, 200, map[string]any{
			"ok":      false,
			"message": err.Error(),
			"items":   []backend.EbookSummary{},
		})
		return
	}
	if len(env.Items) > 5 {
		env.Items = env.Items[:5]
	}
	writeJSON(w, 200, map[string]any{
		"ok":      true,
		"message": "Search completed",
		"items":   env.Items,
	})
}

func (s *Server) handleAdminPatchRequest(w http.ResponseWriter, r *http.Request) {
	reqID := chi.URLParam(r, "id")
	var body struct {
		Action          string `json:"action"`
		DeniedReason    string `json:"denied_reason"`
		FulfilledBookID string `json:"fulfilled_book_id"`
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
		s.submitRequest(r.Context(), cur)
	case "retry":
		s.submitRequest(r.Context(), cur)
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

func (s *Server) submitRequest(ctx context.Context, cur store.Request) {
	_ = s.deps.Store.UpdateRequestStatus(ctx, cur.ID, "submitted", "", "", "", "")
	if s.deps.Ev != nil {
		s.deps.Ev.Publish(ctx, "request_submitted", map[string]any{
			"request_id":       cur.ID,
			"target_plugin_id": cur.TargetPluginID,
			"title":            cur.Title,
			"authors":          cur.Authors,
			"isbn":             cur.ISBN,
			"source_id":        cur.SourceID,
			"format_pref":      cur.FormatPref,
			"media_type":       cur.MediaType,
			"auto_monitor":     cur.AutoMonitor,
		})
	}
}

func (s *Server) handleAdminBulkRequests(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs          []string `json:"ids"`
		Action       string   `json:"action"`
		DeniedReason string   `json:"denied_reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	updated := 0
	for _, id := range body.IDs {
		cur, err := s.deps.Store.GetRequest(r.Context(), id)
		if err != nil {
			continue
		}
		switch body.Action {
		case "approve", "retry":
			s.submitRequest(r.Context(), cur)
			updated++
		case "deny":
			_ = s.deps.Store.UpdateRequestStatus(r.Context(), id, "denied", "", body.DeniedReason, "", "")
			updated++
		}
	}
	writeJSON(w, 200, map[string]any{"updated": updated})
}

func (s *Server) handleAdminProviderHealth(w http.ResponseWriter, r *http.Request) {
	providerID := chi.URLParam(r, "id")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	caps, err := backend.NewEbookBackend(s.deps.Host, providerID).GetCapabilities(ctx)
	if err != nil {
		writeJSON(w, 200, map[string]any{
			"ok":      false,
			"message": err.Error(),
		})
		return
	}
	writeJSON(w, 200, map[string]any{
		"ok":                       true,
		"message":                  "Provider responded",
		"formats":                  caps.Formats,
		"features":                 caps.Features,
		"max_concurrent_downloads": caps.MaxConcurrentDownloads,
		"supports_range_requests":  caps.SupportsRangeRequests,
	})
}

func (s *Server) handleAdminListRoutingRules(w http.ResponseWriter, r *http.Request) {
	rules, err := s.deps.Store.ListRequestRoutingRules(r.Context(), false)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"items": rules})
}

func (s *Server) handleAdminReplaceRoutingRules(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Items []store.RequestRoutingRule `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := s.deps.Store.ReplaceRequestRoutingRules(r.Context(), body.Items); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleAdminGetBackend(w http.ResponseWriter, r *http.Request) {
	cfg, _ := s.deps.Store.GetConfig(r.Context())
	libs, _ := s.deps.Store.ListPortalLibraries(r.Context(), false)
	writeJSON(w, 200, map[string]any{
		"target_backend_plugin_id":   cfg.TargetBackendPluginID,
		"auto_approve_requests":      cfg.AutoApproveRequests,
		"default_streaming_mode":     cfg.DefaultStreamingMode,
		"cache_dir":                  cfg.CacheDir,
		"cache_max_size_gb":          cfg.CacheMaxSizeGB,
		"cache_download_concurrency": cfg.CacheDownloadConcurrency,
		"opds_realm":                 cfg.OpdsRealm,
		"kepubify_path":              cfg.KepubifyPath,
		"libraries":                  libs,
	})
}

func (s *Server) handleAdminPatchBackend(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TargetBackendPluginID    *string `json:"target_backend_plugin_id"`
		AutoApproveRequests      *bool   `json:"auto_approve_requests"`
		DefaultStreamingMode     *string `json:"default_streaming_mode"`
		CacheMaxSizeGB           *int    `json:"cache_max_size_gb"`
		CacheDownloadConcurrency *int    `json:"cache_download_concurrency"`
		OpdsRealm                *string `json:"opds_realm"`
		KepubifyPath             *string `json:"kepubify_path"`
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
	if body.CacheDownloadConcurrency != nil {
		cur.CacheDownloadConcurrency = *body.CacheDownloadConcurrency
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

func (s *Server) handleAdminListLibraries(w http.ResponseWriter, r *http.Request) {
	libs, err := s.deps.Store.ListPortalLibraries(r.Context(), false)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"items": libs})
}

func (s *Server) handleAdminReplaceLibraries(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Items []store.PortalLibrary `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := s.deps.Store.ReplacePortalLibraries(r.Context(), body.Items); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleAdminBackendLibraries(w http.ResponseWriter, r *http.Request) {
	backendID := r.URL.Query().Get("backend_plugin_id")
	if backendID == "" {
		writeJSON(w, 200, map[string]any{"items": []backend.LibraryInfo{}})
		return
	}
	items, err := backend.NewEbookBackend(s.deps.Host, backendID).ListLibraries(r.Context())
	if err != nil {
		writeJSON(w, 200, map[string]any{"items": []backend.LibraryInfo{}})
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (s *Server) handleAdminCacheStats(w http.ResponseWriter, r *http.Request) {
	total, _ := s.deps.Store.TotalCacheBytes(r.Context())
	cfg, _ := s.deps.Store.GetConfig(r.Context())
	writeJSON(w, 200, map[string]any{
		"bytes_used": total,
		"bytes_max":  int64(cfg.CacheMaxSizeGB) * 1024 * 1024 * 1024,
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
