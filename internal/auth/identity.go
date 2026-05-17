// Package auth provides middleware that reads continuum identity headers and
// puts an Identity into the request context. The host sets these headers on
// every authenticated route; readers should never trust them on a route
// declared access:public.
package auth

import (
	"context"
	"net/http"
	"strings"
)

type ctxKey int

const identityKey ctxKey = 1

// Identity is the per-request user the host authenticated.
type Identity struct {
	UserID   string
	Username string
	Email    string
	IsAdmin  bool
}

// FromContext returns the Identity attached to ctx by Middleware, plus a
// boolean indicating presence.
func FromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityKey).(Identity)
	return id, ok
}

// Middleware injects an Identity from X-Continuum-User-* headers.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := Identity{
			UserID:   r.Header.Get("X-Continuum-User-Id"),
			Username: r.Header.Get("X-Continuum-User-Username"),
			Email:    r.Header.Get("X-Continuum-User-Email"),
		}
		roles := r.Header.Get("X-Continuum-User-Role")
		if roles == "" {
			roles = r.Header.Get("X-Continuum-User-Roles")
		}
		for _, role := range strings.Split(roles, ",") {
			if strings.TrimSpace(role) == "admin" {
				id.IsAdmin = true
				break
			}
		}
		ctx := context.WithValue(r.Context(), identityKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAuth aborts 401 when no UserID is in context.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := FromContext(r.Context())
		if !ok || id.UserID == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAdmin aborts 403 when the identity isn't an admin.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := FromContext(r.Context())
		if !ok || !id.IsAdmin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
