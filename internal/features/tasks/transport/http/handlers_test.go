package tasks_transport_http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Reigor1/20h-project/internal/core/domain"
	core_errors "github.com/Reigor1/20h-project/internal/core/errors"
	core_logger "github.com/Reigor1/20h-project/internal/core/logger"
	"github.com/Reigor1/20h-project/internal/testutil"
	"go.uber.org/zap"
)

type tasksServiceStub struct {
	t *testing.T

	expectCreateTask bool
	expectGetTasks   bool
	expectGetTask    bool
	expectDeleteTask bool
	expectPatchTask  bool

	createTaskResult domain.Task
	createTaskError  error
	createTaskCalls  int
	createdTask      domain.Task

	getTasksResult []domain.Task
	getTasksError  error
	getTasksCalls  int
	getTasksUserID *int
	getTasksLimit  *int
	getTasksOffset *int

	getTaskResult domain.Task
	getTaskError  error
	getTaskCalls  int
	getTaskID     int

	deleteTaskError error
	deleteTaskCalls int
	deleteTaskID    int

	patchTaskResult domain.Task
	patchTaskError  error
	patchTaskCalls  int
	patchTaskID     int
	taskPatch       domain.TaskPatch
}

func newTasksServiceStub(t *testing.T) *tasksServiceStub {
	t.Helper()
	return &tasksServiceStub{t: t}
}

func (s *tasksServiceStub) CreateTask(_ context.Context, task domain.Task) (domain.Task, error) {
	s.createTaskCalls++
	s.createdTask = task
	if !s.expectCreateTask {
		s.t.Fatal("unexpected TasksService.CreateTask call")
	}
	return s.createTaskResult, s.createTaskError
}

func (s *tasksServiceStub) GetTasks(
	_ context.Context,
	userID *int,
	limit *int,
	offset *int,
) ([]domain.Task, error) {
	s.getTasksCalls++
	s.getTasksUserID = userID
	s.getTasksLimit = limit
	s.getTasksOffset = offset
	if !s.expectGetTasks {
		s.t.Fatal("unexpected TasksService.GetTasks call")
	}
	return s.getTasksResult, s.getTasksError
}

func (s *tasksServiceStub) GetTask(_ context.Context, id int) (domain.Task, error) {
	s.getTaskCalls++
	s.getTaskID = id
	if !s.expectGetTask {
		s.t.Fatal("unexpected TasksService.GetTask call")
	}
	return s.getTaskResult, s.getTaskError
}

func (s *tasksServiceStub) DeleteTask(_ context.Context, id int) error {
	s.deleteTaskCalls++
	s.deleteTaskID = id
	if !s.expectDeleteTask {
		s.t.Fatal("unexpected TasksService.DeleteTask call")
	}
	return s.deleteTaskError
}

func (s *tasksServiceStub) PathTask(
	_ context.Context,
	id int,
	patch domain.TaskPatch,
) (domain.Task, error) {
	s.patchTaskCalls++
	s.patchTaskID = id
	s.taskPatch = patch
	if !s.expectPatchTask {
		s.t.Fatal("unexpected TasksService.PathTask call")
	}
	return s.patchTaskResult, s.patchTaskError
}

func TestCreatedTask(t *testing.T) {
	description := "Prepare a clear report"
	createdAt := time.Date(2026, 10, 20, 9, 30, 0, 0, time.UTC)
	completedAt := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	wantTask := domain.NewTask(
		17,
		4,
		"Write report",
		&description,
		true,
		createdAt,
		&completedAt,
		23,
	)
	service := newTasksServiceStub(t)
	service.expectCreateTask = true
	service.createTaskResult = wantTask
	handler := NewTasksHTTPHandler(service)
	request := newTaskRequest(
		http.MethodPost,
		"/tasks",
		`{"title":"Write report","description":"Prepare a clear report","author_user_id":23}`,
	)
	response := httptest.NewRecorder()

	handler.CreatedTask(response, request)

	testutil.AssertJSONStatus(t, response, http.StatusCreated)
	var got TaskDTOResponse
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if want := (TaskDTOResponse)(wantTask); !reflect.DeepEqual(got, want) {
		t.Fatalf("response = %+v, want %+v", got, want)
	}
	if service.createTaskCalls != 1 {
		t.Fatalf("CreateTask() calls = %d, want 1", service.createTaskCalls)
	}
	if service.createdTask.Title != "Write report" ||
		service.createdTask.Description == nil ||
		*service.createdTask.Description != description ||
		service.createdTask.AuthorUserID != 23 ||
		service.createdTask.Completed ||
		service.createdTask.CompletedAt != nil ||
		service.createdTask.ID != domain.UninitializedID ||
		service.createdTask.Version != domain.UninitializedVersion ||
		service.createdTask.CreatedAt.IsZero() {
		t.Fatalf("service received task = %+v, want uninitialized task from request", service.createdTask)
	}
}

