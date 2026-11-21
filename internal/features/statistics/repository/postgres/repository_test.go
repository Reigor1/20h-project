package statistics_postgres_repository

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Reigor1/20h-project/internal/core/domain"
	core_postgres_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool"
	"github.com/Reigor1/20h-project/internal/testutil"
)

// repositoryTestPool captures database calls and returns configured rows or errors.
type repositoryTestPool struct {
	t *testing.T

	queryRows  core_postgres_pool.Rows
	queryError error

	queryCalls    int
	queryRowCalls int
	execCalls     int
	gotContext    context.Context
	gotSQL        string
	gotArgs       []any
}

// newRepositoryTestPool creates a strict fake pool that fails on unexpected database operations.
func newRepositoryTestPool(t *testing.T) *repositoryTestPool {
	t.Helper()

	return &repositoryTestPool{t: t}
}

// Query records the SQL, arguments, and context supplied by the repository.
func (p *repositoryTestPool) Query(
	ctx context.Context,
	sql string,
	args ...any,
) (core_postgres_pool.Rows, error) {
	p.queryCalls++
	if p.queryCalls > 1 || p.queryRowCalls != 0 || p.execCalls != 0 {
		p.t.Fatal("unexpected additional PostgreSQL pool call")
	}
	p.gotContext = ctx
	p.gotSQL = sql
	p.gotArgs = append([]any(nil), args...)
	if p.queryRows == nil && p.queryError == nil {
		p.t.Fatal("pool Query() has no configured rows or error")
	}

	return p.queryRows, p.queryError
}

// QueryRow fails because statistics task listing must use the rows query operation.
func (p *repositoryTestPool) QueryRow(context.Context, string, ...any) core_postgres_pool.Row {
	p.queryRowCalls++
	p.t.Fatal("unexpected PostgreSQL pool QueryRow() call")
	return nil
}

// Exec fails because statistics retrieval must not execute database mutations.
func (p *repositoryTestPool) Exec(context.Context, string, ...any) (core_postgres_pool.CommandTag, error) {
	p.execCalls++
	p.t.Fatal("unexpected PostgreSQL pool Exec() call")
	return nil, nil
}

// Close satisfies the pool contract; the fake owns no external resources.
func (*repositoryTestPool) Close() {}

// OpTimeout returns the shared bounded timeout expected by repository tests.
func (*repositoryTestPool) OpTimeout() time.Duration {
	return testutil.RepositoryOperationTimeout
}

// repositoryTestRows simulates row iteration, scanning, iteration errors, and Close calls.
type repositoryTestRows struct {
	rows      [][]any
	current   int
	scanError error
	rowsError error
	closeCall int
}

// Next advances through the configured row fixtures.
func (r *repositoryTestRows) Next() bool {
	if r.current >= len(r.rows) {
		return false
	}
	r.current++
	return true
}

// Scan copies the current fixture values into the destinations supplied by the repository.
func (r *repositoryTestRows) Scan(destinations ...any) error {
	if r.scanError != nil {
		return r.scanError
	}
	if len(destinations) != len(r.rows[r.current-1]) {
		return errors.New("unexpected scan destination count")
	}

	for index, value := range r.rows[r.current-1] {
		if err := assignStatisticsScanValue(destinations[index], value); err != nil {
			return err
		}
	}

	return nil
}

// Err returns the configured error reported after row iteration.
func (r *repositoryTestRows) Err() error {
	return r.rowsError
}

// Close records resource cleanup so tests can verify rows are always closed.
func (r *repositoryTestRows) Close() {
	r.closeCall++
}

// assignStatisticsScanValue assigns a typed fixture value to a supported Scan destination.
func assignStatisticsScanValue(destination, value any) error {
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
		if value != nil && !ok {
			return errors.New("scan value is not a *string or nil")
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
		if value != nil && !ok {
			return errors.New("scan value is not a *time.Time or nil")
		}
		*destination = scannedValue
	default:
		return errors.New("unsupported scan destination")
	}

	return nil
}

