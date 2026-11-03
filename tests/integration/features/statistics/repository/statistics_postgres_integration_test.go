package repository_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Reigor1/20h-project/internal/core/domain"
	core_pgx_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool/pgx"
	statistics_postgres_repository "github.com/Reigor1/20h-project/internal/features/statistics/repository/postgres"
	"github.com/Reigor1/20h-project/internal/testutil"
)

func TestStatisticsRepositoryIntegration(t *testing.T) {
	config := testutil.PostgresTestConfig(t)
	if !strings.HasSuffix(strings.ToLower(config.Database), "_test") {
		t.Fatalf("refusing to modify database %q: test database name must end in _test", config.Database)
	}

	connectionContext, cancelConnection := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelConnection()
	pool, err := core_pgx_pool.NewPool(connectionContext, config, nil)
	if err != nil {
		t.Fatalf("connect to configured PostgreSQL test database: %v", err)
	}
	t.Cleanup(pool.Close)

	ctx, cancelOperations := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancelOperations)
	var connectedDatabase string
	if err := pool.QueryRow(ctx, "SELECT current_database()").Scan(&connectedDatabase); err != nil {
		t.Fatalf("read connected database name: %v", err)
	}
	if connectedDatabase != config.Database {
		t.Fatalf(
			"connected database = %q, want configured test database %q",
			connectedDatabase,
			config.Database,
		)
	}
	testutil.PreparePostgresSchema(t, ctx, pool, "testdata/statistics_schema.sql")
	repository := statistics_postgres_repository.NewStatisticsRepository(pool)

	t.Run("GetTasks applies filters, preserves boundaries, maps fields, and orders results", func(t *testing.T) {
		from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
		beforeFrom := from.Add(-time.Second)
		insideRange := from.Add(15 * 24 * time.Hour)
		otherUserCreatedAt := from.Add(20 * 24 * time.Hour)
		atTo := to

		firstAuthorID := insertStatisticsUser(t, ctx, pool, "Statistics first author")
		secondAuthorID := insertStatisticsUser(t, ctx, pool, "Statistics second author")
		emptyAuthorID := insertStatisticsUser(t, ctx, pool, "Statistics empty author")

		description := "Task with a description"
		completedAt := insideRange.Add(45 * time.Minute)
		beforeFromTask := domain.NewTask(
			0,
			0,
			"Before from boundary",
			nil,
			false,
			beforeFrom,
			nil,
			firstAuthorID,
		)
		atFromTask := domain.NewTask(
			0,
			0,
			"Exactly at from boundary",
			&description,
			true,
			from,
			&completedAt,
			firstAuthorID,
		)
		insideRangeTask := domain.NewTask(
			0,
			0,
			"Inside range without nullable values",
			nil,
			false,
			insideRange,
			nil,
			firstAuthorID,
		)
		otherUserTask := domain.NewTask(
			0,
			0,
			"Other author's task",
			nil,
			false,
			otherUserCreatedAt,
			nil,
			secondAuthorID,
		)
		atToTask := domain.NewTask(
			0,
			0,
			"Exactly at exclusive to boundary",
			nil,
			false,
			atTo,
			nil,
			firstAuthorID,
		)

		beforeFromTask.ID = insertStatisticsTask(t, ctx, pool, beforeFromTask)
		beforeFromTask.Version = 1
		atFromTask.ID = insertStatisticsTask(t, ctx, pool, atFromTask)
		atFromTask.Version = 1
		insideRangeTask.ID = insertStatisticsTask(t, ctx, pool, insideRangeTask)
		insideRangeTask.Version = 1
		otherUserTask.ID = insertStatisticsTask(t, ctx, pool, otherUserTask)
		otherUserTask.Version = 1
		atToTask.ID = insertStatisticsTask(t, ctx, pool, atToTask)
		atToTask.Version = 1

		filters := []struct {
			name   string
			userID *int
			from   *time.Time
			to     *time.Time
			want   []domain.Task
		}{
			{
				name: "without filters returns all tasks in ascending ID order",
				want: []domain.Task{
					beforeFromTask,
					atFromTask,
					insideRangeTask,
					otherUserTask,
					atToTask,
				},
			},
			{
				name:   "user filter excludes tasks from other users",
				userID: &firstAuthorID,
				want: []domain.Task{
					beforeFromTask,
					atFromTask,
					insideRangeTask,
					atToTask,
				},
			},
			{
				name: "from boundary is inclusive",
				from: &from,
				want: []domain.Task{
					atFromTask,
					insideRangeTask,
					otherUserTask,
					atToTask,
				},
			},
			{
				name: "to boundary is exclusive",
				to:   &to,
				want: []domain.Task{
					beforeFromTask,
					atFromTask,
					insideRangeTask,
					otherUserTask,
				},
			},
			{
				name:   "combined filters select only matching tasks",
				userID: &firstAuthorID,
				from:   &from,
				to:     &to,
				want: []domain.Task{
					atFromTask,
					insideRangeTask,
				},
			},
			{
				name:   "user without tasks receives an empty result",
				userID: &emptyAuthorID,
				want:   []domain.Task{},
			},
		}

		for _, testCase := range filters {
			t.Run(testCase.name, func(t *testing.T) {
				got, err := repository.GetTasks(ctx, testCase.userID, testCase.from, testCase.to)
				if err != nil {
					t.Fatalf("GetTasks() error = %v, want nil", err)
				}
				assertStatisticsTasksEqual(t, got, testCase.want)
			})
		}
	})
}

