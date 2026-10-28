package tasks_postgres_repository

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Reigor1/20h-project/internal/core/domain"
	core_errors "github.com/Reigor1/20h-project/internal/core/errors"
	core_postgres_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool"
	"github.com/Reigor1/20h-project/internal/testutil"
)

type repositoryTestPool struct {
	t *testing.T

	operationTimeout time.Duration
	queryCalls       int
	queryRowCalls    int
	execCalls        int

	queryResult core_postgres_pool.Rows
	queryError  error
	rowResult   core_postgres_pool.Row
	execResult  core_postgres_pool.CommandTag
	execError   error

	gotQuerySQL  string
	gotQueryArgs []any
	gotRowSQL    string
	gotRowArgs   []any
	gotExecSQL   string
	gotExecArgs  []any
	gotContext   context.Context
}

func newRepositoryTestPool(t *testing.T) *repositoryTestPool {
	t.Helper()

	return &repositoryTestPool{
		t:                t,
		operationTimeout: testutil.RepositoryOperationTimeout,
	}
}

func (p *repositoryTestPool) Query(
	ctx context.Context,
	sql string,
	args ...any,
) (core_postgres_pool.Rows, error) {
	p.assertNoPreviousPoolCall("Query")
	p.queryCalls++
	p.gotContext = ctx
	p.gotQuerySQL = sql
	p.gotQueryArgs = append([]any(nil), args...)

	if p.queryResult == nil && p.queryError == nil {
		p.t.Fatal("pool Query() has no configured result or error")
	}

	return p.queryResult, p.queryError
}

func (p *repositoryTestPool) QueryRow(
	ctx context.Context,
	sql string,
	args ...any,
) core_postgres_pool.Row {
	p.assertNoPreviousPoolCall("QueryRow")
	p.queryRowCalls++
	p.gotContext = ctx
	p.gotRowSQL = sql
	p.gotRowArgs = append([]any(nil), args...)

	if p.rowResult == nil {
		p.t.Fatal("pool QueryRow() has no configured row result")
	}

	return p.rowResult
}

func (p *repositoryTestPool) Exec(
	ctx context.Context,
	sql string,
	args ...any,
) (core_postgres_pool.CommandTag, error) {
	p.assertNoPreviousPoolCall("Exec")
	p.execCalls++
	p.gotContext = ctx
	p.gotExecSQL = sql
	p.gotExecArgs = append([]any(nil), args...)

	if p.execResult == nil && p.execError == nil {
		p.t.Fatal("pool Exec() has no configured result or error")
	}

	return p.execResult, p.execError
}

func (*repositoryTestPool) Close() {}

func (p *repositoryTestPool) OpTimeout() time.Duration {
	return p.operationTimeout
}

func (p *repositoryTestPool) assertNoPreviousPoolCall(method string) {
	p.t.Helper()

	totalCalls := p.queryCalls + p.queryRowCalls + p.execCalls
	if totalCalls != 0 {
		p.t.Fatalf("unexpected additional pool %s call", method)
	}
}

type repositoryTestRow struct {
	values []any
	err    error
	scans  int
}

func (r *repositoryTestRow) Scan(destinations ...any) error {
	r.scans++
	if r.err != nil {
		return r.err
	}
	if len(destinations) != len(r.values) {
		return errors.New("unexpected scan destination count")
	}

	for index, value := range r.values {
		if err := assignTaskScanValue(destinations[index], value); err != nil {
			return err
		}
	}

	return nil
}

