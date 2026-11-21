package tasks_service

import (
	"context"
	"fmt"

	core_observability "github.com/Reigor1/20h-project/internal/core/observability"
)

// DeleteTask loads the task, checks its version and ownership, and deletes it
// while recording the deletion event within the same transaction.
func (s *TasksService) DeleteTask(
	ctx context.Context,
	id int,
) error {
	ctx, span := s.tracer.Start(ctx, "tasks.delete")
	defer span.End()

	if err := s.tasksRepository.DeleteTask(ctx, id); err != nil {
		core_observability.MarkSpanError(span, "tasks.delete")
		return fmt.Errorf("delete task from repository: %w", err)
	}

	return nil
}
