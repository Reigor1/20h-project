package tasks_postgres_repository

import (
	"context"
	"errors"
	"fmt"

	core_errors "github.com/Reigor1/20h-project/internal/core/errors"
	core_events "github.com/Reigor1/20h-project/internal/core/events"
	core_postgres_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool"
)

func (r *TasksRepository) DeleteTask(
	ctx context.Context,
	id int,
) error {
	txCtx, cancel, tx, err := core_postgres_pool.BeginTransaction(
		ctx,
		r.pool,
		r.pool.OpTimeout(),
	)
	if err != nil {
		return fmt.Errorf("begin task deletion transaction: %w", err)
	}

	defer cancel()
	defer core_postgres_pool.RollbackTransaction(txCtx, tx, r.pool.OpTimeout())

	query := `
	DELETE FROM todoapp.tasks
	WHERE id = $1
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

	deletedTask, err := scanTask(tx.QueryRow(txCtx, query, id))
	if err != nil {
		if errors.Is(err, core_postgres_pool.ErrNoRows) {
			return fmt.Errorf(
				"task with id=%d: %w",
				id,
				core_errors.ErrNotFound,
			)
		}

		return fmt.Errorf("delete task: %w", err)
	}

	event, err := core_events.NewTaskDeleted(
		taskSnapshotFromDomain(deletedTask),
	)
	if err != nil {
		return fmt.Errorf("create task deletion event: %w", err)
	}

	if err := r.outboxRepository.Create(txCtx, tx, event); err != nil {
		return fmt.Errorf("save task deletion event: %w", err)
	}

	if err := tx.Commit(txCtx); err != nil {
		return fmt.Errorf("commit task deletion: %w", err)
	}

	return nil
}