func assignTaskScanValue(destination, value any) error {
	switch destination := destination.(type) {
	case *int:
		scannedValue, ok := value.(int)
		if !ok {
			return errors.New("scan value is not an int")
		}
		*destination = scannedValue
	case *string:
		scannedValue, ok := value.(string)
		if !ok {
			return errors.New("scan value is not a string")
		}
		*destination = scannedValue
	case **string:
		scannedValue, ok := value.(*string)
		if !ok {
			return errors.New("scan value is not a *string")
		}
		*destination = scannedValue
	case *bool:
		scannedValue, ok := value.(bool)
		if !ok {
			return errors.New("scan value is not a bool")
		}
		*destination = scannedValue
	case *time.Time:
		scannedValue, ok := value.(time.Time)
		if !ok {
			return errors.New("scan value is not a time.Time")
		}
		*destination = scannedValue
	case **time.Time:
		scannedValue, ok := value.(*time.Time)
		if !ok {
			return errors.New("scan value is not a *time.Time")
		}
		*destination = scannedValue
	default:
		return errors.New("unsupported scan destination")
	}

	return nil
}

type repositoryTestRows struct {
	rows       [][]any
	current    int
	scanError  error
	rowsError  error
	closeCalls int
}

func (r *repositoryTestRows) Next() bool {
	if r.current >= len(r.rows) {
		return false
	}
	r.current++
	return true
}

func (r *repositoryTestRows) Scan(destinations ...any) error {
	if r.scanError != nil {
		return r.scanError
	}
	return (&repositoryTestRow{values: r.rows[r.current-1]}).Scan(destinations...)
}

func (r *repositoryTestRows) Err() error {
	return r.rowsError
}

func (r *repositoryTestRows) Close() {
	r.closeCalls++
}

type repositoryTestCommandTag struct {
	rowsAffected int64
}

func (tag repositoryTestCommandTag) RowsAffected() int64 {
	return tag.rowsAffected
}

func TestNewTasksRepositoryStoresPool(t *testing.T) {
	pool := newRepositoryTestPool(t)

	repository := NewTasksRepository(pool)

	if repository.pool != pool {
		t.Fatal("NewTasksRepository() did not retain the supplied pool")
	}
}

func TestTaskDomainsFromModels(t *testing.T) {
	description := "Review the submitted work"
	createdAt := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	completedAt := createdAt.Add(time.Hour)
	tests := []struct {
		name   string
		models []TaskModel
		want   []domain.Task
	}{
		{
			name:   "empty models",
			models: []TaskModel{},
			want:   []domain.Task{},
		},
		{
			name: "single model with nullable fields unset",
			models: []TaskModel{
				{
					ID:           7,
					Version:      2,
					Title:        "Draft task",
					CreatedAt:    createdAt,
					AuthorUserID: 23,
				},
			},
			want: []domain.Task{
				domain.NewTask(7, 2, "Draft task", nil, false, createdAt, nil, 23),
			},
		},
		{
			name: "multiple models preserve order and all fields",
			models: []TaskModel{
				{
					ID:           7,
					Version:      2,
					Title:        "Review task",
					Description:  &description,
					Completed:    true,
					CreatedAt:    createdAt,
					CompletedAt:  &completedAt,
					AuthorUserID: 23,
				},
				{
					ID:           12,
					Version:      4,
					Title:        "Unfinished task",
					CreatedAt:    createdAt.Add(2 * time.Hour),
					AuthorUserID: 31,
				},
			},
			want: []domain.Task{
				domain.NewTask(7, 2, "Review task", &description, true, createdAt, &completedAt, 23),
				domain.NewTask(12, 4, "Unfinished task", nil, false, createdAt.Add(2*time.Hour), nil, 31),
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got := taskDomainsFromModels(testCase.models)
			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("taskDomainsFromModels() = %+v, want %+v", got, testCase.want)
			}
		})
	}
}

