package main

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	goruntime "runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/jackc/pgx/v5/pgxpool"

	pluginv1 "github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginproto/continuum/plugin/v1"
	publicmanifest "github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginsdk/manifest"
	sdkruntime "github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginsdk/runtime"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/backend"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/consumer"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/event"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/httproutes"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/koboref"
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
		poolPtr          atomic.Pointer[pgxpool.Pool]
		consumerDepsP    atomic.Pointer[consumer.Deps]
		tasksPtr         atomic.Pointer[scheduler.Tasks]
		standaloneOnce   sync.Once
		standaloneAddr   atomic.Value // string
		standaloneSrvPtr atomic.Pointer[http.Server]
	)

	consumerHandler := consumer.New(func() *consumer.Deps { return consumerDepsP.Load() })
	schedulerSrv := scheduler.New(func() map[string]scheduler.TaskFn {
		t := tasksPtr.Load()
		if t == nil {
			return nil
		}
		return map[string]scheduler.TaskFn{
			"request_reconciler":  t.RequestReconciler,
			"cache_evictor":       t.CacheEvictor,
			"kobo_session_reaper": t.KoboSessionReaper,
			"opds_token_pruner":   t.OPDSTokenPruner,
			"kindle_send_retrier": t.KindleSendRetrier,
		}
	})

	rt := pluginrt.New(manifest, func(cfg pluginrt.Config) error {
		ctx := context.Background()
		// Explicit MaxConns cap. The pgx default scales with GOMAXPROCS and
		// can be as low as 4; the portal + OPDS + kosync + Kobo + Kindle
		// scheduler mix can starve under that. 16 is generous without
		// saturating a shared Postgres. Operators override via DSN
		// (?pool_max_conns=N).
		pcfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
		if err != nil {
			return fmt.Errorf("parse db: %w", err)
		}
		if pcfg.MaxConns < 16 {
			pcfg.MaxConns = 16
		}
		p, err := pgxpool.NewWithConfig(ctx, pcfg)
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
		if cfg.TargetBackendInstallID != "" {
			curCfg.TargetBackendInstallID = cfg.TargetBackendInstallID
		}
		_ = st.UpsertConfig(ctx, curCfg)

		// HostHTTPClient — the portal-→backend proxy URL is the local host
		// HTTP API. Operators set HOST_BASE_URL via env. The token is also
		// env-supplied — the host issues a service token at install time and
		// makes it available through CONTINUUM_PLUGIN_TOKEN.
		hostBase := os.Getenv("CONTINUUM_HOST_BASE_URL")
		if hostBase == "" {
			hostBase = "http://localhost:8080"
		}
		hostToken := os.Getenv("CONTINUUM_PLUGIN_TOKEN")
		host := backend.NewHostHTTPClient(hostBase, hostToken)

		ev := event.New(sdkruntime.Host(), logger.Named("event"))

		var cacheMgr *streaming.Manager
		if cfg.CacheDir != "" {
			maxBytes := int64(cfg.CacheMaxSizeGB) * 1024 * 1024 * 1024
			cacheMgr = streaming.NewManager(cfg.CacheDir, maxBytes, st)
		}

		koboRefs := koboref.New()

		srv := server.New(server.Deps{
			Store:        st,
			Host:         host,
			Ev:           ev,
			CacheDir:     cfg.CacheDir,
			CacheManager: cacheMgr,
			KoboRefs:     koboRefs,
			WebFS:        web.FS(),
		})
		httpSrv.SetHandler(srv.Handler())

		// Optional standalone HTTP listener for reverse-proxied client apps
		// (e.g. ebooks.example.com → OPDS / kosync / Kobo / Kindle inbound).
		// See standalone_http_listen in manifest.json. Bound once at first
		// Configure; subsequent changes require a plugin restart.
		if addr := cfg.StandaloneHTTPListen; addr != "" {
			started := false
			standaloneOnce.Do(func() {
				started = true
				standaloneAddr.Store(addr)
				sl := &http.Server{
					Addr:              addr,
					Handler:           httpSrv,
					ReadHeaderTimeout: 10 * time.Second,
					ReadTimeout:       60 * time.Second,
					WriteTimeout:      120 * time.Second,
					IdleTimeout:       120 * time.Second,
				}
				standaloneSrvPtr.Store(sl)
				go func() {
					logger.Info("standalone http listener starting", "addr", addr)
					if err := sl.ListenAndServe(); err != nil && err != http.ErrServerClosed {
						logger.Error("standalone http listener failed", "addr", addr, "err", err)
					}
				}()
			})
			if !started {
				if prev, _ := standaloneAddr.Load().(string); prev != addr {
					logger.Warn("standalone_http_listen changed; restart the plugin to apply",
						"current", prev, "requested", addr)
				}
			}
		}

		consumerDepsP.Store(&consumer.Deps{Store: st})
		tasksPtr.Store(&scheduler.Tasks{
			Store:        st,
			Host:         host,
			Ev:           ev,
			Log:          logger.Named("scheduler"),
			CacheDir:     cfg.CacheDir,
			CacheManager: cacheMgr,
			KoboRefs:     koboRefs,
		})

		if old := poolPtr.Swap(p); old != nil {
			old.Close()
		}
		logger.Info("configured", "cache_dir", cfg.CacheDir, "target_backend", cfg.TargetBackendPluginID)
		return nil
	})

	// Graceful shutdown for the standalone HTTP listener (if it bound a port
	// during Configure). On SIGTERM/SIGINT we call Shutdown(ctx) with a 10s
	// drain window so in-flight reader/Kobo/OPDS transfers finish instead of
	// being killed mid-byte by process exit. signal.Notify fanning to
	// multiple subscribers is documented and safe; the SDK runtime's own
	// signal handler keeps running independently.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		if sl := standaloneSrvPtr.Load(); sl != nil {
			logger.Info("draining standalone http listener", "addr", sl.Addr)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := sl.Shutdown(ctx); err != nil {
				logger.Warn("standalone http drain returned error", "err", err)
			}
		}
	}()

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
