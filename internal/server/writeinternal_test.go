package server

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

// writeInternal / writeBadGateway must never echo the underlying error to the
// client (these helpers back public /opds, /kosync, /kobo routes too).
func TestWriteInternal_Opaque(t *testing.T) {
	leak := errors.New(`pq: relation "kosync_user" does not exist; host=db.internal user=continuum`)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/kosync/syncs/progress/x", nil)
	writeInternal(w, r, leak)
	if w.Code != 500 {
		t.Fatalf("code = %d, want 500", w.Code)
	}
	if b := w.Body.String(); strings.Contains(b, "kosync_user") || strings.Contains(b, "db.internal") || strings.Contains(b, "continuum") {
		t.Fatalf("client body leaked internal detail: %s", b)
	}
	if !strings.Contains(w.Body.String(), "internal error") {
		t.Fatalf("body not the opaque message: %s", w.Body.String())
	}

	w2 := httptest.NewRecorder()
	writeBadGateway(w2, r, leak)
	if w2.Code != 502 {
		t.Fatalf("code = %d, want 502", w2.Code)
	}
	if strings.Contains(w2.Body.String(), "db.internal") {
		t.Fatalf("bad-gateway body leaked: %s", w2.Body.String())
	}
}
