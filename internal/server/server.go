// Package server constructs the portal's HTTP handler tree. Mount-points are
// declared in the manifest; each route group calls into a dedicated handler
// package (opds, kosync, kobo, admin, etc.).
package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/RXWatcher/continuum-plugin-ebooks/internal/auth"
	"github.com/RXWatcher/continuum-plugin-ebooks/internal/backend"
	"github.com/RXWatcher/continuum-plugin-ebooks/internal/event"
	"github.com/RXWatcher/continuum-plugin-ebooks/internal/koboref"
	"github.com/RXWatcher/continuum-plugin-ebooks/internal/store"
	"github.com/RXWatcher/continuum-plugin-ebooks/internal/streaming"
)

var pwaContentTypes = map[string]string{
	"/manifest.webmanifest": "application/manifest+json; charset=utf-8",
	"/sw.js":                "application/javascript; charset=utf-8",
	"/icon.svg":             "image/svg+xml",
	"/icon-192.png":         "image/png",
	"/icon-512.png":         "image/png",
	"/apple-touch-icon.png": "image/png",
}

type Deps struct {
	Store        *store.Store
	Host         *backend.HostHTTPClient
	Ev           EventPublisher
	CacheDir     string
	CacheManager *streaming.Manager
	// KoboRefs is the shared registry used by handleKoboServeFile and the
	// KoboSessionReaper to coordinate read/delete races on session source
	// files. If nil, the serve path still works but evictions are unguarded.
	KoboRefs *koboref.Registry
	WebFS    http.FileSystem
	// Recommender powers GET /me/books/{id}/similar. nil means
	// embeddings aren't configured and the route returns empty
	// results (200 with items=[]).
	Recommender Recommender
	Credentials CredentialValidator
}

type EventPublisher interface {
	Publish(ctx context.Context, name string, payload map[string]any)
}

type TargetedEventPublisher interface {
	EventPublisher
	PublishTo(ctx context.Context, targetPluginID, name string, payload map[string]any)
}

var _ EventPublisher = (*event.Publisher)(nil)

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
		s.mountReadwiseRoutes(r)
		s.mountHardcoverRoutes(r)
		s.mountEreaderRoutes(r)
		s.mountEnrichRoutes(r)
		s.mountCustomFontRoutes(r)
		s.mountCustomMetadataProviderRoutes(r)
		s.mountDictionaryRoutes(r)
		s.mountTranslateRoutes(r)
		s.mountReadingGoalRoutes(r)
		s.mountShareLinkRoutes(r)
		s.mountYearStatsRoutes(r)
		s.mountNotificationPrefRoutes(r)
		s.mountActivityRoutes(r)
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireAdmin)
			s.mountAdminRoutes(r)
		})
	})
	s.MountPublicShare(r)

	// SPA fallback. Content types for PWA assets are pre-set because plugins
	// run in a minimal container with no /etc/mime.types and Go's mime
	// fallback returns text/plain for .webmanifest, which the browser then
	// refuses to register as either a manifest or a service worker.
	if s.deps.WebFS != nil {
		fileSrv := http.FileServer(s.deps.WebFS)
		r.NotFound(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if strings.HasPrefix(req.URL.Path, "/admin/assets/") {
				req.URL.Path = req.URL.Path[len("/admin"):]
			}
			if ct, ok := pwaContentTypes[req.URL.Path]; ok {
				w.Header().Set("Content-Type", ct)
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
