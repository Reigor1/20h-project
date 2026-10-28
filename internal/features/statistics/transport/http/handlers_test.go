package statistics_transport_http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/Reigor1/20h-project/internal/core/domain"
	core_errors "github.com/Reigor1/20h-project/internal/core/errors"
	core_logger "github.com/Reigor1/20h-project/internal/core/logger"
	core_http_server "github.com/Reigor1/20h-project/internal/core/transport/http/server"
	"github.com/Reigor1/20h-project/internal/testutil"
)

type statisticsServiceStub struct {
	t *testing.T

	expectGetStatistics bool
	getStatisticsCalls  int
	result              domain.Statistics
	err                 error

	gotUserID *int
	gotFrom   *time.Time
	gotTo     *time.Time
}

func newStatisticsServiceStub(t *testing.T) *statisticsServiceStub {
	t.Helper()

	return &statisticsServiceStub{t: t}
}

func (s *statisticsServiceStub) GetStatistics(
	_ context.Context,
	userID *int,
	from *time.Time,
	to *time.Time,
) (domain.Statistics, error) {
	s.getStatisticsCalls++
	s.gotUserID = userID
	s.gotFrom = from
	s.gotTo = to
	if !s.expectGetStatistics {
		s.t.Fatal("unexpected StatisticsService.GetStatistics call")
	}

	return s.result, s.err
}

func TestGetStatisticsReturnsJSONAndForwardsQueryParameters(t *testing.T) {
	userID := 27
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	completedRate := 75.0
	averageCompletionTime := 90 * time.Second
	service := newStatisticsServiceStub(t)
	service.expectGetStatistics = true
	service.result = domain.NewStatistics(8, 6, &completedRate, &averageCompletionTime)
	handler := NewStatisticsHTTPHandler(service)
	request := newStatisticsRequest("/statistics?user_id=27&from=2026-10-01&to=2026-11-01")
	response := httptest.NewRecorder()

	handler.GetStatistics(response, request)

	testutil.AssertJSONStatus(t, response, http.StatusOK)
	var got GetStatisticsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode statistics response: %v; body = %q", err, response.Body.String())
	}
	wantAverage := "1m30s"
	want := GetStatisticsResponse{
		TasksCreated:               8,
		TasksCompleted:             6,
		TasksCompletedRate:         &completedRate,
		TasksAverageCompletionTime: &wantAverage,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("response = %+v, want %+v", got, want)
	}
	wantFrom := from
	wantTo := to
	if service.getStatisticsCalls != 1 || service.gotUserID == nil || *service.gotUserID != userID ||
		service.gotFrom == nil || !service.gotFrom.Equal(wantFrom) ||
		service.gotTo == nil || !service.gotTo.Equal(wantTo) {
		t.Fatalf(
			"GetStatistics() calls/filters = %d/%v/%v/%v, want one call with user_id=%d, from=%s, to=%s",
			service.getStatisticsCalls,
			service.gotUserID,
			service.gotFrom,
			service.gotTo,
			userID,
			wantFrom.Format(time.DateOnly),
			wantTo.Format(time.DateOnly),
		)
	}
}

func TestGetStatisticsAllowsAbsentFiltersAndNilMetrics(t *testing.T) {
	service := newStatisticsServiceStub(t)
	service.expectGetStatistics = true
	service.result = domain.NewStatistics(0, 0, nil, nil)
	handler := NewStatisticsHTTPHandler(service)
	response := httptest.NewRecorder()

	handler.GetStatistics(response, newStatisticsRequest("/statistics"))

	testutil.AssertJSONStatus(t, response, http.StatusOK)
	var got GetStatisticsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode statistics response: %v; body = %q", err, response.Body.String())
	}
	want := GetStatisticsResponse{TasksCreated: 0, TasksCompleted: 0}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("response = %+v, want %+v", got, want)
	}
	if service.getStatisticsCalls != 1 || service.gotUserID != nil || service.gotFrom != nil || service.gotTo != nil {
		t.Fatalf(
			"GetStatistics() calls/filters = %d/%v/%v/%v, want one call with absent filters",
			service.getStatisticsCalls,
			service.gotUserID,
			service.gotFrom,
			service.gotTo,
		)
	}
}

func TestGetStatisticsRejectsInvalidQueryParameters(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{
			name:  "user ID is not an integer",
			query: "?user_id=not-a-number",
		},
		{
			name:  "from is not a date",
			query: "?from=2026-13-01",
		},
		{
			name:  "to is not a date",
			query: "?to=not-a-date",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			service := newStatisticsServiceStub(t)
			handler := NewStatisticsHTTPHandler(service)
			response := httptest.NewRecorder()

			handler.GetStatistics(response, newStatisticsRequest("/statistics"+testCase.query))

			testutil.AssertErrorResponse(t, response, http.StatusBadRequest)
			if service.getStatisticsCalls != 0 {
				t.Fatalf("GetStatistics() calls = %d, want 0 for invalid query parameters", service.getStatisticsCalls)
			}
		})
	}
}

func TestGetStatisticsReturnsServiceError(t *testing.T) {
	tests := []struct {
		name       string
		serviceErr error
		wantStatus int
	}{
		{
			name:       "invalid argument is bad request",
			serviceErr: core_errors.ErrInvalidArgument,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "unexpected service error is internal server error",
			serviceErr: errors.New("statistics unavailable"),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			service := newStatisticsServiceStub(t)
			service.expectGetStatistics = true
			service.err = testCase.serviceErr
			handler := NewStatisticsHTTPHandler(service)
			response := httptest.NewRecorder()

			handler.GetStatistics(response, newStatisticsRequest("/statistics"))

			testutil.AssertErrorResponse(t, response, testCase.wantStatus)
			if service.getStatisticsCalls != 1 {
				t.Fatalf("GetStatistics() calls = %d, want 1", service.getStatisticsCalls)
			}
		})
	}
}

func TestStatisticsHTTPHandlerRoutes(t *testing.T) {
	handler := NewStatisticsHTTPHandler(newStatisticsServiceStub(t))
	routes := handler.Routes()

	if len(routes) != 1 {
		t.Fatalf("Routes() count = %d, want 1", len(routes))
	}
	wantRoute := core_http_server.Route{
		Method: http.MethodGet,
		Path:   "/statistics",
	}
	gotRoute := routes[0]
	if gotRoute.Method != wantRoute.Method || gotRoute.Path != wantRoute.Path {
		t.Fatalf("route = %s %s, want %s %s", gotRoute.Method, gotRoute.Path, wantRoute.Method, wantRoute.Path)
	}
	if gotRoute.Handler == nil {
		t.Fatal("statistics route has a nil handler")
	}
}

func newStatisticsRequest(target string) *http.Request {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	return request.WithContext(
		core_logger.ToContext(request.Context(), testutil.NewNopLogger()),
	)
}
