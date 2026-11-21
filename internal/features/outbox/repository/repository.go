package outbox_postgres_repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	core_events "github.com/Reigor1/20h-project/internal/core/events"
	core_postgres_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool"
	"go.opentelemetry.io/otel/propagation"
)

// ErrLeaseLost indicates that an Outbox update no longer owns the event's lease token.
var ErrLeaseLost = errors.New("outbox lease lost")

// OutboxRepository persists event envelopes and manages their publication leases in PostgreSQL.
type OutboxRepository struct {
	pool core_postgres_pool.Pool
}

// ClaimedEvent contains an event reserved for publication and the token required to update it.
type ClaimedEvent struct {
	ID          int64
	Event       core_events.Envelope
	TraceParent string
	TraceState  string
	Attempts    int
	LeaseToken  string
}

// NewOutboxRepository creates an Outbox repository backed by the supplied PostgreSQL pool.
func NewOutboxRepository(
	pool core_postgres_pool.Pool,
) *OutboxRepository {
	return &OutboxRepository{
		pool: pool,
	}
}

// Create validates and inserts an event through the caller's executor, preserving the
// caller's transaction boundary, and stores the current W3C trace context with it.
func (r *OutboxRepository) Create(
	ctx context.Context,
	executor core_postgres_pool.Executor,
	event core_events.Envelope,
) error {
	if err := event.Validate(); err != nil {
		return fmt.Errorf("validate outbox event: %w", err)
	}

	traceCarrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, traceCarrier)

	query := `
	INSERT INTO todoapp.outbox (
		event_id,
		event_type,
		schema_version,
		aggregate_type,
		aggregate_id,
		aggregate_version,
		occurred_at,
		payload,
		traceparent,
		tracestate
	)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, $10);
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
		traceCarrier.Get("traceparent"),
		traceCarrier.Get("tracestate"),
	)
	if err != nil {
		return fmt.Errorf("insert outbox event: %w", err)
	}

	return nil
}

// ClaimPending atomically leases eligible events using SKIP LOCKED. Earlier unpublished
// events for the same aggregate block later versions to preserve per-aggregate ordering.
func (r *OutboxRepository) ClaimPending(
	ctx context.Context,
	limit int,
	leaseDuration time.Duration,
) ([]ClaimedEvent, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("claim limit must be positive")
	}

	if leaseDuration <= 0 {
		return nil, fmt.Errorf("lease duration must be positive")
	}

	ctx, cancel := context.WithTimeout(ctx, r.pool.OpTimeout())
	defer cancel()

	query := `
	WITH candidates AS (
		SELECT current_event.id
		FROM todoapp.outbox AS current_event
		WHERE current_event.published_at IS NULL
			AND current_event.next_attempt_at <= NOW()
			AND (
				current_event.lease_until IS NULL
				OR current_event.lease_until <= NOW()
			)

			-- A later event of the same aggregate cannot be claimed
			-- until all preceding events have been published.
			AND NOT EXISTS (
				SELECT 1
				FROM todoapp.outbox AS previous_event
				WHERE previous_event.aggregate_type = current_event.aggregate_type
					AND previous_event.aggregate_id = current_event.aggregate_id
					AND previous_event.published_at IS NULL
					AND (
						previous_event.aggregate_version,
						previous_event.id
					) < (
						current_event.aggregate_version,
						current_event.id
					)
			)

		ORDER BY
			current_event.next_attempt_at,
			current_event.created_at,
			current_event.id

		LIMIT $1
		FOR UPDATE OF current_event SKIP LOCKED
	)
	UPDATE todoapp.outbox AS claimed
	SET
		lease_token = gen_random_uuid(),
		lease_until = NOW() + ($2::BIGINT * INTERVAL '1 millisecond')
	FROM candidates
	WHERE claimed.id = candidates.id
	RETURNING
		claimed.id,
		claimed.event_id,
		claimed.event_type,
		claimed.schema_version,
		claimed.aggregate_type,
		claimed.aggregate_id,
		claimed.aggregate_version,
		claimed.occurred_at,
		claimed.payload,
		claimed.traceparent,
		claimed.tracestate,
		claimed.attempts,
		claimed.lease_token::TEXT;
	`

	rows, err := r.pool.Query(
		ctx,
		query,
		limit,
		leaseDuration.Milliseconds(),
	)
	if err != nil {
		return nil, fmt.Errorf("claim pending outbox events: %w", err)
	}
	defer rows.Close()

	events := make([]ClaimedEvent, 0, limit)

	for rows.Next() {
		var event ClaimedEvent

		if err := rows.Scan(
			&event.ID,
			&event.Event.EventID,
			&event.Event.EventType,
			&event.Event.SchemaVersion,
			&event.Event.AggregateType,
			&event.Event.AggregateID,
			&event.Event.AggregateVersion,
			&event.Event.OccurredAt,
			&event.Event.Payload,
			&event.TraceParent,
			&event.TraceState,
			&event.Attempts,
			&event.LeaseToken,
		); err != nil {
			return nil, fmt.Errorf("scan claimed outbox event: %w", err)
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate claimed outbox events: %w", err)
	}

	return events, nil
}

// MarkPublished marks an event delivered only when the supplied lease token still owns it.
func (r *OutboxRepository) MarkPublished(
	ctx context.Context,
	id int64,
	leaseToken string,
) error {
	ctx, cancel := context.WithTimeout(ctx, r.pool.OpTimeout())
	defer cancel()

	query := `
	UPDATE todoapp.outbox
	SET
		published_at = NOW(),
		lease_token = NULL,
		lease_until = NULL,
		last_error = NULL
	WHERE id = $1
		AND lease_token = $2::UUID
		AND published_at IS NULL;
	`

	tag, err := r.pool.Exec(ctx, query, id, leaseToken)
	if err != nil {
		return fmt.Errorf("mark outbox event %d as published: %w", id, err)
	}

	if tag.RowsAffected() != 1 {
		return fmt.Errorf(
			"mark outbox event %d as published: %w",
			id,
			ErrLeaseLost,
		)
	}

	return nil
}

// MarkFailed increments the attempt count, stores a bounded error message, schedules
// the next attempt, and releases the lease when the token is still valid.
func (r *OutboxRepository) MarkFailed(
	ctx context.Context,
	id int64,
	leaseToken string,
	publishError error,
	retryAfter time.Duration,
) error {
	if publishError == nil {
		return errors.New("publish error must not be nil")
	}

	if retryAfter <= 0 {
		return errors.New("retry delay must be positive")
	}

	ctx, cancel := context.WithTimeout(ctx, r.pool.OpTimeout())
	defer cancel()

	query := `
	UPDATE todoapp.outbox
	SET
		attempts = attempts + 1,
		last_error = LEFT($3, 2000),
		next_attempt_at = NOW() + ($4::BIGINT * INTERVAL '1 millisecond'),
		lease_token = NULL,
		lease_until = NULL
	WHERE id = $1
		AND lease_token = $2::UUID
		AND published_at IS NULL;
	`

	tag, err := r.pool.Exec(
		ctx,
		query,
		id,
		leaseToken,
		publishError.Error(),
		retryAfter.Milliseconds(),
	)
	if err != nil {
		return fmt.Errorf("mark outbox event %d as failed: %w", id, err)
	}

	if tag.RowsAffected() != 1 {
		return fmt.Errorf(
			"mark outbox event %d as failed: %w",
			id,
			ErrLeaseLost,
		)
	}

	return nil
}

// PendingStats returns the count of all unpublished events and the age of the oldest one.
func (r *OutboxRepository) PendingStats(
	ctx context.Context,
) (pending int64, oldestEventAgeSeconds float64, err error) {
	ctx, cancel := context.WithTimeout(ctx, r.pool.OpTimeout())
	defer cancel()

	query := `
	SELECT
		COUNT(*)::BIGINT,
		COALESCE(
			EXTRACT(EPOCH FROM (NOW() - MIN(created_at))),
			0
		)::DOUBLE PRECISION
	FROM todoapp.outbox
	WHERE published_at IS NULL;
	`

	if err := r.pool.QueryRow(ctx, query).Scan(
		&pending,
		&oldestEventAgeSeconds,
	); err != nil {
		return 0, 0, fmt.Errorf("query pending Outbox metrics: %w", err)
	}

	return pending, oldestEventAgeSeconds, nil
}
