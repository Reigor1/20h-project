package statistics_service

import (
	"context"
	"time"

	"github.com/Reigor1/20h-project/internal/core/domain"
	"go.opentelemetry.io/otel/trace"
)

const (
	tracerInstrumentationName = "github.com/Reigor1/20h-project/internal/features/statistics/service"

	globalStatisticsCacheTTL = 60 * time.Second
)

type StatisticsService struct {
	statisticsRepository StatisticsRepository
	cache                StatisticsCache
	tracer               trace.Tracer
}

type StatisticsRepository interface {
	GetTasks(
		ctx context.Context,
		userID *int,
		from *time.Time,
		to *time.Time,
	) ([]domain.Task, error)
}

type StatisticsCache interface {
	Get(ctx context.Context) (domain.Statistics, bool, error)
	Set(ctx context.Context, statistics domain.Statistics, ttl time.Duration) error
	Delete(ctx context.Context) error
}

func NewStatisticsService(
	statisticsRepository StatisticsRepository,
	cache StatisticsCache,
	tracerProvider trace.TracerProvider,
) *StatisticsService {
	return &StatisticsService{
		statisticsRepository: statisticsRepository,
		cache:                cache,
		tracer:               tracerProvider.Tracer(tracerInstrumentationName),
	}
}
