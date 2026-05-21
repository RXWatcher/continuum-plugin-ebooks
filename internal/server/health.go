package server

import (
	"encoding/json"
	"net/http"

	"github.com/RXWatcher/continuum-plugin-ebooks/internal/auth"
)

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"user_id":  id.UserID,
		"username": id.Username,
		"email":    id.Email,
		"is_admin": id.IsAdmin,
	})
}
