package users_transport_http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/Reigor1/20h-project/internal/core/domain"
	core_errors "github.com/Reigor1/20h-project/internal/core/errors"
	core_logger "github.com/Reigor1/20h-project/internal/core/logger"
	"github.com/Reigor1/20h-project/internal/testutil"
	"go.uber.org/zap"
)

// userServiceStub captures service or repository calls and returns configured test results.
type userServiceStub struct {
	t *testing.T

	expectCreateUser bool
	createUserResult domain.User
	createUserError  error
	createUserCalls  int
	createdUser      domain.User

	expectGetUser bool
	getUserResult domain.User
	getUserError  error
	getUserCalls  int
	getUserID     int

	expectGetUsers bool
	getUsersResult []domain.User
	getUsersError  error
	getUsersCalls  int
	getUsersLimit  *int
	getUsersOffset *int

	expectPatchUser bool
	patchUserResult domain.User
	patchUserError  error
	patchUserCalls  int
	patchUserID     int
	userPatch       domain.UserPatch

	expectDeleteUser bool
	deleteUserError  error
	deleteUserCalls  int
	deleteUserID     int
}

// newUserServiceStub creates the test fixture used by this test file.
func newUserServiceStub(t *testing.T) *userServiceStub {
	t.Helper()
	return &userServiceStub{t: t}
}

// CreateUser implements the corresponding repository operation for tracing or service tests.
func (s *userServiceStub) CreateUser(_ context.Context, user domain.User) (domain.User, error) {
	if !s.expectCreateUser {
		s.t.Fatal("unexpected UserService.CreateUser call")
	}
	s.createUserCalls++
	s.createdUser = user
	return s.createUserResult, s.createUserError
}

// GetUser implements the corresponding repository operation for tracing or service tests.
func (s *userServiceStub) GetUser(_ context.Context, id int) (domain.User, error) {
	if !s.expectGetUser {
		s.t.Fatal("unexpected UserService.GetUser call")
	}
	s.getUserCalls++
	s.getUserID = id
	return s.getUserResult, s.getUserError
}

// GetUsers implements the corresponding repository operation for tracing or service tests.
func (s *userServiceStub) GetUsers(_ context.Context, limit, offset *int) ([]domain.User, error) {
	if !s.expectGetUsers {
		s.t.Fatal("unexpected UserService.GetUsers call")
	}
	s.getUsersCalls++
	s.getUsersLimit = limit
	s.getUsersOffset = offset
	return s.getUsersResult, s.getUsersError
}

// PatchUser implements the corresponding repository operation for tracing or service tests.
func (s *userServiceStub) PatchUser(
	_ context.Context,
	id int,
	patch domain.UserPatch,
) (domain.User, error) {
	if !s.expectPatchUser {
		s.t.Fatal("unexpected UserService.PatchUser call")
	}
	s.patchUserCalls++
	s.patchUserID = id
	s.userPatch = patch
	return s.patchUserResult, s.patchUserError
}

// DeleteUser implements the corresponding repository operation for tracing or service tests.
func (s *userServiceStub) DeleteUser(_ context.Context, id int) error {
	if !s.expectDeleteUser {
		s.t.Fatal("unexpected UserService.DeleteUser call")
	}
	s.deleteUserCalls++
	s.deleteUserID = id
	return s.deleteUserError
}

// TestCreateUser verifies the behavior and expected outcomes described by its test scenarios.
func TestCreateUser(t *testing.T) {
	phoneNumber := "+1234567890"
	createdUser := domain.NewUser(12, 3, "Ada Lovelace", &phoneNumber)
	service := newUserServiceStub(t)
	service.expectCreateUser = true
	service.createUserResult = createdUser
	handler := NewUsersHTTPHandler(service)
	request := newUserRequest(
		http.MethodPost,
		"/users",
		`{"full_name":"Ada Lovelace","phone_number":"+1234567890"}`,
	)
	response := httptest.NewRecorder()

	handler.CreateUser(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusCreated)
	}
	testutil.AssertJSONContentType(t, response)
	var responseBody UserDTOResponse
	if err := json.Unmarshal(response.Body.Bytes(), &responseBody); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	wantResponse := UserDTOResponse{
		ID:          createdUser.ID,
		Version:     createdUser.Version,
		FullName:    createdUser.FullName,
		PhoneNumber: createdUser.PhoneNumber,
	}
	if !reflect.DeepEqual(responseBody, wantResponse) {
		t.Fatalf("response body = %+v, want %+v", responseBody, wantResponse)
	}
	if service.createUserCalls != 1 {
		t.Fatalf("CreateUser() calls = %d, want 1", service.createUserCalls)
	}
	if service.createdUser.FullName != "Ada Lovelace" ||
		service.createdUser.PhoneNumber == nil ||
		*service.createdUser.PhoneNumber != phoneNumber {
		t.Fatalf("service received user = %+v, want request fields", service.createdUser)
	}
}

