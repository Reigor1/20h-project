package e2e_api_test

import (
	"net/http"
	"net/url"
	"testing"
)

type statisticsResponse struct {
	TasksCreated               int      `json:"tasks_created"`
	TasksCompleted             int      `json:"tasks_completed"`
	TasksCompletedRate         *float64 `json:"tasks_completed_rate"`
	TasksAverageCompletionTime *string  `json:"tasks_average_completion_time"`
}

func testStatisticsAPI(t *testing.T, client *http.Client, baseURL string) {
	api := apiClient{httpClient: client, baseURL: baseURL}
	const statisticsPath = "/api/v1/statistics"

	t.Run("GetStatistics", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			newGetStatisticsCase := newJSONSuccessCase[statisticsResponse, struct{}](
				http.MethodGet,
				http.StatusOK,
				nil,
			)
			query := url.Values{}
			runExpectedSuccesses(t, api, []expectedSuccessCase{
				newGetStatisticsCase("returns statistics without filters", statisticsPath+"?"+query.Encode(), nil),
			})
		})

		t.Run("ExpectedErrors", func(t *testing.T) {
			runExpectedErrors(t, api, []expectedErrorCase{
				{
					name:   "rejects noninteger user ID",
					method: http.MethodGet,
					path:   statisticsPath + "?user_id=abc",
					status: http.StatusBadRequest,
				},
			})
		})
	})
}
