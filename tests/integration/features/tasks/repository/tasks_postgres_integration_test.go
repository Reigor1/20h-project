package repository_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Reigor1/20h-project/internal/core/domain"
	core_errors "github.com/Reigor1/20h-project/internal/core/errors"
	core_postgres_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool"
	core_pgx_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool/pgx"
	tasks_postgres_repository "github.com/Reigor1/20h-project/internal/features/tasks/repository/postgres"
	"github.com/Reigor1/20h-project/internal/testutil"
)

func TestTasksRepositoryIntegration(t *testing.T) {
	config := testutil.PostgresTestConfig(t)
	if !strings.HasSuffix(strings.ToLower(config.Database), "_test") {
		t.Fatalf("refusing to modify database %q: test database name must end in _test", config.Database)
	}

	connectionContext, cancelConnection := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelConnection()
	pool, err := core_pgx_pool.NewPool(connectionContext, config)
	if err != nil {
		t.Fatalf("connect to configured PostgreSQL test database: %v", err)
	}
	t.Cleanup(pool.Close)

	ctx, cancelOperations := context.WithTimeout(context.Background(), time.Minute)
	defer cancelOperations()
	assertConnectedTestDatabase(t, ctx, pool, config.Database)
	testutil.PreparePostgresSchema(t, ctx, pool, "testdata/tasks_schema.sql")
	repository := tasks_postgres_repository.NewTasksRepository(pool)

	t.Run("CreateTask", func(t *testing.T) {
		createdAt := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
		completedAt := time.Date(2026, 10, 20, 10, 15, 0, 0, time.UTC)
		description := "Prepare the final report"
		tests := []struct {
			name            string
			title           string
			description     *string
			completed       bool
			completedAt     *time.Time
			wantCompletedAt *time.Time
		}{
			{
				name:            "creates completed task with description",
				title:           "Write report",
				description:     &description,
				completed:       true,
				completedAt:     &completedAt,
				wantCompletedAt: &completedAt,
			},
			{
				name:  "creates unfinished task with nullable fields",
				title: "Review inbox",
			},
		}

		for _, testCase := range tests {
			t.Run(testCase.name, func(t *testing.T) {
				authorID := insertTaskAuthor(t, ctx, pool)
				inputTask := domain.NewTask(
					domain.UninitializedID,
					domain.UninitializedVersion,
					testCase.title,
					testCase.description,
					testCase.completed,
					createdAt,
					testCase.completedAt,
					authorID,
				)

				createdTask, err := repository.CreateTask(ctx, inputTask)
				if err != nil {
					t.Fatalf("CreateTask() error = %v, want nil", err)
				}
				registerTaskCleanup(t, ctx, pool, createdTask.ID)

				if createdTask.ID <= 0 || createdTask.ID == domain.UninitializedID {
					t.Fatalf("CreateTask() ID = %d, want database-generated ID", createdTask.ID)
				}
				if createdTask.Version != 1 {
					t.Fatalf("CreateTask() version = %d, want database default 1", createdTask.Version)
				}
				wantTask := domain.NewTask(
					createdTask.ID,
					1,
					testCase.title,
					testCase.description,
					testCase.completed,
					createdAt,
					testCase.wantCompletedAt,
					authorID,
				)
				assertTaskEqual(t, "CreateTask()", createdTask, wantTask)

				storedTask := readStoredTask(t, ctx, pool, createdTask.ID)
				assertTaskEqual(t, "stored task", storedTask, wantTask)
			})
		}

		t.Run("maps a missing author foreign key to not found", func(t *testing.T) {
			authorID := insertTaskAuthor(t, ctx, pool)
			deleteTaskAuthorDirectly(t, ctx, pool, authorID)
			tasksBeforeInsert := countTasks(t, ctx, pool)
			createdAt := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
			inputTask := domain.NewTask(0, 0, "Missing author", nil, false, createdAt, nil, authorID)

			_, err := repository.CreateTask(ctx, inputTask)

			if !errors.Is(err, core_errors.ErrNotFound) {
				t.Fatalf("CreateTask() error = %v, want ErrNotFound", err)
			}
			if !strings.Contains(err.Error(), "SQLSTATE 23503") {
				t.Fatalf("CreateTask() error = %v, want foreign key SQLSTATE 23503", err)
			}
			if tasksAfterInsert := countTasks(t, ctx, pool); tasksAfterInsert != tasksBeforeInsert {
				t.Fatalf("task count after rejected insert = %d, want unchanged count %d", tasksAfterInsert, tasksBeforeInsert)
			}
		})

		t.Run("database constraints reject invalid fields", func(t *testing.T) {
			tooLongTitle := strings.Repeat("t", 101)
			tooLongDescription := strings.Repeat("d", 1001)
			createdAt := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
			earlierCompletion := createdAt.Add(-time.Minute)
			tests := []struct {
				name         string
				title        string
				description  *string
				completed    bool
				completedAt  *time.Time
				wantSQLState string
			}{
				{
					name:         "empty title violates check constraint",
					title:        "",
					wantSQLState: "23514",
				},
				{
					name:         "title exceeds varchar length",
					title:        tooLongTitle,
					wantSQLState: "22001",
				},
				{
					name:         "empty description violates check constraint",
					title:        "Valid title",
					description:  stringPointer(""),
					wantSQLState: "23514",
				},
				{
					name:         "description exceeds varchar length",
					title:        "Valid title",
					description:  &tooLongDescription,
					wantSQLState: "22001",
				},
				{
					name:         "incomplete task has completion time",
					title:        "Valid title",
					completedAt:  &earlierCompletion,
					wantSQLState: "23514",
				},
				{
					name:         "completed task has no completion time",
					title:        "Valid title",
					completed:    true,
					wantSQLState: "23514",
				},
				{
					name:         "completion time precedes creation time",
					title:        "Valid title",
					completed:    true,
					completedAt:  &earlierCompletion,
					wantSQLState: "23514",
				},
			}

			for _, testCase := range tests {
				t.Run(testCase.name, func(t *testing.T) {
					authorID := insertTaskAuthor(t, ctx, pool)
					tasksBeforeInsert := countTasks(t, ctx, pool)
					inputTask := domain.NewTask(
						0,
						0,
						testCase.title,
						testCase.description,
						testCase.completed,
						createdAt,
						testCase.completedAt,
						authorID,
					)

					_, err := repository.CreateTask(ctx, inputTask)

					assertUnknownDatabaseError(t, "CreateTask()", err, testCase.wantSQLState)
					if tasksAfterInsert := countTasks(t, ctx, pool); tasksAfterInsert != tasksBeforeInsert {
						t.Fatalf("task count after rejected insert = %d, want unchanged count %d", tasksAfterInsert, tasksBeforeInsert)
					}
				})
			}
		})
	})

	t.Run("GetTask", func(t *testing.T) {
		createdAt := time.Date(2026, 10, 21, 8, 30, 0, 0, time.UTC)
		completedAt := createdAt.Add(time.Hour)
		description := "Review submitted work"
		tests := []struct {
			name        string
			description *string
			completed   bool
			completedAt *time.Time
		}{
			{
				name:        "retrieves task with nullable fields populated",
				description: &description,
				completed:   true,
				completedAt: &completedAt,
			},
			{
				name: "retrieves task with NULL description and completion time",
			},
		}

		for _, testCase := range tests {
			t.Run(testCase.name, func(t *testing.T) {
				authorID := insertTaskAuthor(t, ctx, pool)
				storedTask := insertTask(
					t,
					ctx,
					pool,
					authorID,
					"Get task fixture",
					testCase.description,
					testCase.completed,
					createdAt,
					testCase.completedAt,
				)

				got, err := repository.GetTask(ctx, storedTask.ID)
				if err != nil {
					t.Fatalf("GetTask() error = %v, want nil", err)
				}
				assertTaskEqual(t, "GetTask()", got, storedTask)
			})
		}

		t.Run("returns not found for a missing task", func(t *testing.T) {
			authorID := insertTaskAuthor(t, ctx, pool)
			storedTask := insertTask(
				t,
				ctx,
				pool,
				authorID,
				"Get missing task",
				nil,
				false,
				createdAt,
				nil,
			)
			deleteTaskDirectly(t, ctx, pool, storedTask.ID)

			_, err := repository.GetTask(ctx, storedTask.ID)

			if !errors.Is(err, core_errors.ErrNotFound) {
				t.Fatalf("GetTask() error = %v, want ErrNotFound", err)
			}
		})
	})

	t.Run("GetTasks", func(t *testing.T) {
		t.Run("filters by author and keeps ascending ID order", func(t *testing.T) {
			createdAt := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
			authorID := insertTaskAuthor(t, ctx, pool)
			otherAuthorID := insertTaskAuthor(t, ctx, pool)
			description := "Task with details"

			firstTask := insertTask(t, ctx, pool, authorID, "First owned task", &description, false, createdAt, nil)
			insertTask(t, ctx, pool, otherAuthorID, "Task from another author", nil, false, createdAt.Add(time.Minute), nil)
			lastTask := insertTask(t, ctx, pool, authorID, "Second owned task", nil, true, createdAt.Add(2*time.Minute), timePointer(createdAt.Add(3*time.Minute)))

			got, err := repository.GetTasks(ctx, &authorID, nil, nil)
			if err != nil {
				t.Fatalf("GetTasks() error = %v, want nil", err)
			}
			wantTasks := []domain.Task{firstTask, lastTask}
			assertTasksEqual(t, "GetTasks()", got, wantTasks)
		})

		t.Run("paginates without an author filter", func(t *testing.T) {
			createdAt := time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
			taskCountBefore := countTasks(t, ctx, pool)
			firstAuthorID := insertTaskAuthor(t, ctx, pool)
			secondAuthorID := insertTaskAuthor(t, ctx, pool)
			insertTask(t, ctx, pool, firstAuthorID, "Page first", nil, false, createdAt, nil)
			secondTask := insertTask(t, ctx, pool, secondAuthorID, "Page second", nil, false, createdAt.Add(time.Minute), nil)
			thirdTask := insertTask(t, ctx, pool, firstAuthorID, "Page third", nil, false, createdAt.Add(2*time.Minute), nil)
			limit := 2
			offset := taskCountBefore + 1

			got, err := repository.GetTasks(ctx, nil, &limit, &offset)
			if err != nil {
				t.Fatalf("GetTasks() error = %v, want nil", err)
			}
			wantTasks := []domain.Task{secondTask, thirdTask}
			assertTasksEqual(t, "GetTasks()", got, wantTasks)
		})

		t.Run("returns a non-nil empty slice for an author without tasks", func(t *testing.T) {
			authorID := insertTaskAuthor(t, ctx, pool)

			got, err := repository.GetTasks(ctx, &authorID, nil, nil)
			if err != nil {
				t.Fatalf("GetTasks() error = %v, want nil", err)
			}
			if got == nil || len(got) != 0 {
				t.Fatalf("GetTasks() = %#v, want non-nil empty slice", got)
			}
		})

		t.Run("returns an empty page after the final task", func(t *testing.T) {
			createdAt := time.Date(2026, 10, 22, 11, 0, 0, 0, time.UTC)
			authorID := insertTaskAuthor(t, ctx, pool)
			insertTask(t, ctx, pool, authorID, "Final page fixture", nil, false, createdAt, nil)
			offset := countTasks(t, ctx, pool)

			got, err := repository.GetTasks(ctx, nil, nil, &offset)
			if err != nil {
				t.Fatalf("GetTasks() error = %v, want nil", err)
			}
			if got == nil || len(got) != 0 {
				t.Fatalf("GetTasks() = %#v, want non-nil empty slice", got)
			}
		})
	})

	t.Run("PatchTask", func(t *testing.T) {
		t.Run("updates fields increments version and clears nullable values", func(t *testing.T) {
			createdAt := time.Date(2026, 10, 23, 8, 0, 0, 0, time.UTC)
			completedAt := createdAt.Add(time.Hour)
			description := "Old task details"
			authorID := insertTaskAuthor(t, ctx, pool)
			storedTask := insertTask(t, ctx, pool, authorID, "Old title", &description, true, createdAt, &completedAt)
			updatedTask := domain.NewTask(
				storedTask.ID,
				storedTask.Version,
				"Updated title",
				nil,
				false,
				storedTask.CreatedAt,
				nil,
				storedTask.AuthorUserID,
			)

			got, err := repository.PatchTask(ctx, storedTask.ID, updatedTask)
			if err != nil {
				t.Fatalf("PatchTask() error = %v, want nil", err)
			}
			wantTask := domain.NewTask(
				storedTask.ID,
				storedTask.Version+1,
				"Updated title",
				nil,
				false,
				storedTask.CreatedAt,
				nil,
				storedTask.AuthorUserID,
			)
			assertTaskEqual(t, "PatchTask()", got, wantTask)
			assertTaskEqual(t, "stored patched task", readStoredTask(t, ctx, pool, storedTask.ID), wantTask)
		})

		t.Run("updates completion timestamp for a completed task", func(t *testing.T) {
			createdAt := time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC)
			authorID := insertTaskAuthor(t, ctx, pool)
			storedTask := insertTask(t, ctx, pool, authorID, "Complete task", nil, false, createdAt, nil)
			completedAt := createdAt.Add(2 * time.Hour)
			updatedTask := domain.NewTask(
				storedTask.ID,
				storedTask.Version,
				"Complete task",
				storedTask.Description,
				true,
				storedTask.CreatedAt,
				&completedAt,
				storedTask.AuthorUserID,
			)

			got, err := repository.PatchTask(ctx, storedTask.ID, updatedTask)
			if err != nil {
				t.Fatalf("PatchTask() error = %v, want nil", err)
			}
			wantTask := domain.NewTask(
				storedTask.ID,
				storedTask.Version+1,
				"Complete task",
				nil,
				true,
				storedTask.CreatedAt,
				&completedAt,
				storedTask.AuthorUserID,
			)
			assertTaskEqual(t, "PatchTask()", got, wantTask)
			assertTaskEqual(t, "stored patched task", readStoredTask(t, ctx, pool, storedTask.ID), wantTask)
		})

		t.Run("returns conflict for a stale version without changing the task", func(t *testing.T) {
			createdAt := time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)
			authorID := insertTaskAuthor(t, ctx, pool)
			storedTask := insertTask(t, ctx, pool, authorID, "Stale update fixture", nil, false, createdAt, nil)
			staleTask := domain.NewTask(
				storedTask.ID,
				storedTask.Version-1,
				"Should not be saved",
				nil,
				false,
				storedTask.CreatedAt,
				nil,
				storedTask.AuthorUserID,
			)

			_, err := repository.PatchTask(ctx, storedTask.ID, staleTask)
			if !errors.Is(err, core_errors.ErrConflict) {
				t.Fatalf("PatchTask() error = %v, want ErrConflict", err)
			}
			assertTaskEqual(t, "task after stale update", readStoredTask(t, ctx, pool, storedTask.ID), storedTask)
		})

		t.Run("returns conflict for a missing task", func(t *testing.T) {
			createdAt := time.Date(2026, 10, 23, 11, 0, 0, 0, time.UTC)
			authorID := insertTaskAuthor(t, ctx, pool)
			storedTask := insertTask(t, ctx, pool, authorID, "Missing update fixture", nil, false, createdAt, nil)
			deleteTaskDirectly(t, ctx, pool, storedTask.ID)

			_, err := repository.PatchTask(ctx, storedTask.ID, storedTask)
			if !errors.Is(err, core_errors.ErrConflict) {
				t.Fatalf("PatchTask() error = %v, want ErrConflict", err)
			}
		})

		t.Run("returns database error for an invalid update without changing the task", func(t *testing.T) {
			createdAt := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
			authorID := insertTaskAuthor(t, ctx, pool)
			storedTask := insertTask(t, ctx, pool, authorID, "Valid update fixture", nil, false, createdAt, nil)
			invalidTask := domain.NewTask(
				storedTask.ID,
				storedTask.Version,
				"",
				nil,
				false,
				storedTask.CreatedAt,
				nil,
				storedTask.AuthorUserID,
			)

			_, err := repository.PatchTask(ctx, storedTask.ID, invalidTask)
			assertUnknownDatabaseError(t, "PatchTask()", err, "23514")
			assertTaskEqual(t, "task after rejected update", readStoredTask(t, ctx, pool, storedTask.ID), storedTask)
		})
	})

	t.Run("DeleteTask", func(t *testing.T) {
		t.Run("deletes existing task and reports a missing task", func(t *testing.T) {
			createdAt := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
			authorID := insertTaskAuthor(t, ctx, pool)
			storedTask := insertTask(t, ctx, pool, authorID, "Delete task fixture", nil, false, createdAt, nil)

			if err := repository.DeleteTask(ctx, storedTask.ID); err != nil {
				t.Fatalf("DeleteTask() error = %v, want nil", err)
			}
			if taskExists(t, ctx, pool, storedTask.ID) {
				t.Fatalf("task %d still exists after DeleteTask()", storedTask.ID)
			}

			err := repository.DeleteTask(ctx, storedTask.ID)
			if !errors.Is(err, core_errors.ErrNotFound) {
				t.Fatalf("DeleteTask() for missing task error = %v, want ErrNotFound", err)
			}
		})
	})
}

