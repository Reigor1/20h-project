package tasks_service

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Reigor1/20h-project/internal/core/domain"
	core_errors "github.com/Reigor1/20h-project/internal/core/errors"
	"go.opentelemetry.io/otel/trace/noop"
)

type tasksRepositoryStub struct {
	t *testing.T

	expectCreateTask bool
	createTaskResult domain.Task
	createTaskError  error
	createTaskCalls  int
	createdTask      domain.Task

	expectGetTasks bool
	getTasksResult []domain.Task
	getTasksError  error
	getTasksCalls  int
	getTasksUserID *int
	getTasksLimit  *int
	getTasksOffset *int

	expectGetTask bool
	getTaskResult domain.Task
	getTaskError  error
	getTaskCalls  int
	getTaskID     int

	expectDeleteTask bool
	deleteTaskError  error
	deleteTaskCalls  int
	deleteTaskID     int

	expectPatchTask bool
	patchTaskResult domain.Task
	patchTaskError  error
	patchTaskCalls  int
	patchTaskID     int
	patchTaskBefore domain.Task
	patchTaskAfter  domain.Task

	calls []string
}

func newTasksRepositoryStub(t *testing.T) *tasksRepositoryStub {
	t.Helper()
	return &tasksRepositoryStub{t: t}
}

func (r *tasksRepositoryStub) CreateTask(
	_ context.Context,
	task domain.Task,
) (domain.Task, error) {
	if !r.expectCreateTask {
		r.t.Fatal("unexpected TasksRepository.CreateTask call")
	}

	r.createTaskCalls++
	r.createdTask = task
	r.calls = append(r.calls, "CreateTask")

	return r.createTaskResult, r.createTaskError
}

func (r *tasksRepositoryStub) GetTasks(
	_ context.Context,
	userID *int,
	limit *int,
	offset *int,
) ([]domain.Task, error) {
	if !r.expectGetTasks {
		r.t.Fatal("unexpected TasksRepository.GetTasks call")
	}

	r.getTasksCalls++
	r.getTasksUserID = userID
	r.getTasksLimit = limit
	r.getTasksOffset = offset
	r.calls = append(r.calls, "GetTasks")

	return r.getTasksResult, r.getTasksError
}

func (r *tasksRepositoryStub) GetTask(
	_ context.Context,
	id int,
) (domain.Task, error) {
	if !r.expectGetTask {
		r.t.Fatal("unexpected TasksRepository.GetTask call")
	}

	r.getTaskCalls++
	r.getTaskID = id
	r.calls = append(r.calls, "GetTask")

	return r.getTaskResult, r.getTaskError
}

func (r *tasksRepositoryStub) DeleteTask(
	_ context.Context,
	id int,
) error {
	if !r.expectDeleteTask {
		r.t.Fatal("unexpected TasksRepository.DeleteTask call")
	}

	r.deleteTaskCalls++
	r.deleteTaskID = id
	r.calls = append(r.calls, "DeleteTask")

	return r.deleteTaskError
}

func (r *tasksRepositoryStub) PatchTask(
	_ context.Context,
	id int,
	before domain.Task,
	after domain.Task,
) (domain.Task, error) {
	if !r.expectPatchTask {
		r.t.Fatal("unexpected TasksRepository.PatchTask call")
	}

	r.patchTaskCalls++
	r.patchTaskID = id
	r.patchTaskBefore = before
	r.patchTaskAfter = after
	r.calls = append(r.calls, "PatchTask")

	return r.patchTaskResult, r.patchTaskError
}

