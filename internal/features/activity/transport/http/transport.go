package activity_transport_http

import (
	"context"
	"net/http"

	core_http_server "github.com/Reigor1/20h-project/internal/core/transport/http/server"
	activity_service "github.com/Reigor1/20h-project/internal/features/activity/service"
)

type ActivityService interface {
	GetActivities(
		ctx context.Context,
		taskID int,
		limit *int,
		offset *int,
	) ([]activity_service.Activity, error)
}

type ActivityHTTPHandler struct {
	activityService ActivityService
}

func NewActivityHTTPHandler(
	activityService ActivityService,
) *ActivityHTTPHandler {
	return &ActivityHTTPHandler{
		activityService: activityService,
	}
}

func (h *ActivityHTTPHandler) Routes() []core_http_server.Route {
	return []core_http_server.Route{
		{
			Method:  http.MethodGet,
			Path:    "/tasks/{task_id}/activity",
			Handler: h.GetActivities,
		},
	}
}
