// Package scheduler implements the scheduled_task.v1 gRPC server, dispatching
// task_key values declared in the manifest to the appropriate handler.
package scheduler

import (
	"context"

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
	if s.tasksFn == nil {
		return &pluginv1.RunScheduledTaskResponse{}, nil
	}
	tasks := s.tasksFn()
	fn, ok := tasks[req.GetTaskKey()]
	if !ok || fn == nil {
		return &pluginv1.RunScheduledTaskResponse{}, nil
	}
	if err := fn(ctx); err != nil {
		return nil, err
	}
	return &pluginv1.RunScheduledTaskResponse{}, nil
}
