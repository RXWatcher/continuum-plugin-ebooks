package main

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	goruntime "runtime"
	"sync/atomic"

	"github.com/hashicorp/go-hclog"
	"github.com/jackc/pgx/v5/pgxpool"

	pluginv1 "github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginproto/continuum/plugin/v1"
	publicmanifest "github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginsdk/manifest"
	sdkruntime "github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginsdk/runtime"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/backend"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/consumer"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/event"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/httproutes"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/migrate"
	pluginrt "github.com/ContinuumApp/continuum-plugin-ebooks/internal/runtime"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/scheduler"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/server"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/store"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/streaming"
	"github.com/ContinuumApp/continuum-plugin-ebooks/web"
)

//go:embed manifest.json
var manifestRaw []byte

func main() {
	logger := hclog.New(&hclog.LoggerOptions{Name: "continuum-plugin-ebooks"})

	manifest, err := loadManifest()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load manifest: %v\n", err)
		os.Exit(1)
	}

	httpSrv := httproutes.NewServer()

	var (
		poolPtr       atomic.Pointer[pgxpool.Pool]
		consumerDepsP atomic.Pointer[consumer.Deps]
		tasksPtr      atomic.Pointer[scheduler.Tasks]
	)

	consumerHandler := consumer.New(func() *consumer.Deps { return consumerDepsP.Load() })
	schedulerSrv := scheduler.New(func() map[string]scheduler.TaskFn {
		t := tasksPtr.Load()
		if t == nil {
			return nil
		}
		return map[string]scheduler.TaskFn{
			"request_reconciler":   t.RequestReconciler,
			"cache_evictor":        t.CacheEvictor,
			"kobo_session_reaper":  t.KoboSessionReaper,
			"opds_token_pruner":    t.OPDSTokenPruner,
			"kindle_send_retrier":  t.KindleSendRetrier,
		}
	})

	rt := pluginrt.New(manifest, func(cfg pluginrt.Config) error {
		ctx := context.Background()
		p, err := pgxpool.New(ctx, cfg.DatabaseURL)
		if err != nil {
			return fmt.Errorf("pgxpool: %w", err)
		}
		if err := migrate.Run(ctx, cfg.DatabaseURL); err != nil {
			p.Close()
			return fmt.Errorf("migrate: %w", err)
		}
		st := store.New(p)

		// Seed backend_config defaults using runtime config values.
		curCfg, _ := st.GetConfig(ctx)
		if cfg.CacheDir != "" {
			curCfg.CacheDir = cfg.CacheDir
		}
		if cfg.CacheMaxSizeGB > 0 {
			curCfg.CacheMaxSizeGB = cfg.CacheMaxSizeGB
		}
		if cfg.CacheDownloadConcurrency > 0 {
			curCfg.CacheDownloadConcurrency = cfg.CacheDownloadConcurrency
		}
		if cfg.DefaultStreamingMode != "" {
			curCfg.DefaultStreamingMode = cfg.DefaultStreamingMode
		}
		if cfg.KepubifyPath != "" {
			curCfg.KepubifyPath = cfg.KepubifyPath
		}
		if cfg.OpdsRealm != "" {
			curCfg.OpdsRealm = cfg.OpdsRealm
		}
		if len(cfg.KindleSMTPConfig) > 2 {
			curCfg.KindleSMTPConfig = cfg.KindleSMTPConfig
		}
		if len(cfg.PathRemappings) > 2 {
			curCfg.PathRemappings = cfg.PathRemappings
		}
		curCfg.AutoApproveRequests = cfg.AutoApproveRequests
		if cfg.TargetBackendPluginID != "" {
			curCfg.TargetBackendPluginID = cfg.TargetBackendPluginID
		}
		_ = st.UpsertConfig(ctx, curCfg)

		// HostHTTPClient — the portal-→backend proxy URL is the local host
		// HTTP API. Operators set HOST_BASE_URL via env. The token is also
		// env-supplied — the host issues a service token at install time and
		// makes it available through CONTINUUM_PLUGIN_TOKEN.
		hostBase := os.Getenv("CONTINUUM_HOST_BASE_URL")
		if hostBase == "" {
			hostBase = "http://localhost:8090"
		}
		hostToken := os.Getenv("CONTINUUM_PLUGIN_TOKEN")
		host := backend.NewHostHTTPClient(hostBase, hostToken)

		ev := event.New(sdkruntime.Host(), logger.Named("event"))

		var cacheMgr *streaming.Manager
		if cfg.CacheDir != "" {
			maxBytes := int64(cfg.CacheMaxSizeGB) * 1024 * 1024 * 1024
			cacheMgr = streaming.NewManager(cfg.CacheDir, maxBytes, st)
		}

		srv := server.New(server.Deps{
			Store:        st,
			Host:         host,
			Ev:           ev,
			CacheDir:     cfg.CacheDir,
			CacheManager: cacheMgr,
			WebFS:        web.FS(),
		})
		httpSrv.SetHandler(srv.Handler())

		consumerDepsP.Store(&consumer.Deps{Store: st})
		tasksPtr.Store(&scheduler.Tasks{
			Store:        st,
			Host:         host,
			Ev:           ev,
			Log:          logger.Named("scheduler"),
			CacheDir:     cfg.CacheDir,
			CacheManager: cacheMgr,
		})

		if old := poolPtr.Swap(p); old != nil {
			old.Close()
		}
		logger.Info("configured", "cache_dir", cfg.CacheDir, "target_backend", cfg.TargetBackendPluginID)
		return nil
	})

	sdkruntime.Serve(sdkruntime.ServeConfig{
		Logger: logger,
		Servers: sdkruntime.CapabilityServers{
			Runtime:       rt,
			HttpRoutes:    httpSrv,
			EventConsumer: consumerHandler,
			ScheduledTask: schedulerSrv,
		},
	})
}

func loadManifest() (*pluginv1.PluginManifest, error) {
	manifest, err := publicmanifest.Load(manifestRaw)
	if err != nil {
		return nil, fmt.Errorf("load embedded manifest: %w", err)
	}
	executablePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve executable path: %w", err)
	}
	binaryData, err := os.ReadFile(executablePath)
	if err != nil {
		return nil, fmt.Errorf("read executable %q: %w", executablePath, err)
	}
	checksum := sha256.Sum256(binaryData)
	manifest.Checksum = hex.EncodeToString(checksum[:])
	if len(manifest.GetSupportedPlatforms()) == 0 {
		manifest.SupportedPlatforms = []*pluginv1.SupportedPlatform{
			{Os: goruntime.GOOS, Arch: goruntime.GOARCH},
		}
	}
	return manifest, nil
}