func assertConnectedTestDatabase(
	t *testing.T,
	ctx context.Context,
	pool *core_pgx_pool.Pool,
	wantDatabase string,
) {
	t.Helper()

	var connectedDatabase string
	if err := pool.QueryRow(ctx, "SELECT current_database()").Scan(&connectedDatabase); err != nil {
		t.Fatalf("read connected database name: %v", err)
	}
	if connectedDatabase != wantDatabase {
		t.Fatalf("connected database = %q, want configured test database %q", connectedDatabase, wantDatabase)
	}
}

func insertTaskAuthor(t *testing.T, ctx context.Context, pool *core_pgx_pool.Pool) int {
	t.Helper()

	var authorID int
	if err := pool.QueryRow(
		ctx,
		`INSERT INTO todoapp.users (full_name)
		 VALUES ($1)
		 RETURNING id`,
		"Tasks integration author",
	).Scan(&authorID); err != nil {
		t.Fatalf("insert task author fixture: %v", err)
	}
	t.Cleanup(func() {
		deleteTaskAuthorDirectly(t, ctx, pool, authorID)
	})

	return authorID
}

func insertTask(
	t *testing.T,
	ctx context.Context,
	pool *core_pgx_pool.Pool,
	authorID int,
	title string,
	description *string,
	completed bool,
	createdAt time.Time,
	completedAt *time.Time,
) domain.Task {
	t.Helper()

	var task domain.Task
	err := pool.QueryRow(
		ctx,
		`INSERT INTO todoapp.tasks (
			title,
			description,
			completed,
			created_at,
			completed_at,
			author_user_id
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, version, title, description, completed, created_at, completed_at, author_user_id`,
		title,
		description,
		completed,
		createdAt,
		completedAt,
		authorID,
	).Scan(
		&task.ID,
		&task.Version,
		&task.Title,
		&task.Description,
		&task.Completed,
		&task.CreatedAt,
		&task.CompletedAt,
		&task.AuthorUserID,
	)
	if err != nil {
		t.Fatalf("insert task fixture: %v", err)
	}
	t.Cleanup(func() {
		deleteTaskDirectly(t, ctx, pool, task.ID)
	})

	return task
}

