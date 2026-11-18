package tasks_postgres_repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Reigor1/20h-project/internal/core/domain"
	core_errors "github.com/Reigor1/20h-project/internal/core/errors"
	core_events "github.com/Reigor1/20h-project/internal/core/events"
	core_postgres_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool"
)

func (r *TasksRepository) PatchTask(
	ctx context.Context,
	id int,
	before domain.Task,
	after domain.Task,
) (domain.Task, error) {
	if before.ID != id || after.ID != id || before.Version != after.Version {
		return domain.Task{}, fmt.Errorf(
			"invalid task update state: %w",
			core_errors.ErrInvalidArgument,
		)
	}

	if sameTaskState(before, after) {
		return before, nil
	}

	txCtx, cancel, tx, err := core_postgres_pool.BeginTransaction(
		ctx,
		r.pool,
		r.pool.OpTimeout(),
	)
	if err != nil {
		return domain.Task{}, fmt.Errorf("begin task update transaction: %w", err)
	}

	defer cancel()
	defer core_postgres_pool.RollbackTransaction(txCtx, tx, r.pool.OpTimeout())

	query := `
	UPDATE todoapp.tasks
	SET
		title = $1,
		description = $2,
		completed = $3,
		completed_at = $4,
		version = version + 1
	WHERE id = $5 AND version = $6
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

	updatedTask, err := scanTask(tx.QueryRow(
		txCtx,
		query,
		after.Title,
		after.Description,
		after.Completed,
		after.CompletedAt,
		id,
		before.Version,
	))
	if err != nil {
		if errors.Is(err, core_postgres_pool.ErrNoRows) {
			return domain.Task{}, fmt.Errorf(
				"task with id=%d was concurrently modified: %w",
				id,
				core_errors.ErrConflict,
			)
		}

		return domain.Task{}, fmt.Errorf("update task: %w", err)
	}

	event, err := core_events.NewTaskUpdated(
		taskSnapshotFromDomain(before),
		taskSnapshotFromDomain(updatedTask),
	)
	if err != nil {
		return domain.Task{}, fmt.Errorf("create task update event: %w", err)
	}

	if err := r.outboxRepository.Create(txCtx, tx, event); err != nil {
		return domain.Task{}, fmt.Errorf("save task update event: %w", err)
	}

	if err := tx.Commit(txCtx); err != nil {
		return domain.Task{}, fmt.Errorf("commit task update: %w", err)
	}

	return updatedTask, nil
}

func sameTaskState(before, after domain.Task) bool {
	return before.ID == after.ID &&
		before.Version == after.Version &&
		before.Title == after.Title &&
		equalStringPointers(before.Description, after.Description) &&
		before.Completed == after.Completed &&
		before.CreatedAt.Equal(after.CreatedAt) &&
		equalTimePointers(before.CompletedAt, after.CompletedAt) &&
		before.AuthorUserID == after.AuthorUserID
}

func equalStringPointers(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}

	return *left == *right
}

func equalTimePointers(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}

	return left.Equal(*right)
}