// TestNewStatisticsRepositoryStoresPool verifies the repository retains its database dependency.
func TestNewStatisticsRepositoryStoresPool(t *testing.T) {
	pool := newRepositoryTestPool(t)

	repository := NewStatisticsRepository(pool)

	if repository.pool != pool {
		t.Fatal("NewStatisticsRepository() did not retain the supplied pool")
	}
}

// TestGetTasksBuildsQueriesForOptionalFilters verifies SQL predicates and argument order
// for each combination of owner and date filters.
func TestGetTasksBuildsQueriesForOptionalFilters(t *testing.T) {
	userID := 27
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	baseQuery := strings.Join(strings.Fields(`
		SELECT id, version, title, description, completed, created_at, completed_at, author_user_id
		FROM todoapp.tasks
	`), " ")
	tests := []struct {
		name     string
		userID   *int
		from     *time.Time
		to       *time.Time
		where    string
		wantArgs []any
	}{
		{
			name: "without filters",
		},
		{
			name:     "filters by user",
			userID:   &userID,
			where:    " WHERE author_user_id=$1",
			wantArgs: []any{&userID},
		},
		{
			name:     "filters from date without user",
			from:     &from,
			where:    " WHERE created_at>=$1",
			wantArgs: []any{&from},
		},
		{
			name:     "filters to date without user",
			to:       &to,
			where:    " WHERE created_at<$1",
			wantArgs: []any{&to},
		},
		{
			name:     "applies all filters in argument order",
			userID:   &userID,
			from:     &from,
			to:       &to,
			where:    " WHERE author_user_id=$1 AND created_at>=$2 AND created_at<$3",
			wantArgs: []any{&userID, &from, &to},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			rows := &repositoryTestRows{}
			pool := newRepositoryTestPool(t)
			pool.queryRows = rows
			repository := NewStatisticsRepository(pool)
			parentContext := context.WithValue(
				context.Background(),
				testutil.RepositoryContextKey{},
				"statistics request",
			)

			got, err := repository.GetTasks(parentContext, testCase.userID, testCase.from, testCase.to)

			if err != nil {
				t.Fatalf("GetTasks() error = %v, want nil", err)
			}
			if got == nil || len(got) != 0 {
				t.Fatalf("GetTasks() = %#v, want non-nil empty slice", got)
			}
			wantSQL := baseQuery + testCase.where + " ORDER BY id ASC"
			gotSQL := strings.Join(strings.Fields(pool.gotSQL), " ")
			if gotSQL != wantSQL {
				t.Fatalf("Query() SQL = %q, want %q", gotSQL, wantSQL)
			}
			if !reflect.DeepEqual(pool.gotArgs, testCase.wantArgs) {
				t.Fatalf("Query() args = %#v, want %#v", pool.gotArgs, testCase.wantArgs)
			}
			if pool.queryCalls != 1 || pool.queryRowCalls != 0 || pool.execCalls != 0 {
				t.Fatalf(
					"pool calls = Query:%d QueryRow:%d Exec:%d, want one Query call",
					pool.queryCalls,
					pool.queryRowCalls,
					pool.execCalls,
				)
			}
			testutil.AssertRepositoryContext(t, pool.gotContext, parentContext)
			if rows.closeCall != 1 {
				t.Fatalf("Rows.Close() calls = %d, want 1", rows.closeCall)
			}
		})
	}
}

