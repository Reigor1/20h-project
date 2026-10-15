package tasks_service

import (
	"context"
	"fmt"

	"github.com/Reigor1/20h-project/internal/core/domain"
)

func (s *TasksService) PathTask(
	ctx context.Context,
	id int,
	path domain.TaskPatch,
) (domain.Task, error) {
	task, err := s.tasksRepository.GetTask(ctx, id)
	if err != nil {
		return domain.Task{}, fmt.Errorf("get task: %w", err)
	}

	if err := task.ApplyPatch(path); err != nil {
		return domain.Task{}, fmt.Errorf("apply task path: %w", err)
	}

	patchedTask, err := s.tasksRepository.PatchTask(ctx, id, task)
	if err != nil {
		return domain.Task{}, fmt.Errorf("patch task: %w", err)
	}

	return patchedTask, nil
}
