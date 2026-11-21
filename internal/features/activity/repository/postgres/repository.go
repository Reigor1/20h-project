package activity_postgres_repository

import (
	"context"
	"fmt"

	core_events "github.com/Reigor1/20h-project/internal/core/events"
	core_postgres_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool"
)

// ActivityRepository stores and retrieves Activity Log records in PostgreSQL.
type ActivityRepository struct {
	pool core_postgres_pool.TransactionalPool
}

// NewActivityRepository creates an Activity repository backed by the supplied PostgreSQL pool.
func NewActivityRepository(
	pool core_postgres_pool.TransactionalPool,
) *ActivityRepository {
	return &ActivityRepository{
		pool: pool,
	}
}

// Record inserts an event into Activity Log once per event ID. It reports inserted=false
// when the event was already persisted, making redelivery safe.
func (r *ActivityRepository) Record(
	ctx context.Context,
	event core_events.Envelope,
) (bool, error) {
	txCtx, cancel, tx, err := core_postgres_pool.BeginTransaction(
		ctx,
		r.pool,
		r.pool.OpTimeout(),
	)
	if err != nil {
		return false, fmt.Errorf("begin Activity transaction: %w", err)
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

	tag, err := tx.Exec(
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
		return false, fmt.Errorf("insert Activity event: %w", err)
	}

	if err := tx.Commit(txCtx); err != nil {
		return false, fmt.Errorf("commit Activity transaction: %w", err)
	}

	return tag.RowsAffected() == 1, nil
}