func TestCreateTask(t *testing.T) {
	description := "Prepare a clear report"
	createdAt := time.Date(2026, 10, 20, 9, 30, 0, 0, time.UTC)
	inputTask := domain.NewTask(
		domain.UninitializedID,
		domain.UninitializedVersion,
		"Write report",
		&description,
		false,
		createdAt,
		nil,
		23,
	)
	createdTask := domain.NewTask(
		17,
		1,
		"Write report",
		&description,
		false,
		createdAt,
		nil,
		23,
	)
	repository := newTasksRepositoryStub(t)
	repository.expectCreateTask = true
	repository.createTaskResult = createdTask
	service := newTasksServiceForTest(repository)

	got, err := service.CreateTask(context.Background(), inputTask)
	if err != nil {
		t.Fatalf("CreateTask() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(got, createdTask) {
		t.Fatalf("CreateTask() = %+v, want %+v", got, createdTask)
	}
	if repository.createTaskCalls != 1 || !reflect.DeepEqual(repository.createdTask, inputTask) {
		t.Fatalf(
			"repository CreateTask() calls/task = %d/%+v, want 1/%+v",
			repository.createTaskCalls,
			repository.createdTask,
			inputTask,
		)
	}
}

func TestCreateTaskRejectsInvalidTask(t *testing.T) {
	validCreatedAt := time.Date(2026, 10, 20, 9, 30, 0, 0, time.UTC)
	emptyTitleTask := domain.NewTask(0, 0, "", nil, false, validCreatedAt, nil, 23)
	completedWithoutTimestamp := domain.NewTask(0, 0, "Task", nil, true, validCreatedAt, nil, 23)
	tests := []struct {
		name string
		task domain.Task
	}{
		{
			name: "empty title",
			task: emptyTitleTask,
		},
		{
			name: "completed task without completion time",
			task: completedWithoutTimestamp,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			repository := newTasksRepositoryStub(t)
			service := newTasksServiceForTest(repository)

			got, err := service.CreateTask(context.Background(), testCase.task)

			if !errors.Is(err, core_errors.ErrInvalidArgument) {
				t.Fatalf("CreateTask() error = %v, want ErrInvalidArgument", err)
			}
			if !reflect.DeepEqual(got, domain.Task{}) {
				t.Fatalf("CreateTask() task = %+v, want zero value", got)
			}
			if repository.createTaskCalls != 0 {
				t.Fatalf("repository CreateTask() calls = %d, want 0 for invalid task", repository.createTaskCalls)
			}
		})
	}
}

func TestCreateTaskReturnsRepositoryError(t *testing.T) {
	repositoryError := errors.New("storage unavailable")
	createdAt := time.Date(2026, 10, 20, 9, 30, 0, 0, time.UTC)
	inputTask := domain.NewTask(0, 0, "Write report", nil, false, createdAt, nil, 23)
	repository := newTasksRepositoryStub(t)
	repository.expectCreateTask = true
	repository.createTaskError = repositoryError
	service := newTasksServiceForTest(repository)

	got, err := service.CreateTask(context.Background(), inputTask)

	if !errors.Is(err, repositoryError) {
		t.Fatalf("CreateTask() error = %v, want wrapped repository error", err)
	}
	if !reflect.DeepEqual(got, domain.Task{}) {
		t.Fatalf("CreateTask() task = %+v, want zero value", got)
	}
	if repository.createTaskCalls != 1 || !reflect.DeepEqual(repository.createdTask, inputTask) {
		t.Fatalf("repository CreateTask() calls/task = %d/%+v, want 1/%+v", repository.createTaskCalls, repository.createdTask, inputTask)
	}
}