// TestCreateUserRejectsInvalidRequest verifies the behavior and expected outcomes described by its test scenarios.
func TestCreateUserRejectsInvalidRequest(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "malformed JSON",
			body: `{"full_name":`,
		},
		{
			name: "name fails validation",
			body: `{"full_name":"Al"}`,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			service := newUserServiceStub(t)
			handler := NewUsersHTTPHandler(service)
			request := newUserRequest(http.MethodPost, "/users", testCase.body)
			response := httptest.NewRecorder()

			handler.CreateUser(response, request)

			testutil.AssertErrorResponse(t, response, http.StatusBadRequest)
			if service.createUserCalls != 0 {
				t.Fatalf("CreateUser() calls = %d, want 0 for invalid request", service.createUserCalls)
			}
		})
	}
}

// TestCreateUserReturnsServiceError verifies the behavior and expected outcomes described by its test scenarios.
func TestCreateUserReturnsServiceError(t *testing.T) {
	service := newUserServiceStub(t)
	service.expectCreateUser = true
	service.createUserError = errors.New("storage unavailable")
	handler := NewUsersHTTPHandler(service)
	request := newUserRequest(http.MethodPost, "/users", `{"full_name":"Ada Lovelace"}`)
	response := httptest.NewRecorder()

	handler.CreateUser(response, request)

	testutil.AssertErrorResponse(t, response, http.StatusInternalServerError)
	if service.createUserCalls != 1 {
		t.Fatalf("CreateUser() calls = %d, want 1", service.createUserCalls)
	}
}

