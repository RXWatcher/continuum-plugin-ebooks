package scheduler

import (
	"context"
	"strings"
	"testing"

	pluginv1 "github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
)

func TestTaskID(t *testing.T) {
	cases := map[string]string{
		"plugin:42:request_reconciler": "request_reconciler", // real host wire format
		"plugin:1:portal_library_sync": "portal_library_sync",
		"cache_evictor":                "cache_evictor", // bare (host integration tests)
	}
	for in, want := range cases {
		if got := taskID(in); got != want {
			t.Errorf("taskID(%q) = %q, want %q", in, got, want)
		}
	}
}

// The host sends TaskKey="plugin:<installationID>:<id>"; dispatch must
// resolve it to the bare-id-keyed task map (previously every tick errored
// "unknown task_key" and no scheduled task ever ran).
func TestRun_RoutesPrefixedKey(t *testing.T) {
	ran := false
	s := New(func() map[string]TaskFn {
		return map[string]TaskFn{
			"request_reconciler": func(context.Context) error { ran = true; return nil },
		}
	})
	if _, err := s.Run(context.Background(),
		&pluginv1.RunScheduledTaskRequest{TaskKey: "plugin:42:request_reconciler"}); err != nil {
		t.Fatalf("prefixed key must dispatch; got err=%v", err)
	}
	if !ran {
		t.Fatal("registered task was not invoked for the prefixed key")
	}
}

func TestRun_UnknownKeyStillErrors(t *testing.T) {
	s := New(func() map[string]TaskFn { return map[string]TaskFn{"x": func(context.Context) error { return nil }} })
	_, err := s.Run(context.Background(),
		&pluginv1.RunScheduledTaskRequest{TaskKey: "plugin:42:bogus"})
	if err == nil || !strings.Contains(err.Error(), "unknown task_key") {
		t.Fatalf("unknown key must still error; got %v", err)
	}
}

func TestIsPluginHTTPUnsupported(t *testing.T) {
	err := assertErr("fetch backend libraries: rpc error: code = Unimplemented desc = method CallPluginHTTP not implemented")
	if !isPluginHTTPUnsupported(err) {
		t.Fatal("expected CallPluginHTTP unimplemented error to be treated as unsupported backend")
	}
	if isPluginHTTPUnsupported(assertErr("fetch backend libraries: connection refused")) {
		t.Fatal("ordinary backend errors must still surface")
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
