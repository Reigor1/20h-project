package e2e_api_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"testing"
)

// statisticsResponse models the aggregate statistics fields returned by the API.
type statisticsResponse struct {
	TasksCreated               int      `json:"tasks_created"`
	TasksCompleted             int      `json:"tasks_completed"`
	TasksCompletedRate         *float64 `json:"tasks_completed_rate"`
	TasksAverageCompletionTime *string  `json:"tasks_average_completion_time"`
}

// testStatisticsAPI verifies statistics filters, aggregation values, and invalid query handling over HTTP.
func testStatisticsAPI(t *testing.T, client *http.Client, baseURL string) {
	api := apiClient{httpClient: client, baseURL: baseURL}

	const statisticsPath = "/api/v1/statistics"
	const tasksPath = "/api/v1/tasks"

	newGetStatisticsCase := func(
		name string,
		path string,
		validate func(t *testing.T, got statisticsResponse),
	) expectedSuccessCase {
		return newJSONSuccessCase[statisticsResponse, struct{}](
			http.MethodGet,
			http.StatusOK,
			func(t *testing.T, got statisticsResponse, _ *struct{}) {
				validate(t, got)
			},
		)(name, path, nil)
	}

	t.Run("GetStatistics", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			t.Run("EmptyStatistics", func(t *testing.T) {
				runExpectedSuccesses(t, api, []expectedSuccessCase{
					newGetStatisticsCase(
						"returns empty statistics without data",
						statisticsPath,
						func(t *testing.T, got statisticsResponse) {
							if got.TasksCreated != 0 || got.TasksCompleted != 0 {
								t.Fatalf(
									"task counts = created %d, completed %d; want both zero",
									got.TasksCreated,
									got.TasksCompleted,
								)
							}

							if got.TasksCompletedRate != nil {
								t.Fatalf("tasks completed rate = %v; want nil", *got.TasksCompletedRate)
							}

							if got.TasksAverageCompletionTime != nil {
								t.Fatalf("average completion time = %q; want nil", *got.TasksAverageCompletionTime)
							}
						},
					),
				})
			})

			t.Run("StatisticsWithTasks", func(t *testing.T) {
				user := createUser(t, &api, "E2E Statistics", nil)

				createTask := func(title string) taskResponse {
					t.Helper()

					result, err := api.request(
						http.MethodPost,
						tasksPath,
						taskRequest{
							Title:        title,
							AuthorUserID: user.ID,
						},
					)
					if err != nil {
						t.Fatalf("create task: %v", err)
					}

					assertStatus(t, result, http.MethodPost, tasksPath, http.StatusCreated)

					var task taskResponse
					if err := json.Unmarshal(result.Body, &task); err != nil {
						t.Fatalf("decode created task: %v; body: %s", err, result.Body)
					}

					taskPath := tasksPath + "/" + strconv.Itoa(task.ID)

					t.Cleanup(func() {
						result, err := api.request(http.MethodDelete, taskPath, nil)
						if err != nil {
							t.Errorf("delete task %d: %v", task.ID, err)
							return
						}

						if result.StatusCode != http.StatusNoContent &&
							result.StatusCode != http.StatusNotFound {
							t.Errorf(
								"delete task %d: status = %d; want %d or %d",
								task.ID,
								result.StatusCode,
								http.StatusNoContent,
								http.StatusNotFound,
							)
						}
					})

					return task
				}

				completedTask := createTask("E2E Completed Statistics Task")
				createTask("E2E Incomplete Statistics Task")

				completedTaskPath := tasksPath + "/" + strconv.Itoa(completedTask.ID)

				result, err := api.request(
					http.MethodPatch,
					completedTaskPath,
					map[string]bool{"completed": true},
				)
				if err != nil {
					t.Fatalf("complete task: %v", err)
				}

				assertStatus(t, result, http.MethodPatch, completedTaskPath, http.StatusOK)

				query := url.Values{
					"user_id": {strconv.Itoa(user.ID)},
				}

				runExpectedSuccesses(t, api, []expectedSuccessCase{
					newGetStatisticsCase(
						"returns statistics for user with tasks",
						statisticsPath+"?"+query.Encode(),
						func(t *testing.T, got statisticsResponse) {
							if got.TasksCreated != 2 {
								t.Fatalf("tasks created = %d; want 2", got.TasksCreated)
							}

							if got.TasksCompleted != 1 {
								t.Fatalf("tasks completed = %d; want 1", got.TasksCompleted)
							}

							if got.TasksCompletedRate == nil {
								t.Fatal("tasks completed rate = nil; want a value")
							}

							if got.TasksAverageCompletionTime == nil {
								t.Fatal("average completion time = nil; want a value")
							}
						},
					),
				})
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
				{
					name:   "rejects decimal user ID",
					method: http.MethodGet,
					path:   statisticsPath + "?user_id=1.5",
					status: http.StatusBadRequest,
				},
				{
					name:   "rejects overflowing user ID",
					method: http.MethodGet,
					path:   statisticsPath + "?user_id=999999999999999999999999",
					status: http.StatusBadRequest,
				},
				{
					name:   "rejects invalid from date format",
					method: http.MethodGet,
					path:   statisticsPath + "?from=2006/01/02",
					status: http.StatusBadRequest,
				},
				{
					name:   "rejects invalid from date value",
					method: http.MethodGet,
					path:   statisticsPath + "?from=2006-02-30",
					status: http.StatusBadRequest,
				},
				{
					name:   "rejects invalid to date format",
					method: http.MethodGet,
					path:   statisticsPath + "?to=2006/01/02",
					status: http.StatusBadRequest,
				},
				{
					name:   "rejects invalid to date value",
					method: http.MethodGet,
					path:   statisticsPath + "?to=2006-02-30",
					status: http.StatusBadRequest,
				},
			})
		})
	})
}
