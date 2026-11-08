package statistics_service

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Reigor1/20h-project/internal/core/domain"
	core_errors "github.com/Reigor1/20h-project/internal/core/errors"
	"go.opentelemetry.io/otel/trace/noop"
)

type statisticsRepositoryStub struct {
	t *testing.T

	called bool

	tasks []domain.Task
	err   error

	gotUserID *int
	gotFrom   *time.Time
	gotTo     *time.Time
}

func newStatisticsRepositoryStub(t *testing.T) *statisticsRepositoryStub {
	t.Helper()

	return &statisticsRepositoryStub{t: t}
}

func (r *statisticsRepositoryStub) GetTasks(
	_ context.Context,
	userID *int,
	from *time.Time,
	to *time.Time,
) ([]domain.Task, error) {
	if r.called {
		r.t.Fatal("unexpected additional StatisticsRepository.GetTasks call")
	}
	r.called = true
	r.gotUserID = userID
	r.gotFrom = from
	r.gotTo = to

	return r.tasks, r.err
}

func TestGetStatisticsCalculatesAggregatesAndForwardsFilters(t *testing.T) {
	userID := 27
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	createdAt := time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)
	firstCompletedAt := createdAt.Add(30 * time.Minute)
	secondCreatedAt := createdAt.Add(time.Hour)
	secondCompletedAt := secondCreatedAt.Add(90 * time.Minute)
	firstTask := domain.NewTask(
		1,
		1,
		"First completed task",
		nil,
		true,
		createdAt,
		&firstCompletedAt,
		userID,
	)
	secondTask := domain.NewTask(
		2,
		1,
		"Second completed task",
		nil,
		true,
		secondCreatedAt,
		&secondCompletedAt,
		userID,
	)
	incompleteTask := domain.NewTask(
		3,
		1,
		"Incomplete task",
		nil,
		false,
		createdAt,
		nil,
		userID,
	)
	repository := newStatisticsRepositoryStub(t)
	repository.tasks = []domain.Task{firstTask, secondTask, incompleteTask}
	service := newStatisticsServiceForTest(repository)

	got, err := service.GetStatistics(context.Background(), &userID, &from, &to)
	if err != nil {
		t.Fatalf("GetStatistics() error = %v, want nil", err)
	}
	wantRate := float64(2) / float64(3) * 100
	wantAverage := time.Hour
	want := domain.NewStatistics(3, 2, &wantRate, &wantAverage)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetStatistics() = %+v, want %+v", got, want)
	}
	if !repository.called || repository.gotUserID != &userID ||
		repository.gotFrom != &from || repository.gotTo != &to {
		t.Fatalf(
			"GetTasks() called/filters = %t/%p/%p/%p, want true/%p/%p/%p",
			repository.called,
			repository.gotUserID,
			repository.gotFrom,
			repository.gotTo,
			&userID,
			&from,
			&to,
		)
	}
}

func TestGetStatisticsReturnsZeroValuesForNoTasks(t *testing.T) {
	repository := newStatisticsRepositoryStub(t)
	repository.tasks = []domain.Task{}
	service := newStatisticsServiceForTest(repository)

	got, err := service.GetStatistics(context.Background(), nil, nil, nil)
	if err != nil {
		t.Fatalf("GetStatistics() error = %v, want nil", err)
	}
	want := domain.NewStatistics(0, 0, nil, nil)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetStatistics() = %+v, want %+v", got, want)
	}
	if !repository.called || repository.gotUserID != nil || repository.gotFrom != nil || repository.gotTo != nil {
		t.Fatalf(
			"GetTasks() called/filters = %t/%v/%v/%v, want true/nil/nil/nil",
			repository.called,
			repository.gotUserID,
			repository.gotFrom,
			repository.gotTo,
		)
	}
}

func TestGetStatisticsOmitsAverageWhenCompletedDurationIsZero(t *testing.T) {
	createdAt := time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)
	completedAt := createdAt
	completedTask := domain.NewTask(1, 1, "Instant task", nil, true, createdAt, &completedAt, 27)
	incompleteTask := domain.NewTask(2, 1, "Open task", nil, false, createdAt, nil, 27)
	repository := newStatisticsRepositoryStub(t)
	repository.tasks = []domain.Task{completedTask, incompleteTask}
	service := newStatisticsServiceForTest(repository)

	got, err := service.GetStatistics(context.Background(), nil, nil, nil)
	if err != nil {
		t.Fatalf("GetStatistics() error = %v, want nil", err)
	}
	wantRate := float64(1) / float64(2) * 100
	want := domain.NewStatistics(2, 1, &wantRate, nil)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetStatistics() = %+v, want %+v", got, want)
	}
}

func TestGetStatisticsRejectsInvalidDateRange(t *testing.T) {
	from := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		to   time.Time
	}{
		{
			name: "same start and end",
			to:   from,
		},
		{
			name: "end before start",
			to:   from.Add(-24 * time.Hour),
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			repository := newStatisticsRepositoryStub(t)
			service := newStatisticsServiceForTest(repository)

			got, err := service.GetStatistics(context.Background(), nil, &from, &testCase.to)

			if !errors.Is(err, core_errors.ErrInvalidArgument) {
				t.Fatalf("GetStatistics() error = %v, want ErrInvalidArgument", err)
			}
			if !reflect.DeepEqual(got, domain.Statistics{}) {
				t.Fatalf("GetStatistics() = %+v, want zero-value statistics", got)
			}
			if repository.called {
				t.Fatal("GetTasks() was called for an invalid date range")
			}
		})
	}
}

func TestGetStatisticsReturnsRepositoryError(t *testing.T) {
	repositoryError := errors.New("database unavailable")
	repository := newStatisticsRepositoryStub(t)
	repository.err = repositoryError
	service := newStatisticsServiceForTest(repository)

	got, err := service.GetStatistics(context.Background(), nil, nil, nil)

	if !errors.Is(err, repositoryError) {
		t.Fatalf("GetStatistics() error = %v, want wrapped repository error", err)
	}
	if !reflect.DeepEqual(got, domain.Statistics{}) {
		t.Fatalf("GetStatistics() = %+v, want zero-value statistics", got)
	}
	if !repository.called {
		t.Fatal("GetTasks() was not called")
	}
}

func newStatisticsServiceForTest(repository StatisticsRepository) *StatisticsService {
	return NewStatisticsService(repository, nil, noop.NewTracerProvider())
}
