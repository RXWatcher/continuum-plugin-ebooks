package store_test

import (
	"context"
	"testing"

	"github.com/RXWatcher/continuum-plugin-ebooks/internal/store"
)

func TestImportLegacyConfigOnlyWhenDefault(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	legacy := store.Config{
		TargetBackendPluginID:    "continuum.local-ebooks",
		TargetBackendInstallID:   "42",
		AutoApproveRequests:      true,
		DefaultStreamingMode:     "cache",
		CacheDir:                 "/var/cache/ebooks",
		CacheMaxSizeGB:           25,
		CacheDownloadConcurrency: 6,
		PathRemappings:           []byte(`[{"from":"/downloads","to":"/books"}]`),
		OpdsRealm:                "Imported OPDS",
		KindleSMTPConfig:         []byte(`{"host":"smtp.example.test"}`),
		KepubifyPath:             "/opt/bin/kepubify",
		StandaloneHTTPListen:     ":7878",
	}
	imported, err := s.ImportLegacyConfig(ctx, legacy)
	if err != nil {
		t.Fatal(err)
	}
	if !imported {
		t.Fatal("legacy config was not imported into default backend_config")
	}
	cfg, err := s.GetConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CacheDir != "/var/cache/ebooks" || cfg.TargetBackendInstallID != "42" || cfg.DefaultStreamingMode != "cache" || cfg.StandaloneHTTPListen != ":7878" {
		t.Fatalf("imported config = %+v", cfg)
	}

	other := store.Config{
		TargetBackendPluginID:    "other",
		DefaultStreamingMode:     "proxy",
		CacheDir:                 "/tmp/other",
		CacheMaxSizeGB:           1,
		CacheDownloadConcurrency: 1,
	}
	imported, err = s.ImportLegacyConfig(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	if imported {
		t.Fatal("legacy import overwrote plugin-managed config")
	}
	cfg, err = s.GetConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CacheDir != "/var/cache/ebooks" || cfg.TargetBackendPluginID != "continuum.local-ebooks" {
		t.Fatalf("config was overwritten: %+v", cfg)
	}
}
