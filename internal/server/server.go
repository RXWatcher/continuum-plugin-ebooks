// Package server constructs the portal's HTTP handler tree. Mount-points are
// declared in the manifest; each route group calls into a dedicated handler
// package (opds, kosync, kobo, admin, etc.).
package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/auth"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/backend"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/event"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/store"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/streaming"
)

type Deps struct {
	Store        *store.Store
	Host         *backend.HostHTTPClient
	Ev           *event.Publisher
	CacheDir     string
	CacheManager *streaming.Manager
	WebFS        http.FileSystem
}

type Server struct {
	deps Deps
}

func New(d Deps) *Server { return &Server{deps: d} }

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(auth.Middleware)

	// Public (no auth-required, handlers manage their own auth)
	r.Route("/opds", s.mountOPDS)
	r.Route("/kosync", s.mountKosync)
	r.Route("/kobo", s.mountKobo)

	// Authenticated API
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(auth.RequireAuth)
		r.Get("/health", s.handleHealth)
		r.Get("/me", s.handleMe)
		s.mountUserRoutes(r)
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireAdmin)
			s.mountAdminRoutes(r)
		})
	})

	// SPA fallback
	if s.deps.WebFS != nil {
		fileSrv := http.FileServer(s.deps.WebFS)
		r.NotFound(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			f, err := s.deps.WebFS.Open(req.URL.Path)
			if err != nil {
				req.URL.Path = "/"
			} else {
				_ = f.Close()
			}
			fileSrv.ServeHTTP(w, req)
		}).ServeHTTP)
	}
	return r
}
