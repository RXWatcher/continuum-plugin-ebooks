// Package scheduler implements the scheduled_task.v1 gRPC server, dispatching
// task_key values declared in the manifest to the appropriate handler.
package scheduler

import (
	"context"
	"fmt"

	pluginv1 "github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginproto/continuum/plugin/v1"
)

// TaskFn is invoked when its registered task fires.
type TaskFn func(ctx context.Context) error

type Server struct {
	pluginv1.UnimplementedScheduledTaskServer
	tasksFn func() map[string]TaskFn
}

func New(tasksFn func() map[string]TaskFn) *Server { return &Server{tasksFn: tasksFn} }

func (s *Server) Run(ctx context.Context, req *pluginv1.RunScheduledTaskRequest) (*pluginv1.RunScheduledTaskResponse, error) {
	tasks := map[string]TaskFn(nil)
	if s.tasksFn != nil {
		tasks = s.tasksFn()
	}
	if tasks == nil {
		// Not configured yet — return an error so the host retries this tick
		// once Configure has run, instead of reporting a successful no-op.
		return nil, fmt.Errorf("plugin not configured yet")
	}
	fn, ok := tasks[req.GetTaskKey()]
	if !ok || fn == nil {
		return nil, fmt.Errorf("unknown task_key %q", req.GetTaskKey())
	}
	if err := fn(ctx); err != nil {
		return nil, err
	}
	return &pluginv1.RunScheduledTaskResponse{}, nil
}
