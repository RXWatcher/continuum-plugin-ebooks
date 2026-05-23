package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"

	"github.com/RXWatcher/silo-plugin-ebooks/internal/auth"
	"github.com/RXWatcher/silo-plugin-ebooks/internal/backend"
	"github.com/RXWatcher/silo-plugin-ebooks/internal/smartcoll"
	"github.com/RXWatcher/silo-plugin-ebooks/internal/store"
)

// Smart Collection HTTP surface for the ebooks plugin. Ports the
// audiobooks plugin's smart_collection_handler.go to the ebook
// server's chi router + auth context conventions. Manual collections
// (the /me/collections* surface) stay untouched.

type smartCollectionBody struct {
	Name        string                    `json:"name"`
	Description string                    `json:"description"`
	Color       string                    `json:"color"`
	IsPublic    bool                      `json:"is_public"`
	IsPinned    bool                      `json:"is_pinned"`
	QueryDef    smartcoll.QueryDefinition `json:"query_def"`
}

func (s *Server) mountSmartCollectionRoutes(r chi.Router) {
	r.Get("/me/smart-collections", s.handleListSmartCollections)
	r.Post("/me/smart-collections", s.handleCreateSmartCollection)
	r.Get("/me/smart-collections/{id}", s.handleGetSmartCollection)
	r.Get("/me/smart-collections/{id}/items", s.handleSmartCollectionItems)
	r.Patch("/me/smart-collections/{id}", s.handleUpdateSmartCollection)
	r.Delete("/me/smart-collections/{id}", s.handleDeleteSmartCollection)
}

