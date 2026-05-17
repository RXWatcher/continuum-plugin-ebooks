package libsync

import (
	"testing"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/backend"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/store"
)

func i64(v int64) *int64 { return &v }

func TestReconcile_CreateMissing(t *testing.T) {
	out, st := Reconcile(nil,
		[]backend.LibraryInfo{{ID: 5, Name: "Comics", MediaType: "comics"}}, "42")
	if st.Created != 1 || st.Updated != 0 || st.Pruned != 0 {
		t.Fatalf("stats=%+v", st)
	}
	if len(out) != 1 || out[0].ID != 0 || out[0].Name != "Comics" ||
		out[0].MediaType != "comics" || out[0].BackendPluginID != "42" ||
		out[0].BackendLibraryID == nil || *out[0].BackendLibraryID != 5 ||
		!out[0].Enabled {
		t.Fatalf("created row wrong: %+v", out[0])
	}
}

func TestReconcile_DefaultsEmptyNameAndMediaType(t *testing.T) {
	out, _ := Reconcile(nil, []backend.LibraryInfo{{ID: 9}}, "42")
	if out[0].Name != "Library 9" || out[0].MediaType != "book" {
		t.Fatalf("defaults wrong: %+v", out[0])
	}
}

func TestReconcile_UpdatePreservesOperatorFields(t *testing.T) {
	existing := []store.PortalLibrary{{
		ID: 7, Name: "Old", MediaType: "book", BackendPluginID: "42",
		BackendLibraryID: i64(5), Enabled: false, SortOrder: 3,
	}}
	out, st := Reconcile(existing,
		[]backend.LibraryInfo{{ID: 5, Name: "Comics", MediaType: "comics"}}, "42")
	if st.Updated != 1 || st.Created != 0 || st.Pruned != 0 {
		t.Fatalf("stats=%+v", st)
	}
	g := out[0]
	if g.ID != 7 || g.Name != "Comics" || g.MediaType != "comics" ||
		g.Enabled != false || g.SortOrder != 3 || g.BackendLibraryID == nil || *g.BackendLibraryID != 5 {
		t.Fatalf("update must change only name/media_type: %+v", g)
	}
}

func TestReconcile_PruneGoneBackendDerived(t *testing.T) {
	existing := []store.PortalLibrary{{
		ID: 7, Name: "Gone", MediaType: "book", BackendPluginID: "42",
		BackendLibraryID: i64(99), Enabled: true, SortOrder: 0,
	}}
	out, st := Reconcile(existing, []backend.LibraryInfo{{ID: 5, Name: "Keep"}}, "42")
	if st.Pruned != 1 || st.Created != 1 {
		t.Fatalf("stats=%+v", st)
	}
	for _, l := range out {
		if l.BackendLibraryID != nil && *l.BackendLibraryID == 99 {
			t.Fatal("pruned row must be omitted")
		}
	}
}

func TestReconcile_PassthroughUntouched(t *testing.T) {
	existing := []store.PortalLibrary{
		{ID: 1, Name: "Manual", MediaType: "book", BackendPluginID: "42", BackendLibraryID: nil, Enabled: true, SortOrder: 0},
		{ID: 2, Name: "OtherBackend", MediaType: "book", BackendPluginID: "99", BackendLibraryID: i64(5), Enabled: true, SortOrder: 1},
	}
	out, st := Reconcile(existing, []backend.LibraryInfo{{ID: 5, Name: "X"}}, "42")
	if st.Kept != 2 || st.Pruned != 0 || st.Created != 1 {
		t.Fatalf("stats=%+v", st)
	}
	var sawManual, sawOther bool
	for _, l := range out {
		if l.ID == 1 {
			sawManual = true
		}
		if l.ID == 2 {
			sawOther = true
		}
	}
	if !sawManual || !sawOther {
		t.Fatal("non-managed rows must pass through unchanged")
	}
}

func TestReconcile_Idempotent(t *testing.T) {
	bl := []backend.LibraryInfo{{ID: 5, Name: "Comics", MediaType: "comics"}}
	out1, _ := Reconcile(nil, bl, "42")
	out1[0].ID = 7
	out2, st := Reconcile(out1, bl, "42")
	if st.Created != 0 || st.Updated != 0 || st.Pruned != 0 || st.Kept != 1 {
		t.Fatalf("second run must be a no-op: %+v", st)
	}
	if len(out2) != 1 || out2[0].ID != 7 {
		t.Fatalf("idempotent run changed rows: %+v", out2)
	}
}