func registerTaskCleanup(t *testing.T, ctx context.Context, pool *core_pgx_pool.Pool, taskID int) {
	t.Helper()
	t.Cleanup(func() {
		deleteTaskDirectly(t, ctx, pool, taskID)
	})
}

func deleteTaskDirectly(t *testing.T, ctx context.Context, pool *core_pgx_pool.Pool, taskID int) {
	t.Helper()
	if _, err := pool.Exec(ctx, "DELETE FROM todoapp.tasks WHERE id = $1", taskID); err != nil {
		t.Errorf("delete task fixture %d: %v", taskID, err)
	}
}

func deleteTaskAuthorDirectly(t *testing.T, ctx context.Context, pool *core_pgx_pool.Pool, authorID int) {
	t.Helper()
	if _, err := pool.Exec(ctx, "DELETE FROM todoapp.users WHERE id = $1", authorID); err != nil {
		t.Errorf("delete task author fixture %d: %v", authorID, err)
	}
}

func readStoredTask(t *testing.T, ctx context.Context, pool *core_pgx_pool.Pool, taskID int) domain.Task {
	t.Helper()

	var task domain.Task
	err := pool.QueryRow(
		ctx,
		`SELECT id, version, title, description, completed, created_at, completed_at, author_user_id
		 FROM todoapp.tasks
		 WHERE id = $1`,
		taskID,
	).Scan(
		&task.ID,
		&task.Version,
		&task.Title,
		&task.Description,
		&task.Completed,
		&task.CreatedAt,
		&task.CompletedAt,
		&task.AuthorUserID,
	)
	if err != nil {
		t.Fatalf("read stored task %d: %v", taskID, err)
	}
	return task
}

