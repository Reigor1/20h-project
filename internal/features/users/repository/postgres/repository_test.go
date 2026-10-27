package users_postgres_repository

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
)

const repositoryOperationTimeout = 3 * time.Second

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
		operationTimeout: repositoryOperationTimeout,
	}
}

func (p *repositoryTestPool) Query(
	ctx context.Context,
	sql string,
	args ...any,
) (core_postgres_pool.Rows, error) {
	if p.queryCalls != 0 || p.queryRowCalls != 0 || p.execCalls != 0 {
		p.t.Fatalf("unexpected additional pool query: %q", sql)
	}
	p.queryCalls++
	p.gotContext = ctx
	p.gotQuerySQL = sql
	p.gotQueryArgs = append([]any(nil), args...)

	return p.queryResult, p.queryError
}

func (p *repositoryTestPool) QueryRow(
	ctx context.Context,
	sql string,
	args ...any,
) core_postgres_pool.Row {
	if p.queryCalls != 0 || p.queryRowCalls != 0 || p.execCalls != 0 {
		p.t.Fatalf("unexpected additional pool query row: %q", sql)
	}
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
	if p.queryCalls != 0 || p.queryRowCalls != 0 || p.execCalls != 0 {
		p.t.Fatalf("unexpected additional pool exec: %q", sql)
	}
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

type repositoryTestRow struct {
	values []any
	err    error
	scans  int
}

func (r *repositoryTestRow) Scan(dest ...any) error {
	r.scans++
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return errors.New("unexpected scan destination count")
	}
	for index, value := range r.values {
		switch destination := dest[index].(type) {
		case *int:
			*destination = value.(int)
		case *string:
			*destination = value.(string)
		case **string:
			*destination = value.(*string)
		default:
			return errors.New("unsupported scan destination")
		}
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

func (r *repositoryTestRows) Scan(dest ...any) error {
	if r.scanError != nil {
		return r.scanError
	}
	values := r.rows[r.current-1]
	return (&repositoryTestRow{values: values}).Scan(dest...)
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

func TestNewUsersRepositoryStoresPool(t *testing.T) {
	pool := newRepositoryTestPool(t)

	repository := NewUsersRepository(pool)

	if repository.pool != pool {
		t.Fatal("NewUsersRepository() did not retain the supplied pool")
	}
}

func TestUserDomainsFromModels(t *testing.T) {
	firstPhoneNumber := "+1234567890"
	secondPhoneNumber := "+19876543210"
	tests := []struct {
		name   string
		models []UserModel
		want   []domain.User
	}{
		{
			name:   "empty models",
			models: []UserModel{},
			want:   []domain.User{},
		},
		{
			name: "single model without phone number",
			models: []UserModel{
				{
					ID:       7,
					Version:  2,
					FullName: "Ada Lovelace",
				},
			},
			want: []domain.User{
				domain.NewUser(7, 2, "Ada Lovelace", nil),
			},
		},
		{
			name: "multiple models preserve order and nullable phones",
			models: []UserModel{
				{
					ID:          7,
					Version:     2,
					FullName:    "Ada Lovelace",
					PhoneNumber: &firstPhoneNumber,
				},
				{
					ID:       12,
					Version:  4,
					FullName: "Grace Hopper",
				},
				{
					ID:          18,
					Version:     1,
					FullName:    "Katherine Johnson",
					PhoneNumber: &secondPhoneNumber,
				},
			},
			want: []domain.User{
				domain.NewUser(7, 2, "Ada Lovelace", &firstPhoneNumber),
				domain.NewUser(12, 4, "Grace Hopper", nil),
				domain.NewUser(18, 1, "Katherine Johnson", &secondPhoneNumber),
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got := userDomainsFromModels(testCase.models)
			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("userDomainsFromModels() = %+v, want %+v", got, testCase.want)
			}
		})
	}
}

func TestCreateUser(t *testing.T) {
	phoneNumber := "+1234567890"
	inputUser := domain.NewUserUninitialized("Ada Lovelace", &phoneNumber)
	wantUser := domain.NewUser(7, 1, "Ada Lovelace", &phoneNumber)
	row := &repositoryTestRow{
		values: []any{7, 1, "Ada Lovelace", &phoneNumber},
	}
	pool := newRepositoryTestPool(t)
	pool.rowResult = row
	repository := NewUsersRepository(pool)
	ctx := context.WithValue(context.Background(), repositoryContextKey{}, "request")

	got, err := repository.CreateUser(ctx, inputUser)

	if err != nil {
		t.Fatalf("CreateUser() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(got, wantUser) {
		t.Fatalf("CreateUser() = %+v, want %+v", got, wantUser)
	}
	if pool.queryRowCalls != 1 || !strings.Contains(pool.gotRowSQL, "INSERT INTO todoapp.users") {
		t.Fatalf("QueryRow() calls/sql = %d/%q, want one INSERT", pool.queryRowCalls, pool.gotRowSQL)
	}
	if !reflect.DeepEqual(pool.gotRowArgs, []any{"Ada Lovelace", &phoneNumber}) {
		t.Fatalf("QueryRow() args = %#v, want name then phone number", pool.gotRowArgs)
	}
	assertRepositoryContext(t, pool.gotContext, ctx)
	if row.scans != 1 {
		t.Fatalf("row Scan() calls = %d, want 1", row.scans)
	}
}

func TestCreateUserReturnsScanError(t *testing.T) {
	scanError := errors.New("scan failed")
	row := &repositoryTestRow{err: scanError}
	pool := newRepositoryTestPool(t)
	pool.rowResult = row
	repository := NewUsersRepository(pool)
	user := domain.NewUserUninitialized("Ada Lovelace", nil)

	_, err := repository.CreateUser(context.Background(), user)

	if !errors.Is(err, scanError) || !strings.Contains(err.Error(), "scan error") {
		t.Fatalf("CreateUser() error = %v, want wrapped scan error", err)
	}
	if row.scans != 1 {
		t.Fatalf("row Scan() calls = %d, want 1", row.scans)
	}
}

func TestGetUser(t *testing.T) {
	phoneNumber := "+1234567890"
	row := &repositoryTestRow{
		values: []any{42, 3, "Ada Lovelace", &phoneNumber},
	}
	pool := newRepositoryTestPool(t)
	pool.rowResult = row
	repository := NewUsersRepository(pool)
	ctx := context.WithValue(context.Background(), repositoryContextKey{}, "request")

	got, err := repository.GetUser(ctx, 42)

	if err != nil {
		t.Fatalf("GetUser() error = %v, want nil", err)
	}
	wantUser := domain.NewUser(42, 3, "Ada Lovelace", &phoneNumber)
	if !reflect.DeepEqual(got, wantUser) {
		t.Fatalf("GetUser() = %+v, want %+v", got, wantUser)
	}
	if pool.queryRowCalls != 1 || !strings.Contains(pool.gotRowSQL, "WHERE id = $1") {
		t.Fatalf("QueryRow() calls/sql = %d/%q, want one lookup by ID", pool.queryRowCalls, pool.gotRowSQL)
	}
	if !reflect.DeepEqual(pool.gotRowArgs, []any{42}) {
		t.Fatalf("QueryRow() args = %#v, want [42]", pool.gotRowArgs)
	}
	assertRepositoryContext(t, pool.gotContext, ctx)
}

func TestGetUserTranslatesNoRowsToNotFound(t *testing.T) {
	row := &repositoryTestRow{err: core_postgres_pool.ErrNoRows}
	pool := newRepositoryTestPool(t)
	pool.rowResult = row
	repository := NewUsersRepository(pool)

	_, err := repository.GetUser(context.Background(), 42)

	if !errors.Is(err, core_errors.ErrNotFound) {
		t.Fatalf("GetUser() error = %v, want ErrNotFound", err)
	}
	if !strings.Contains(err.Error(), "id='42'") {
		t.Fatalf("GetUser() error = %v, want requested ID in context", err)
	}
}

func TestGetUserReturnsScanError(t *testing.T) {
	scanError := errors.New("database decoding failed")
	row := &repositoryTestRow{err: scanError}
	pool := newRepositoryTestPool(t)
	pool.rowResult = row
	repository := NewUsersRepository(pool)

	_, err := repository.GetUser(context.Background(), 42)

	if !errors.Is(err, scanError) || !strings.Contains(err.Error(), "scan error") {
		t.Fatalf("GetUser() error = %v, want wrapped scan error", err)
	}
}

func TestGetUsers(t *testing.T) {
	firstPhoneNumber := "+1234567890"
	rows := &repositoryTestRows{
		rows: [][]any{
			{1, 2, "Ada Lovelace", &firstPhoneNumber},
			{3, 4, "Grace Hopper", (*string)(nil)},
		},
	}
	pool := newRepositoryTestPool(t)
	pool.queryResult = rows
	repository := NewUsersRepository(pool)
	limit := 25
	offset := 50
	ctx := context.WithValue(context.Background(), repositoryContextKey{}, "request")

	got, err := repository.GetUsers(ctx, &limit, &offset)

	if err != nil {
		t.Fatalf("GetUsers() error = %v, want nil", err)
	}
	wantUsers := []domain.User{
		domain.NewUser(1, 2, "Ada Lovelace", &firstPhoneNumber),
		domain.NewUser(3, 4, "Grace Hopper", nil),
	}
	if !reflect.DeepEqual(got, wantUsers) {
		t.Fatalf("GetUsers() = %+v, want %+v", got, wantUsers)
	}
	if pool.queryCalls != 1 || !strings.Contains(pool.gotQuerySQL, "ORDER BY id ASC") {
		t.Fatalf("Query() calls/sql = %d/%q, want one ordered SELECT", pool.queryCalls, pool.gotQuerySQL)
	}
	if !reflect.DeepEqual(pool.gotQueryArgs, []any{&limit, &offset}) {
		t.Fatalf("Query() args = %#v, want limit then offset", pool.gotQueryArgs)
	}
	assertRepositoryContext(t, pool.gotContext, ctx)
	if rows.closeCalls != 1 {
		t.Fatalf("rows Close() calls = %d, want 1", rows.closeCalls)
	}
}

func TestGetUsersReturnsEmptySliceForNoRows(t *testing.T) {
	rows := &repositoryTestRows{}
	pool := newRepositoryTestPool(t)
	pool.queryResult = rows
	repository := NewUsersRepository(pool)

	got, err := repository.GetUsers(context.Background(), nil, nil)

	if err != nil {
		t.Fatalf("GetUsers() error = %v, want nil", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("GetUsers() = %#v, want a non-nil empty slice", got)
	}
	if !reflect.DeepEqual(pool.gotQueryArgs, []any{(*int)(nil), (*int)(nil)}) {
		t.Fatalf("Query() args = %#v, want nil limit and offset", pool.gotQueryArgs)
	}
	if rows.closeCalls != 1 {
		t.Fatalf("rows Close() calls = %d, want 1", rows.closeCalls)
	}
}

func TestGetUsersReturnsQueryError(t *testing.T) {
	queryError := errors.New("query failed")
	pool := newRepositoryTestPool(t)
	pool.queryError = queryError
	repository := NewUsersRepository(pool)

	_, err := repository.GetUsers(context.Background(), nil, nil)

	if !errors.Is(err, queryError) || !strings.Contains(err.Error(), "selects users") {
		t.Fatalf("GetUsers() error = %v, want wrapped query error", err)
	}
	if pool.queryCalls != 1 {
		t.Fatalf("Query() calls = %d, want 1", pool.queryCalls)
	}
}

func TestGetUsersReturnsScanErrorAndClosesRows(t *testing.T) {
	scanError := errors.New("row scan failed")
	rows := &repositoryTestRows{
		rows:      [][]any{{1, 2, "Ada Lovelace", (*string)(nil)}},
		scanError: scanError,
	}
	pool := newRepositoryTestPool(t)
	pool.queryResult = rows
	repository := NewUsersRepository(pool)

	_, err := repository.GetUsers(context.Background(), nil, nil)

	if !errors.Is(err, scanError) || !strings.Contains(err.Error(), "scan users") {
		t.Fatalf("GetUsers() error = %v, want wrapped scan error", err)
	}
	if rows.closeCalls != 1 {
		t.Fatalf("rows Close() calls = %d, want 1 after scan failure", rows.closeCalls)
	}
}

func TestGetUsersReturnsRowsIterationErrorAndClosesRows(t *testing.T) {
	iterationError := errors.New("iteration failed")
	rows := &repositoryTestRows{rowsError: iterationError}
	pool := newRepositoryTestPool(t)
	pool.queryResult = rows
	repository := NewUsersRepository(pool)

	_, err := repository.GetUsers(context.Background(), nil, nil)

	if !errors.Is(err, iterationError) || !strings.Contains(err.Error(), "next rows") {
		t.Fatalf("GetUsers() error = %v, want wrapped iteration error", err)
	}
	if rows.closeCalls != 1 {
		t.Fatalf("rows Close() calls = %d, want 1 after iteration failure", rows.closeCalls)
	}
}

func TestPatchUser(t *testing.T) {
	phoneNumber := "+1234567890"
	inputUser := domain.NewUser(42, 3, "Grace Hopper", &phoneNumber)
	row := &repositoryTestRow{
		values: []any{42, 4, "Grace Hopper", &phoneNumber},
	}
	pool := newRepositoryTestPool(t)
	pool.rowResult = row
	repository := NewUsersRepository(pool)
	ctx := context.WithValue(context.Background(), repositoryContextKey{}, "request")

	got, err := repository.PatchUser(ctx, 42, inputUser)

	if err != nil {
		t.Fatalf("PatchUser() error = %v, want nil", err)
	}
	wantUser := domain.NewUser(42, 4, "Grace Hopper", &phoneNumber)
	if !reflect.DeepEqual(got, wantUser) {
		t.Fatalf("PatchUser() = %+v, want %+v", got, wantUser)
	}
	if pool.queryRowCalls != 1 || !strings.Contains(pool.gotRowSQL, "version=version+1") {
		t.Fatalf("QueryRow() calls/sql = %d/%q, want one versioned UPDATE", pool.queryRowCalls, pool.gotRowSQL)
	}
	wantArgs := []any{"Grace Hopper", &phoneNumber, 42, 3}
	if !reflect.DeepEqual(pool.gotRowArgs, wantArgs) {
		t.Fatalf("QueryRow() args = %#v, want %#v", pool.gotRowArgs, wantArgs)
	}
	assertRepositoryContext(t, pool.gotContext, ctx)
}

func TestPatchUserTranslatesNoRowsToConflict(t *testing.T) {
	row := &repositoryTestRow{err: core_postgres_pool.ErrNoRows}
	pool := newRepositoryTestPool(t)
	pool.rowResult = row
	repository := NewUsersRepository(pool)
	user := domain.NewUser(42, 3, "Grace Hopper", nil)

	_, err := repository.PatchUser(context.Background(), 42, user)

	if !errors.Is(err, core_errors.ErrConflict) {
		t.Fatalf("PatchUser() error = %v, want ErrConflict", err)
	}
	if !strings.Contains(err.Error(), "id='42'") || !strings.Contains(err.Error(), "concurrently accessed") {
		t.Fatalf("PatchUser() error = %v, want conflict context", err)
	}
}

func TestPatchUserReturnsScanError(t *testing.T) {
	scanError := errors.New("return row decoding failed")
	row := &repositoryTestRow{err: scanError}
	pool := newRepositoryTestPool(t)
	pool.rowResult = row
	repository := NewUsersRepository(pool)
	user := domain.NewUser(42, 3, "Grace Hopper", nil)

	_, err := repository.PatchUser(context.Background(), 42, user)

	if !errors.Is(err, scanError) || !strings.Contains(err.Error(), "scan error") {
		t.Fatalf("PatchUser() error = %v, want wrapped scan error", err)
	}
}

func TestDeleteUser(t *testing.T) {
	pool := newRepositoryTestPool(t)
	pool.execResult = repositoryTestCommandTag{rowsAffected: 1}
	repository := NewUsersRepository(pool)
	ctx := context.WithValue(context.Background(), repositoryContextKey{}, "request")

	err := repository.DeleteUser(ctx, 42)

	if err != nil {
		t.Fatalf("DeleteUser() error = %v, want nil", err)
	}
	if pool.execCalls != 1 || !strings.Contains(pool.gotExecSQL, "DELETE FROM todoapp.users") {
		t.Fatalf("Exec() calls/sql = %d/%q, want one DELETE", pool.execCalls, pool.gotExecSQL)
	}
	if !reflect.DeepEqual(pool.gotExecArgs, []any{42}) {
		t.Fatalf("Exec() args = %#v, want [42]", pool.gotExecArgs)
	}
	assertRepositoryContext(t, pool.gotContext, ctx)
}

func TestDeleteUserReturnsExecutionError(t *testing.T) {
	executionError := errors.New("delete failed")
	pool := newRepositoryTestPool(t)
	pool.execError = executionError
	repository := NewUsersRepository(pool)

	err := repository.DeleteUser(context.Background(), 42)

	if !errors.Is(err, executionError) || !strings.Contains(err.Error(), "exec query") {
		t.Fatalf("DeleteUser() error = %v, want wrapped execution error", err)
	}
}

func TestDeleteUserReturnsNotFoundWhenNoRowsAffected(t *testing.T) {
	pool := newRepositoryTestPool(t)
	pool.execResult = repositoryTestCommandTag{rowsAffected: 0}
	repository := NewUsersRepository(pool)

	err := repository.DeleteUser(context.Background(), 42)

	if !errors.Is(err, core_errors.ErrNotFound) {
		t.Fatalf("DeleteUser() error = %v, want ErrNotFound", err)
	}
	if !strings.Contains(err.Error(), "id='42'") {
		t.Fatalf("DeleteUser() error = %v, want requested ID in context", err)
	}
}

type repositoryContextKey struct{}

func assertRepositoryContext(t *testing.T, got, parent context.Context) {
	t.Helper()

	if got == nil {
		t.Fatal("repository did not pass a context to the pool")
	}
	if got.Value(repositoryContextKey{}) != parent.Value(repositoryContextKey{}) {
		t.Fatal("repository context did not preserve the parent context value")
	}
	deadline, ok := got.Deadline()
	if !ok {
		t.Fatal("repository context has no timeout deadline")
	}
	remaining := time.Until(deadline)
	if remaining <= 0 || remaining > repositoryOperationTimeout {
		t.Fatalf("repository context deadline remaining = %v, want within %v", remaining, repositoryOperationTimeout)
	}
}
