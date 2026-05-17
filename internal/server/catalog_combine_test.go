package server

import (
	"errors"
	"testing"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/backend"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/store"
)

func sumEnv(ids ...string) backend.PageEnvelope[backend.EbookSummary] {
	items := make([]backend.EbookSummary, 0, len(ids))
	for _, id := range ids {
		items = append(items, backend.EbookSummary{ID: id, Title: id})
	}
	return backend.PageEnvelope[backend.EbookSummary]{Items: items}
}

// TestCombineCatalogResults_AllFailSurfacesError guards the silent-failure
// root cause: when every configured library's backend errors, the portal
// must return the error, not a misleading empty 200 that looks like "no
// books" to the user who just created the libraries.
func TestCombineCatalogResults_AllFailSurfacesError(t *testing.T) {
	boom := errors.New("upstream 502")
	_, err := combineCatalogResults([]libResult{
		{lib: store.PortalLibrary{ID: 1}, err: boom},
		{lib: store.PortalLibrary{ID: 2}, err: boom},
	}, 50)
	if err == nil {
		t.Fatal("all libraries failed but combineCatalogResults returned nil error")
	}
}

// Partial failure must still return the libraries that worked (no error),
// rather than failing the whole catalog.
func TestCombineCatalogResults_PartialSucceeds(t *testing.T) {
	env, err := combineCatalogResults([]libResult{
		{lib: store.PortalLibrary{ID: 1, Name: "A"}, env: sumEnv("a1", "a2")},
		{lib: store.PortalLibrary{ID: 2}, err: errors.New("down")},
	}, 50)
	if err != nil {
		t.Fatalf("partial success should not error: %v", err)
	}
	if len(env.Items) != 2 {
		t.Fatalf("want 2 items from the working library, got %d", len(env.Items))
	}
	// wrapCatalogItems must have run: IDs are namespaced with the portal lib id.
	if env.Items[0].ID == "a1" {
		t.Errorf("item ID not namespaced via wrapCatalogItems: %q", env.Items[0].ID)
	}
	if env.Items[0].LibraryName != "A" {
		t.Errorf("library metadata not applied: %+v", env.Items[0])
	}
}

func TestCombineCatalogResults_TruncatesToLimit(t *testing.T) {
	env, err := combineCatalogResults([]libResult{
		{lib: store.PortalLibrary{ID: 1}, env: sumEnv("a", "b", "c")},
		{lib: store.PortalLibrary{ID: 2}, env: sumEnv("d", "e", "f")},
	}, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(env.Items) != 4 {
		t.Fatalf("want truncation to 4, got %d", len(env.Items))
	}
}

// limit <= 0 means "no truncation" (search path).
func TestCombineCatalogResults_NoLimitKeepsAll(t *testing.T) {
	env, _ := combineCatalogResults([]libResult{
		{lib: store.PortalLibrary{ID: 1}, env: sumEnv("a", "b", "c", "d", "e")},
	}, 0)
	if len(env.Items) != 5 {
		t.Fatalf("want all 5 items with no limit, got %d", len(env.Items))
	}
}
