package server

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/RXWatcher/silo-plugin-ebooks/internal/dictionary"
)

// Dictionary lookup proxy. The reader's selection menu posts here
// when the user hits "Define"; the response is a list of
// {part_of_speech, definition, example} entries the popover
// renders.
//
// All authenticated users may look up — no per-user gating since
// the data is public and the proxy doesn't add cost.

func (s *Server) mountDictionaryRoutes(r chi.Router) {
	r.Get("/dictionary/lookup", s.handleDictionaryLookup)
}

func (s *Server) handleDictionaryLookup(w http.ResponseWriter, r *http.Request) {
	word := strings.TrimSpace(r.URL.Query().Get("word"))
	lang := strings.TrimSpace(r.URL.Query().Get("lang"))
	if word == "" {
		http.Error(w, "word required", http.StatusBadRequest)
		return
	}
	out, err := dictionary.Lookup(r.Context(), word, lang)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
