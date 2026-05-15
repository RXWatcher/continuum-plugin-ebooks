// Package server constructs the portal's HTTP handler tree. Mount-points are
// declared in the manifest; each route group calls into a dedicated handler
// package (opds, kosync, kobo, admin, etc.).
package server

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/auth"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/backend"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/event"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/koboref"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/store"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/streaming"
)

type Deps struct {
	Store        *store.Store
	Host         *backend.HostHTTPClient
	Ev           *event.Publisher
	CacheDir     string
	CacheManager *streaming.Manager
	// KoboRefs is the shared registry used by handleKoboServeFile and the
	// KoboSessionReaper to coordinate read/delete races on session source
	// files. If nil, the serve path still works but evictions are unguarded.
	KoboRefs *koboref.Registry
	WebFS    http.FileSystem
}

type Server struct {
	deps        Deps
	publicLimit *ipLimiter
}

func New(d Deps) *Server {
	return &Server{
		deps:        d,
		publicLimit: newIPLimiter(rateLimitRPS, rateLimitBurst),
	}
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(auth.Middleware)

	// Public (no auth-required, handlers manage their own auth). Brute-force
	// surface — apply a per-IP rate limit so password-spray against /kosync
	// auth or OPDS basic-auth can't burn unlimited CPU.
	publicRL := rateLimitMiddleware(s.publicLimit)
	r.Route("/opds", func(r chi.Router) { r.Use(publicRL); s.mountOPDS(r) })
	r.Route("/kosync", func(r chi.Router) { r.Use(publicRL); s.mountKosync(r) })
	r.Route("/kobo", func(r chi.Router) { r.Use(publicRL); s.mountKobo(r) })

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
			if strings.HasPrefix(req.URL.Path, "/admin/assets/") {
				req.URL.Path = req.URL.Path[len("/admin"):]
			}
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
