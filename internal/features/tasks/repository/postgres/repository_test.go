package tasks_postgres_repository

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Reigor1/20h-project/internal/core/domain"
	core_errors "github.com/Reigor1/20h-project/internal/core/errors"
	core_events "github.com/Reigor1/20h-project/internal/core/events"
	core_postgres_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool"
	"github.com/Reigor1/20h-project/internal/testutil"
)

type repositoryTestPool struct {
	t *testing.T

	operationTimeout time.Duration
	beginCalls       int
	beginError       error
	transaction      *repositoryTestTransaction
	beginContext     context.Context

	queryCalls    int
	queryRowCalls int
	execCalls     int

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
		transaction: &repositoryTestTransaction{
			t:          t,
			execResult: repositoryTestCommandTag{rowsAffected: 1},
		},
	}
}

func (p *repositoryTestPool) Begin(ctx context.Context) (core_postgres_pool.Transaction, error) {
	p.beginCalls++
	p.beginContext = ctx
	if p.beginError != nil {
		return nil, p.beginError
	}
	if p.transaction == nil {
		p.t.Fatal("pool Begin() has no configured transaction")
	}

	return p.transaction, nil
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

type repositoryTestTransaction struct {
	t *testing.T

	queryCalls    int
	queryRowCalls int
	execCalls     int

	queryResult core_postgres_pool.Rows
	queryError  error
	rowResult   core_postgres_pool.Row
	execResult  core_postgres_pool.CommandTag
	execError   error

	commitCalls   int
	commitError   error
	rollbackCalls int
	rollbackError error

	gotQuerySQL  string
	gotQueryArgs []any
	gotRowSQL    string
	gotRowArgs   []any
	gotExecSQL   string
	gotExecArgs  []any
	gotQueryCtx  context.Context
	gotRowCtx    context.Context
	gotExecCtx   context.Context
	commitCtx    context.Context
	rollbackCtx  context.Context
	callOrder    []string
}

func (tx *repositoryTestTransaction) Query(
	ctx context.Context,
	sql string,
	args ...any,
) (core_postgres_pool.Rows, error) {
	tx.queryCalls++
	tx.gotQueryCtx = ctx
	tx.gotQuerySQL = sql
	tx.gotQueryArgs = append([]any(nil), args...)
	tx.callOrder = append(tx.callOrder, "query")

	if tx.queryResult == nil && tx.queryError == nil {
		tx.t.Fatal("transaction Query() has no configured result or error")
	}

	return tx.queryResult, tx.queryError
}

func (tx *repositoryTestTransaction) QueryRow(
	ctx context.Context,
	sql string,
	args ...any,
) core_postgres_pool.Row {
	tx.queryRowCalls++
	tx.gotRowCtx = ctx
	tx.gotRowSQL = sql
	tx.gotRowArgs = append([]any(nil), args...)
	tx.callOrder = append(tx.callOrder, "query_row")

	if tx.rowResult == nil {
		tx.t.Fatal("transaction QueryRow() has no configured row result")
	}

	return tx.rowResult
}

func (tx *repositoryTestTransaction) Exec(
	ctx context.Context,
	sql string,
	args ...any,
) (core_postgres_pool.CommandTag, error) {
	tx.execCalls++
	tx.gotExecCtx = ctx
	tx.gotExecSQL = sql
	tx.gotExecArgs = append([]any(nil), args...)
	tx.callOrder = append(tx.callOrder, "exec")

	if tx.execResult == nil && tx.execError == nil {
		tx.t.Fatal("transaction Exec() has no configured result or error")
	}

	return tx.execResult, tx.execError
}

func (tx *repositoryTestTransaction) Commit(ctx context.Context) error {
	tx.commitCalls++
	tx.commitCtx = ctx
	tx.callOrder = append(tx.callOrder, "commit")
	return tx.commitError
}

func (tx *repositoryTestTransaction) Rollback(ctx context.Context) error {
	tx.rollbackCalls++
	tx.rollbackCtx = ctx
	tx.callOrder = append(tx.callOrder, "rollback")
	return tx.rollbackError
}

type repositoryTestOutboxRepository struct {
	calls int
	err   error

	gotContext  context.Context
	gotExecutor core_postgres_pool.Executor
	gotEvent    core_events.Envelope
}

func (r *repositoryTestOutboxRepository) Create(
	ctx context.Context,
	executor core_postgres_pool.Executor,
	event core_events.Envelope,
) error {
	r.calls++
	r.gotContext = ctx
	r.gotExecutor = executor
	r.gotEvent = event

	if r.err != nil {
		return r.err
	}

	_, err := executor.Exec(
		ctx,
		`INSERT INTO todoapp.outbox (
			event_id,
			event_type,
			schema_version,
			aggregate_type,
			aggregate_id,
			aggregate_version,
			occurred_at,
			payload
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb)`,
		event.EventID,
		string(event.EventType),
		event.SchemaVersion,
		event.AggregateType,
		event.AggregateID,
		event.AggregateVersion,
		event.OccurredAt,
		event.Payload,
	)
	if err != nil {
		return err
	}

	return nil
}

func newTasksRepositoryForTest(
	pool *repositoryTestPool,
) (*TasksRepository, *repositoryTestOutboxRepository) {
	outboxRepository := &repositoryTestOutboxRepository{}
	repository := NewTasksRepository(pool, outboxRepository)

	return repository, outboxRepository
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

func TestNewTasksRepositoryStoresPoolAndOutboxRepository(t *testing.T) {
	pool := newRepositoryTestPool(t)
	outboxRepository := &repositoryTestOutboxRepository{}

	repository := NewTasksRepository(pool, outboxRepository)

	if repository.pool != pool {
		t.Fatal("NewTasksRepository() did not retain the supplied pool")
	}
	if repository.outboxRepository != outboxRepository {
		t.Fatal("NewTasksRepository() did not retain the supplied outbox repository")
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
	pool.transaction.rowResult = row
	repository, outboxRepository := newTasksRepositoryForTest(pool)
	ctx := context.WithValue(context.Background(), testutil.RepositoryContextKey{}, "request")

	got, err := repository.CreateTask(ctx, inputTask)
	if err != nil {
		t.Fatalf("CreateTask() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(got, wantTask) {
		t.Fatalf("CreateTask() = %+v, want %+v", got, wantTask)
	}
	if pool.beginCalls != 1 {
		t.Fatalf("pool Begin() calls = %d, want 1", pool.beginCalls)
	}
	if pool.transaction.queryRowCalls != 1 ||
		!strings.Contains(pool.transaction.gotRowSQL, "INSERT INTO todoapp.tasks") ||
		!strings.Contains(pool.transaction.gotRowSQL, "RETURNING") {
		t.Fatalf("transaction QueryRow() calls/sql = %d/%q, want one task INSERT", pool.transaction.queryRowCalls, pool.transaction.gotRowSQL)
	}
	wantArgs := []any{"Write report", &description, true, createdAt, &completedAt, 23}
	if !reflect.DeepEqual(pool.transaction.gotRowArgs, wantArgs) {
		t.Fatalf("transaction QueryRow() args = %#v, want %#v", pool.transaction.gotRowArgs, wantArgs)
	}
	testutil.AssertRepositoryContext(t, pool.transaction.gotRowCtx, ctx)
	if row.scans != 1 {
		t.Fatalf("row Scan() calls = %d, want 1", row.scans)
	}
	assertOutboxEvent(t, pool, outboxRepository, core_events.EventTypeTaskCreated, wantTask, nil)
}

func TestCreateTaskMapsForeignKeyViolationToNotFound(t *testing.T) {
	row := &repositoryTestRow{err: core_postgres_pool.ErrViolatesForeignKey}
	pool := newRepositoryTestPool(t)
	pool.transaction.rowResult = row
	repository, outboxRepository := newTasksRepositoryForTest(pool)
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
	if outboxRepository.calls != 0 {
		t.Fatalf("outbox Create() calls = %d, want 0", outboxRepository.calls)
	}
	if pool.transaction.commitCalls != 0 || pool.transaction.rollbackCalls != 1 {
		t.Fatalf("transaction commit/rollback calls = %d/%d, want 0/1", pool.transaction.commitCalls, pool.transaction.rollbackCalls)
	}
}

func TestCreateTaskReturnsScanError(t *testing.T) {
	scanError := errors.New("returned task could not be decoded")
	row := &repositoryTestRow{err: scanError}
	pool := newRepositoryTestPool(t)
	pool.transaction.rowResult = row
	repository, outboxRepository := newTasksRepositoryForTest(pool)
	task := domain.NewTask(0, 0, "Write report", nil, false, time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC), nil, 23)

	_, err := repository.CreateTask(context.Background(), task)

	if !errors.Is(err, scanError) || !strings.Contains(err.Error(), "insert task") {
		t.Fatalf("CreateTask() error = %v, want wrapped insert/scan error", err)
	}
	if outboxRepository.calls != 0 {
		t.Fatalf("outbox Create() calls = %d, want 0", outboxRepository.calls)
	}
	if pool.transaction.commitCalls != 0 || pool.transaction.rollbackCalls != 1 {
		t.Fatalf("transaction commit/rollback calls = %d/%d, want 0/1", pool.transaction.commitCalls, pool.transaction.rollbackCalls)
	}
}

func TestCreateTaskReturnsBeginError(t *testing.T) {
	beginError := errors.New("begin transaction failed")
	pool := newRepositoryTestPool(t)
	pool.beginError = beginError
	repository, outboxRepository := newTasksRepositoryForTest(pool)
	task := domain.NewTask(0, 0, "Write report", nil, false, time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC), nil, 23)

	_, err := repository.CreateTask(context.Background(), task)

	if !errors.Is(err, beginError) {
		t.Fatalf("CreateTask() error = %v, want wrapped begin error", err)
	}
	if outboxRepository.calls != 0 {
		t.Fatalf("outbox Create() calls = %d, want 0", outboxRepository.calls)
	}
}

func TestCreateTaskRollsBackWhenOutboxInsertFails(t *testing.T) {
	outboxError := errors.New("outbox insert failed")
	row := &repositoryTestRow{
		values: []any{17, 1, "Write report", (*string)(nil), false, time.Date(2026, 10, 21, 9, 15, 0, 0, time.UTC), (*time.Time)(nil), 23},
	}
	pool := newRepositoryTestPool(t)
	pool.transaction.rowResult = row
	repository, outboxRepository := newTasksRepositoryForTest(pool)
	pool.transaction.execError = outboxError
	task := domain.NewTask(0, 0, "Write report", nil, false, time.Date(2026, 10, 21, 9, 15, 0, 0, time.UTC), nil, 23)

	_, err := repository.CreateTask(context.Background(), task)

	if !errors.Is(err, outboxError) || !strings.Contains(err.Error(), "save task event") {
		t.Fatalf("CreateTask() error = %v, want wrapped outbox error", err)
	}
	if outboxRepository.calls != 1 {
		t.Fatalf("outbox Create() calls = %d, want 1", outboxRepository.calls)
	}
	if pool.transaction.commitCalls != 0 || pool.transaction.rollbackCalls != 1 {
		t.Fatalf("transaction commit/rollback calls = %d/%d, want 0/1", pool.transaction.commitCalls, pool.transaction.rollbackCalls)
	}
	if pool.transaction.execCalls != 1 {
		t.Fatalf("transaction Exec() calls = %d, want 1 failed outbox INSERT", pool.transaction.execCalls)
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
	repository, _ := newTasksRepositoryForTest(pool)
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
	if pool.beginCalls != 0 {
		t.Fatalf("pool Begin() calls = %d, want 0 for GetTask", pool.beginCalls)
	}
}

func TestGetTaskPreservesNullDescriptionAndCompletionTime(t *testing.T) {
	createdAt := time.Date(2026, 10, 22, 8, 30, 0, 0, time.UTC)
	row := &repositoryTestRow{
		values: []any{42, 3, "Unfinished task", (*string)(nil), false, createdAt, (*time.Time)(nil), 23},
	}
	pool := newRepositoryTestPool(t)
	pool.rowResult = row
	repository, _ := newTasksRepositoryForTest(pool)

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
	repository, _ := newTasksRepositoryForTest(pool)

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
	repository, _ := newTasksRepositoryForTest(pool)

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
	repository, _ := newTasksRepositoryForTest(pool)
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
	repository, _ := newTasksRepositoryForTest(pool)

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
	repository, _ := newTasksRepositoryForTest(pool)

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
	repository, _ := newTasksRepositoryForTest(pool)

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
	repository, _ := newTasksRepositoryForTest(pool)

	_, err := repository.GetTasks(context.Background(), nil, nil, nil)

	if !errors.Is(err, iterationError) || !strings.Contains(err.Error(), "scan tasks") {
		t.Fatalf("GetTasks() error = %v, want wrapped iteration error", err)
	}
	if rows.closeCalls != 1 {
		t.Fatalf("rows Close() calls = %d, want 1 after iteration failure", rows.closeCalls)
	}
}

func TestPatchTask(t *testing.T) {
	oldDescription := "Previous task details"
	newDescription := "Updated task details"
	createdAt := time.Date(2026, 10, 24, 8, 0, 0, 0, time.UTC)
	completedAt := time.Date(2026, 10, 24, 9, 30, 0, 0, time.UTC)
	before := domain.NewTask(42, 5, "Prepare report", &oldDescription, true, createdAt, &completedAt, 23)
	after := domain.NewTask(42, 5, "Publish report", &newDescription, true, createdAt, &completedAt, 23)
	wantTask := domain.NewTask(42, 6, "Publish report", &newDescription, true, createdAt, &completedAt, 23)
	row := &repositoryTestRow{
		values: []any{42, 6, "Publish report", &newDescription, true, createdAt, &completedAt, 23},
	}
	pool := newRepositoryTestPool(t)
	pool.transaction.rowResult = row
	repository, outboxRepository := newTasksRepositoryForTest(pool)
	ctx := context.WithValue(context.Background(), testutil.RepositoryContextKey{}, "request")

	got, err := repository.PatchTask(ctx, 42, before, after)
	if err != nil {
		t.Fatalf("PatchTask() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(got, wantTask) {
		t.Fatalf("PatchTask() = %+v, want %+v", got, wantTask)
	}
	if pool.beginCalls != 1 {
		t.Fatalf("pool Begin() calls = %d, want 1", pool.beginCalls)
	}
	if pool.transaction.queryRowCalls != 1 || !strings.Contains(pool.transaction.gotRowSQL, "UPDATE todoapp.tasks") {
		t.Fatalf("transaction QueryRow() calls/sql = %d/%q, want one task UPDATE", pool.transaction.queryRowCalls, pool.transaction.gotRowSQL)
	}
	for _, sqlFragment := range []string{
		"version = version + 1",
		"WHERE id = $5 AND version = $6",
		"RETURNING",
	} {
		if !strings.Contains(pool.transaction.gotRowSQL, sqlFragment) {
			t.Errorf("PatchTask() SQL = %q, missing %q", pool.transaction.gotRowSQL, sqlFragment)
		}
	}
	wantArgs := []any{"Publish report", &newDescription, true, &completedAt, 42, 5}
	if !reflect.DeepEqual(pool.transaction.gotRowArgs, wantArgs) {
		t.Fatalf("transaction QueryRow() args = %#v, want %#v", pool.transaction.gotRowArgs, wantArgs)
	}
	testutil.AssertRepositoryContext(t, pool.transaction.gotRowCtx, ctx)
	assertOutboxEvent(t, pool, outboxRepository, core_events.EventTypeTaskUpdated, wantTask, &before)
}

func TestPatchTaskCreatesTaskCompletedEvent(t *testing.T) {
	createdAt := time.Date(2026, 10, 24, 8, 0, 0, 0, time.UTC)
	completedAt := createdAt.Add(time.Hour)
	before := domain.NewTask(42, 5, "Finish report", nil, false, createdAt, nil, 23)
	after := domain.NewTask(42, 5, "Finish report", nil, true, createdAt, &completedAt, 23)
	wantTask := domain.NewTask(42, 6, "Finish report", nil, true, createdAt, &completedAt, 23)
	row := &repositoryTestRow{
		values: []any{42, 6, "Finish report", (*string)(nil), true, createdAt, &completedAt, 23},
	}
	pool := newRepositoryTestPool(t)
	pool.transaction.rowResult = row
	repository, outboxRepository := newTasksRepositoryForTest(pool)

	got, err := repository.PatchTask(context.Background(), 42, before, after)
	if err != nil {
		t.Fatalf("PatchTask() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(got, wantTask) {
		t.Fatalf("PatchTask() = %+v, want %+v", got, wantTask)
	}
	assertOutboxEvent(t, pool, outboxRepository, core_events.EventTypeTaskCompleted, wantTask, &before)
}

func TestPatchTaskPassesNilForClearedNullableFields(t *testing.T) {
	oldDescription := "Remove this description"
	createdAt := time.Date(2026, 10, 24, 8, 0, 0, 0, time.UTC)
	completedAt := createdAt.Add(time.Hour)
	before := domain.NewTask(42, 5, "Unfinished task", &oldDescription, true, createdAt, &completedAt, 23)
	after := domain.NewTask(42, 5, "Unfinished task", nil, false, createdAt, nil, 23)
	row := &repositoryTestRow{
		values: []any{42, 6, "Unfinished task", (*string)(nil), false, createdAt, (*time.Time)(nil), 23},
	}
	pool := newRepositoryTestPool(t)
	pool.transaction.rowResult = row
	repository, _ := newTasksRepositoryForTest(pool)

	got, err := repository.PatchTask(context.Background(), 42, before, after)
	if err != nil {
		t.Fatalf("PatchTask() error = %v, want nil", err)
	}
	if got.Description != nil || got.CompletedAt != nil {
		t.Fatalf("PatchTask() nullable fields = description %v, completedAt %v, want nil/nil", got.Description, got.CompletedAt)
	}
	if len(pool.transaction.gotRowArgs) != 6 ||
		pool.transaction.gotRowArgs[1] != (*string)(nil) ||
		pool.transaction.gotRowArgs[3] != (*time.Time)(nil) {
		t.Fatalf("PatchTask() nullable SQL args = %#v, want nil description and completion time", pool.transaction.gotRowArgs)
	}
}

func TestPatchTaskNoOpDoesNotStartTransaction(t *testing.T) {
	createdAt := time.Date(2026, 10, 24, 8, 0, 0, 0, time.UTC)
	completedAt := createdAt.Add(time.Hour)
	description := "Unchanged task"
	task := domain.NewTask(42, 5, "Unchanged task", &description, true, createdAt, &completedAt, 23)
	pool := newRepositoryTestPool(t)
	repository, outboxRepository := newTasksRepositoryForTest(pool)

	got, err := repository.PatchTask(context.Background(), 42, task, task)
	if err != nil {
		t.Fatalf("PatchTask() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(got, task) {
		t.Fatalf("PatchTask() = %+v, want unchanged task %+v", got, task)
	}
	if pool.beginCalls != 0 {
		t.Fatalf("pool Begin() calls = %d, want 0 for no-op update", pool.beginCalls)
	}
	if outboxRepository.calls != 0 {
		t.Fatalf("outbox Create() calls = %d, want 0 for no-op update", outboxRepository.calls)
	}
	if pool.transaction.queryRowCalls != 0 || pool.transaction.execCalls != 0 {
		t.Fatalf("transaction QueryRow()/Exec() calls = %d/%d, want 0/0", pool.transaction.queryRowCalls, pool.transaction.execCalls)
	}
}

func TestPatchTaskRejectsMismatchedTaskState(t *testing.T) {
	createdAt := time.Date(2026, 10, 24, 8, 0, 0, 0, time.UTC)
	before := domain.NewTask(42, 5, "Before", nil, false, createdAt, nil, 23)
	after := domain.NewTask(43, 5, "After", nil, false, createdAt, nil, 23)
	pool := newRepositoryTestPool(t)
	repository, outboxRepository := newTasksRepositoryForTest(pool)

	_, err := repository.PatchTask(context.Background(), 42, before, after)

	if !errors.Is(err, core_errors.ErrInvalidArgument) {
		t.Fatalf("PatchTask() error = %v, want ErrInvalidArgument", err)
	}
	if pool.beginCalls != 0 || outboxRepository.calls != 0 {
		t.Fatalf("Begin()/outbox Create() calls = %d/%d, want 0/0", pool.beginCalls, outboxRepository.calls)
	}
}

func TestPatchTaskTranslatesNoRowsToConflict(t *testing.T) {
	row := &repositoryTestRow{err: core_postgres_pool.ErrNoRows}
	pool := newRepositoryTestPool(t)
	pool.transaction.rowResult = row
	repository, outboxRepository := newTasksRepositoryForTest(pool)
	createdAt := time.Date(2026, 10, 24, 8, 0, 0, 0, time.UTC)
	before := domain.NewTask(42, 5, "Original task", nil, false, createdAt, nil, 23)
	after := domain.NewTask(42, 5, "Updated task", nil, false, createdAt, nil, 23)

	_, err := repository.PatchTask(context.Background(), 42, before, after)

	if !errors.Is(err, core_errors.ErrConflict) {
		t.Fatalf("PatchTask() error = %v, want ErrConflict", err)
	}
	if !strings.Contains(err.Error(), "id=42") || !strings.Contains(err.Error(), "concurrently modified") {
		t.Fatalf("PatchTask() error = %v, want stale update context", err)
	}
	if outboxRepository.calls != 0 {
		t.Fatalf("outbox Create() calls = %d, want 0", outboxRepository.calls)
	}
	if pool.transaction.commitCalls != 0 || pool.transaction.rollbackCalls != 1 {
		t.Fatalf("transaction commit/rollback calls = %d/%d, want 0/1", pool.transaction.commitCalls, pool.transaction.rollbackCalls)
	}
}

func TestPatchTaskReturnsScanError(t *testing.T) {
	scanError := errors.New("updated task could not be decoded")
	row := &repositoryTestRow{err: scanError}
	pool := newRepositoryTestPool(t)
	pool.transaction.rowResult = row
	repository, outboxRepository := newTasksRepositoryForTest(pool)
	createdAt := time.Date(2026, 10, 24, 8, 0, 0, 0, time.UTC)
	before := domain.NewTask(42, 5, "Original task", nil, false, createdAt, nil, 23)
	after := domain.NewTask(42, 5, "Updated task", nil, false, createdAt, nil, 23)

	_, err := repository.PatchTask(context.Background(), 42, before, after)

	if !errors.Is(err, scanError) || !strings.Contains(err.Error(), "update task") {
		t.Fatalf("PatchTask() error = %v, want wrapped update/scan error", err)
	}
	if outboxRepository.calls != 0 {
		t.Fatalf("outbox Create() calls = %d, want 0", outboxRepository.calls)
	}
}

func TestPatchTaskRollsBackWhenOutboxInsertFails(t *testing.T) {
	outboxError := errors.New("outbox insert failed")
	createdAt := time.Date(2026, 10, 24, 8, 0, 0, 0, time.UTC)
	before := domain.NewTask(42, 5, "Original task", nil, false, createdAt, nil, 23)
	after := domain.NewTask(42, 5, "Updated task", nil, false, createdAt, nil, 23)
	row := &repositoryTestRow{
		values: []any{42, 6, "Updated task", (*string)(nil), false, createdAt, (*time.Time)(nil), 23},
	}
	pool := newRepositoryTestPool(t)
	pool.transaction.rowResult = row
	repository, _ := newTasksRepositoryForTest(pool)
	pool.transaction.execError = outboxError

	_, err := repository.PatchTask(context.Background(), 42, before, after)

	if !errors.Is(err, outboxError) || !strings.Contains(err.Error(), "save task update event") {
		t.Fatalf("PatchTask() error = %v, want wrapped outbox error", err)
	}
	if pool.transaction.commitCalls != 0 || pool.transaction.rollbackCalls != 1 {
		t.Fatalf("transaction commit/rollback calls = %d/%d, want 0/1", pool.transaction.commitCalls, pool.transaction.rollbackCalls)
	}
}

func TestDeleteTask(t *testing.T) {
	createdAt := time.Date(2026, 10, 25, 8, 0, 0, 0, time.UTC)
	description := "Delete this task after preserving its snapshot"
	completedAt := createdAt.Add(time.Hour)
	deletedTask := domain.NewTask(42, 7, "Delete task", &description, true, createdAt, &completedAt, 23)
	row := &repositoryTestRow{
		values: []any{42, 7, "Delete task", &description, true, createdAt, &completedAt, 23},
	}
	pool := newRepositoryTestPool(t)
	pool.transaction.rowResult = row
	repository, outboxRepository := newTasksRepositoryForTest(pool)
	ctx := context.WithValue(context.Background(), testutil.RepositoryContextKey{}, "request")

	err := repository.DeleteTask(ctx, 42)
	if err != nil {
		t.Fatalf("DeleteTask() error = %v, want nil", err)
	}
	if pool.beginCalls != 1 {
		t.Fatalf("pool Begin() calls = %d, want 1", pool.beginCalls)
	}
	if pool.transaction.queryRowCalls != 1 ||
		!strings.Contains(pool.transaction.gotRowSQL, "DELETE FROM todoapp.tasks") ||
		!strings.Contains(pool.transaction.gotRowSQL, "RETURNING") {
		t.Fatalf("transaction QueryRow() calls/sql = %d/%q, want DELETE ... RETURNING", pool.transaction.queryRowCalls, pool.transaction.gotRowSQL)
	}
	if !reflect.DeepEqual(pool.transaction.gotRowArgs, []any{42}) {
		t.Fatalf("transaction QueryRow() args = %#v, want [42]", pool.transaction.gotRowArgs)
	}
	testutil.AssertRepositoryContext(t, pool.transaction.gotRowCtx, ctx)
	assertOutboxEvent(t, pool, outboxRepository, core_events.EventTypeTaskDeleted, deletedTask, nil)
}

func TestDeleteTaskReturnsDeleteError(t *testing.T) {
	executionError := errors.New("delete failed")
	row := &repositoryTestRow{err: executionError}
	pool := newRepositoryTestPool(t)
	pool.transaction.rowResult = row
	repository, outboxRepository := newTasksRepositoryForTest(pool)

	err := repository.DeleteTask(context.Background(), 42)

	if !errors.Is(err, executionError) || !strings.Contains(err.Error(), "delete task") {
		t.Fatalf("DeleteTask() error = %v, want wrapped delete error", err)
	}
	if pool.transaction.queryRowCalls != 1 {
		t.Fatalf("transaction QueryRow() calls = %d, want 1", pool.transaction.queryRowCalls)
	}
	if outboxRepository.calls != 0 {
		t.Fatalf("outbox Create() calls = %d, want 0", outboxRepository.calls)
	}
	if pool.transaction.commitCalls != 0 || pool.transaction.rollbackCalls != 1 {
		t.Fatalf("transaction commit/rollback calls = %d/%d, want 0/1", pool.transaction.commitCalls, pool.transaction.rollbackCalls)
	}
}

func TestDeleteTaskReturnsNotFoundWhenTaskDoesNotExist(t *testing.T) {
	row := &repositoryTestRow{err: core_postgres_pool.ErrNoRows}
	pool := newRepositoryTestPool(t)
	pool.transaction.rowResult = row
	repository, outboxRepository := newTasksRepositoryForTest(pool)

	err := repository.DeleteTask(context.Background(), 42)

	if !errors.Is(err, core_errors.ErrNotFound) {
		t.Fatalf("DeleteTask() error = %v, want ErrNotFound", err)
	}
	if !strings.Contains(err.Error(), "id=42") {
		t.Fatalf("DeleteTask() error = %v, want requested ID in context", err)
	}
	if outboxRepository.calls != 0 {
		t.Fatalf("outbox Create() calls = %d, want 0", outboxRepository.calls)
	}
	if pool.transaction.commitCalls != 0 || pool.transaction.rollbackCalls != 1 {
		t.Fatalf("transaction commit/rollback calls = %d/%d, want 0/1", pool.transaction.commitCalls, pool.transaction.rollbackCalls)
	}
}

func TestDeleteTaskRollsBackWhenOutboxInsertFails(t *testing.T) {
	outboxError := errors.New("outbox insert failed")
	createdAt := time.Date(2026, 10, 25, 8, 0, 0, 0, time.UTC)
	row := &repositoryTestRow{
		values: []any{42, 7, "Delete task", (*string)(nil), false, createdAt, (*time.Time)(nil), 23},
	}
	pool := newRepositoryTestPool(t)
	pool.transaction.rowResult = row
	repository, _ := newTasksRepositoryForTest(pool)
	pool.transaction.execError = outboxError

	err := repository.DeleteTask(context.Background(), 42)

	if !errors.Is(err, outboxError) || !strings.Contains(err.Error(), "save task deletion event") {
		t.Fatalf("DeleteTask() error = %v, want wrapped outbox error", err)
	}
	if pool.transaction.commitCalls != 0 || pool.transaction.rollbackCalls != 1 {
		t.Fatalf("transaction commit/rollback calls = %d/%d, want 0/1", pool.transaction.commitCalls, pool.transaction.rollbackCalls)
	}
}

func assertOutboxEvent(
	t *testing.T,
	pool *repositoryTestPool,
	outboxRepository *repositoryTestOutboxRepository,
	wantEventType core_events.EventType,
	task domain.Task,
	before *domain.Task,
) {
	t.Helper()

	if outboxRepository.calls != 1 {
		t.Fatalf("outbox Create() calls = %d, want 1", outboxRepository.calls)
	}

	transaction, ok := outboxRepository.gotExecutor.(*repositoryTestTransaction)
	if !ok || transaction != pool.transaction {
		t.Fatal("outbox Create() did not receive the task transaction as its Executor")
	}

	if outboxRepository.gotEvent.EventType != wantEventType {
		t.Errorf("outbox event type = %q, want %q", outboxRepository.gotEvent.EventType, wantEventType)
	}
	if outboxRepository.gotEvent.SchemaVersion != core_events.CurrentSchemaVersion {
		t.Errorf(
			"outbox schema version = %d, want %d",
			outboxRepository.gotEvent.SchemaVersion,
			core_events.CurrentSchemaVersion,
		)
	}
	if outboxRepository.gotEvent.AggregateType != core_events.AggregateTypeTask {
		t.Errorf(
			"outbox aggregate type = %q, want %q",
			outboxRepository.gotEvent.AggregateType,
			core_events.AggregateTypeTask,
		)
	}
	if outboxRepository.gotEvent.AggregateID != int64(task.ID) {
		t.Errorf("outbox aggregate ID = %d, want %d", outboxRepository.gotEvent.AggregateID, task.ID)
	}
	if outboxRepository.gotEvent.AggregateVersion != int64(task.Version) {
		t.Errorf("outbox aggregate version = %d, want %d", outboxRepository.gotEvent.AggregateVersion, task.Version)
	}
	if err := outboxRepository.gotEvent.Validate(); err != nil {
		t.Errorf("outbox event Validate() error = %v, want nil", err)
	}

	if transaction.execCalls != 1 || !strings.Contains(transaction.gotExecSQL, "INSERT INTO todoapp.outbox") {
		t.Fatalf("transaction Exec() calls/sql = %d/%q, want one outbox INSERT", transaction.execCalls, transaction.gotExecSQL)
	}
	if transaction.commitCalls != 1 {
		t.Fatalf("transaction Commit() calls = %d, want 1", transaction.commitCalls)
	}
	if transaction.rollbackCalls != 1 {
		t.Fatalf("transaction Rollback() calls = %d, want 1 deferred cleanup", transaction.rollbackCalls)
	}
	if len(transaction.callOrder) < 4 {
		t.Fatalf("transaction call order = %#v, want task query, outbox insert, commit and rollback", transaction.callOrder)
	}
	if transaction.callOrder[len(transaction.callOrder)-3] != "exec" ||
		transaction.callOrder[len(transaction.callOrder)-2] != "commit" ||
		transaction.callOrder[len(transaction.callOrder)-1] != "rollback" {
		t.Fatalf("transaction call order = %#v, want outbox INSERT before COMMIT", transaction.callOrder)
	}

	switch wantEventType {
	case core_events.EventTypeTaskCreated:
		var payload core_events.TaskCreatedPayload
		if err := json.Unmarshal(outboxRepository.gotEvent.Payload, &payload); err != nil {
			t.Fatalf("unmarshal task.created payload: %v", err)
		}
		if !taskSnapshotsEqual(payload.Task, taskSnapshotForTest(task)) {
			t.Errorf("task.created payload = %+v, want task snapshot %+v", payload.Task, taskSnapshotForTest(task))
		}
	case core_events.EventTypeTaskUpdated, core_events.EventTypeTaskCompleted:
		if before == nil {
			t.Fatal("before task is required when asserting task update event")
		}
		var payload core_events.TaskUpdatedPayload
		if err := json.Unmarshal(outboxRepository.gotEvent.Payload, &payload); err != nil {
			t.Fatalf("unmarshal task update payload: %v", err)
		}
		if !taskSnapshotsEqual(payload.Before, taskSnapshotForTest(*before)) {
			t.Errorf("task update before snapshot = %+v, want %+v", payload.Before, taskSnapshotForTest(*before))
		}
		if !taskSnapshotsEqual(payload.After, taskSnapshotForTest(task)) {
			t.Errorf("task update after snapshot = %+v, want %+v", payload.After, taskSnapshotForTest(task))
		}
	case core_events.EventTypeTaskDeleted:
		var payload core_events.TaskDeletedPayload
		if err := json.Unmarshal(outboxRepository.gotEvent.Payload, &payload); err != nil {
			t.Fatalf("unmarshal task.deleted payload: %v", err)
		}
		if !taskSnapshotsEqual(payload.Task, taskSnapshotForTest(task)) {
			t.Errorf("task.deleted payload = %+v, want task snapshot %+v", payload.Task, taskSnapshotForTest(task))
		}
	}
}

func taskSnapshotForTest(task domain.Task) core_events.TaskSnapshot {
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

func taskSnapshotsEqual(left, right core_events.TaskSnapshot) bool {
	if left.ID != right.ID ||
		left.Version != right.Version ||
		left.Title != right.Title ||
		left.Completed != right.Completed ||
		!left.CreatedAt.Equal(right.CreatedAt) ||
		left.AuthorUserID != right.AuthorUserID {
		return false
	}

	if left.Description == nil || right.Description == nil {
		if left.Description != nil || right.Description != nil {
			return false
		}
	} else if *left.Description != *right.Description {
		return false
	}

	if left.CompletedAt == nil || right.CompletedAt == nil {
		return left.CompletedAt == nil && right.CompletedAt == nil
	}

	return left.CompletedAt.Equal(*right.CompletedAt)
}
