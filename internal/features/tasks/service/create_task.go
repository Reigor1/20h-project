package tasks_service

import (
	"context"
	"fmt"

	"github.com/Reigor1/20h-project/internal/core/domain"
	core_observability "github.com/Reigor1/20h-project/internal/core/observability"
)

func (s *TasksService) CreateTask(
	ctx context.Context,
	task domain.Task,
) (domain.Task, error) {
	ctx, span := s.tracer.Start(ctx, "tasks.create")
	defer span.End()

	if err := task.Validate(); err != nil {
		core_observability.MarkSpanError(span, "tasks.create")
		return domain.Task{}, fmt.Errorf("validate task domain: %w", err)
	}

	task, err := s.tasksRepository.CreateTask(ctx, task)
	if err != nil {
		core_observability.MarkSpanError(span, "tasks.create")
		return domain.Task{}, fmt.Errorf("create task: %w", err)
	}

	return task, nil
}
