package tasks_service

import (
	"context"
	"fmt"

	"github.com/Reigor1/20h-project/internal/core/domain"
	core_errors "github.com/Reigor1/20h-project/internal/core/errors"
	core_observability "github.com/Reigor1/20h-project/internal/core/observability"
)

// GetTasks validates pagination and optional user filtering before querying the repository.
func (s *TasksService) GetTasks(
	ctx context.Context,
	userID *int,
	limit *int,
	offset *int,
) ([]domain.Task, error) {
	ctx, span := s.tracer.Start(ctx, "tasks.list")
	defer span.End()
	
	if limit != nil && *limit < 0 {
		core_observability.MarkSpanError(span, "tasks.list")
		return nil, fmt.Errorf(
			"limit must be non-negative: %w",
			core_errors.ErrInvalidArgument,
		)
	}

	if offset != nil && *offset < 0 {
		core_observability.MarkSpanError(span, "tasks.list")
		return nil, fmt.Errorf(
			"offset must be non-negative: %w",
			core_errors.ErrInvalidArgument,
		)
	}

	tasks, err := s.tasksRepository.GetTasks(ctx, userID, limit, offset)
	if err != nil {
		core_observability.MarkSpanError(span, "tasks.list")
		return nil, fmt.Errorf("get tasks from repository: %w", err)
	}

	return tasks, nil
}
