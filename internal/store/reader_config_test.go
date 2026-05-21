package store_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/RXWatcher/continuum-plugin-ebooks/internal/store"
)

func TestReaderConfigRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	initial := json.RawMessage(`{"location":"epubcfi(/6/2)","progress":[3,10]}`)
	if err := s.UpsertReaderConfig(ctx, store.ReaderConfig{
		UserID:     "u-reader",
		BookID:     "b-reader",
		ConfigJSON: initial,
	}); err != nil {
		t.Fatalf("UpsertReaderConfig initial: %v", err)
	}

	got, err := s.GetReaderConfig(ctx, "u-reader", "b-reader")
	if err != nil {
		t.Fatalf("GetReaderConfig initial: %v", err)
	}
	assertJSONEqual(t, initial, got.ConfigJSON)

	updated := json.RawMessage(`{"location":"epubcfi(/6/4)","progress":[4,10],"booknotes":[]}`)
	if err := s.UpsertReaderConfig(ctx, store.ReaderConfig{
		UserID:     "u-reader",
		BookID:     "b-reader",
		ConfigJSON: updated,
	}); err != nil {
		t.Fatalf("UpsertReaderConfig updated: %v", err)
	}

	got, err = s.GetReaderConfig(ctx, "u-reader", "b-reader")
	if err != nil {
		t.Fatalf("GetReaderConfig updated: %v", err)
	}
	assertJSONEqual(t, updated, got.ConfigJSON)
}

func assertJSONEqual(t *testing.T, want, got []byte) {
	t.Helper()
	var wantValue any
	var gotValue any
	if err := json.Unmarshal(want, &wantValue); err != nil {
		t.Fatalf("unmarshal want JSON: %v", err)
	}
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("unmarshal got JSON %q: %v", string(got), err)
	}
	if !jsonEqual(wantValue, gotValue) {
		t.Fatalf("JSON mismatch\nwant: %s\n got: %s", string(want), string(got))
	}
}

func jsonEqual(a, b any) bool {
	aBytes, err := json.Marshal(a)
	if err != nil {
		return false
	}
	bBytes, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return string(aBytes) == string(bBytes)
}