func TestCreatedTaskRejectsInvalidRequest(t *testing.T) {
	longTitle := strings.Repeat("a", 101)
	tests := []struct {
		name string
		body string
	}{
		{name: "malformed JSON", body: `{"title":`},
		{name: "empty body", body: ""},
		{name: "missing required title", body: `{"author_user_id":23}`},
		{name: "title too long", body: fmt.Sprintf(`{"title":%q,"author_user_id":23}`, longTitle)},
		{name: "description has wrong type", body: `{"title":"Task","description":42,"author_user_id":23}`},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			service := newTasksServiceStub(t)
			handler := NewTasksHTTPHandler(service)
			response := httptest.NewRecorder()

			handler.CreatedTask(response, newTaskRequest(http.MethodPost, "/tasks", testCase.body))

			testutil.AssertErrorResponse(t, response, http.StatusBadRequest)
			if service.createTaskCalls != 0 {
				t.Fatalf("CreateTask() calls = %d, want 0 for invalid request", service.createTaskCalls)
			}
		})
	}
}

func TestCreatedTaskReturnsServiceError(t *testing.T) {
	service := newTasksServiceStub(t)
	service.expectCreateTask = true
	service.createTaskError = core_errors.ErrNotFound
	handler := NewTasksHTTPHandler(service)
	response := httptest.NewRecorder()

	handler.CreatedTask(
		response,
		newTaskRequest(http.MethodPost, "/tasks", `{"title":"Task","author_user_id":23}`),
	)

	testutil.AssertErrorResponse(t, response, http.StatusNotFound)
	if service.createTaskCalls != 1 || service.createdTask.Title != "Task" || service.createdTask.AuthorUserID != 23 {
		t.Fatalf("CreateTask() calls/task = %d/%+v, want one call with request values", service.createTaskCalls, service.createdTask)
	}
}

func TestGetTask(t *testing.T) {
	description := "Check the submitted work"
	createdAt := time.Date(2026, 10, 21, 11, 12, 13, 0, time.UTC)
	completedAt := time.Date(2026, 10, 21, 12, 12, 13, 0, time.UTC)
	wantTask := domain.NewTask(42, 6, "Review report", &description, true, createdAt, &completedAt, 9)
	service := newTasksServiceStub(t)
	service.expectGetTask = true
	service.getTaskResult = wantTask
	handler := NewTasksHTTPHandler(service)
	request := newTaskRequest(http.MethodGet, "/tasks/42", "")
	request.SetPathValue("id", "42")
	response := httptest.NewRecorder()

	handler.GetTask(response, request)

	testutil.AssertJSONStatus(t, response, http.StatusOK)
	var got TaskDTOResponse
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode get task response: %v", err)
	}
	if want := (TaskDTOResponse)(wantTask); !reflect.DeepEqual(got, want) {
		t.Fatalf("response = %+v, want %+v", got, want)
	}
	if service.getTaskCalls != 1 || service.getTaskID != 42 {
		t.Fatalf("GetTask() calls/id = %d/%d, want 1/42", service.getTaskCalls, service.getTaskID)
	}
}

func TestGetTaskRejectsInvalidID(t *testing.T) {
	tests := []struct {
		name string
		id   string
	}{
		{name: "non-numeric ID", id: "abc"},
		{name: "integer overflow", id: "999999999999999999999999999999999"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			service := newTasksServiceStub(t)
			handler := NewTasksHTTPHandler(service)
			request := newTaskRequest(http.MethodGet, "/tasks/"+testCase.id, "")
			request.SetPathValue("id", testCase.id)
			response := httptest.NewRecorder()

			handler.GetTask(response, request)

			testutil.AssertErrorResponse(t, response, http.StatusBadRequest)
			if service.getTaskCalls != 0 {
				t.Fatalf("GetTask() calls = %d, want 0 for invalid ID", service.getTaskCalls)
			}
		})
	}
}

func TestGetTaskReturnsServiceError(t *testing.T) {
	assertTaskLookupError(t, core_errors.ErrNotFound, http.StatusNotFound)
	assertTaskLookupError(t, errors.New("database unavailable"), http.StatusInternalServerError)
}

