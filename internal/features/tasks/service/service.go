package tasks_service

import (
	"context"

	"github.com/Reigor1/20h-project/internal/core/domain"
	"go.opentelemetry.io/otel/trace"
)

const tracerInstrumentationName = "github.com/Reigor1/20h-project/internal/features/tasks/service"

type TasksService struct {
	tasksRepository TasksRepository
	tracer          trace.Tracer
}

type TasksRepository interface {
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

	PatchTask(
		ctx context.Context,
		id int,
		before domain.Task,
		after domain.Task,
	) (domain.Task, error)
}

func NewTasksService(
	tasksRepository TasksRepository,
	tracerProvider trace.TracerProvider,
) *TasksService {
	return &TasksService{
		tasksRepository: tasksRepository,
		tracer:          tracerProvider.Tracer(tracerInstrumentationName),
	}
}
