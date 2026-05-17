package server

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleAdminSyncLibraries_MissingBackendID(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/admin/libraries/sync", nil)
	s.handleAdminSyncLibraries(rec, req)
	if rec.Code != 400 {
		t.Fatalf("status=%d want 400 (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "backend_plugin_id") {
		t.Fatalf("body should mention backend_plugin_id: %s", rec.Body.String())
	}
}