func assertTaskLookupError(t *testing.T, serviceError error, wantStatus int) {
	t.Helper()
	service := newTasksServiceStub(t)
	service.expectGetTask = true
	service.getTaskError = serviceError
	handler := NewTasksHTTPHandler(service)
	request := newTaskRequest(http.MethodGet, "/tasks/42", "")
	request.SetPathValue("id", "42")
	response := httptest.NewRecorder()

	handler.GetTask(response, request)

	testutil.AssertErrorResponse(t, response, wantStatus)
	if service.getTaskCalls != 1 || service.getTaskID != 42 {
		t.Fatalf("GetTask() calls/id = %d/%d, want 1/42", service.getTaskCalls, service.getTaskID)
	}
}

func TestGetTasks(t *testing.T) {
	description := "An optional description"
	createdAt := time.Date(2026, 10, 22, 8, 0, 0, 0, time.UTC)
	wantTasks := []domain.Task{
		domain.NewTask(1, 2, "First task", &description, false, createdAt, nil, 7),
		domain.NewTask(3, 4, "Second task", nil, false, createdAt.Add(time.Hour), nil, 8),
	}
	service := newTasksServiceStub(t)
	service.expectGetTasks = true
	service.getTasksResult = wantTasks
	handler := NewTasksHTTPHandler(service)
	response := httptest.NewRecorder()

	handler.GetTasks(
		response,
		newTaskRequest(http.MethodGet, "/tasks?user_id=7&limit=20&offset=40", ""),
	)

	testutil.AssertJSONStatus(t, response, http.StatusOK)
	var got []TaskDTOResponse
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode get tasks response: %v", err)
	}
	want := []TaskDTOResponse{
		(TaskDTOResponse)(wantTasks[0]),
		(TaskDTOResponse)(wantTasks[1]),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("response = %+v, want %+v", got, want)
	}
	if service.getTasksCalls != 1 ||
		!intPointerEquals(service.getTasksUserID, 7) ||
		!intPointerEquals(service.getTasksLimit, 20) ||
		!intPointerEquals(service.getTasksOffset, 40) {
		t.Fatalf("GetTasks() calls/filters = %d/%v/%v/%v, want 1/7/20/40", service.getTasksCalls, service.getTasksUserID, service.getTasksLimit, service.getTasksOffset)
	}
}

func TestGetTasksPassesOmittedQueryParametersAsNil(t *testing.T) {
	service := newTasksServiceStub(t)
	service.expectGetTasks = true
	service.getTasksResult = []domain.Task{}
	handler := NewTasksHTTPHandler(service)
	response := httptest.NewRecorder()

	handler.GetTasks(response, newTaskRequest(http.MethodGet, "/tasks", ""))

	testutil.AssertJSONStatus(t, response, http.StatusOK)
	if response.Body.String() != "[]\n" {
		t.Fatalf("empty task list body = %q, want [] JSON", response.Body.String())
	}
	if service.getTasksCalls != 1 || service.getTasksUserID != nil || service.getTasksLimit != nil || service.getTasksOffset != nil {
		t.Fatalf("GetTasks() calls/filters = %d/%v/%v/%v, want 1/nil/nil/nil", service.getTasksCalls, service.getTasksUserID, service.getTasksLimit, service.getTasksOffset)
	}
}

func TestGetTasksRejectsInvalidQueryParameter(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{name: "user_id is not an integer", query: "?user_id=abc"},
		{name: "limit is not an integer", query: "?limit=many"},
		{name: "offset overflows int", query: "?offset=999999999999999999999999999999"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			service := newTasksServiceStub(t)
			handler := NewTasksHTTPHandler(service)
			response := httptest.NewRecorder()

			handler.GetTasks(response, newTaskRequest(http.MethodGet, "/tasks"+testCase.query, ""))

			testutil.AssertErrorResponse(t, response, http.StatusBadRequest)
			if service.getTasksCalls != 0 {
				t.Fatalf("GetTasks() calls = %d, want 0 for invalid query", service.getTasksCalls)
			}
		})
	}
}

