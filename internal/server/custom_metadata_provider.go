package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"

	"github.com/RXWatcher/silo-plugin-ebooks/internal/auth"
	"github.com/RXWatcher/silo-plugin-ebooks/internal/store"
)

// Custom metadata provider surface for the ebooks plugin. Mirrors
// the audiobooks plugin: admin CRUD + user-facing proxied search
// against an external provider that implements the upstream
// custom-metadata-provider spec.

func (s *Server) mountCustomMetadataProviderRoutes(r chi.Router) {
	r.Get("/admin/custom-metadata-providers", s.handleListProviders)
	r.Post("/admin/custom-metadata-providers", s.handleCreateProvider)
	r.Patch("/admin/custom-metadata-providers/{id}", s.handleUpdateProvider)
	r.Delete("/admin/custom-metadata-providers/{id}", s.handleDeleteProvider)
	r.Get("/search/providers", s.handleListProvidersPublic)
	r.Get("/search/providers/{id}", s.handleProviderSearch)
}

type providerBody struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	AuthHeader string `json:"auth_header"`
	Enabled    bool   `json:"enabled"`
}

func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	if !id.IsAdmin {
		http.Error(w, "admin required", http.StatusForbidden)
		return
	}
	rows, err := s.deps.Store.ListCustomMetadataProviders(r.Context(), false)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (s *Server) handleCreateProvider(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	if !id.IsAdmin {
		http.Error(w, "admin required", http.StatusForbidden)
		return
	}
	var body providerBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.Name == "" || body.URL == "" {
		http.Error(w, "name and url required", http.StatusBadRequest)
		return
	}
	p := store.CustomMetadataProvider{
		ID:         ulid.Make().String(),
		Name:       body.Name,
		URL:        body.URL,
		AuthHeader: body.AuthHeader,
		Enabled:    body.Enabled,
	}
	if err := s.deps.Store.UpsertCustomMetadataProvider(r.Context(), p); err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handleUpdateProvider(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	if !id.IsAdmin {
		http.Error(w, "admin required", http.StatusForbidden)
		return
	}
	pid := chi.URLParam(r, "id")
	existing, err := s.deps.Store.GetCustomMetadataProvider(r.Context(), pid)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	var body providerBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.Name != "" {
		existing.Name = body.Name
	}
	if body.URL != "" {
		existing.URL = body.URL
	}
	existing.AuthHeader = body.AuthHeader
	existing.Enabled = body.Enabled
	if err := s.deps.Store.UpsertCustomMetadataProvider(r.Context(), existing); err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (s *Server) handleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	if !id.IsAdmin {
		http.Error(w, "admin required", http.StatusForbidden)
		return
	}
	if err := s.deps.Store.DeleteCustomMetadataProvider(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeInternal(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListProvidersPublic(w http.ResponseWriter, r *http.Request) {
	rows, err := s.deps.Store.ListCustomMetadataProviders(r.Context(), true)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, p := range rows {
		out = append(out, map[string]any{"id": p.ID, "name": p.Name})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) handleProviderSearch(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, err := s.deps.Store.GetCustomMetadataProvider(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) || !p.Enabled {
		http.Error(w, "provider not available", http.StatusNotFound)
		return
	}
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("query"))
	if query == "" {
		http.Error(w, "query required", http.StatusBadRequest)
		return
	}
	target, err := url.Parse(p.URL)
	if err != nil {
		http.Error(w, "invalid provider url: "+err.Error(), http.StatusBadGateway)
		return
	}
	if !strings.HasSuffix(target.Path, "/search") {
		target.Path = strings.TrimRight(target.Path, "/") + "/search"
	}
	q := target.Query()
	q.Set("query", query)
	if author := r.URL.Query().Get("author"); author != "" {
		q.Set("author", author)
	}
	target.RawQuery = q.Encode()

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	if p.AuthHeader != "" {
		req.Header.Set("Authorization", p.AuthHeader)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, "provider request failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, io.LimitReader(resp.Body, 10<<20))
}