func (s *Server) handleListSmartCollections(w http.ResponseWriter, r *http.Request) {
	ident, _ := auth.FromContext(r.Context())
	userID := ident.UserID
	profileID := ident.ProfileID
	rows, err := s.deps.Store.ListSmartCollections(r.Context(), userID, profileID, 200)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, c := range rows {
		out = append(out, smartCollectionToMap(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) handleGetSmartCollection(w http.ResponseWriter, r *http.Request) {
	ident, _ := auth.FromContext(r.Context())
	userID := ident.UserID
	profileID := ident.ProfileID
	c, err := s.deps.Store.GetSmartCollection(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "smart collection not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !c.IsPublic && (c.UserID != userID || c.ProfileID != profileID) {
		http.Error(w, "not visible to this user", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, smartCollectionToMap(c))
}

func (s *Server) handleCreateSmartCollection(w http.ResponseWriter, r *http.Request) {
	c, err := s.persistSmartCollection(w, r, "")
	if err != nil {
		return
	}
	writeJSON(w, http.StatusCreated, smartCollectionToMap(c))
}

func (s *Server) handleUpdateSmartCollection(w http.ResponseWriter, r *http.Request) {
	ident, _ := auth.FromContext(r.Context())
	userID := ident.UserID
	profileID := ident.ProfileID
	existing, err := s.deps.Store.GetSmartCollection(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "smart collection not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if existing.UserID != userID || existing.ProfileID != profileID {
		http.Error(w, "not owned by this user", http.StatusForbidden)
		return
	}
	c, err := s.persistSmartCollection(w, r, existing.ID)
	if err != nil {
		return
	}
	writeJSON(w, http.StatusOK, smartCollectionToMap(c))
}

func (s *Server) handleDeleteSmartCollection(w http.ResponseWriter, r *http.Request) {
	ident, _ := auth.FromContext(r.Context())
	userID := ident.UserID
	profileID := ident.ProfileID
	if err := s.deps.Store.DeleteSmartCollection(r.Context(), chi.URLParam(r, "id"), userID, profileID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleSmartCollectionItems evaluates the collection's rules against
// the configured backend's ebook catalog. Returns a paged envelope
// matching the rest of the /ebooks surface.
//
// Limits: ?limit/?page override the collection's own Limit when
// supplied (so the SPA can browse without recreating the collection).
// We over-fetch 5000 books from the backend per library and evaluate
// locally; the same approach as the audiobooks plugin.
func (s *Server) handleSmartCollectionItems(w http.ResponseWriter, r *http.Request) {
	ident, _ := auth.FromContext(r.Context())
	userID := ident.UserID
	profileID := ident.ProfileID
	c, err := s.deps.Store.GetSmartCollection(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "smart collection not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !c.IsPublic && (c.UserID != userID || c.ProfileID != profileID) {
		http.Error(w, "not visible to this user", http.StatusNotFound)
		return
	}

	var qd smartcoll.QueryDefinition
	if err := json.Unmarshal(c.QueryDef, &qd); err != nil {
		http.Error(w, "invalid query_def: "+err.Error(), http.StatusInternalServerError)
		return
	}

	limit := 30
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	} else if qd.Limit != nil && *qd.Limit > 0 {
		limit = *qd.Limit
	}
	page := 0
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			page = n
		}
	}

	// Resolve target libraries — same scheme as audiobooks: empty
	// library_ids → all visible portal libraries.
	allLibs, err := s.deps.Store.ListPortalLibraries(r.Context(), true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	libByID := make(map[int64]store.PortalLibrary, len(allLibs))
	for _, lib := range allLibs {
		libByID[lib.ID] = lib
	}
	var targetLibs []store.PortalLibrary
	if len(qd.LibraryIDs) > 0 {
		for _, id := range qd.LibraryIDs {
			if lib, ok := libByID[id]; ok {
				targetLibs = append(targetLibs, lib)
			}
		}
	} else {
		targetLibs = allLibs
	}

	bk, err := s.targetBackend(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	candidates := make([]smartcoll.Candidate, 0, 1024)
	for _, lib := range targetLibs {
		out, err := bk.ListCatalog(r.Context(), backend.CatalogQuery{
			Limit:     5000,
			LibraryID: portalLibraryBackendID(lib),
		})
		if err != nil {
			continue
		}
		for _, e := range out.Items {
			candidates = append(candidates, smartcoll.Candidate{Item: e})
		}
	}

	matched := smartcoll.Evaluate(r.Context(), qd, candidates, smartcoll.EvaluateOptions{
		AllowPersonalized: c.UserID == userID && c.ProfileID == profileID,
		UserSeed:          userID + ":" + c.ID,
		Now:               time.Now(),
	})

	total := len(matched)
	start := page * limit
	if start > len(matched) {
		start = len(matched)
	}
	end := start + limit
	if end > len(matched) {
		end = len(matched)
	}
	results := make([]any, 0, end-start)
	for _, cand := range matched[start:end] {
		results = append(results, cand.Item)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":    results,
		"total":    total,
		"limit":    limit,
		"page":     page,
		"sortBy":   qd.Sort.Field,
		"sortDesc": qd.Sort.Order == "desc",
	})
}

func (s *Server) persistSmartCollection(w http.ResponseWriter, r *http.Request, existingID string) (store.SmartCollection, error) {
	ident, _ := auth.FromContext(r.Context())
	userID := ident.UserID
	profileID := ident.ProfileID
	var body smartCollectionBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return store.SmartCollection{}, err
	}
	if body.Name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return store.SmartCollection{}, errors.New("name required")
	}
	normalized := body.QueryDef.Normalize()
	if err := normalized.Validate(true); err != nil {
		http.Error(w, "invalid query_def: "+err.Error(), http.StatusBadRequest)
		return store.SmartCollection{}, err
	}
	defJSON, err := json.Marshal(normalized)
	if err != nil {
		http.Error(w, "encode query_def: "+err.Error(), http.StatusInternalServerError)
		return store.SmartCollection{}, err
	}
	id := existingID
	if id == "" {
		id = ulid.Make().String()
	}
	c := store.SmartCollection{
		ID: id, UserID: userID, ProfileID: profileID, Name: body.Name, Description: body.Description,
		Color: body.Color, IsPublic: body.IsPublic, IsPinned: body.IsPinned,
		QueryDef: defJSON,
	}
	if err := s.deps.Store.UpsertSmartCollection(r.Context(), c); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return store.SmartCollection{}, err
	}
	persisted, err := s.deps.Store.GetSmartCollection(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return store.SmartCollection{}, err
	}
	return persisted, nil
}

func smartCollectionToMap(c store.SmartCollection) map[string]any {
	var qd any
	_ = json.Unmarshal(c.QueryDef, &qd)
	return map[string]any{
		"id":          c.ID,
		"userId":      c.UserID,
		"name":        c.Name,
		"description": c.Description,
		"color":       c.Color,
		"isPublic":    c.IsPublic,
		"isPinned":    c.IsPinned,
		"queryDef":    qd,
		"createdAt":   c.CreatedAt.UnixMilli(),
		"updatedAt":   c.UpdatedAt.UnixMilli(),
	}
}

// portalLibraryBackendID is the ebook plugin's analogue of the
// audiobooks helper of the same name — returns the BackendLibraryID
// when present, 0 otherwise.
func portalLibraryBackendID(lib store.PortalLibrary) int64 {
	if lib.BackendLibraryID == nil {
		return 0
	}
	return *lib.BackendLibraryID
}
