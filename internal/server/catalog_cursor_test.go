package server

import "testing"

func TestCatalogCursor_RoundTrip(t *testing.T) {
	in := map[int64]string{1: "bk-cursor-aaa", 7: "bk-cursor-zzz"}
	tok := encodeCatalogCursor(in)
	if tok == "" {
		t.Fatal("expected a non-empty composite cursor")
	}
	out, ok := decodeCatalogCursor(tok)
	if !ok {
		t.Fatal("decode failed for a token we just produced")
	}
	if len(out) != 2 || out[1] != "bk-cursor-aaa" || out[7] != "bk-cursor-zzz" {
		t.Fatalf("round trip mismatch: %#v", out)
	}
}

func TestCatalogCursor_EmptyMeansNoMorePages(t *testing.T) {
	if got := encodeCatalogCursor(nil); got != "" {
		t.Errorf("nil map should yield empty cursor, got %q", got)
	}
	if got := encodeCatalogCursor(map[int64]string{}); got != "" {
		t.Errorf("empty map should yield empty cursor, got %q", got)
	}
}

func TestCatalogCursor_InvalidTokenResetsToFirstPage(t *testing.T) {
	for _, raw := range []string{"", "not-base64-!!!", "YWJj" /* base64 of "abc", not JSON */} {
		if m, ok := decodeCatalogCursor(raw); ok || m != nil {
			t.Errorf("decodeCatalogCursor(%q) = (%v,%v), want (nil,false)", raw, m, ok)
		}
	}
}
