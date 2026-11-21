package domain

import (
	"fmt"
	"time"

	core_errors "github.com/Reigor1/20h-project/internal/core/errors"
)

// Task is the domain model for a user-owned task. Version is incremented on updates
// and is used by persistence adapters to detect concurrent modifications.
type Task struct {
	ID      int
	Version int

	Title       string
	Description *string
	Completed   bool
	CreatedAt   time.Time
	CompletedAt *time.Time

	AuthorUserID int
}

// NewTask constructs a Task with the supplied persisted or explicitly assigned state.
func NewTask(
	id int,
	version int,
	title string,
	description *string,
	completed bool,
	createAt time.Time,
	completedAt *time.Time,
	authorUserID int,
) Task {
	return Task{
		ID:           id,
		Version:      version,
		Title:        title,
		Description:  description,
		Completed:    completed,
		CreatedAt:    createAt,
		CompletedAt:  completedAt,
		AuthorUserID: authorUserID,
	}
}

// NewTaskUninitialized creates a task that has not yet received a database ID or version.
// It initializes the creation time and leaves the task incomplete.
func NewTaskUninitialized(
	title string,
	description *string,
	authorUserID int,
) Task {
	return NewTask(
		UninitializedID,
		UninitializedVersion,
		title,
		description,
		false,
		time.Now(),
		nil,
		authorUserID,
	)
}

// CompletionDuration returns the elapsed time between task creation and completion.
// It returns nil when the task is incomplete or has no completion timestamp.
func (t *Task) CompletionDuration() *time.Duration {
	if !t.Completed {
		return nil
	}

	if t.CompletedAt == nil {
		return nil
	}

	duration := t.CompletedAt.Sub(t.CreatedAt)

	return &duration
}

// Validate checks task field constraints and consistency between Completed and CompletedAt.
// Text length limits are measured in Unicode code points rather than UTF-8 bytes.
func (t *Task) Validate() error {
	titleLen := len([]rune(t.Title))
	if titleLen < 1 || titleLen > 100 {
		return fmt.Errorf(
			"invalid `Title` len: %d: %w",
			titleLen,
			core_errors.ErrInvalidArgument,
		)
	}

	if t.Description != nil {
		descriptionLen := len([]rune(*t.Description))
		if descriptionLen < 1 || descriptionLen > 1000 {
			return fmt.Errorf(
				"invalid `Description` len: %d: %w",
				descriptionLen,
				core_errors.ErrInvalidArgument,
			)
		}
	}

	if t.Completed {
		if t.CompletedAt == nil {
			return fmt.Errorf(
				"`CompletedAt` can't be `nil` if `Completed`==`true`: %w",
				core_errors.ErrInvalidArgument,
			)
		}

		if t.CompletedAt.Before(t.CreatedAt) {
			return fmt.Errorf(
				"`CompletedAt` can't be before `CreatedAt`: %w",
				core_errors.ErrInvalidArgument,
			)
		}
	} else {
		if t.CompletedAt != nil {
			return fmt.Errorf(
				"`CompletedAt` must be `nil` if `Completed`==`false`: %w",
				core_errors.ErrInvalidArgument,
			)
		}
	}

	return nil
}

// TaskPatch represents a partial task update. Nullable.Set distinguishes an omitted
// field from a field explicitly set to a value or nil.
type TaskPatch struct {
	Title       Nullable[string]
	Description Nullable[string]
	Completed   Nullable[bool]
}

// NewTaskPatch constructs a partial update without applying it to a Task.
func NewTaskPatch(
	title Nullable[string],
	description Nullable[string],
	completed Nullable[bool],
) TaskPatch {
	return TaskPatch{
		Title:       title,
		Description: description,
		Completed:   completed,
	}
}

// Validate rejects patch operations that would assign NULL to required task fields.
func (p *TaskPatch) Validate() error {
	if p.Title.Set && p.Title.Value == nil {
		return fmt.Errorf(
			"`Title` can't be patched to NULL: %w",
			core_errors.ErrInvalidArgument,
		)
	}

	if p.Completed.Set && p.Completed.Value == nil {
		return fmt.Errorf(
			"`Completed` can't be patched to NULL: %w",
			core_errors.ErrInvalidArgument,
		)
	}

	return nil
}

// ApplyPatch applies only fields marked as present, validates the resulting task,
// and updates the receiver only when the complete patch is valid.
func (t *Task) ApplyPatch(path TaskPatch) error {
	if err := path.Validate(); err != nil {
		return fmt.Errorf("validate task patch: %w", err)
	}

	tmp := *t

	if path.Title.Set {
		tmp.Title = *path.Title.Value
	}

	if path.Description.Set {
		tmp.Description = path.Description.Value
	}

	if path.Completed.Set {
		newCompleted := *path.Completed.Value

		if newCompleted != tmp.Completed {
			tmp.Completed = newCompleted

			if tmp.Completed {
				completedAt := time.Now()
				tmp.CompletedAt = &completedAt
			} else {
				tmp.CompletedAt = nil
			}
		}
	}

	if err := tmp.Validate(); err != nil {
		return fmt.Errorf("validate patched task: %w", err)
	}

	*t = tmp

	return nil
}
