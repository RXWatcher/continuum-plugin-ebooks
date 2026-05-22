package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeCredentials struct {
	userID, profileID string
	err               error
}

func (f fakeCredentials) ValidateProfileCredential(context.Context, string, string) (string, string, error) {
	return f.userID, f.profileID, f.err
}

func basicReq(user, pass string) *http.Request {
	r := httptest.NewRequest("GET", "/opds/catalog", nil)
	r.SetBasicAuth(user, pass)
	return r
}

func TestOPDSAuth_Resolves(t *testing.T) {
	s := &Server{deps: Deps{Credentials: fakeCredentials{userID: "u-1", profileID: "p-9"}}}
	uid, pid, err := s.opdsAuth(basicReq("jim#laura", "pw"))
	if err != nil || uid != "u-1" || pid != "p-9" {
		t.Errorf("got (%q,%q,%v)", uid, pid, err)
	}
}

func TestOPDSAuth_BadCredentialIsNotServiceError(t *testing.T) {
	s := &Server{deps: Deps{Credentials: fakeCredentials{err: status.Error(codes.Unauthenticated, "no")}}}
	uid, _, err := s.opdsAuth(basicReq("jim", "bad"))
	if err != nil || uid != "" {
		t.Errorf("want empty uid + nil err, got (%q,%v)", uid, err)
	}
}

func TestOPDSAuth_ServiceErrorPropagates(t *testing.T) {
	s := &Server{deps: Deps{Credentials: fakeCredentials{err: status.Error(codes.Unavailable, "down")}}}
	if _, _, err := s.opdsAuth(basicReq("jim", "pw")); err == nil {
		t.Error("want service error, got nil")
	}
}

func TestOPDSAuth_NoHeader(t *testing.T) {
	s := &Server{deps: Deps{Credentials: fakeCredentials{}}}
	uid, _, err := s.opdsAuth(httptest.NewRequest("GET", "/opds/catalog", nil))
	if err != nil || uid != "" {
		t.Errorf("want empty + nil, got (%q,%v)", uid, err)
	}
}