func TestCreateTask(t *testing.T) {
	description := "Prepare the final report"
	createdAt := time.Date(2026, 10, 21, 9, 15, 0, 0, time.UTC)
	completedAt := time.Date(2026, 10, 21, 10, 45, 0, 0, time.UTC)
	inputTask := domain.NewTask(0, 0, "Write report", &description, true, createdAt, &completedAt, 23)
	wantTask := domain.NewTask(17, 1, "Write report", &description, true, createdAt, &completedAt, 23)
	row := &repositoryTestRow{
		values: []any{17, 1, "Write report", &description, true, createdAt, &completedAt, 23},
	}
	pool := newRepositoryTestPool(t)
	pool.rowResult = row
	repository := NewTasksRepository(pool)
	ctx := context.WithValue(context.Background(), testutil.RepositoryContextKey{}, "request")

	got, err := repository.CreateTask(ctx, inputTask)

	if err != nil {
		t.Fatalf("CreateTask() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(got, wantTask) {
		t.Fatalf("CreateTask() = %+v, want %+v", got, wantTask)
	}
	if pool.queryRowCalls != 1 ||
		!strings.Contains(pool.gotRowSQL, "INSERT INTO todoapp.tasks") ||
		!strings.Contains(pool.gotRowSQL, "RETURNING id, version") {
		t.Fatalf("QueryRow() calls/sql = %d/%q, want one task INSERT", pool.queryRowCalls, pool.gotRowSQL)
	}
	wantArgs := []any{"Write report", &description, true, createdAt, &completedAt, 23}
	if !reflect.DeepEqual(pool.gotRowArgs, wantArgs) {
		t.Fatalf("QueryRow() args = %#v, want %#v", pool.gotRowArgs, wantArgs)
	}
	testutil.AssertRepositoryContext(t, pool.gotContext, ctx)
	if row.scans != 1 {
		t.Fatalf("row Scan() calls = %d, want 1", row.scans)
	}
}

func TestCreateTaskMapsForeignKeyViolationToNotFound(t *testing.T) {
	row := &repositoryTestRow{err: core_postgres_pool.ErrViolatesForeignKey}
	pool := newRepositoryTestPool(t)
	pool.rowResult = row
	repository := NewTasksRepository(pool)
	task := domain.NewTask(0, 0, "Write report", nil, false, time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC), nil, 23)

	_, err := repository.CreateTask(context.Background(), task)

	if !errors.Is(err, core_errors.ErrNotFound) {
		t.Fatalf("CreateTask() error = %v, want ErrNotFound", err)
	}
	if !strings.Contains(err.Error(), "user with id=`23`") {
		t.Fatalf("CreateTask() error = %v, want author ID in context", err)
	}
	if row.scans != 1 {
		t.Fatalf("row Scan() calls = %d, want 1", row.scans)
	}
}

func TestCreateTaskReturnsScanError(t *testing.T) {
	scanError := errors.New("returned task could not be decoded")
	row := &repositoryTestRow{err: scanError}
	pool := newRepositoryTestPool(t)
	pool.rowResult = row
	repository := NewTasksRepository(pool)
	task := domain.NewTask(0, 0, "Write report", nil, false, time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC), nil, 23)

	_, err := repository.CreateTask(context.Background(), task)

	if !errors.Is(err, scanError) || !strings.Contains(err.Error(), "scan error") {
		t.Fatalf("CreateTask() error = %v, want wrapped scan error", err)
	}
}

func TestGetTask(t *testing.T) {
	description := "Review the submitted report"
	createdAt := time.Date(2026, 10, 22, 8, 30, 0, 0, time.UTC)
	completedAt := time.Date(2026, 10, 22, 9, 10, 0, 0, time.UTC)
	row := &repositoryTestRow{
		values: []any{42, 3, "Review report", &description, true, createdAt, &completedAt, 23},
	}
	pool := newRepositoryTestPool(t)
	pool.rowResult = row
	repository := NewTasksRepository(pool)
	ctx := context.WithValue(context.Background(), testutil.RepositoryContextKey{}, "request")

	got, err := repository.GetTask(ctx, 42)

	if err != nil {
		t.Fatalf("GetTask() error = %v, want nil", err)
	}
	wantTask := domain.NewTask(42, 3, "Review report", &description, true, createdAt, &completedAt, 23)
	if !reflect.DeepEqual(got, wantTask) {
		t.Fatalf("GetTask() = %+v, want %+v", got, wantTask)
	}
	if pool.queryRowCalls != 1 || !strings.Contains(pool.gotRowSQL, "WHERE id = $1") {
		t.Fatalf("QueryRow() calls/sql = %d/%q, want one lookup by ID", pool.queryRowCalls, pool.gotRowSQL)
	}
	if !reflect.DeepEqual(pool.gotRowArgs, []any{42}) {
		t.Fatalf("QueryRow() args = %#v, want [42]", pool.gotRowArgs)
	}
	testutil.AssertRepositoryContext(t, pool.gotContext, ctx)
}

