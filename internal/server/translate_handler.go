package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/translate"
)

// In-text translation proxy. POST a {text, source?, target} body
// and receive the translated text. Backed by a LibreTranslate-
// compatible upstream configured via env (LIBRETRANSLATE_URL +
// LIBRETRANSLATE_API_KEY).
//
// Operator runs their own LibreTranslate instance or points at a
// hosted one; the plugin doesn't bundle a translation model. We
// proxy rather than direct-from-client so the API key never
// reaches the SPA.

func (s *Server) mountTranslateRoutes(r chi.Router) {
	r.Post("/translate", s.handleTranslate)
}

type translateBody struct {
	Text   string `json:"text"`
	Source string `json:"source"`
	Target string `json:"target"`
}

func (s *Server) handleTranslate(w http.ResponseWriter, r *http.Request) {
	var body translateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.Text == "" || body.Target == "" {
		http.Error(w, "text and target required", http.StatusBadRequest)
		return
	}
	if len(body.Text) > 5000 {
		http.Error(w, "text exceeds 5000 char limit", http.StatusRequestEntityTooLarge)
		return
	}
	cfg := translate.LoadFromEnv()
	if !cfg.Configured() {
		http.Error(w, "translation not configured", http.StatusServiceUnavailable)
		return
	}
	res, err := translate.Translate(r.Context(), cfg, body.Text, body.Source, body.Target)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// _ = chi.URLParam reserved for future routes (per-book translate
// preferences, etc.).
var _ = chi.URLParam
