package store_test

import (
	"context"
	"testing"

	"github.com/RXWatcher/silo-plugin-ebooks/internal/store"
)

func TestKosyncProgressScopedByProfile(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	must(t, s.UpsertKosyncProgress(ctx, store.KosyncProgress{
		UserID: "u-1", ProfileID: "", Document: "doc-a", Progress: "10", DeviceID: "d1",
	}))
	must(t, s.UpsertKosyncProgress(ctx, store.KosyncProgress{
		UserID: "u-1", ProfileID: "p-laura", Document: "doc-a", Progress: "90", DeviceID: "d1",
	}))

	primary, _ := s.GetKosyncProgress(ctx, "u-1", "", "doc-a")
	laura, _ := s.GetKosyncProgress(ctx, "u-1", "p-laura", "doc-a")
	if primary.Progress != "10" || laura.Progress != "90" {
		t.Errorf("primary=%q laura=%q, want 10/90", primary.Progress, laura.Progress)
	}
}
