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
	ctx, cancel, tx, err := r.beginTransaction(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	defer rollbackTransaction(tx)

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

	deletedTask, err := scanTask(tx.QueryRow(ctx, query, id))
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

	if err := r.outboxRepository.Create(ctx, tx, event); err != nil {
		return fmt.Errorf("save task deletion event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit task deletion: %w", err)
	}

	return nil
}