// TestGetUser verifies the behavior and expected outcomes described by its test scenarios.
func TestGetUser(t *testing.T) {
	phoneNumber := "+1234567890"
	wantUser := domain.NewUser(42, 5, "Ada Lovelace", &phoneNumber)
	service := newUserServiceStub(t)
	service.expectGetUser = true
	service.getUserResult = wantUser
	handler := NewUsersHTTPHandler(service)
	request := newUserRequest(http.MethodGet, "/users/42", "")
	request.SetPathValue("id", "42")
	response := httptest.NewRecorder()

	handler.GetUser(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	testutil.AssertJSONContentType(t, response)
	var responseBody UserDTOResponse
	if err := json.Unmarshal(response.Body.Bytes(), &responseBody); err != nil {
		t.Fatalf("decode get user response: %v", err)
	}
	wantResponse := UserDTOResponse{
		ID:          wantUser.ID,
		Version:     wantUser.Version,
		FullName:    wantUser.FullName,
		PhoneNumber: wantUser.PhoneNumber,
	}
	if !reflect.DeepEqual(responseBody, wantResponse) {
		t.Fatalf("response body = %+v, want %+v", responseBody, wantResponse)
	}
	if service.getUserCalls != 1 || service.getUserID != 42 {
		t.Fatalf("GetUser() calls/id = %d/%d, want 1/42", service.getUserCalls, service.getUserID)
	}
}

// TestGetUserRejectsInvalidID verifies the behavior and expected outcomes described by its test scenarios.
func TestGetUserRejectsInvalidID(t *testing.T) {
	service := newUserServiceStub(t)
	handler := NewUsersHTTPHandler(service)
	request := newUserRequest(http.MethodGet, "/users/not-an-id", "")
	request.SetPathValue("id", "not-an-id")
	response := httptest.NewRecorder()

	handler.GetUser(response, request)

	testutil.AssertErrorResponse(t, response, http.StatusBadRequest)
	if service.getUserCalls != 0 {
		t.Fatalf("GetUser() calls = %d, want 0 for invalid ID", service.getUserCalls)
	}
}

// TestGetUserReturnsServiceError verifies the behavior and expected outcomes described by its test scenarios.
func TestGetUserReturnsServiceError(t *testing.T) {
	service := newUserServiceStub(t)
	service.expectGetUser = true
	service.getUserError = core_errors.ErrNotFound
	handler := NewUsersHTTPHandler(service)
	request := newUserRequest(http.MethodGet, "/users/42", "")
	request.SetPathValue("id", "42")
	response := httptest.NewRecorder()

	handler.GetUser(response, request)

	testutil.AssertErrorResponse(t, response, http.StatusNotFound)
	if service.getUserCalls != 1 || service.getUserID != 42 {
		t.Fatalf("GetUser() calls/id = %d/%d, want 1/42", service.getUserCalls, service.getUserID)
	}
}

// TestGetUsers verifies the behavior and expected outcomes described by its test scenarios.
func TestGetUsers(t *testing.T) {
	phoneNumber := "+1234567890"
	users := []domain.User{
		domain.NewUser(1, 2, "Ada Lovelace", &phoneNumber),
		domain.NewUser(3, 4, "Grace Hopper", nil),
	}
	service := newUserServiceStub(t)
	service.expectGetUsers = true
	service.getUsersResult = users
	handler := NewUsersHTTPHandler(service)
	request := newUserRequest(http.MethodGet, "/users?limit=20&offset=40", "")
	response := httptest.NewRecorder()

	handler.GetUsers(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	testutil.AssertJSONContentType(t, response)
	var responseBody []UserDTOResponse
	if err := json.Unmarshal(response.Body.Bytes(), &responseBody); err != nil {
		t.Fatalf("decode get users response: %v", err)
	}
	if len(responseBody) != len(users) {
		t.Fatalf("response users = %d, want %d", len(responseBody), len(users))
	}
	for index, user := range users {
		want := UserDTOResponse{
			ID:          user.ID,
			Version:     user.Version,
			FullName:    user.FullName,
			PhoneNumber: user.PhoneNumber,
		}
		if !reflect.DeepEqual(responseBody[index], want) {
			t.Errorf("response user %d = %+v, want %+v", index, responseBody[index], want)
		}
	}
	if service.getUsersCalls != 1 ||
		service.getUsersLimit == nil ||
		*service.getUsersLimit != 20 ||
		service.getUsersOffset == nil ||
		*service.getUsersOffset != 40 {
		t.Fatalf(
			"GetUsers() calls/limit/offset = %d/%v/%v, want 1/20/40",
			service.getUsersCalls,
			service.getUsersLimit,
			service.getUsersOffset,
		)
	}
}

// TestGetUsersInvalidPagination verifies the behavior and expected outcomes described by its test scenarios.
func TestGetUsersInvalidPagination(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{
			name: "invalid limit",
			path: "/users?limit=many",
		},
		{
			name: "invalid offset",
			path: "/users?offset=many",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			service := newUserServiceStub(t)
			handler := NewUsersHTTPHandler(service)
			response := httptest.NewRecorder()

			handler.GetUsers(response, newUserRequest(http.MethodGet, testCase.path, ""))

			testutil.AssertErrorResponse(t, response, http.StatusBadRequest)
			if service.getUsersCalls != 0 {
				t.Fatalf("GetUsers() calls = %d, want 0 for invalid pagination", service.getUsersCalls)
			}
		})
	}
}

// TestGetUsersReturnsServiceError verifies the behavior and expected outcomes described by its test scenarios.
func TestGetUsersReturnsServiceError(t *testing.T) {
	service := newUserServiceStub(t)
	service.expectGetUsers = true
	service.getUsersError = errors.New("storage unavailable")
	handler := NewUsersHTTPHandler(service)
	response := httptest.NewRecorder()

	handler.GetUsers(response, newUserRequest(http.MethodGet, "/users", ""))

	testutil.AssertErrorResponse(t, response, http.StatusInternalServerError)
	if service.getUsersCalls != 1 {
		t.Fatalf("GetUsers() calls = %d, want 1", service.getUsersCalls)
	}
}

