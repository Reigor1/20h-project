package activity_transport_http

import (
	"fmt"
	"net/http"

	core_logger "github.com/Reigor1/20h-project/internal/core/logger"
	core_http_request "github.com/Reigor1/20h-project/internal/core/transport/http/request"
	core_http_response "github.com/Reigor1/20h-project/internal/core/transport/http/response"
)

type GetActivitiesResponse []ActivityDTOResponse

// GetActivities godoc
// @Summary     Получение истории событий задачи
// @Description Получение истории событий конкретной задачи из PostgreSQL с опциональной пагинацией
// @Description Сортировка выполняется по версии задачи в порядке возрастания.
// @Description author_user_id обозначает владельца задачи, а не обязательно пользователя, выполнившего изменение.
// @Description Для task.updated и task.completed поле payload содержит снимки before и after.
// @Tags        activity
// @Produce     json
// @Param       task_id path int true                         "ID задачи"
// @Param       limit query int false                         "Размер страницы с историей"
// @Param       offset query int false                        "Смещение страницы с историей"
// @Success     200 {object} GetActivitiesResponse             "История событий задачи"
// @Failure     400 {object} core_http_response.ErrorResponse "Bad request"
// @Failure     500 {object} core_http_response.ErrorResponse "Internal server error"
// @Router      /tasks/{task_id}/activity [get]
func (h *ActivityHTTPHandler) GetActivities(
	rw http.ResponseWriter,
	r *http.Request,
) {
	ctx := r.Context()
	log := core_logger.FromContext(ctx)
	responseHandler := core_http_response.NewHTTPResponseHandler(log, rw)

	taskID, err := core_http_request.GetIntPathValue(r, "task_id")
	if err != nil {
		responseHandler.ErrorResponse(
			err,
			"failed to get taskID path value",
		)

		return
	}

	limit, offset, err := getActivityLimitOffsetQueryParams(r)
	if err != nil {
		responseHandler.ErrorResponse(
			err,
			"failed to get limit/offset query param",
		)

		return
	}

	activities, err := h.activityService.GetActivities(
		ctx,
		taskID,
		limit,
		offset,
	)
	if err != nil {
		responseHandler.ErrorResponse(
			err,
			"failed to get task activities",
		)

		return
	}

	response := GetActivitiesResponse(
		activitiesDTOFromDomains(activities),
	)

	responseHandler.JSONResponse(
		response,
		http.StatusOK,
	)
}

func getActivityLimitOffsetQueryParams(
	r *http.Request,
) (*int, *int, error) {
	const (
		limitQueryParamKey  = "limit"
		offsetQueryParamKey = "offset"
	)

	limit, err := core_http_request.GetIntQueryParam(
		r,
		limitQueryParamKey,
	)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"get 'limit' query param: %w",
			err,
		)
	}

	offset, err := core_http_request.GetIntQueryParam(
		r,
		offsetQueryParamKey,
	)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"get 'offset' query param: %w",
			err,
		)
	}

	return limit, offset, nil
}