func countTasks(t *testing.T, ctx context.Context, pool *core_pgx_pool.Pool) int {
	t.Helper()

	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM todoapp.tasks").Scan(&count); err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	return count
}

func taskExists(t *testing.T, ctx context.Context, pool *core_pgx_pool.Pool, taskID int) bool {
	t.Helper()

	var exists bool
	if err := pool.QueryRow(
		ctx,
		"SELECT EXISTS (SELECT 1 FROM todoapp.tasks WHERE id = $1)",
		taskID,
	).Scan(&exists); err != nil {
		t.Fatalf("check whether task %d exists: %v", taskID, err)
	}
	return exists
}

func assertTaskEqual(t *testing.T, operation string, got, want domain.Task) {
	t.Helper()

	if got.ID != want.ID || got.Version != want.Version || got.Title != want.Title ||
		!equalStringPointers(got.Description, want.Description) || got.Completed != want.Completed ||
		!got.CreatedAt.Equal(want.CreatedAt) || !equalTimePointers(got.CompletedAt, want.CompletedAt) ||
		got.AuthorUserID != want.AuthorUserID {
		t.Fatalf("%s = %+v, want %+v", operation, got, want)
	}
}

func assertTasksEqual(t *testing.T, operation string, got, want []domain.Task) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("%s returned %d tasks, want %d: got %+v, want %+v", operation, len(got), len(want), got, want)
	}

	for index := range want {
		assertTaskEqual(t, fmt.Sprintf("%s task at index %d", operation, index), got[index], want[index])
	}
}

func equalStringPointers(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}

	return *left == *right
}

func equalTimePointers(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}

	return left.Equal(*right)
}

func assertUnknownDatabaseError(t *testing.T, operation string, err error, wantSQLState string) {
	t.Helper()

	if err == nil {
		t.Fatalf("%s error = nil, want PostgreSQL constraint error", operation)
	}
	if !errors.Is(err, core_postgres_pool.ErrUnknown) {
		t.Fatalf("%s error = %v, want ErrUnknown from PostgreSQL adapter", operation, err)
	}
	if !strings.Contains(err.Error(), "SQLSTATE "+wantSQLState) {
		t.Fatalf("%s error = %v, want PostgreSQL SQLSTATE %s", operation, err, wantSQLState)
	}
}

func stringPointer(value string) *string {
	return &value
}

func timePointer(value time.Time) *time.Time {
	return &value
}
