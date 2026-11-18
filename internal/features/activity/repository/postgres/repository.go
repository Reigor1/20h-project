package activity_postgres_repository

import (
	"context"
	"fmt"

	core_events "github.com/Reigor1/20h-project/internal/core/events"
	core_postgres_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool"
)

type ActivityRepository struct {
	pool core_postgres_pool.TransactionalPool
}

func NewActivityRepository(
	pool core_postgres_pool.TransactionalPool,
) *ActivityRepository {
	return &ActivityRepository{
		pool: pool,
	}
}

func (r *ActivityRepository) Record(
	ctx context.Context,
	event core_events.Envelope,
) error {
	txCtx, cancel, tx, err := core_postgres_pool.BeginTransaction(
		ctx,
		r.pool,
		r.pool.OpTimeout(),
	)
	if err != nil {
		return fmt.Errorf("begin Activity transaction: %w", err)
	}

	defer cancel()
	defer core_postgres_pool.RollbackTransaction(txCtx, tx, r.pool.OpTimeout())

	query := `
	INSERT INTO todoapp.task_activity (
		event_id,
		task_id,
		task_version,
		event_type,
		schema_version,
		occurred_at,
		payload
	)
	VALUES ($1, $2, $3, $4, $5, $6, $7::JSONB)
	ON CONFLICT (event_id) DO NOTHING;
	`

	_, err = tx.Exec(
		txCtx,
		query,
		event.EventID,
		event.AggregateID,
		event.AggregateVersion,
		string(event.EventType),
		event.SchemaVersion,
		event.OccurredAt,
		event.Payload,
	)
	if err != nil {
		return fmt.Errorf("insert Activity event: %w", err)
	}

	if err := tx.Commit(txCtx); err != nil {
		return fmt.Errorf("commit Activity transaction: %w", err)
	}

	return nil
}