// TestGetTasksMapsRowsInOrderIncludingNullableFields verifies model conversion, result order,
// and preservation of nullable task columns.
func TestGetTasksMapsRowsInOrderIncludingNullableFields(t *testing.T) {
	firstDescription := "Review completed work"
	firstCreatedAt := time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)
	firstCompletedAt := firstCreatedAt.Add(45 * time.Minute)
	secondCreatedAt := firstCreatedAt.Add(time.Hour)
	rows := &repositoryTestRows{
		rows: [][]any{
			{11, 2, "Completed task", &firstDescription, true, firstCreatedAt, &firstCompletedAt, 27},
			{18, 1, "Open task", nil, false, secondCreatedAt, nil, 31},
		},
	}
	pool := newRepositoryTestPool(t)
	pool.queryRows = rows
	repository := NewStatisticsRepository(pool)

	got, err := repository.GetTasks(context.Background(), nil, nil, nil)

	if err != nil {
		t.Fatalf("GetTasks() error = %v, want nil", err)
	}
	want := []domain.Task{
		domain.NewTask(11, 2, "Completed task", &firstDescription, true, firstCreatedAt, &firstCompletedAt, 27),
		domain.NewTask(18, 1, "Open task", nil, false, secondCreatedAt, nil, 31),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetTasks() = %+v, want %+v", got, want)
	}
	if rows.closeCall != 1 {
		t.Fatalf("Rows.Close() calls = %d, want 1", rows.closeCall)
	}
}

// TestGetTasksReturnsQueryError checks query error wrapping and the nil result on failure.
func TestGetTasksReturnsQueryError(t *testing.T) {
	queryError := errors.New("connection unavailable")
	pool := newRepositoryTestPool(t)
	pool.queryError = queryError
	repository := NewStatisticsRepository(pool)

	got, err := repository.GetTasks(context.Background(), nil, nil, nil)

	if !errors.Is(err, queryError) {
		t.Fatalf("GetTasks() error = %v, want wrapped query error", err)
	}
	if !strings.Contains(err.Error(), "select tasks") {
		t.Fatalf("GetTasks() error = %v, want query context", err)
	}
	if got != nil {
		t.Fatalf("GetTasks() = %+v, want nil on query error", got)
	}
	if pool.queryCalls != 1 {
		t.Fatalf("Query() calls = %d, want 1", pool.queryCalls)
	}
}

// TestGetTasksReturnsScanErrorAndClosesRows verifies scan failures are returned and rows are closed.
func TestGetTasksReturnsScanErrorAndClosesRows(t *testing.T) {
	scanError := errors.New("invalid row")
	rows := &repositoryTestRows{
		rows: [][]any{
			{11, 2, "Task", nil, false, time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC), nil, 27},
		},
		scanError: scanError,
	}
	pool := newRepositoryTestPool(t)
	pool.queryRows = rows
	repository := NewStatisticsRepository(pool)

	got, err := repository.GetTasks(context.Background(), nil, nil, nil)

	if !errors.Is(err, scanError) {
		t.Fatalf("GetTasks() error = %v, want wrapped scan error", err)
	}
	if !strings.Contains(err.Error(), "scan tasks") {
		t.Fatalf("GetTasks() error = %v, want scan context", err)
	}
	if got != nil {
		t.Fatalf("GetTasks() = %+v, want nil on scan error", got)
	}
	if rows.closeCall != 1 {
		t.Fatalf("Rows.Close() calls = %d, want 1", rows.closeCall)
	}
}

// TestGetTasksReturnsRowsIterationErrorAndClosesRows checks iteration failures and row cleanup.
func TestGetTasksReturnsRowsIterationErrorAndClosesRows(t *testing.T) {
	iterationError := errors.New("iteration failed")
	rows := &repositoryTestRows{rowsError: iterationError}
	pool := newRepositoryTestPool(t)
	pool.queryRows = rows
	repository := NewStatisticsRepository(pool)

	got, err := repository.GetTasks(context.Background(), nil, nil, nil)

	if !errors.Is(err, iterationError) {
		t.Fatalf("GetTasks() error = %v, want wrapped rows error", err)
	}
	if !strings.Contains(err.Error(), "next rows") {
		t.Fatalf("GetTasks() error = %v, want iteration context", err)
	}
	if got != nil {
		t.Fatalf("GetTasks() = %+v, want nil on iteration error", got)
	}
	if rows.closeCall != 1 {
		t.Fatalf("Rows.Close() calls = %d, want 1", rows.closeCall)
	}
}
