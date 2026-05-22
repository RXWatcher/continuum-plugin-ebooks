package store_test

import (
	"context"
	"testing"

	"github.com/RXWatcher/continuum-plugin-ebooks/internal/store"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestCollectionsScopedByProfile(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	must(t, s.CreateCollection(ctx, store.Collection{ID: "c-primary", UserID: "u-1", ProfileID: "", Name: "Primary"}))
	must(t, s.CreateCollection(ctx, store.Collection{ID: "c-laura", UserID: "u-1", ProfileID: "p-laura", Name: "Laura"}))
	must(t, s.CreateCollection(ctx, store.Collection{ID: "c-other", UserID: "u-2", ProfileID: "", Name: "Other"}))

	primary, err := s.ListCollectionsByProfile(ctx, "u-1", "")
	if err != nil {
		t.Fatalf("list primary: %v", err)
	}
	if len(primary) != 1 || primary[0].ID != "c-primary" {
		t.Errorf("u-1 primary = %+v, want only c-primary", primary)
	}
	laura, _ := s.ListCollectionsByProfile(ctx, "u-1", "p-laura")
	if len(laura) != 1 || laura[0].ID != "c-laura" {
		t.Errorf("u-1 laura = %+v, want only c-laura", laura)
	}
}