func TestGetTasksReturnsServiceError(t *testing.T) {
	service := newTasksServiceStub(t)
	service.expectGetTasks = true
	service.getTasksError = errors.New("database unavailable")
	handler := NewTasksHTTPHandler(service)
	response := httptest.NewRecorder()

	handler.GetTasks(response, newTaskRequest(http.MethodGet, "/tasks?limit=5", ""))

	testutil.AssertErrorResponse(t, response, http.StatusInternalServerError)
	if service.getTasksCalls != 1 || !intPointerEquals(service.getTasksLimit, 5) || service.getTasksOffset != nil {
		t.Fatalf("GetTasks() calls/limit/offset = %d/%v/%v, want 1/5/nil", service.getTasksCalls, service.getTasksLimit, service.getTasksOffset)
	}
}

func TestPatchTaskPreservesOmittedNullAndFalseFields(t *testing.T) {
	createdAt := time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC)
	wantTask := domain.NewTask(42, 5, "Existing title", nil, false, createdAt, nil, 13)
	service := newTasksServiceStub(t)
	service.expectPatchTask = true
	service.patchTaskResult = wantTask
	handler := NewTasksHTTPHandler(service)
	request := newTaskRequest(
		http.MethodPatch,
		"/tasks/42",
		`{"description":null,"completed":false}`,
	)
	request.SetPathValue("id", "42")
	response := httptest.NewRecorder()

	handler.PatchTask(response, request)

	testutil.AssertJSONStatus(t, response, http.StatusOK)
	var got TaskDTOResponse
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode patch response: %v", err)
	}
	if want := (TaskDTOResponse)(wantTask); !reflect.DeepEqual(got, want) {
		t.Fatalf("response = %+v, want %+v", got, want)
	}
	if service.patchTaskCalls != 1 || service.patchTaskID != 42 {
		t.Fatalf("PathTask() calls/id = %d/%d, want 1/42", service.patchTaskCalls, service.patchTaskID)
	}
	if service.taskPatch.Title.Set {
		t.Errorf("Title patch = %+v, want omitted", service.taskPatch.Title)
	}
	if !service.taskPatch.Description.Set || service.taskPatch.Description.Value != nil {
		t.Errorf("Description patch = %+v, want explicit null", service.taskPatch.Description)
	}
	if !service.taskPatch.Completed.Set || service.taskPatch.Completed.Value == nil || *service.taskPatch.Completed.Value {
		t.Errorf("Completed patch = %+v, want explicit false", service.taskPatch.Completed)
	}
}

func TestPatchTaskPassesProvidedValues(t *testing.T) {
	title := "Updated title"
	description := "Updated description"
	createdAt := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	service := newTasksServiceStub(t)
	service.expectPatchTask = true
	service.patchTaskResult = domain.NewTask(42, 6, title, &description, true, createdAt, nil, 13)
	handler := NewTasksHTTPHandler(service)
	request := newTaskRequest(
		http.MethodPatch,
		"/tasks/42",
		`{"title":"Updated title","description":"Updated description","completed":true}`,
	)
	request.SetPathValue("id", "42")
	response := httptest.NewRecorder()

	handler.PatchTask(response, request)

	testutil.AssertJSONStatus(t, response, http.StatusOK)
	if !nullableStringEquals(service.taskPatch.Title, title) ||
		!nullableStringEquals(service.taskPatch.Description, description) ||
		!nullableBoolEquals(service.taskPatch.Completed, true) {
		t.Fatalf("PathTask() patch = %+v, want all supplied values", service.taskPatch)
	}
}

func TestPatchTaskRejectsInvalidPathOrBody(t *testing.T) {
	longTitle := strings.Repeat("a", 101)
	longDescription := strings.Repeat("a", 1001)
	tests := []struct {
		name string
		id   string
		body string
	}{
		{name: "invalid task ID", id: "invalid", body: `{"title":"Valid title"}`},
		{name: "malformed JSON", id: "42", body: `{"title":`},
		{name: "title cannot be null", id: "42", body: `{"title":null}`},
		{name: "title cannot be empty", id: "42", body: `{"title":""}`},
		{name: "title exceeds maximum length", id: "42", body: fmt.Sprintf(`{"title":%q}`, longTitle)},
		{name: "description exceeds maximum length", id: "42", body: fmt.Sprintf(`{"description":%q}`, longDescription)},
		{name: "completed cannot be null", id: "42", body: `{"completed":null}`},
		{name: "completed has wrong type", id: "42", body: `{"completed":"false"}`},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			service := newTasksServiceStub(t)
			handler := NewTasksHTTPHandler(service)
			request := newTaskRequest(http.MethodPatch, "/tasks/"+testCase.id, testCase.body)
			request.SetPathValue("id", testCase.id)
			response := httptest.NewRecorder()

			handler.PatchTask(response, request)

			testutil.AssertErrorResponse(t, response, http.StatusBadRequest)
			if service.patchTaskCalls != 0 {
				t.Fatalf("PathTask() calls = %d, want 0 for invalid request", service.patchTaskCalls)
			}
		})
	}
}

