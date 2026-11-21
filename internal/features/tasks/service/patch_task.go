package tasks_service

import (
	"context"
	"fmt"

	"github.com/Reigor1/20h-project/internal/core/domain"
	core_observability "github.com/Reigor1/20h-project/internal/core/observability"
)

// PathTask applies a validated partial update, checks ownership and version,
// and records the resulting task event in the same transaction.
func (s *TasksService) PathTask(
	ctx context.Context,
	id int,
	path domain.TaskPatch,
) (domain.Task, error) {
	ctx, span := s.tracer.Start(ctx, "tasks.update")
	defer span.End()

	before, err := s.tasksRepository.GetTask(ctx, id)
	if err != nil {
		core_observability.MarkSpanError(span, "tasks.update")
		return domain.Task{}, fmt.Errorf("get task: %w", err)
	}

	after := before

	if err := after.ApplyPatch(path); err != nil {
		core_observability.MarkSpanError(span, "tasks.update")
		return domain.Task{}, fmt.Errorf("apply task patch: %w", err)
	}

	if sameTaskState(before, after) {
		return before, nil
	}

	patchedTask, err := s.tasksRepository.PatchTask(
		ctx,
		id,
		before,
		after,
	)
	if err != nil {
		core_observability.MarkSpanError(span, "tasks.update")
		return domain.Task{}, fmt.Errorf("patch task: %w", err)
	}

	return patchedTask, nil
}
