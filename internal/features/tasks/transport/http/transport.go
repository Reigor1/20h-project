package tasks_transport_http

import (
	"context"
	"net/http"

	"github.com/Reigor1/20h-project/internal/core/domain"
	core_http_server "github.com/Reigor1/20h-project/internal/core/transport/http/server"
)

// TasksHTTPHandler adapts task service operations to HTTP requests and responses.
type TasksHTTPHandler struct {
	tasksService TasksService
}

// TasksService defines the task use cases exposed by the HTTP transport.
type TasksService interface {
	CreateTask(
		ctx context.Context,
		task domain.Task,
	) (domain.Task, error)

	GetTasks(
		ctx context.Context,
		userID *int,
		limit *int,
		offset *int,
	) ([]domain.Task, error)

	GetTask(
		ctx context.Context,
		id int,
	) (domain.Task, error)

	DeleteTask(
		ctx context.Context,
		id int,
	) error

	PathTask(
		ctx context.Context,
		id int,
		path domain.TaskPatch,
	) (domain.Task, error)
}

// NewTasksHTTPHandler creates the task handler with its service dependency.
func NewTasksHTTPHandler(
	tasksService TasksService,
) *TasksHTTPHandler {
	return &TasksHTTPHandler{
		tasksService: tasksService,
	}
}

// Routes returns the create, list, read, delete, and patch endpoints for Tasks.
func (h *TasksHTTPHandler) Routes() []core_http_server.Route {
	return []core_http_server.Route{
		{
			Method:  http.MethodPost,
			Path:    "/tasks",
			Handler: h.CreatedTask,
		},
		{
			Method:  http.MethodGet,
			Path:    "/tasks",
			Handler: h.GetTasks,
		},
		{
			Method:  http.MethodGet,
			Path:    "/tasks/{id}",
			Handler: h.GetTask,
		},
		{
			Method:  http.MethodDelete,
			Path:    "/tasks/{id}",
			Handler: h.DeleteTask,
		},
		{
			Method:  http.MethodPatch,
			Path:    "/tasks/{id}",
			Handler: h.PatchTask,
		},
	}
}
