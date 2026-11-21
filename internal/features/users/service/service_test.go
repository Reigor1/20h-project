package users_service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Reigor1/20h-project/internal/core/domain"
	core_errors "github.com/Reigor1/20h-project/internal/core/errors"
	"go.opentelemetry.io/otel/trace/noop"
)

// usersRepositoryStub captures service or repository calls and returns configured test results.
type usersRepositoryStub struct {
	t *testing.T

	expectCreateUser bool
	createUserResult domain.User
	createUserError  error
	createUserCalls  int
	createdUser      domain.User

	expectGetUsers bool
	getUsersResult []domain.User
	getUsersError  error
	getUsersCalls  int
	getUsersLimit  *int
	getUsersOffset *int

	expectGetUser bool
	getUserResult domain.User
	getUserError  error
	getUserCalls  int
	getUserID     int

	expectDeleteUser bool
	deleteUserError  error
	deleteUserCalls  int
	deleteUserID     int

	expectPatchUser bool
	patchUserResult domain.User
	patchUserError  error
	patchUserCalls  int
	patchUserID     int
	patchedUser     domain.User

	calls []string
}

// newUsersRepositoryStub creates the test fixture used by this test file.
func newUsersRepositoryStub(t *testing.T) *usersRepositoryStub {
	t.Helper()

	return &usersRepositoryStub{t: t}
}

// CreateUser implements the corresponding repository operation for tracing or service tests.
func (r *usersRepositoryStub) CreateUser(
	_ context.Context,
	user domain.User,
) (domain.User, error) {
	if !r.expectCreateUser {
		r.t.Fatal("unexpected UsersRepository.CreateUser call")
	}

	r.createUserCalls++
	r.createdUser = user
	r.calls = append(r.calls, "CreateUser")

	return r.createUserResult, r.createUserError
}

// GetUsers implements the corresponding repository operation for tracing or service tests.
func (r *usersRepositoryStub) GetUsers(
	_ context.Context,
	limit *int,
	offset *int,
) ([]domain.User, error) {
	if !r.expectGetUsers {
		r.t.Fatal("unexpected UsersRepository.GetUsers call")
	}

	r.getUsersCalls++
	r.getUsersLimit = limit
	r.getUsersOffset = offset
	r.calls = append(r.calls, "GetUsers")

	return r.getUsersResult, r.getUsersError
}

// GetUser implements the corresponding repository operation for tracing or service tests.
func (r *usersRepositoryStub) GetUser(
	_ context.Context,
	id int,
) (domain.User, error) {
	if !r.expectGetUser {
		r.t.Fatal("unexpected UsersRepository.GetUser call")
	}

	r.getUserCalls++
	r.getUserID = id
	r.calls = append(r.calls, "GetUser")

	return r.getUserResult, r.getUserError
}

// DeleteUser implements the corresponding repository operation for tracing or service tests.
func (r *usersRepositoryStub) DeleteUser(
	_ context.Context,
	id int,
) error {
	if !r.expectDeleteUser {
		r.t.Fatal("unexpected UsersRepository.DeleteUser call")
	}

	r.deleteUserCalls++
	r.deleteUserID = id
	r.calls = append(r.calls, "DeleteUser")

	return r.deleteUserError
}

// PatchUser implements the corresponding repository operation for tracing or service tests.
func (r *usersRepositoryStub) PatchUser(
	_ context.Context,
	id int,
	user domain.User,
) (domain.User, error) {
	if !r.expectPatchUser {
		r.t.Fatal("unexpected UsersRepository.PatchUser call")
	}

	r.patchUserCalls++
	r.patchUserID = id
	r.patchedUser = user
	r.calls = append(r.calls, "PatchUser")

	return r.patchUserResult, r.patchUserError
}

