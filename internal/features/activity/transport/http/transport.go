package activity_transport_http

import (
	"context"
	"net/http"

	core_http_server "github.com/Reigor1/20h-project/internal/core/transport/http/server"
	activity_service "github.com/Reigor1/20h-project/internal/features/activity/service"
)

// ActivityService defines the listing operation exposed by the Activity HTTP transport.
type ActivityService interface {
	GetActivities(
		ctx context.Context,
		taskID int,
		limit *int,
		offset *int,
	) ([]activity_service.Activity, error)
}

// ActivityHTTPHandler adapts Activity service operations to HTTP requests and responses.
type ActivityHTTPHandler struct {
	activityService ActivityService
}

// NewActivityHTTPHandler creates the Activity handler with its service and logger dependencies.
func NewActivityHTTPHandler(
	activityService ActivityService,
) *ActivityHTTPHandler {
	return &ActivityHTTPHandler{
		activityService: activityService,
	}
}

// Routes returns the HTTP routes served by the Activity feature.
func (h *ActivityHTTPHandler) Routes() []core_http_server.Route {
	return []core_http_server.Route{
		{
			Method:  http.MethodGet,
			Path:    "/tasks/{task_id}/activity",
			Handler: h.GetActivities,
		},
	}
}
