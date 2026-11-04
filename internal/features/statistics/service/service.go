package statistics_service

import (
	"context"
	"time"

	"github.com/Reigor1/20h-project/internal/core/domain"
	"go.opentelemetry.io/otel/trace"
)

const tracerInstrumentationName = "github.com/Reigor1/20h-project/internal/features/statistics/service"

type StatisticsService struct {
	statisticsRepository StatisticsRepository
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

func NewStatisticsService(
	statisticsRepository StatisticsRepository,
	tracerProvider trace.TracerProvider,
) *StatisticsService {
	return &StatisticsService{
		statisticsRepository: statisticsRepository,
		tracer:               tracerProvider.Tracer(tracerInstrumentationName),
	}
}
