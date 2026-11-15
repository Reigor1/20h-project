package outbox_postgres_repository

import (
	"context"
	"fmt"

	core_events "github.com/Reigor1/20h-project/internal/core/events"
	core_postgres_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool"
)

type OutboxRepository struct{}

func NewOutboxRepository() *OutboxRepository {
	return &OutboxRepository{}
}

func (r *OutboxRepository) Create(
	ctx context.Context,
	executor core_postgres_pool.Executor,
	event core_events.Envelope,
) error {
	if err := event.Validate(); err != nil {
		return fmt.Errorf("validate outbox event: %w", err)
	}

	query := `
	INSERT INTO todoapp.outbox (
		event_id,
		event_type,
		schema_version,
		aggregate_type,
		aggregate_id,
		aggregate_version,
		occurred_at,
		payload
	)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb);
	`

	_, err := executor.Exec(
		ctx,
		query,
		event.EventID,
		string(event.EventType),
		event.SchemaVersion,
		event.AggregateType,
		event.AggregateID,
		event.AggregateVersion,
		event.OccurredAt,
		event.Payload,
	)
	if err != nil {
		return fmt.Errorf("insert outbox event: %w", err)
	}

	return nil
}
