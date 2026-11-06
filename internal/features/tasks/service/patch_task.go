package tasks_service

import (
	"context"
	"fmt"

	"github.com/Reigor1/20h-project/internal/core/domain"
	core_observability "github.com/Reigor1/20h-project/internal/core/observability"
)

func (s *TasksService) PathTask(
	ctx context.Context,
	id int,
	path domain.TaskPatch,
) (domain.Task, error) {
	ctx, span := s.tracer.Start(ctx, "tasks.update")
	defer span.End()

	task, err := s.tasksRepository.GetTask(ctx, id)
	if err != nil {
		core_observability.MarkSpanError(span, "tasks.update")
		return domain.Task{}, fmt.Errorf("get task: %w", err)
	}

	if err := task.ApplyPatch(path); err != nil {
		core_observability.MarkSpanError(span, "tasks.update")
		return domain.Task{}, fmt.Errorf("apply task path: %w", err)
	}

	patchedTask, err := s.tasksRepository.PatchTask(ctx, id, task)
	if err != nil {
		core_observability.MarkSpanError(span, "tasks.update")
		return domain.Task{}, fmt.Errorf("patch task: %w", err)
	}

	return patchedTask, nil
}