func TestGetTask(t *testing.T) {
	wantTask := sampleTask()
	repository := newTasksRepositoryStub(t)
	repository.expectGetTask = true
	repository.getTaskResult = wantTask
	service := newTasksServiceForTest(repository)

	got, err := service.GetTask(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetTask() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(got, wantTask) {
		t.Fatalf("GetTask() = %+v, want %+v", got, wantTask)
	}
	if repository.getTaskCalls != 1 || repository.getTaskID != 42 {
		t.Fatalf("repository GetTask() calls/id = %d/%d, want 1/42", repository.getTaskCalls, repository.getTaskID)
	}
}

func TestGetTaskReturnsRepositoryError(t *testing.T) {
	repositoryError := core_errors.ErrNotFound
	repository := newTasksRepositoryStub(t)
	repository.expectGetTask = true
	repository.getTaskError = repositoryError
	service := newTasksServiceForTest(repository)

	got, err := service.GetTask(context.Background(), 42)

	if !errors.Is(err, repositoryError) {
		t.Fatalf("GetTask() error = %v, want wrapped ErrNotFound", err)
	}
	if !reflect.DeepEqual(got, domain.Task{}) {
		t.Fatalf("GetTask() task = %+v, want zero value", got)
	}
	if repository.getTaskCalls != 1 || repository.getTaskID != 42 {
		t.Fatalf("repository GetTask() calls/id = %d/%d, want 1/42", repository.getTaskCalls, repository.getTaskID)
	}
}

func TestGetTasksPassesFiltersAndPaginationToRepository(t *testing.T) {
	userID := 23
	limit := 20
	offset := 40
	wantTasks := []domain.Task{sampleTask(), sampleTaskWithID(43)}
	repository := newTasksRepositoryStub(t)
	repository.expectGetTasks = true
	repository.getTasksResult = wantTasks
	service := newTasksServiceForTest(repository)

	got, err := service.GetTasks(context.Background(), &userID, &limit, &offset)
	if err != nil {
		t.Fatalf("GetTasks() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(got, wantTasks) {
		t.Fatalf("GetTasks() = %+v, want %+v", got, wantTasks)
	}
	if repository.getTasksCalls != 1 ||
		repository.getTasksUserID != &userID ||
		repository.getTasksLimit != &limit ||
		repository.getTasksOffset != &offset {
		t.Fatalf(
			"repository GetTasks() calls/arguments = %d/%p/%p/%p, want 1/%p/%p/%p",
			repository.getTasksCalls,
			repository.getTasksUserID,
			repository.getTasksLimit,
			repository.getTasksOffset,
			&userID,
			&limit,
			&offset,
		)
	}
}

func TestGetTasksAllowsUnsetPaginationAndEmptyResults(t *testing.T) {
	repository := newTasksRepositoryStub(t)
	repository.expectGetTasks = true
	repository.getTasksResult = []domain.Task{}
	service := newTasksServiceForTest(repository)

	got, err := service.GetTasks(context.Background(), nil, nil, nil)
	if err != nil {
		t.Fatalf("GetTasks() error = %v, want nil", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("GetTasks() = %v, want empty non-nil slice", got)
	}
	if repository.getTasksCalls != 1 ||
		repository.getTasksUserID != nil ||
		repository.getTasksLimit != nil ||
		repository.getTasksOffset != nil {
		t.Fatalf(
			"repository GetTasks() calls/arguments = %d/%v/%v/%v, want 1/nil/nil/nil",
			repository.getTasksCalls,
			repository.getTasksUserID,
			repository.getTasksLimit,
			repository.getTasksOffset,
		)
	}
}

func TestGetTasksRejectsNegativePagination(t *testing.T) {
	negativeValue := -1
	tests := []struct {
		name   string
		limit  *int
		offset *int
	}{
		{
			name:  "negative limit",
			limit: &negativeValue,
		},
		{
			name:   "negative offset",
			offset: &negativeValue,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			repository := newTasksRepositoryStub(t)
			service := newTasksServiceForTest(repository)

			got, err := service.GetTasks(
				context.Background(),
				nil,
				testCase.limit,
				testCase.offset,
			)

			if !errors.Is(err, core_errors.ErrInvalidArgument) {
				t.Fatalf("GetTasks() error = %v, want ErrInvalidArgument", err)
			}
			if got != nil {
				t.Fatalf("GetTasks() = %v, want nil", got)
			}
			if repository.getTasksCalls != 0 {
				t.Fatalf("repository GetTasks() calls = %d, want 0 for negative pagination", repository.getTasksCalls)
			}
		})
	}
}

func TestGetTasksReturnsRepositoryError(t *testing.T) {
	repositoryError := errors.New("query failed")
	repository := newTasksRepositoryStub(t)
	repository.expectGetTasks = true
	repository.getTasksError = repositoryError
	service := newTasksServiceForTest(repository)

	got, err := service.GetTasks(context.Background(), nil, nil, nil)

	if !errors.Is(err, repositoryError) {
		t.Fatalf("GetTasks() error = %v, want wrapped repository error", err)
	}
	if got != nil {
		t.Fatalf("GetTasks() = %v, want nil", got)
	}
	if repository.getTasksCalls != 1 {
		t.Fatalf("repository GetTasks() calls = %d, want 1", repository.getTasksCalls)
	}
}

func TestPathTaskAppliesPatchBeforeRepositoryUpdate(t *testing.T) {
	oldDescription := "Review the draft"
	updatedTitle := "Publish report"
	createdAt := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	completedAt := createdAt.Add(time.Hour)
	initialTask := domain.NewTask(42, 3, "Write report", &oldDescription, true, createdAt, &completedAt, 23)
	updatedTask := domain.NewTask(42, 3, updatedTitle, nil, false, createdAt, nil, 23)
	patchedTask := domain.NewTask(42, 4, updatedTitle, nil, false, createdAt, nil, 23)
	patch := domain.NewTaskPatch(
		domain.Nullable[string]{Value: &updatedTitle, Set: true},
		domain.Nullable[string]{Set: true},
		domain.Nullable[bool]{Value: boolPointer(false), Set: true},
	)
	repository := newTasksRepositoryStub(t)
	repository.expectGetTask = true
	repository.getTaskResult = initialTask
	repository.expectPatchTask = true
	repository.patchTaskResult = patchedTask
	service := newTasksServiceForTest(repository)

	got, err := service.PathTask(context.Background(), 42, patch)
	if err != nil {
		t.Fatalf("PathTask() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(got, patchedTask) {
		t.Fatalf("PathTask() = %+v, want %+v", got, patchedTask)
	}
	if !reflect.DeepEqual(repository.calls, []string{"GetTask", "PatchTask"}) {
		t.Fatalf("repository calls = %v, want [GetTask PatchTask]", repository.calls)
	}
	if repository.getTaskCalls != 1 || repository.getTaskID != 42 {
		t.Fatalf("repository GetTask() calls/id = %d/%d, want 1/42", repository.getTaskCalls, repository.getTaskID)
	}
	if repository.patchTaskCalls != 1 || repository.patchTaskID != 42 {
		t.Fatalf("repository PatchTask() calls/id = %d/%d, want 1/42", repository.patchTaskCalls, repository.patchTaskID)
	}
	if !reflect.DeepEqual(repository.patchTaskBefore, initialTask) {
		t.Fatalf("repository PatchTask() before = %+v, want %+v", repository.patchTaskBefore, initialTask)
	}
	if !reflect.DeepEqual(repository.patchTaskAfter, updatedTask) {
		t.Fatalf("repository PatchTask() after = %+v, want %+v", repository.patchTaskAfter, updatedTask)
	}
}

func TestPathTaskDoesNotCallPatchTaskForEmptyPatch(t *testing.T) {
	initialTask := sampleTask()
	repository := newTasksRepositoryStub(t)
	repository.expectGetTask = true
	repository.getTaskResult = initialTask
	service := newTasksServiceForTest(repository)

	got, err := service.PathTask(context.Background(), initialTask.ID, domain.TaskPatch{})
	if err != nil {
		t.Fatalf("PathTask() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(got, initialTask) {
		t.Fatalf("PathTask() = %+v, want unchanged task %+v", got, initialTask)
	}
	if !reflect.DeepEqual(repository.calls, []string{"GetTask"}) {
		t.Fatalf("repository calls = %v, want [GetTask]", repository.calls)
	}
	if repository.patchTaskCalls != 0 {
		t.Fatalf("repository PatchTask() calls = %d, want 0 for empty patch", repository.patchTaskCalls)
	}
}

func TestPathTaskCompletesTaskAndPreservesUnchangedFields(t *testing.T) {
	createdAt := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	initialTask := domain.NewTask(42, 7, "Complete report", nil, false, createdAt, nil, 23)
	title := "Publish report"
	description := "Send the final report to the team"
	patch := domain.NewTaskPatch(
		domain.Nullable[string]{Value: &title, Set: true},
		domain.Nullable[string]{Value: &description, Set: true},
		domain.Nullable[bool]{Value: boolPointer(true), Set: true},
	)
	repository := newTasksRepositoryStub(t)
	repository.expectGetTask = true
	repository.getTaskResult = initialTask
	repository.expectPatchTask = true
	repository.patchTaskResult = initialTask
	service := newTasksServiceForTest(repository)

	_, err := service.PathTask(context.Background(), initialTask.ID, patch)
	if err != nil {
		t.Fatalf("PathTask() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(repository.patchTaskBefore, initialTask) {
		t.Fatalf("repository PatchTask() before = %+v, want %+v", repository.patchTaskBefore, initialTask)
	}

	updatedTask := repository.patchTaskAfter
	if updatedTask.ID != initialTask.ID ||
		updatedTask.Version != initialTask.Version ||
		updatedTask.Title != title ||
		updatedTask.Description == nil ||
		*updatedTask.Description != description ||
		!updatedTask.Completed ||
		updatedTask.CompletedAt == nil ||
		updatedTask.CompletedAt.Before(createdAt) ||
		!updatedTask.CreatedAt.Equal(createdAt) ||
		updatedTask.AuthorUserID != initialTask.AuthorUserID {
		t.Fatalf("repository PatchTask() after = %+v, want requested changes and preserved fields", updatedTask)
	}
	if !reflect.DeepEqual(repository.calls, []string{"GetTask", "PatchTask"}) {
		t.Fatalf("repository calls = %v, want [GetTask PatchTask]", repository.calls)
	}
}

func TestPathTaskReturnsGetErrorWithoutUpdating(t *testing.T) {
	repositoryError := core_errors.ErrNotFound
	repository := newTasksRepositoryStub(t)
	repository.expectGetTask = true
	repository.getTaskError = repositoryError
	service := newTasksServiceForTest(repository)

	got, err := service.PathTask(context.Background(), 42, domain.TaskPatch{})

	if !errors.Is(err, repositoryError) {
		t.Fatalf("PathTask() error = %v, want wrapped ErrNotFound", err)
	}
	if !reflect.DeepEqual(got, domain.Task{}) {
		t.Fatalf("PathTask() task = %+v, want zero value", got)
	}
	if !reflect.DeepEqual(repository.calls, []string{"GetTask"}) {
		t.Fatalf("repository calls = %v, want [GetTask]", repository.calls)
	}
}

func TestPathTaskRejectsInvalidPatchWithoutUpdating(t *testing.T) {
	initialTask := sampleTask()
	repository := newTasksRepositoryStub(t)
	repository.expectGetTask = true
	repository.getTaskResult = initialTask
	service := newTasksServiceForTest(repository)
	patch := domain.NewTaskPatch(
		domain.Nullable[string]{Set: true},
		domain.Nullable[string]{},
		domain.Nullable[bool]{},
	)

	got, err := service.PathTask(context.Background(), initialTask.ID, patch)

	if !errors.Is(err, core_errors.ErrInvalidArgument) {
		t.Fatalf("PathTask() error = %v, want ErrInvalidArgument", err)
	}
	if !reflect.DeepEqual(got, domain.Task{}) {
		t.Fatalf("PathTask() task = %+v, want zero value", got)
	}
	if !reflect.DeepEqual(repository.calls, []string{"GetTask"}) {
		t.Fatalf("repository calls = %v, want [GetTask]", repository.calls)
	}
}

func TestPathTaskReturnsPatchRepositoryError(t *testing.T) {
	repositoryError := core_errors.ErrConflict
	initialTask := sampleTask()
	title := "Updated title"
	patch := domain.NewTaskPatch(
		domain.Nullable[string]{Value: &title, Set: true},
		domain.Nullable[string]{},
		domain.Nullable[bool]{},
	)
	repository := newTasksRepositoryStub(t)
	repository.expectGetTask = true
	repository.getTaskResult = initialTask
	repository.expectPatchTask = true
	repository.patchTaskError = repositoryError
	service := newTasksServiceForTest(repository)

	got, err := service.PathTask(context.Background(), initialTask.ID, patch)

	if !errors.Is(err, repositoryError) {
		t.Fatalf("PathTask() error = %v, want wrapped ErrConflict", err)
	}
	if !reflect.DeepEqual(got, domain.Task{}) {
		t.Fatalf("PathTask() task = %+v, want zero value", got)
	}
	if !reflect.DeepEqual(repository.calls, []string{"GetTask", "PatchTask"}) {
		t.Fatalf("repository calls = %v, want [GetTask PatchTask]", repository.calls)
	}
	wantUpdatedTask := initialTask
	wantUpdatedTask.Title = title
	if !reflect.DeepEqual(repository.patchTaskBefore, initialTask) {
		t.Fatalf("repository PatchTask() before = %+v, want %+v", repository.patchTaskBefore, initialTask)
	}
	if !reflect.DeepEqual(repository.patchTaskAfter, wantUpdatedTask) {
		t.Fatalf("repository PatchTask() after = %+v, want %+v", repository.patchTaskAfter, wantUpdatedTask)
	}
}

func TestDeleteTask(t *testing.T) {
	repository := newTasksRepositoryStub(t)
	repository.expectDeleteTask = true
	service := newTasksServiceForTest(repository)

	err := service.DeleteTask(context.Background(), 42)
	if err != nil {
		t.Fatalf("DeleteTask() error = %v, want nil", err)
	}
	if repository.deleteTaskCalls != 1 || repository.deleteTaskID != 42 {
		t.Fatalf("repository DeleteTask() calls/id = %d/%d, want 1/42", repository.deleteTaskCalls, repository.deleteTaskID)
	}
}

func TestDeleteTaskReturnsRepositoryError(t *testing.T) {
	repositoryError := core_errors.ErrNotFound
	repository := newTasksRepositoryStub(t)
	repository.expectDeleteTask = true
	repository.deleteTaskError = repositoryError
	service := newTasksServiceForTest(repository)

	err := service.DeleteTask(context.Background(), 42)

	if !errors.Is(err, repositoryError) {
		t.Fatalf("DeleteTask() error = %v, want wrapped ErrNotFound", err)
	}
	if repository.deleteTaskCalls != 1 || repository.deleteTaskID != 42 {
		t.Fatalf("repository DeleteTask() calls/id = %d/%d, want 1/42", repository.deleteTaskCalls, repository.deleteTaskID)
	}
}

func sampleTask() domain.Task {
	return sampleTaskWithID(42)
}

func sampleTaskWithID(id int) domain.Task {
	description := "Review the submitted work"
	createdAt := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	completedAt := createdAt.Add(time.Hour)

	return domain.NewTask(
		id,
		3,
		"Complete report",
		&description,
		true,
		createdAt,
		&completedAt,
		23,
	)
}

func boolPointer(value bool) *bool {
	return &value
}

func newTasksServiceForTest(repository TasksRepository) *TasksService {
	return NewTasksService(repository, noop.NewTracerProvider())
}
