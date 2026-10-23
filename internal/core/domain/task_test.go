package domain

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	core_errors "github.com/Reigor1/20h-project/internal/core/errors"
)

func TestTaskCompletionDuration(t *testing.T) {
	createdAt := time.Date(2026, time.January, 1, 10, 0, 0, 0, time.UTC)
	completedAt := createdAt.Add(90 * time.Minute)

	tests := []struct {
		name         string
		task         Task
		wantDuration *time.Duration
	}{
		{
			name: "completed task returns elapsed duration",
			task: Task{
				Completed:   true,
				CreatedAt:   createdAt,
				CompletedAt: &completedAt,
			},
			wantDuration: durationPointer(90 * time.Minute),
		},
		{
			name: "incomplete task has no duration",
			task: Task{
				CreatedAt: createdAt,
			},
		},
		{
			name: "completed task without completion time has no duration",
			task: Task{
				Completed: true,
				CreatedAt: createdAt,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDuration := tt.task.CompletionDuration()
			if tt.wantDuration == nil {
				if gotDuration != nil {
					t.Fatalf("CompletionDuration() = %v, want nil", *gotDuration)
				}
				return
			}
			if gotDuration == nil || *gotDuration != *tt.wantDuration {
				t.Fatalf("CompletionDuration() = %v, want %v", gotDuration, *tt.wantDuration)
			}
		})
	}
}

func TestTaskValidate(t *testing.T) {
	createdAt := time.Date(2026, time.January, 1, 10, 0, 0, 0, time.UTC)
	completedAt := createdAt.Add(time.Hour)
	completionBeforeCreation := createdAt.Add(-time.Second)
	emptyDescription := ""
	minimumLengthTitle := "Я"
	maximumLengthTitle := strings.Repeat("Я", 100)
	overlongTitle := strings.Repeat("Я", 101)

	minimumLengthDescription := "Я"
	maximumLengthDescription := strings.Repeat("Я", 1000)
	overlongDescription := strings.Repeat("Я", 1001)

	tests := []struct {
		name    string
		task    Task
		wantErr bool
	}{
		{
			name: "valid incomplete task",
			task: Task{
				Title:     "Task",
				CreatedAt: createdAt,
			},
		},
		{
			name: "valid completed task",
			task: Task{
				Title:       "Task",
				Completed:   true,
				CreatedAt:   createdAt,
				CompletedAt: &completedAt,
			},
		},
		{
			name: "emptyDescription title",
			task: Task{
				CreatedAt: createdAt,
			},
			wantErr: true,
		},
		{
			name: "empty description",
			task: Task{
				Title:       "Task",
				Description: &emptyDescription,
				CreatedAt:   createdAt,
			},
			wantErr: true,
		},
		{
			name: "completed task missing completion time",
			task: Task{
				Title:     "Task",
				Completed: true,
				CreatedAt: createdAt,
			},
			wantErr: true,
		},
		{
			name: "completion time before creation",
			task: Task{
				Title:       "Task",
				Completed:   true,
				CreatedAt:   createdAt,
				CompletedAt: &completionBeforeCreation,
			},
			wantErr: true,
		},
		{
			name: "title with minimum length",
			task: Task{
				Title:     minimumLengthTitle,
				CreatedAt: createdAt,
			},
		},
		{
			name: "title with maximum length",
			task: Task{
				Title:     maximumLengthTitle,
				CreatedAt: createdAt,
			},
		},
		{
			name: "title exceeds maximum length",
			task: Task{
				Title:     overlongTitle,
				CreatedAt: createdAt,
			},
			wantErr: true,
		},
		{
			name: "description with minimum length",
			task: Task{
				Title:       "Task",
				Description: &minimumLengthDescription,
				CreatedAt:   createdAt,
			},
		},
		{
			name: "description with maximum length",
			task: Task{
				Title:       "Task",
				Description: &maximumLengthDescription,
				CreatedAt:   createdAt,
			},
		},
		{
			name: "description exceeds maximum length",
			task: Task{
				Title:       "Task",
				Description: &overlongDescription,
				CreatedAt:   createdAt,
			},
			wantErr: true,
		},
		{
			name: "completed task with completion time equal to creation time",
			task: Task{
				Title:       "Task",
				Completed:   true,
				CreatedAt:   createdAt,
				CompletedAt: &createdAt,
			},
		},
		{
			name: "incomplete task with completion time",
			task: Task{
				Title:       "Task",
				CreatedAt:   createdAt,
				CompletedAt: &completedAt,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.task.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %t", err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, core_errors.ErrInvalidArgument) {
				t.Fatalf("Validate() error = %v, want ErrInvalidArgument", err)
			}
		})
	}
}

