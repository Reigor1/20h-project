package tasks_postgres_repository

import (
	"context"

	core_events "github.com/Reigor1/20h-project/internal/core/events"
	core_postgres_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool"
)

// OutboxRepository defines the event insertion operation required to persist task changes
// and their Outbox records through the same database transaction.
type OutboxRepository interface {
	Create(
		ctx context.Context,
		executor core_postgres_pool.Executor,
		event core_events.Envelope,
	) error
}

// TasksRepository persists task aggregates and their transactional Outbox events in PostgreSQL.
type TasksRepository struct {
	pool             core_postgres_pool.TransactionalPool
	outboxRepository OutboxRepository
}

// NewTasksRepository creates the task repository with a transactional pool and Outbox adapter.
func NewTasksRepository(
	pool core_postgres_pool.TransactionalPool,
	outboxRepository OutboxRepository,
) *TasksRepository {
	return &TasksRepository{
		pool:             pool,
		outboxRepository: outboxRepository,
	}
}
