package tasks_postgres_repository

import (
	"github.com/Reigor1/20h-project/internal/core/domain"
	core_events "github.com/Reigor1/20h-project/internal/core/events"
)

// taskSnapshotFromDomain copies the domain task state into the event snapshot contract.
func taskSnapshotFromDomain(task domain.Task) core_events.TaskSnapshot {
	return core_events.TaskSnapshot{
		ID:           int64(task.ID),
		Version:      int64(task.Version),
		Title:        task.Title,
		Description:  task.Description,
		Completed:    task.Completed,
		CreatedAt:    task.CreatedAt,
		CompletedAt:  task.CompletedAt,
		AuthorUserID: int64(task.AuthorUserID),
	}
}