func TestTaskPatchValidate(t *testing.T) {
	updatedTitle := "Updated title"
	completedState := true

	tests := []struct {
		name    string
		patch   TaskPatch
		wantErr bool
	}{
		{
			name: "valid field updates",
			patch: TaskPatch{
				Title: Nullable[string]{
					Value: &updatedTitle,
					Set:   true,
				},
			},
		},
		{
			name: "description can be cleared",
			patch: TaskPatch{
				Description: Nullable[string]{
					Set: true,
				},
			},
		},
		{
			name: "title cannot be cleared",
			patch: TaskPatch{
				Title: Nullable[string]{
					Set: true,
				},
			},
			wantErr: true,
		},
		{
			name: "completion state cannot be null",
			patch: TaskPatch{
				Completed: Nullable[bool]{
					Set: true,
				},
			},
			wantErr: true,
		},
		{
			name: "completion state can be set",
			patch: TaskPatch{
				Completed: Nullable[bool]{
					Value: &completedState,
					Set:   true,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.patch.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %t", err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, core_errors.ErrInvalidArgument) {
				t.Fatalf("Validate() error = %v, want ErrInvalidArgument", err)
			}
		})
	}
}

func TestTaskApplyPatch(t *testing.T) {
	createdAt := time.Date(2026, time.January, 1, 10, 0, 0, 0, time.UTC)
	originalDescription := "Old description"
	updatedTitle := "Updated title"
	completedState := true
	emptyTitle := ""

	updatedDescription := "New description"
	emptyDescription := ""
	incomplete := false

	initialCompletedAt := createdAt.Add(time.Hour)

	tooLongTitle := strings.Repeat("T", 101)
	tooLongDescription := strings.Repeat("D", 1001)

	tests := []struct {
		name            string
		initialTask     *Task
		patch           TaskPatch
		wantTitle       string
		wantDescription *string
		wantCompleted   bool
		wantCompletedAt *time.Time
		wantErr         bool
	}{
		{
			name: "updates title clears description and completes task",
			patch: TaskPatch{
				Title: Nullable[string]{
					Value: &updatedTitle,
					Set:   true,
				},
				Description: Nullable[string]{
					Set: true,
				},
				Completed: Nullable[bool]{
					Value: &completedState,
					Set:   true,
				},
			},
			wantTitle:     updatedTitle,
			wantCompleted: true,
		},
		{
			name: "invalid patch leaves task unchanged",
			patch: TaskPatch{
				Title: Nullable[string]{
					Value: &emptyTitle,
					Set:   true,
				},
			},
			wantTitle:       "Original title",
			wantDescription: &originalDescription,
			wantErr:         true,
		},
		{
			name:            "empty patch leaves task unchanged",
			patch:           TaskPatch{},
			wantTitle:       "Original title",
			wantDescription: &originalDescription,
		},
		{
			name: "updates title only",
			patch: TaskPatch{
				Title: Nullable[string]{
					Value: &updatedTitle,
					Set:   true,
				},
			},
			wantTitle:       updatedTitle,
			wantDescription: &originalDescription,
		},
		{
			name: "updates description only",
			patch: TaskPatch{
				Description: Nullable[string]{
					Value: &updatedDescription,
					Set:   true,
				},
			},
			wantTitle:       "Original title",
			wantDescription: &updatedDescription,
		},
		{
			name: "clears description",
			patch: TaskPatch{
				Description: Nullable[string]{
					Set: true,
				},
			},
			wantTitle: "Original title",
		},
		{
			name: "completes task",
			patch: TaskPatch{
				Completed: Nullable[bool]{
					Value: &completedState,
					Set:   true,
				},
			},
			wantTitle:       "Original title",
			wantDescription: &originalDescription,
			wantCompleted:   true,
		},
		{
			name: "reopens completed task",
			initialTask: &Task{
				Title:       "Original title",
				Description: &originalDescription,
				Completed:   true,
				CreatedAt:   createdAt,
				CompletedAt: &initialCompletedAt,
			},
			patch: TaskPatch{
				Completed: Nullable[bool]{
					Value: &incomplete,
					Set:   true,
				},
			},
			wantTitle:       "Original title",
			wantDescription: &originalDescription,
		},
		{
			name: "title update preserves completion state",
			initialTask: &Task{
				Title:       "Original title",
				Description: &originalDescription,
				Completed:   true,
				CreatedAt:   createdAt,
				CompletedAt: &initialCompletedAt,
			},
			patch: TaskPatch{
				Title: Nullable[string]{
					Value: &updatedTitle,
					Set:   true,
				},
			},
			wantTitle:       updatedTitle,
			wantDescription: &originalDescription,
			wantCompleted:   true,
			wantCompletedAt: &initialCompletedAt,
		},
		{
			name: "unset fields are ignored",
			patch: TaskPatch{
				Title: Nullable[string]{
					Value: &updatedTitle,
					Set:   false,
				},
			},
			wantTitle:       "Original title",
			wantDescription: &originalDescription,
		},
		{
			name: "title cannot be patched to nil",
			patch: TaskPatch{
				Title: Nullable[string]{
					Set: true,
				},
			},
			wantTitle:       "Original title",
			wantDescription: &originalDescription,
			wantErr:         true,
		},
		{
			name: "completion state cannot be patched to nil",
			patch: TaskPatch{
				Completed: Nullable[bool]{
					Set: true,
				},
			},
			wantTitle:       "Original title",
			wantDescription: &originalDescription,
			wantErr:         true,
		},
		{
			name: "empty description is rejected",
			patch: TaskPatch{
				Description: Nullable[string]{
					Value: &emptyDescription,
					Set:   true,
				},
			},
			wantTitle:       "Original title",
			wantDescription: &originalDescription,
			wantErr:         true,
		},
		{
			name: "title exceeding maximum length is rejected",
			patch: TaskPatch{
				Title: Nullable[string]{
					Value: &tooLongTitle,
					Set:   true,
				},
			},
			wantTitle:       "Original title",
			wantDescription: &originalDescription,
			wantErr:         true,
		},
		{
			name: "description exceeding maximum length is rejected",
			patch: TaskPatch{
				Description: Nullable[string]{
					Value: &tooLongDescription,
					Set:   true,
				},
			},
			wantTitle:       "Original title",
			wantDescription: &originalDescription,
			wantErr:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := Task{
				Title:       "Original title",
				Description: &originalDescription,
				CreatedAt:   createdAt,
			}

			if tt.initialTask != nil {
				task = *tt.initialTask
			}

			original := task

			if task.Description != nil {
				description := *task.Description
				original.Description = &description
			}

			if task.CompletedAt != nil {
				completedAt := *task.CompletedAt
				original.CompletedAt = &completedAt
			}

			err := task.ApplyPatch(tt.patch)

			if (err != nil) != tt.wantErr {
				t.Fatalf(
					"ApplyPatch() error = %v, wantErr %t",
					err,
					tt.wantErr,
				)
			}

			if tt.wantErr && !reflect.DeepEqual(task, original) {
				t.Fatalf(
					"ApplyPatch() modified task on error: got %+v, want %+v",
					task,
					original,
				)
			}

			if tt.wantErr && !errors.Is(err, core_errors.ErrInvalidArgument) {
				t.Fatalf(
					"ApplyPatch() error = %v, want ErrInvalidArgument",
					err,
				)
			}

			if task.Title != tt.wantTitle || task.Completed != tt.wantCompleted {
				t.Fatalf(
					"ApplyPatch() task = {Title: %q, Completed: %t}, want {Title: %q, Completed: %t}",
					task.Title,
					task.Completed,
					tt.wantTitle,
					tt.wantCompleted,
				)
			}

			if !sameStringPointerValue(task.Description, tt.wantDescription) {
				t.Fatalf(
					"ApplyPatch() Description = %v, want %v",
					task.Description,
					tt.wantDescription,
				)
			}

			if tt.wantCompleted {
				if task.CompletedAt == nil ||
					task.CompletedAt.Before(task.CreatedAt) {
					t.Fatalf(
						"ApplyPatch() CompletedAt = %v, want non-nil time at or after creation",
						task.CompletedAt,
					)
				}
			} else if task.CompletedAt != nil {
				t.Fatalf(
					"ApplyPatch() CompletedAt = %v, want nil",
					task.CompletedAt,
				)
			}

			if tt.wantCompletedAt != nil {
				if task.CompletedAt == nil ||
					!task.CompletedAt.Equal(*tt.wantCompletedAt) {
					t.Fatalf(
						"ApplyPatch() CompletedAt = %v, want %v",
						task.CompletedAt,
						*tt.wantCompletedAt,
					)
				}
			}
		})
	}
}

func TestNewTaskUninitialized(t *testing.T) {
	description := "Details"
	task := NewTaskUninitialized("Task", &description, 42)

	if task.ID != UninitializedID || task.Version != UninitializedVersion {
		t.Fatalf(
			"NewTaskUninitialized() ID/version = %d/%d, want %d/%d",
			task.ID,
			task.Version,
			UninitializedID,
			UninitializedVersion,
		)
	}
	if task.Title != "Task" ||
		!sameStringPointerValue(task.Description, &description) ||
		task.AuthorUserID != 42 {
		t.Fatalf("NewTaskUninitialized() fields = %+v, want input title, description, and author", task)
	}
	if task.Completed || task.CompletedAt != nil {
		t.Fatalf(
			"NewTaskUninitialized() completion state = %t/%v, want false/nil",
			task.Completed,
			task.CompletedAt,
		)
	}
	if task.CreatedAt.IsZero() {
		t.Fatal("NewTaskUninitialized() CreatedAt is zero, want initialized timestamp")
	}
}

func durationPointer(value time.Duration) *time.Duration { return &value }