// TestPatchUser verifies the behavior and expected outcomes described by its test scenarios.
func TestPatchUser(t *testing.T) {
	patchedUser := domain.NewUser(42, 6, "Grace Hopper", nil)
	service := newUserServiceStub(t)
	service.expectPatchUser = true
	service.patchUserResult = patchedUser
	handler := NewUsersHTTPHandler(service)
	request := newUserRequest(
		http.MethodPatch,
		"/users/42",
		`{"full_name":"Grace Hopper","phone_number":null}`,
	)
	request.SetPathValue("id", "42")
	response := httptest.NewRecorder()

	handler.PatchUser(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	testutil.AssertJSONContentType(t, response)
	var responseBody UserDTOResponse
	if err := json.Unmarshal(response.Body.Bytes(), &responseBody); err != nil {
		t.Fatalf("decode patch user response: %v", err)
	}
	wantResponse := UserDTOResponse{
		ID:       patchedUser.ID,
		Version:  patchedUser.Version,
		FullName: patchedUser.FullName,
	}
	if !reflect.DeepEqual(responseBody, wantResponse) {
		t.Fatalf("response body = %+v, want %+v", responseBody, wantResponse)
	}
	if service.patchUserCalls != 1 || service.patchUserID != 42 {
		t.Fatalf("PatchUser() calls/id = %d/%d, want 1/42", service.patchUserCalls, service.patchUserID)
	}
	if !service.userPatch.FullName.Set ||
		service.userPatch.FullName.Value == nil ||
		*service.userPatch.FullName.Value != "Grace Hopper" {
		t.Errorf("FullName patch = %+v, want set to Grace Hopper", service.userPatch.FullName)
	}
	if !service.userPatch.PhoneNumber.Set || service.userPatch.PhoneNumber.Value != nil {
		t.Errorf("PhoneNumber patch = %+v, want explicitly set to null", service.userPatch.PhoneNumber)
	}
}

// TestPatchUserRejectsInvalidPathOrBody verifies the behavior and expected outcomes described by its test scenarios.
func TestPatchUserRejectsInvalidPathOrBody(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		body       string
		wantStatus int
	}{
		{
			name:       "invalid ID",
			id:         "invalid",
			body:       `{"full_name":"Grace Hopper"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "malformed JSON",
			id:         "42",
			body:       `{"full_name":`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "full name cannot be null",
			id:         "42",
			body:       `{"full_name":null}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			service := newUserServiceStub(t)
			handler := NewUsersHTTPHandler(service)
			request := newUserRequest(http.MethodPatch, "/users/"+testCase.id, testCase.body)
			request.SetPathValue("id", testCase.id)
			response := httptest.NewRecorder()

			handler.PatchUser(response, request)

			testutil.AssertErrorResponse(t, response, testCase.wantStatus)
			if service.patchUserCalls != 0 {
				t.Fatalf("PatchUser() calls = %d, want 0 for invalid request", service.patchUserCalls)
			}
		})
	}
}

// TestPatchUserReturnsServiceError verifies the behavior and expected outcomes described by its test scenarios.
func TestPatchUserReturnsServiceError(t *testing.T) {
	service := newUserServiceStub(t)
	service.expectPatchUser = true
	service.patchUserError = core_errors.ErrConflict
	handler := NewUsersHTTPHandler(service)
	request := newUserRequest(http.MethodPatch, "/users/42", `{"full_name":"Grace Hopper"}`)
	request.SetPathValue("id", "42")
	response := httptest.NewRecorder()

	handler.PatchUser(response, request)

	testutil.AssertErrorResponse(t, response, http.StatusConflict)
	if service.patchUserCalls != 1 || service.patchUserID != 42 {
		t.Fatalf("PatchUser() calls/id = %d/%d, want 1/42", service.patchUserCalls, service.patchUserID)
	}
}