// TestCreateUser verifies the behavior and expected outcomes described by its test scenarios.
func TestCreateUser(t *testing.T) {
	phoneNumber := "+1234567890"
	inputUser := domain.NewUserUninitialized("Ada Lovelace", &phoneNumber)
	createdUser := domain.NewUser(12, 1, "Ada Lovelace", &phoneNumber)
	repository := newUsersRepositoryStub(t)
	repository.expectCreateUser = true
	repository.createUserResult = createdUser
	service := newUsersServiceForTest(repository)

	got, err := service.CreateUser(context.Background(), inputUser)
	if err != nil {
		t.Fatalf("CreateUser() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(got, createdUser) {
		t.Fatalf("CreateUser() = %+v, want %+v", got, createdUser)
	}
	if repository.createUserCalls != 1 || !reflect.DeepEqual(repository.createdUser, inputUser) {
		t.Fatalf(
			"repository CreateUser() calls/user = %d/%+v, want 1/%+v",
			repository.createUserCalls,
			repository.createdUser,
			inputUser,
		)
	}
}

// TestCreateUserRejectsInvalidUser verifies the behavior and expected outcomes described by its test scenarios.
func TestCreateUserRejectsInvalidUser(t *testing.T) {
	repository := newUsersRepositoryStub(t)
	service := newUsersServiceForTest(repository)
	invalidUser := domain.NewUserUninitialized("Al", nil)

	got, err := service.CreateUser(context.Background(), invalidUser)

	if !errors.Is(err, core_errors.ErrInvalidArgument) {
		t.Fatalf("CreateUser() error = %v, want ErrInvalidArgument", err)
	}
	if !reflect.DeepEqual(got, domain.User{}) {
		t.Fatalf("CreateUser() user = %+v, want zero value", got)
	}
	if repository.createUserCalls != 0 {
		t.Fatalf("repository CreateUser() calls = %d, want 0 for invalid user", repository.createUserCalls)
	}
}

// TestCreateUserReturnsRepositoryError verifies the behavior and expected outcomes described by its test scenarios.
func TestCreateUserReturnsRepositoryError(t *testing.T) {
	repositoryError := errors.New("storage unavailable")
	repository := newUsersRepositoryStub(t)
	repository.expectCreateUser = true
	repository.createUserError = repositoryError
	service := newUsersServiceForTest(repository)
	inputUser := domain.NewUserUninitialized("Ada Lovelace", nil)

	_, err := service.CreateUser(context.Background(), inputUser)

	if !errors.Is(err, repositoryError) {
		t.Fatalf("CreateUser() error = %v, want wrapped repository error", err)
	}
	if repository.createUserCalls != 1 {
		t.Fatalf("repository CreateUser() calls = %d, want 1", repository.createUserCalls)
	}
}

// TestGetUser verifies the behavior and expected outcomes described by its test scenarios.
func TestGetUser(t *testing.T) {
	wantUser := domain.NewUser(42, 3, "Ada Lovelace", nil)
	repository := newUsersRepositoryStub(t)
	repository.expectGetUser = true
	repository.getUserResult = wantUser
	service := newUsersServiceForTest(repository)

	got, err := service.GetUser(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetUser() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(got, wantUser) {
		t.Fatalf("GetUser() = %+v, want %+v", got, wantUser)
	}
	if repository.getUserCalls != 1 || repository.getUserID != 42 {
		t.Fatalf("repository GetUser() calls/id = %d/%d, want 1/42", repository.getUserCalls, repository.getUserID)
	}
}

// TestGetUserReturnsRepositoryError verifies the behavior and expected outcomes described by its test scenarios.
func TestGetUserReturnsRepositoryError(t *testing.T) {
	repositoryError := core_errors.ErrNotFound
	repository := newUsersRepositoryStub(t)
	repository.expectGetUser = true
	repository.getUserError = repositoryError
	service := newUsersServiceForTest(repository)

	_, err := service.GetUser(context.Background(), 42)

	if !errors.Is(err, repositoryError) {
		t.Fatalf("GetUser() error = %v, want wrapped ErrNotFound", err)
	}
	if repository.getUserCalls != 1 || repository.getUserID != 42 {
		t.Fatalf("repository GetUser() calls/id = %d/%d, want 1/42", repository.getUserCalls, repository.getUserID)
	}
}

// TestGetUsersPassesPaginationToRepository verifies the behavior and expected outcomes described by its test scenarios.
func TestGetUsersPassesPaginationToRepository(t *testing.T) {
	limit := 20
	offset := 40
	wantUsers := []domain.User{
		domain.NewUser(1, 2, "Ada Lovelace", nil),
		domain.NewUser(3, 1, "Grace Hopper", nil),
	}
	repository := newUsersRepositoryStub(t)
	repository.expectGetUsers = true
	repository.getUsersResult = wantUsers
	service := newUsersServiceForTest(repository)

	got, err := service.GetUsers(context.Background(), &limit, &offset)
	if err != nil {
		t.Fatalf("GetUsers() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(got, wantUsers) {
		t.Fatalf("GetUsers() = %+v, want %+v", got, wantUsers)
	}
	if repository.getUsersCalls != 1 || repository.getUsersLimit != &limit || repository.getUsersOffset != &offset {
		t.Fatalf(
			"repository GetUsers() calls/limit/offset = %d/%p/%p, want 1/%p/%p",
			repository.getUsersCalls,
			repository.getUsersLimit,
			repository.getUsersOffset,
			&limit,
			&offset,
		)
	}
}

// TestGetUsersAllowsUnsetPagination verifies the behavior and expected outcomes described by its test scenarios.
func TestGetUsersAllowsUnsetPagination(t *testing.T) {
	repository := newUsersRepositoryStub(t)
	repository.expectGetUsers = true
	repository.getUsersResult = []domain.User{}
	service := newUsersServiceForTest(repository)

	got, err := service.GetUsers(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("GetUsers() error = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Fatalf("GetUsers() length = %d, want 0", len(got))
	}
	if repository.getUsersCalls != 1 || repository.getUsersLimit != nil || repository.getUsersOffset != nil {
		t.Fatalf(
			"repository GetUsers() calls/limit/offset = %d/%v/%v, want 1/nil/nil",
			repository.getUsersCalls,
			repository.getUsersLimit,
			repository.getUsersOffset,
		)
	}
}

// TestGetUsersRejectsNegativePagination verifies the behavior and expected outcomes described by its test scenarios.
func TestGetUsersRejectsNegativePagination(t *testing.T) {
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
			repository := newUsersRepositoryStub(t)
			service := newUsersServiceForTest(repository)

			got, err := service.GetUsers(context.Background(), testCase.limit, testCase.offset)

			if !errors.Is(err, core_errors.ErrInvalidArgument) {
				t.Fatalf("GetUsers() error = %v, want ErrInvalidArgument", err)
			}
			if got != nil {
				t.Fatalf("GetUsers() = %v, want nil", got)
			}
			if repository.getUsersCalls != 0 {
				t.Fatalf("repository GetUsers() calls = %d, want 0 for invalid pagination", repository.getUsersCalls)
			}
		})
	}
}

// TestGetUsersReturnsRepositoryError verifies the behavior and expected outcomes described by its test scenarios.
func TestGetUsersReturnsRepositoryError(t *testing.T) {
	repositoryError := errors.New("query failed")
	repository := newUsersRepositoryStub(t)
	repository.expectGetUsers = true
	repository.getUsersError = repositoryError
	service := newUsersServiceForTest(repository)

	_, err := service.GetUsers(context.Background(), nil, nil)

	if !errors.Is(err, repositoryError) {
		t.Fatalf("GetUsers() error = %v, want wrapped repository error", err)
	}
	if repository.getUsersCalls != 1 {
		t.Fatalf("repository GetUsers() calls = %d, want 1", repository.getUsersCalls)
	}
}

// TestPatchUserAppliesPatchBeforeRepositoryUpdate verifies the behavior and expected outcomes described by its test scenarios.
func TestPatchUserAppliesPatchBeforeRepositoryUpdate(t *testing.T) {
	originalPhoneNumber := "+1234567890"
	updatedFullName := "Grace Hopper"
	initialUser := domain.NewUser(42, 3, "Ada Lovelace", &originalPhoneNumber)
	updatedUser := domain.NewUser(42, 3, updatedFullName, nil)
	wantPatchedUser := domain.NewUser(42, 4, updatedFullName, nil)
	patch := domain.NewUserPatch(
		domain.Nullable[string]{Value: &updatedFullName, Set: true},
		domain.Nullable[string]{Set: true},
	)
	repository := newUsersRepositoryStub(t)
	repository.expectGetUser = true
	repository.getUserResult = initialUser
	repository.expectPatchUser = true
	repository.patchUserResult = wantPatchedUser
	service := newUsersServiceForTest(repository)

	got, err := service.PatchUser(context.Background(), 42, patch)
	if err != nil {
		t.Fatalf("PatchUser() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(got, wantPatchedUser) {
		t.Fatalf("PatchUser() = %+v, want %+v", got, wantPatchedUser)
	}
	if !reflect.DeepEqual(repository.calls, []string{"GetUser", "PatchUser"}) {
		t.Fatalf("repository calls = %v, want [GetUser PatchUser]", repository.calls)
	}
	if repository.getUserCalls != 1 || repository.getUserID != 42 {
		t.Fatalf("repository GetUser() calls/id = %d/%d, want 1/42", repository.getUserCalls, repository.getUserID)
	}
	if repository.patchUserCalls != 1 || repository.patchUserID != 42 {
		t.Fatalf("repository PatchUser() calls/id = %d/%d, want 1/42", repository.patchUserCalls, repository.patchUserID)
	}
	if !reflect.DeepEqual(repository.patchedUser, updatedUser) {
		t.Fatalf("repository PatchUser() user = %+v, want %+v", repository.patchedUser, updatedUser)
	}
}

// TestPatchUserReturnsGetErrorWithoutUpdating verifies the behavior and expected outcomes described by its test scenarios.
func TestPatchUserReturnsGetErrorWithoutUpdating(t *testing.T) {
	repositoryError := core_errors.ErrNotFound
	repository := newUsersRepositoryStub(t)
	repository.expectGetUser = true
	repository.getUserError = repositoryError
	service := newUsersServiceForTest(repository)

	_, err := service.PatchUser(context.Background(), 42, domain.UserPatch{})

	if !errors.Is(err, repositoryError) {
		t.Fatalf("PatchUser() error = %v, want wrapped ErrNotFound", err)
	}
	if !reflect.DeepEqual(repository.calls, []string{"GetUser"}) {
		t.Fatalf("repository calls = %v, want [GetUser]", repository.calls)
	}
}

// TestPatchUserRejectsInvalidPatchWithoutUpdating verifies the behavior and expected outcomes described by its test scenarios.
func TestPatchUserRejectsInvalidPatchWithoutUpdating(t *testing.T) {
	initialUser := domain.NewUser(42, 3, "Ada Lovelace", nil)
	repository := newUsersRepositoryStub(t)
	repository.expectGetUser = true
	repository.getUserResult = initialUser
	service := newUsersServiceForTest(repository)
	patch := domain.NewUserPatch(domain.Nullable[string]{Set: true}, domain.Nullable[string]{})

	_, err := service.PatchUser(context.Background(), 42, patch)

	if !errors.Is(err, core_errors.ErrInvalidArgument) {
		t.Fatalf("PatchUser() error = %v, want ErrInvalidArgument", err)
	}
	if !reflect.DeepEqual(repository.calls, []string{"GetUser"}) {
		t.Fatalf("repository calls = %v, want [GetUser]", repository.calls)
	}
}

// TestPatchUserReturnsRepositoryUpdateError verifies the behavior and expected outcomes described by its test scenarios.
func TestPatchUserReturnsRepositoryUpdateError(t *testing.T) {
	repositoryError := core_errors.ErrConflict
	initialUser := domain.NewUser(42, 3, "Ada Lovelace", nil)
	repository := newUsersRepositoryStub(t)
	repository.expectGetUser = true
	repository.getUserResult = initialUser
	repository.expectPatchUser = true
	repository.patchUserError = repositoryError
	service := newUsersServiceForTest(repository)
	updatedFullName := "Grace Hopper"
	patch := domain.NewUserPatch(
		domain.Nullable[string]{Value: &updatedFullName, Set: true},
		domain.Nullable[string]{},
	)

	_, err := service.PatchUser(context.Background(), 42, patch)

	if !errors.Is(err, repositoryError) {
		t.Fatalf("PatchUser() error = %v, want wrapped ErrConflict", err)
	}
	if !reflect.DeepEqual(repository.calls, []string{"GetUser", "PatchUser"}) {
		t.Fatalf("repository calls = %v, want [GetUser PatchUser]", repository.calls)
	}
}

// TestDeleteUser verifies the behavior and expected outcomes described by its test scenarios.
func TestDeleteUser(t *testing.T) {
	repository := newUsersRepositoryStub(t)
	repository.expectDeleteUser = true
	service := newUsersServiceForTest(repository)

	err := service.DeleteUser(context.Background(), 42)
	if err != nil {
		t.Fatalf("DeleteUser() error = %v, want nil", err)
	}
	if repository.deleteUserCalls != 1 || repository.deleteUserID != 42 {
		t.Fatalf("repository DeleteUser() calls/id = %d/%d, want 1/42", repository.deleteUserCalls, repository.deleteUserID)
	}
}

// TestDeleteUserReturnsRepositoryError verifies the behavior and expected outcomes described by its test scenarios.
func TestDeleteUserReturnsRepositoryError(t *testing.T) {
	repositoryError := core_errors.ErrNotFound
	repository := newUsersRepositoryStub(t)
	repository.expectDeleteUser = true
	repository.deleteUserError = repositoryError
	service := newUsersServiceForTest(repository)

	err := service.DeleteUser(context.Background(), 42)

	if !errors.Is(err, repositoryError) {
		t.Fatalf("DeleteUser() error = %v, want wrapped ErrNotFound", err)
	}
	if repository.deleteUserCalls != 1 || repository.deleteUserID != 42 {
		t.Fatalf("repository DeleteUser() calls/id = %d/%d, want 1/42", repository.deleteUserCalls, repository.deleteUserID)
	}
}

// newUsersServiceForTest creates the test fixture used by this test file.
func newUsersServiceForTest(repository UsersRepository) *UsersService {
	return NewUsersService(repository, noop.NewTracerProvider())
}
