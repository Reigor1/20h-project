package statistics_transport_http

import (
	"context"
	"net/http"
	"time"

	"github.com/Reigor1/20h-project/internal/core/domain"
	core_http_server "github.com/Reigor1/20h-project/internal/core/transport/http/server"
)

// StatisticsHTTPHandler exposes statistics use cases through HTTP endpoints.
type StatisticsHTTPHandler struct {
	statisticsService StatisticsService
}

// StatisticsService defines the filtered statistics operation used by the HTTP layer.
type StatisticsService interface {
	GetStatistics(
		ctx context.Context,
		userID *int,
		from *time.Time,
		to *time.Time,
	) (domain.Statistics, error)
}

// NewStatisticsHTTPHandler creates the statistics handler with its service dependency.
func NewStatisticsHTTPHandler(
	statisticsService StatisticsService,
) *StatisticsHTTPHandler {
	return &StatisticsHTTPHandler{
		statisticsService: statisticsService,
	}
}

// Routes returns the HTTP routes registered by the Statistics feature.
func (h *StatisticsHTTPHandler) Routes() []core_http_server.Route {
	return []core_http_server.Route{
		{
			Method:  http.MethodGet,
			Path:    "/statistics",
			Handler: h.GetStatistics,
		},
	}
}
