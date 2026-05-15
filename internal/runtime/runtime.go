// Package runtime implements the plugin's Runtime gRPC server. Per spec
// Layer 9.1, the portal exposes a large global_config_schema; this Config
// captures the fields the portal needs at runtime.
package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	pluginv1 "github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginproto/continuum/plugin/v1"
	"github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginsdk/runtimedefault"
)

type Config struct {
	DatabaseURL              string
	CacheDir                 string
	CacheMaxSizeGB           int
	CacheDownloadConcurrency int
	DefaultStreamingMode     string // proxy | cache
	KepubifyPath             string
	OpdsRealm                string
	KindleSMTPConfig         json.RawMessage
	PathRemappings           json.RawMessage
	AutoApproveRequests      bool
	TargetBackendPluginID    string
	StandaloneHTTPListen     string
}

func (c Config) Configured() bool { return c.DatabaseURL != "" }

type Server struct {
	runtimedefault.Server
	manifest *pluginv1.PluginManifest
	onCfg    func(Config) error

	mu  sync.RWMutex
	cfg Config
}

func New(manifest *pluginv1.PluginManifest, onConfig func(Config) error) *Server {
	return &Server{manifest: manifest, onCfg: onConfig}
}

func (s *Server) GetManifest(_ context.Context, _ *pluginv1.GetManifestRequest) (*pluginv1.GetManifestResponse, error) {
	return &pluginv1.GetManifestResponse{Manifest: s.manifest}, nil
}

func (s *Server) Configure(_ context.Context, req *pluginv1.ConfigureRequest) (*pluginv1.ConfigureResponse, error) {
	cfg := Config{
		CacheMaxSizeGB:           10,
		CacheDownloadConcurrency: 4,
		DefaultStreamingMode:     "proxy",
		KepubifyPath:             "/usr/local/bin/kepubify",
		OpdsRealm:                "Continuum Library",
	}
	for _, e := range req.GetConfig() {
		v := e.GetValue()
		if v == nil {
			continue
		}
		m := v.AsMap()
		val := m["value"]
		switch e.GetKey() {
		case "database_url":
			cfg.DatabaseURL = stringFrom(val)
		case "cache_dir":
			cfg.CacheDir = stringFrom(val)
		case "cache_max_size_gb":
			if n, ok := intFrom(val); ok {
				cfg.CacheMaxSizeGB = n
			}
		case "cache_download_concurrency":
			if n, ok := intFrom(val); ok {
				cfg.CacheDownloadConcurrency = n
			}
		case "default_streaming_mode":
			if s := stringFrom(val); s != "" {
				cfg.DefaultStreamingMode = s
			}
		case "kepubify_path":
			if s := stringFrom(val); s != "" {
				cfg.KepubifyPath = s
			}
		case "opds_realm":
			if s := stringFrom(val); s != "" {
				cfg.OpdsRealm = s
			}
		case "kindle_smtp_config":
			if b, err := json.Marshal(val); err == nil {
				cfg.KindleSMTPConfig = b
			}
		case "path_remappings":
			if b, err := json.Marshal(val); err == nil {
				cfg.PathRemappings = b
			}
		case "auto_approve_requests":
			if b, ok := val.(bool); ok {
				cfg.AutoApproveRequests = b
			}
		case "target_backend_plugin_id":
			cfg.TargetBackendPluginID = stringFrom(val)
		case "standalone_http_listen":
			cfg.StandaloneHTTPListen = stringFrom(val)
		}
	}
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("database_url is required")
	}
	if s.onCfg != nil {
		if err := s.onCfg(cfg); err != nil {
			return nil, err
		}
	}
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
	return &pluginv1.ConfigureResponse{}, nil
}

func (s *Server) Snapshot() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

func stringFrom(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func intFrom(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	}
	return 0, false
}