func TestGetTaskPreservesNullDescriptionAndCompletionTime(t *testing.T) {
	createdAt := time.Date(2026, 10, 22, 8, 30, 0, 0, time.UTC)
	row := &repositoryTestRow{
		values: []any{42, 3, "Unfinished task", (*string)(nil), false, createdAt, (*time.Time)(nil), 23},
	}
	pool := newRepositoryTestPool(t)
	pool.rowResult = row
	repository := NewTasksRepository(pool)

	got, err := repository.GetTask(context.Background(), 42)

	if err != nil {
		t.Fatalf("GetTask() error = %v, want nil", err)
	}
	wantTask := domain.NewTask(42, 3, "Unfinished task", nil, false, createdAt, nil, 23)
	if !reflect.DeepEqual(got, wantTask) {
		t.Fatalf("GetTask() = %+v, want %+v", got, wantTask)
	}
}

func TestGetTaskTranslatesNoRowsToNotFound(t *testing.T) {
	row := &repositoryTestRow{err: core_postgres_pool.ErrNoRows}
	pool := newRepositoryTestPool(t)
	pool.rowResult = row
	repository := NewTasksRepository(pool)

	_, err := repository.GetTask(context.Background(), 42)

	if !errors.Is(err, core_errors.ErrNotFound) {
		t.Fatalf("GetTask() error = %v, want ErrNotFound", err)
	}
	if !strings.Contains(err.Error(), "id ='42'") {
		t.Fatalf("GetTask() error = %v, want requested ID in context", err)
	}
}

func TestGetTaskReturnsScanError(t *testing.T) {
	scanError := errors.New("task row decoding failed")
	row := &repositoryTestRow{err: scanError}
	pool := newRepositoryTestPool(t)
	pool.rowResult = row
	repository := NewTasksRepository(pool)

	_, err := repository.GetTask(context.Background(), 42)

	if !errors.Is(err, scanError) || !strings.Contains(err.Error(), "scan task from database") {
		t.Fatalf("GetTask() error = %v, want wrapped scan error", err)
	}
}

func TestGetTasksFiltersByAuthorAndPreservesOrder(t *testing.T) {
	firstDescription := "First task description"
	firstCreatedAt := time.Date(2026, 10, 23, 8, 0, 0, 0, time.UTC)
	firstCompletedAt := firstCreatedAt.Add(time.Hour)
	secondCreatedAt := firstCreatedAt.Add(2 * time.Hour)
	rows := &repositoryTestRows{
		rows: [][]any{
			{1, 2, "First task", &firstDescription, true, firstCreatedAt, &firstCompletedAt, 23},
			{3, 4, "Second task", (*string)(nil), false, secondCreatedAt, (*time.Time)(nil), 23},
		},
	}
	pool := newRepositoryTestPool(t)
	pool.queryResult = rows
	repository := NewTasksRepository(pool)
	userID := 23
	limit := 10
	offset := 20
	ctx := context.WithValue(context.Background(), testutil.RepositoryContextKey{}, "request")

	got, err := repository.GetTasks(ctx, &userID, &limit, &offset)

	if err != nil {
		t.Fatalf("GetTasks() error = %v, want nil", err)
	}
	wantTasks := []domain.Task{
		domain.NewTask(1, 2, "First task", &firstDescription, true, firstCreatedAt, &firstCompletedAt, 23),
		domain.NewTask(3, 4, "Second task", nil, false, secondCreatedAt, nil, 23),
	}
	if !reflect.DeepEqual(got, wantTasks) {
		t.Fatalf("GetTasks() = %+v, want %+v", got, wantTasks)
	}
	if pool.queryCalls != 1 || !strings.Contains(pool.gotQuerySQL, "WHERE author_user_id=$3") {
		t.Fatalf("Query() calls/sql = %d/%q, want one author-filtered SELECT", pool.queryCalls, pool.gotQuerySQL)
	}
	if !strings.Contains(pool.gotQuerySQL, "ORDER BY id ASC") ||
		!strings.Contains(pool.gotQuerySQL, "LIMIT $1") ||
		!strings.Contains(pool.gotQuerySQL, "OFFSET $2") {
		t.Fatalf("Query() SQL = %q, want ascending order and pagination", pool.gotQuerySQL)
	}
	wantArgs := []any{&limit, &offset, &userID}
	if !reflect.DeepEqual(pool.gotQueryArgs, wantArgs) {
		t.Fatalf("Query() args = %#v, want %#v", pool.gotQueryArgs, wantArgs)
	}
	testutil.AssertRepositoryContext(t, pool.gotContext, ctx)
	if rows.closeCalls != 1 {
		t.Fatalf("rows Close() calls = %d, want 1", rows.closeCalls)
	}
}