// TestDeleteUser verifies the behavior and expected outcomes described by its test scenarios.
func TestDeleteUser(t *testing.T) {
	service := newUserServiceStub(t)
	service.expectDeleteUser = true
	handler := NewUsersHTTPHandler(service)
	request := newUserRequest(http.MethodDelete, "/users/42", "")
	request.SetPathValue("id", "42")
	response := httptest.NewRecorder()

	handler.DeleteUser(response, request)

	if response.Code != http.StatusNoContent || response.Body.Len() != 0 {
		t.Fatalf(
			"response = %d %q, want empty 204 response",
			response.Code,
			response.Body.String(),
		)
	}
	if service.deleteUserCalls != 1 || service.deleteUserID != 42 {
		t.Fatalf("DeleteUser() calls/id = %d/%d, want 1/42", service.deleteUserCalls, service.deleteUserID)
	}
}

// TestDeleteUserRejectsInvalidID verifies the behavior and expected outcomes described by its test scenarios.
func TestDeleteUserRejectsInvalidID(t *testing.T) {
	service := newUserServiceStub(t)
	handler := NewUsersHTTPHandler(service)
	request := newUserRequest(http.MethodDelete, "/users/invalid", "")
	request.SetPathValue("id", "invalid")
	response := httptest.NewRecorder()

	handler.DeleteUser(response, request)

	testutil.AssertErrorResponse(t, response, http.StatusBadRequest)
	if service.deleteUserCalls != 0 {
		t.Fatalf("DeleteUser() calls = %d, want 0 for invalid ID", service.deleteUserCalls)
	}
}

// TestDeleteUserReturnsServiceError verifies the behavior and expected outcomes described by its test scenarios.
func TestDeleteUserReturnsServiceError(t *testing.T) {
	service := newUserServiceStub(t)
	service.expectDeleteUser = true
	service.deleteUserError = core_errors.ErrNotFound
	handler := NewUsersHTTPHandler(service)
	request := newUserRequest(http.MethodDelete, "/users/42", "")
	request.SetPathValue("id", "42")
	response := httptest.NewRecorder()

	handler.DeleteUser(response, request)

	testutil.AssertErrorResponse(t, response, http.StatusNotFound)
	if service.deleteUserCalls != 1 || service.deleteUserID != 42 {
		t.Fatalf("DeleteUser() calls/id = %d/%d, want 1/42", service.deleteUserCalls, service.deleteUserID)
	}
}

// TestUsersHTTPHandlerRoutes verifies the behavior and expected outcomes described by its test scenarios.
func TestUsersHTTPHandlerRoutes(t *testing.T) {
	handler := NewUsersHTTPHandler(newUserServiceStub(t))
	routes := handler.Routes()
	type routeKey struct {
		method string
		path   string
	}
	wantRoutes := map[routeKey]struct{}{
		{method: http.MethodPost, path: "/users"}:        {},
		{method: http.MethodGet, path: "/users"}:         {},
		{method: http.MethodGet, path: "/users/{id}"}:    {},
		{method: http.MethodDelete, path: "/users/{id}"}: {},
		{method: http.MethodPatch, path: "/users/{id}"}:  {},
	}

	if len(routes) != len(wantRoutes) {
		t.Fatalf("Routes() count = %d, want %d", len(routes), len(wantRoutes))
	}
	gotRoutes := make(map[routeKey]int, len(routes))
	for _, route := range routes {
		key := routeKey{method: route.Method, path: route.Path}
		gotRoutes[key]++
		if route.Handler == nil {
			t.Errorf("route %s %s has a nil handler", route.Method, route.Path)
		}
	}
	for route, count := range gotRoutes {
		if _, exists := wantRoutes[route]; !exists {
			t.Errorf("unexpected route %s %s", route.method, route.path)
			continue
		}
		if count != 1 {
			t.Errorf("route %s %s appears %d times, want once", route.method, route.path, count)
		}
	}
	for route := range wantRoutes {
		if gotRoutes[route] == 0 {
			t.Errorf("missing route %s %s", route.method, route.path)
		}
	}
}

// newUserRequest creates the test fixture used by this test file.
func newUserRequest(method, target, body string) *http.Request {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	return request.WithContext(
		core_logger.ToContext(request.Context(), &core_logger.Logger{Logger: zap.NewNop()}),
	)
}
