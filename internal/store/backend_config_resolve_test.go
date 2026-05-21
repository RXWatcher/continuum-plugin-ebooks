package store_test

import (
	"testing"

	"github.com/RXWatcher/continuum-plugin-ebooks/internal/store"
)

// TestConfigBackendTarget pins the single resolver every backend-targeting
// call site must use after the installation-id migration (0013). The
// regression we are guarding against: the portal/OPDS/scheduler read the raw
// TargetBackendPluginID and report "no backend configured" when the operator
// configured the backend via the newer target_backend_installation_id field.
func TestConfigBackendTarget(t *testing.T) {
	cases := []struct {
		name       string
		cfg        store.Config
		wantTarget string
		wantHas    bool
	}{
		{
			name:       "nothing configured",
			cfg:        store.Config{},
			wantTarget: "",
			wantHas:    false,
		},
		{
			name:       "only installation id (new config flow)",
			cfg:        store.Config{TargetBackendInstallID: "42"},
			wantTarget: "42",
			wantHas:    true,
		},
		{
			name:       "numeric plugin id, no install id (pre-0013 backfill gap)",
			cfg:        store.Config{TargetBackendPluginID: "42"},
			wantTarget: "42",
			wantHas:    true,
		},
		{
			name:       "legacy non-numeric plugin id preserved",
			cfg:        store.Config{TargetBackendPluginID: "continuum-plugin-ebookdb"},
			wantTarget: "continuum-plugin-ebookdb",
			wantHas:    true,
		},
		{
			name:       "install id wins over plugin id",
			cfg:        store.Config{TargetBackendPluginID: "continuum-plugin-ebookdb", TargetBackendInstallID: "7"},
			wantTarget: "7",
			wantHas:    true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.BackendTarget(); got != tc.wantTarget {
				t.Errorf("BackendTarget() = %q, want %q", got, tc.wantTarget)
			}
			if got := tc.cfg.HasBackend(); got != tc.wantHas {
				t.Errorf("HasBackend() = %v, want %v", got, tc.wantHas)
			}
		})
	}
}