func TestGetTasksWithoutAuthorFilterReturnsNonNilEmptySlice(t *testing.T) {
	rows := &repositoryTestRows{}
	pool := newRepositoryTestPool(t)
	pool.queryResult = rows
	repository := NewTasksRepository(pool)

	got, err := repository.GetTasks(context.Background(), nil, nil, nil)

	if err != nil {
		t.Fatalf("GetTasks() error = %v, want nil", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("GetTasks() = %#v, want non-nil empty slice", got)
	}
	if strings.Contains(pool.gotQuerySQL, "WHERE author_user_id") {
		t.Fatalf("Query() SQL = %q, want no author filter", pool.gotQuerySQL)
	}
	if !reflect.DeepEqual(pool.gotQueryArgs, []any{(*int)(nil), (*int)(nil)}) {
		t.Fatalf("Query() args = %#v, want nil limit and offset", pool.gotQueryArgs)
	}
	if rows.closeCalls != 1 {
		t.Fatalf("rows Close() calls = %d, want 1", rows.closeCalls)
	}
}

func TestGetTasksReturnsQueryError(t *testing.T) {
	queryError := errors.New("query failed")
	pool := newRepositoryTestPool(t)
	pool.queryError = queryError
	repository := NewTasksRepository(pool)

	got, err := repository.GetTasks(context.Background(), nil, nil, nil)

	if !errors.Is(err, queryError) || !strings.Contains(err.Error(), "select tasks") {
		t.Fatalf("GetTasks() error = %v, want wrapped query error", err)
	}
	if got != nil || pool.queryCalls != 1 {
		t.Fatalf("GetTasks() result/calls = %v/%d, want nil/1", got, pool.queryCalls)
	}
}

func TestGetTasksReturnsScanErrorAndClosesRows(t *testing.T) {
	scanError := errors.New("task row scan failed")
	rows := &repositoryTestRows{
		rows:      [][]any{{1, 2, "Task", (*string)(nil), false, time.Time{}, (*time.Time)(nil), 23}},
		scanError: scanError,
	}
	pool := newRepositoryTestPool(t)
	pool.queryResult = rows
	repository := NewTasksRepository(pool)

	_, err := repository.GetTasks(context.Background(), nil, nil, nil)

	if !errors.Is(err, scanError) || !strings.Contains(err.Error(), "scan tasks") {
		t.Fatalf("GetTasks() error = %v, want wrapped scan error", err)
	}
	if rows.closeCalls != 1 {
		t.Fatalf("rows Close() calls = %d, want 1 after scan failure", rows.closeCalls)
	}
}

func TestGetTasksReturnsRowsIterationErrorAndClosesRows(t *testing.T) {
	iterationError := errors.New("row iteration failed")
	rows := &repositoryTestRows{rowsError: iterationError}
	pool := newRepositoryTestPool(t)
	pool.queryResult = rows
	repository := NewTasksRepository(pool)

	_, err := repository.GetTasks(context.Background(), nil, nil, nil)

	if !errors.Is(err, iterationError) || !strings.Contains(err.Error(), "scan tasks") {
		t.Fatalf("GetTasks() error = %v, want wrapped iteration error", err)
	}
	if rows.closeCalls != 1 {
		t.Fatalf("rows Close() calls = %d, want 1 after iteration failure", rows.closeCalls)
	}
}

func TestPatchTask(t *testing.T) {
	description := "Updated task details"
	createdAt := time.Date(2026, 10, 24, 8, 0, 0, 0, time.UTC)
	completedAt := time.Date(2026, 10, 24, 9, 30, 0, 0, time.UTC)
	inputTask := domain.NewTask(42, 5, "Publish report", &description, true, createdAt, &completedAt, 23)
	row := &repositoryTestRow{
		values: []any{42, 6, "Publish report", &description, true, createdAt, &completedAt, 23},
	}
	pool := newRepositoryTestPool(t)
	pool.rowResult = row
	repository := NewTasksRepository(pool)
	ctx := context.WithValue(context.Background(), testutil.RepositoryContextKey{}, "request")

	got, err := repository.PatchTask(ctx, 42, inputTask)

	if err != nil {
		t.Fatalf("PatchTask() error = %v, want nil", err)
	}
	wantTask := domain.NewTask(42, 6, "Publish report", &description, true, createdAt, &completedAt, 23)
	if !reflect.DeepEqual(got, wantTask) {
		t.Fatalf("PatchTask() = %+v, want %+v", got, wantTask)
	}
	if pool.queryRowCalls != 1 || !strings.Contains(pool.gotRowSQL, "UPDATE todoapp.tasks") {
		t.Fatalf("QueryRow() calls/sql = %d/%q, want one task UPDATE", pool.queryRowCalls, pool.gotRowSQL)
	}
	for _, sqlFragment := range []string{
		"version=version+1",
		"WHERE id=$5 AND version=$6",
		"RETURNING",
	} {
		if !strings.Contains(pool.gotRowSQL, sqlFragment) {
			t.Errorf("PatchTask() SQL = %q, missing %q", pool.gotRowSQL, sqlFragment)
		}
	}
	wantArgs := []any{"Publish report", &description, true, &completedAt, 42, 5}
	if !reflect.DeepEqual(pool.gotRowArgs, wantArgs) {
		t.Fatalf("QueryRow() args = %#v, want %#v", pool.gotRowArgs, wantArgs)
	}
	testutil.AssertRepositoryContext(t, pool.gotContext, ctx)
}

func TestPatchTaskPassesNilForClearedNullableFields(t *testing.T) {
	createdAt := time.Date(2026, 10, 24, 8, 0, 0, 0, time.UTC)
	inputTask := domain.NewTask(42, 5, "Unfinished task", nil, false, createdAt, nil, 23)
	row := &repositoryTestRow{
		values: []any{42, 6, "Unfinished task", (*string)(nil), false, createdAt, (*time.Time)(nil), 23},
	}
	pool := newRepositoryTestPool(t)
	pool.rowResult = row
	repository := NewTasksRepository(pool)

	got, err := repository.PatchTask(context.Background(), 42, inputTask)

	if err != nil {
		t.Fatalf("PatchTask() error = %v, want nil", err)
	}
	if got.Description != nil || got.CompletedAt != nil {
		t.Fatalf("PatchTask() nullable fields = description %v, completedAt %v, want nil/nil", got.Description, got.CompletedAt)
	}
	if len(pool.gotRowArgs) != 6 || pool.gotRowArgs[1] != (*string)(nil) || pool.gotRowArgs[3] != (*time.Time)(nil) {
		t.Fatalf("PatchTask() nullable SQL args = %#v, want nil description and completion time", pool.gotRowArgs)
	}
}

func TestPatchTaskTranslatesNoRowsToConflict(t *testing.T) {
	row := &repositoryTestRow{err: core_postgres_pool.ErrNoRows}
	pool := newRepositoryTestPool(t)
	pool.rowResult = row
	repository := NewTasksRepository(pool)
	task := domain.NewTask(42, 5, "Updated task", nil, false, time.Date(2026, 10, 24, 8, 0, 0, 0, time.UTC), nil, 23)

	_, err := repository.PatchTask(context.Background(), 42, task)

	if !errors.Is(err, core_errors.ErrConflict) {
		t.Fatalf("PatchTask() error = %v, want ErrConflict", err)
	}
	if !strings.Contains(err.Error(), "id='42'") || !strings.Contains(err.Error(), "concurrently accessed") {
		t.Fatalf("PatchTask() error = %v, want stale update context", err)
	}
}

func TestPatchTaskReturnsScanError(t *testing.T) {
	scanError := errors.New("updated task could not be decoded")
	row := &repositoryTestRow{err: scanError}
	pool := newRepositoryTestPool(t)
	pool.rowResult = row
	repository := NewTasksRepository(pool)
	task := domain.NewTask(42, 5, "Updated task", nil, false, time.Date(2026, 10, 24, 8, 0, 0, 0, time.UTC), nil, 23)

	_, err := repository.PatchTask(context.Background(), 42, task)

	if !errors.Is(err, scanError) || !strings.Contains(err.Error(), "scan error") {
		t.Fatalf("PatchTask() error = %v, want wrapped scan error", err)
	}
}

func TestDeleteTask(t *testing.T) {
	pool := newRepositoryTestPool(t)
	pool.execResult = repositoryTestCommandTag{rowsAffected: 1}
	repository := NewTasksRepository(pool)
	ctx := context.WithValue(context.Background(), testutil.RepositoryContextKey{}, "request")

	err := repository.DeleteTask(ctx, 42)

	if err != nil {
		t.Fatalf("DeleteTask() error = %v, want nil", err)
	}
	if pool.execCalls != 1 || !strings.Contains(pool.gotExecSQL, "DELETE FROM todoapp.tasks") {
		t.Fatalf("Exec() calls/sql = %d/%q, want one task DELETE", pool.execCalls, pool.gotExecSQL)
	}
	if !reflect.DeepEqual(pool.gotExecArgs, []any{42}) {
		t.Fatalf("Exec() args = %#v, want [42]", pool.gotExecArgs)
	}
	testutil.AssertRepositoryContext(t, pool.gotContext, ctx)
}

func TestDeleteTaskReturnsExecutionError(t *testing.T) {
	executionError := errors.New("delete failed")
	pool := newRepositoryTestPool(t)
	pool.execError = executionError
	repository := NewTasksRepository(pool)

	err := repository.DeleteTask(context.Background(), 42)

	if !errors.Is(err, executionError) || !strings.Contains(err.Error(), "exec query") {
		t.Fatalf("DeleteTask() error = %v, want wrapped execution error", err)
	}
	if pool.execCalls != 1 {
		t.Fatalf("Exec() calls = %d, want 1", pool.execCalls)
	}
}

func TestDeleteTaskReturnsNotFoundWhenNoRowsAffected(t *testing.T) {
	pool := newRepositoryTestPool(t)
	pool.execResult = repositoryTestCommandTag{rowsAffected: 0}
	repository := NewTasksRepository(pool)

	err := repository.DeleteTask(context.Background(), 42)

	if !errors.Is(err, core_errors.ErrNotFound) {
		t.Fatalf("DeleteTask() error = %v, want ErrNotFound", err)
	}
	if !strings.Contains(err.Error(), "id='42'") {
		t.Fatalf("DeleteTask() error = %v, want requested ID in context", err)
	}
}
