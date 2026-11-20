package tasks_postgres_repository

import (
	"context"

	core_events "github.com/Reigor1/20h-project/internal/core/events"
	core_postgres_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool"
)

type OutboxRepository interface {
	Create(
		ctx context.Context,
		executor core_postgres_pool.Executor,
		event core_events.Envelope,
	) error
}

type TasksRepository struct {
	pool             core_postgres_pool.TransactionalPool
	outboxRepository OutboxRepository
}

func NewTasksRepository(
	pool core_postgres_pool.TransactionalPool,
	outboxRepository OutboxRepository,
) *TasksRepository {
	return &TasksRepository{
		pool:             pool,
		outboxRepository: outboxRepository,
	}
}
