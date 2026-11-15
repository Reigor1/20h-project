package tasks_postgres_repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/Reigor1/20h-project/internal/core/domain"
	core_errors "github.com/Reigor1/20h-project/internal/core/errors"
	core_events "github.com/Reigor1/20h-project/internal/core/events"
	core_postgres_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool"
)

func (r *TasksRepository) CreateTask(
	ctx context.Context,
	task domain.Task,
) (domain.Task, error) {
	ctx, cancel, tx, err := r.beginTransaction(ctx)
	if err != nil {
		return domain.Task{}, err
	}
	defer cancel()
	defer rollbackTransaction(tx)

	query := `
	INSERT INTO todoapp.tasks (
		title,
		description,
		completed,
		created_at,
		completed_at,
		author_user_id
	)
	VALUES ($1, $2, $3, $4, $5, $6)
	RETURNING
		id,
		version,
		title,
		description,
		completed,
		created_at,
		completed_at,
		author_user_id;
	`

	createdTask, err := scanTask(tx.QueryRow(
		ctx,
		query,
		task.Title,
		task.Description,
		task.Completed,
		task.CreatedAt,
		task.CompletedAt,
		task.AuthorUserID,
	))
	if err != nil {
		if errors.Is(err, core_postgres_pool.ErrViolatesForeignKey) {
			return domain.Task{}, fmt.Errorf(
				"%v: user with id=`%d`: %w",
				err,
				task.AuthorUserID,
				core_errors.ErrNotFound,
			)
		}

		return domain.Task{}, fmt.Errorf("insert task: %w", err)
	}

	event, err := core_events.NewTaskCreated(
		taskSnapshotFromDomain(createdTask),
	)
	if err != nil {
		return domain.Task{}, fmt.Errorf("create task event: %w", err)
	}

	if err := r.outboxRepository.Create(ctx, tx, event); err != nil {
		return domain.Task{}, fmt.Errorf("save task event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Task{}, fmt.Errorf("commit task creation: %w", err)
	}

	return createdTask, nil
}

func scanTask(row core_postgres_pool.Row) (domain.Task, error) {
	var taskModel TaskModel

	err := row.Scan(
		&taskModel.ID,
		&taskModel.Version,
		&taskModel.Title,
		&taskModel.Description,
		&taskModel.Completed,
		&taskModel.CreatedAt,
		&taskModel.CompletedAt,
		&taskModel.AuthorUserID,
	)
	if err != nil {
		return domain.Task{}, fmt.Errorf("scan task: %w", err)
	}

	return taskDomainFromModel(taskModel), nil
}