func TestPatchTaskReturnsServiceError(t *testing.T) {
	service := newTasksServiceStub(t)
	service.expectPatchTask = true
	service.patchTaskError = core_errors.ErrConflict
	handler := NewTasksHTTPHandler(service)
	request := newTaskRequest(http.MethodPatch, "/tasks/42", `{"title":"Updated title"}`)
	request.SetPathValue("id", "42")
	response := httptest.NewRecorder()

	handler.PatchTask(response, request)

	testutil.AssertErrorResponse(t, response, http.StatusConflict)
	if service.patchTaskCalls != 1 || service.patchTaskID != 42 || !nullableStringEquals(service.taskPatch.Title, "Updated title") {
		t.Fatalf("PathTask() calls/id/patch = %d/%d/%+v, want 1/42/title update", service.patchTaskCalls, service.patchTaskID, service.taskPatch)
	}
}

func TestDeleteTask(t *testing.T) {
	service := newTasksServiceStub(t)
	service.expectDeleteTask = true
	handler := NewTasksHTTPHandler(service)
	request := newTaskRequest(http.MethodDelete, "/tasks/42", "")
	request.SetPathValue("id", "42")
	response := httptest.NewRecorder()

	handler.DeleteTask(response, request)

	if response.Code != http.StatusNoContent || response.Body.Len() != 0 {
		t.Fatalf("response = %d %q, want empty 204 response", response.Code, response.Body.String())
	}
	if service.deleteTaskCalls != 1 || service.deleteTaskID != 42 {
		t.Fatalf("DeleteTask() calls/id = %d/%d, want 1/42", service.deleteTaskCalls, service.deleteTaskID)
	}
}

func TestDeleteTaskRejectsInvalidID(t *testing.T) {
	service := newTasksServiceStub(t)
	handler := NewTasksHTTPHandler(service)
	request := newTaskRequest(http.MethodDelete, "/tasks/not-an-id", "")
	request.SetPathValue("id", "not-an-id")
	response := httptest.NewRecorder()

	handler.DeleteTask(response, request)

	testutil.AssertErrorResponse(t, response, http.StatusBadRequest)
	if service.deleteTaskCalls != 0 {
		t.Fatalf("DeleteTask() calls = %d, want 0 for invalid ID", service.deleteTaskCalls)
	}
}

func TestDeleteTaskReturnsServiceError(t *testing.T) {
	service := newTasksServiceStub(t)
	service.expectDeleteTask = true
	service.deleteTaskError = core_errors.ErrNotFound
	handler := NewTasksHTTPHandler(service)
	request := newTaskRequest(http.MethodDelete, "/tasks/42", "")
	request.SetPathValue("id", "42")
	response := httptest.NewRecorder()

	handler.DeleteTask(response, request)

	testutil.AssertErrorResponse(t, response, http.StatusNotFound)
	if service.deleteTaskCalls != 1 || service.deleteTaskID != 42 {
		t.Fatalf("DeleteTask() calls/id = %d/%d, want 1/42", service.deleteTaskCalls, service.deleteTaskID)
	}
}

func TestTasksHTTPHandlerRoutes(t *testing.T) {
	handler := NewTasksHTTPHandler(newTasksServiceStub(t))
	routes := handler.Routes()
	type routeKey struct {
		method string
		path   string
	}
	wantRoutes := map[routeKey]struct{}{
		{method: http.MethodPost, path: "/tasks"}:        {},
		{method: http.MethodGet, path: "/tasks"}:         {},
		{method: http.MethodGet, path: "/tasks/{id}"}:    {},
		{method: http.MethodDelete, path: "/tasks/{id}"}: {},
		{method: http.MethodPatch, path: "/tasks/{id}"}:  {},
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

func newTaskRequest(method, target, body string) *http.Request {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	return request.WithContext(
		core_logger.ToContext(request.Context(), &core_logger.Logger{Logger: zap.NewNop()}),
	)
}

func intPointerEquals(got *int, want int) bool {
	return got != nil && *got == want
}

func nullableStringEquals(got domain.Nullable[string], want string) bool {
	return got.Set && got.Value != nil && *got.Value == want
}

func nullableBoolEquals(got domain.Nullable[bool], want bool) bool {
	return got.Set && got.Value != nil && *got.Value == want
}
