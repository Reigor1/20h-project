package tasks_transport_http

import (
	"time"

	"github.com/Reigor1/20h-project/internal/core/domain"
)

type TaskDTOResponse struct {
	ID           int        `json:"id"             example:"7"`
	Version      int        `json:"version"        example:"3"`
	Title        string     `json:"title"          example:"Урок танцев"`
	Description  *string    `json:"description"    example:"Занятие в 17:30"`
	Completed    bool       `json:"completed"      example:"true"`
	CreatedAt    time.Time  `json:"created_at"     example:"2026-10-16T08:15:33Z"`
	CompletedAt  *time.Time `json:"completed_at"   example:"2026-10-16T09:00:00Z"`
	AuthorUserID int        `json:"author_user_id" example:"2"`
}

func taskDTOFromDomain(task domain.Task) TaskDTOResponse {
	return TaskDTOResponse{
		ID:           task.ID,
		Version:      task.Version,
		Title:        task.Title,
		Description:  task.Description,
		Completed:    task.Completed,
		CreatedAt:    task.CreatedAt,
		CompletedAt:  task.CompletedAt,
		AuthorUserID: task.AuthorUserID,
	}
}

func tasksDTOFromDomain(tasks []domain.Task) []TaskDTOResponse {
	tasksDTO := make([]TaskDTOResponse, len(tasks))

	for i, task := range tasks {
		tasksDTO[i] = taskDTOFromDomain(task)
	}

	return tasksDTO
}