func insertStatisticsUser(t *testing.T, ctx context.Context, pool *core_pgx_pool.Pool, fullName string) int {
	t.Helper()

	var userID int
	if err := pool.QueryRow(
		ctx,
		"INSERT INTO todoapp.users (full_name) VALUES ($1) RETURNING id",
		fullName,
	).Scan(&userID); err != nil {
		t.Fatalf("insert statistics user fixture: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, "DELETE FROM todoapp.users WHERE id = $1", userID); err != nil {
			t.Errorf("delete statistics user fixture %d: %v", userID, err)
		}
	})

	return userID
}

func insertStatisticsTask(
	t *testing.T,
	ctx context.Context,
	pool *core_pgx_pool.Pool,
	task domain.Task,
) int {
	t.Helper()

	var taskID int
	if err := pool.QueryRow(
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
		RETURNING id`,
		task.Title,
		task.Description,
		task.Completed,
		task.CreatedAt,
		task.CompletedAt,
		task.AuthorUserID,
	).Scan(&taskID); err != nil {
		t.Fatalf("insert statistics task fixture %q: %v", task.Title, err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, "DELETE FROM todoapp.tasks WHERE id = $1", taskID); err != nil {
			t.Errorf("delete statistics task fixture %d: %v", taskID, err)
		}
	})

	return taskID
}

func assertStatisticsTasksEqual(t *testing.T, got, want []domain.Task) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("GetTasks() returned %d tasks, want %d: got %+v, want %+v", len(got), len(want), got, want)
	}

	for index := range want {
		gotTask := got[index]
		wantTask := want[index]
		if gotTask.ID != wantTask.ID || gotTask.Version != wantTask.Version ||
			gotTask.Title != wantTask.Title || !equalStatisticsStringPointers(gotTask.Description, wantTask.Description) ||
			gotTask.Completed != wantTask.Completed || !gotTask.CreatedAt.Equal(wantTask.CreatedAt) ||
			!equalStatisticsTimePointers(gotTask.CompletedAt, wantTask.CompletedAt) ||
			gotTask.AuthorUserID != wantTask.AuthorUserID {
			t.Fatalf("GetTasks() task at index %d = %+v, want %+v", index, gotTask, wantTask)
		}
	}
}

func equalStatisticsStringPointers(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}

	return *left == *right
}

func equalStatisticsTimePointers(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}

	return left.Equal(*right)
}
