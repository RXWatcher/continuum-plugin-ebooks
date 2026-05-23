package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMiddlewareReadsHostSingularRoleHeader(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Silo-User-Id", "u-1")
	req.Header.Set("X-Silo-User-Role", "admin")

	var got Identity
	handler := Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		var ok bool
		got, ok = FromContext(r.Context())
		if !ok {
			t.Fatal("identity missing from context")
		}
	}))
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if got.UserID != "u-1" {
		t.Fatalf("UserID = %q, want u-1", got.UserID)
	}
	if !got.IsAdmin {
		t.Fatal("IsAdmin = false, want true for X-Silo-User-Role: admin")
	}
}

func TestMiddlewareFallsBackToPluralRolesHeader(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Silo-User-Id", "u-2")
	req.Header.Set("X-Silo-User-Roles", "user, admin")

	var got Identity
	handler := Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		var ok bool
		got, ok = FromContext(r.Context())
		if !ok {
			t.Fatal("identity missing from context")
		}
	}))
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if got.UserID != "u-2" {
		t.Fatalf("UserID = %q, want u-2", got.UserID)
	}
	if !got.IsAdmin {
		t.Fatal("IsAdmin = false, want true for fallback plural roles header")
	}
}

func TestMiddlewareReadsProfileID(t *testing.T) {
	var got Identity
	h := Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got, _ = FromContext(r.Context())
	}))
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Silo-User-Id", "u-1")
	r.Header.Set("X-Silo-Profile-Id", "p-9")
	h.ServeHTTP(httptest.NewRecorder(), r)
	if got.UserID != "u-1" || got.ProfileID != "p-9" {
		t.Errorf("identity = %+v, want user u-1 profile p-9", got)
	}
}
