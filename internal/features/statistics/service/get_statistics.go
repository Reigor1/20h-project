package statistics_service

import (
	"context"
	"fmt"
	"time"

	"github.com/Reigor1/20h-project/internal/core/domain"
	core_errors "github.com/Reigor1/20h-project/internal/core/errors"
	core_observability "github.com/Reigor1/20h-project/internal/core/observability"
)

func (s *StatisticsService) GetStatistics(
	ctx context.Context,
	userID *int,
	from *time.Time,
	to *time.Time,
) (domain.Statistics, error) {
	ctx, span := s.tracer.Start(ctx, "statistics.get")
	defer span.End()

	if from != nil && to != nil {
		if to.Before(*from) || to.Equal(*from) {
			core_observability.MarkSpanError(span, "statistics.get")
			return domain.Statistics{}, fmt.Errorf(
				"`to` must be after `from`: %w",
				core_errors.ErrInvalidArgument,
			)
		}
	}

	cacheable := userID == nil && from == nil && to == nil

	if cacheable && s.cache != nil {
		statistics, found, err := s.cache.Get(ctx)

		if err != nil {
			span.RecordError(fmt.Errorf("get statistics from cache: %w", err))
		} else if found {
			return statistics, nil
		}
	}

	tasks, err := s.statisticsRepository.GetTasks(ctx, userID, from, to)
	if err != nil {
		core_observability.MarkSpanError(span, "statistics.get")
		return domain.Statistics{}, fmt.Errorf("get tasks from repository: %w", err)
	}

	statistics := calcStatistics(tasks)

	if cacheable && s.cache != nil {
		if err := s.cache.Set(ctx, statistics, globalStatisticsCacheTTL); err != nil {
			span.RecordError(fmt.Errorf("set statistics cache: %w", err))
		}
	}

	return statistics, nil
}

// calcStatistics derives aggregate task counts and completion-time statistics from task records.
func calcStatistics(tasks []domain.Task) domain.Statistics {
	if len(tasks) == 0 {
		return domain.NewStatistics(0, 0, nil, nil)
	}

	tasksCreated := len(tasks)

	tasksCompleted := 0
	var totalCompletionDuration time.Duration
	for _, task := range tasks {
		if task.Completed {
			tasksCompleted++
		}

		completionDuration := task.CompletionDuration()
		if completionDuration != nil {
			totalCompletionDuration += *completionDuration
		}
	}

	tasksCompletedRate := float64(tasksCompleted) / float64(tasksCreated) * 100

	var tasksAverageCompletionTime *time.Duration
	if tasksCompleted > 0 && totalCompletionDuration != 0 {
		avg := totalCompletionDuration / time.Duration(tasksCompleted)

		tasksAverageCompletionTime = &avg
	}

	return domain.NewStatistics(
		tasksCreated,
		tasksCompleted,
		&tasksCompletedRate,
		tasksAverageCompletionTime,
	)
}
