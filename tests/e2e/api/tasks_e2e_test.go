package e2e_api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

type taskRequest struct {
	Title        string  `json:"title"`
	Description  *string `json:"description,omitempty"`
	AuthorUserID int     `json:"author_user_id"`
}

type taskResponse struct {
	ID           int     `json:"id"`
	Title        string  `json:"title"`
	Description  *string `json:"description"`
	Completed    bool    `json:"completed"`
	AuthorUserID int     `json:"author_user_id"`
}

func testTasksAPI(t *testing.T, client *http.Client, baseURL string) {
	api := apiClient{
		httpClient: client,
		baseURL:    baseURL,
	}
	const tasksPath = "/api/v1/tasks"

	createTask := func(t *testing.T) taskResponse {
		t.Helper()

		author := createUser(t, &api, "E2E Task Author", nil)

		result, err := api.request(http.MethodPost, tasksPath, taskRequest{
			Title:        "E2E task",
			AuthorUserID: author.ID,
		})
		if err != nil {
			t.Fatalf("create task: %v", err)
		}

		assertStatus(t, result, http.MethodPost, tasksPath, http.StatusCreated)

		var task taskResponse
		if err := json.Unmarshal(result.Body, &task); err != nil {
			t.Fatalf("decode created task: %v; body: %s", err, result.Body)
		}

		if task.ID <= 0 {
			t.Fatalf("created task ID = %d, want positive ID", task.ID)
		}

		registerTaskCleanup(t, api, tasksPath, task.ID)

		return task
	}

	t.Run("CreateTask", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			author := createUser(t, &api, "E2E Task Author", nil)

			newCreateTaskCase := newJSONSuccessCase[taskResponse, taskRequest](
				http.MethodPost,
				http.StatusCreated,
				func(t *testing.T, got taskResponse, input *taskRequest) {
					if input == nil {
						t.Fatal("task request must not be nil")
					}

					if got.ID <= 0 {
						t.Fatalf("created task ID = %d, want positive ID", got.ID)
					}

					registerTaskCleanup(t, api, tasksPath, got.ID)

					if got.Title != input.Title ||
						got.AuthorUserID != input.AuthorUserID ||
						!reflect.DeepEqual(got.Description, input.Description) {
						t.Fatalf("created task = %+v, want %+v", got, input)
					}
				},
			)

			runExpectedSuccesses(t, api, []expectedSuccessCase{
				newCreateTaskCase(
					"creates task without description",
					tasksPath,
					&taskRequest{
						Title:        "E2E task",
						AuthorUserID: author.ID,
					},
				),
				newCreateTaskCase(
					"accepts minimum title length",
					tasksPath,
					&taskRequest{
						Title:        "T",
						AuthorUserID: author.ID,
					},
				),
				newCreateTaskCase(
					"accepts maximum title length",
					tasksPath,
					&taskRequest{
						Title:        strings.Repeat("T", 100),
						AuthorUserID: author.ID,
					},
				),
				newCreateTaskCase(
					"accepts minimum description length",
					tasksPath,
					&taskRequest{
						Title:        "E2E task",
						Description:  stringPointer("D"),
						AuthorUserID: author.ID,
					},
				),
				newCreateTaskCase(
					"accepts maximum description length",
					tasksPath,
					&taskRequest{
						Title:        "E2E task",
						Description:  stringPointer(strings.Repeat("D", 1000)),
						AuthorUserID: author.ID,
					},
				),
			})
		})

		t.Run("ExpectedErrors", func(t *testing.T) {
			author := createUser(t, &api, "E2E Task Author", nil)

			emptyDescription := ""
			tooLongDescription := strings.Repeat("D", 1001)

			runExpectedErrors(t, api, []expectedErrorCase{
				{
					name:   "rejects empty title",
					method: http.MethodPost,
					path:   tasksPath,
					body: taskRequest{
						AuthorUserID: author.ID,
					},
					status: http.StatusBadRequest,
				},
				{
					name:   "rejects title longer than maximum",
					method: http.MethodPost,
					path:   tasksPath,
					body: taskRequest{
						Title:        strings.Repeat("T", 101),
						AuthorUserID: author.ID,
					},
					status: http.StatusBadRequest,
				},
				{
					name:   "rejects empty description",
					method: http.MethodPost,
					path:   tasksPath,
					body: taskRequest{
						Title:        "E2E task",
						Description:  &emptyDescription,
						AuthorUserID: author.ID,
					},
					status: http.StatusBadRequest,
				},
				{
					name:   "rejects description longer than maximum",
					method: http.MethodPost,
					path:   tasksPath,
					body: taskRequest{
						Title:        "E2E task",
						Description:  &tooLongDescription,
						AuthorUserID: author.ID,
					},
					status: http.StatusBadRequest,
				},
				{
					name:   "rejects missing author ID",
					method: http.MethodPost,
					path:   tasksPath,
					body: taskRequest{
						Title: "E2E task",
					},
					status: http.StatusBadRequest,
				},
				{
					name:   "returns not found for missing author",
					method: http.MethodPost,
					path:   tasksPath,
					body: taskRequest{
						Title:        "E2E task",
						AuthorUserID: 2147483647,
					},
					status: http.StatusNotFound,
				},
			})
		})
	})

	t.Run("GetTasks", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			created := createTask(t)

			query := url.Values{
				"limit":  {"10"},
				"offset": {"0"},
			}
			path := tasksPath + "?" + query.Encode()

			runExpectedSuccesses(t, api, []expectedSuccessCase{
				{
					name:   "lists tasks with pagination",
					method: http.MethodGet,
					path:   path,
					status: http.StatusOK,
					check: func(t *testing.T, result apiResult) {
						t.Helper()

						var got []taskResponse
						if err := json.Unmarshal(result.Body, &got); err != nil {
							t.Fatalf("decode tasks: %v; body: %s", err, result.Body)
						}

						for _, task := range got {
							if task.ID == created.ID {
								return
							}
						}

						t.Fatalf("task %d missing from list: %+v", created.ID, got)
					},
				},
			})
		})

		t.Run("ExpectedErrors", func(t *testing.T) {
			runExpectedErrors(t, api, []expectedErrorCase{
				{
					name:   "rejects noninteger limit",
					method: http.MethodGet,
					path:   tasksPath + "?limit=abc",
					status: http.StatusBadRequest,
				},
			})
		})
	})

	t.Run("GetTask", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			created := createTask(t)
			path := fmt.Sprintf("%s/%d", tasksPath, created.ID)

			runExpectedSuccesses(t, api, []expectedSuccessCase{
				{
					name:   "retrieves task by ID",
					method: http.MethodGet,
					path:   path,
					status: http.StatusOK,
					check: func(t *testing.T, result apiResult) {
						t.Helper()

						var got taskResponse
						if err := json.Unmarshal(result.Body, &got); err != nil {
							t.Fatalf("decode task: %v; body: %s", err, result.Body)
						}

						if got.ID != created.ID || got.Title != created.Title {
							t.Fatalf("task = %+v, want %+v", got, created)
						}
					},
				},
			})
		})

		t.Run("ExpectedErrors", func(t *testing.T) {
			runExpectedErrors(t, api, []expectedErrorCase{
				{
					name:   "returns not found for missing task",
					method: http.MethodGet,
					path:   tasksPath + "/2147483647",
					status: http.StatusNotFound,
				},
			})
		})
	})

	t.Run("DeleteTask", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			task := createTask(t)
			path := fmt.Sprintf("%s/%d", tasksPath, task.ID)

			runExpectedSuccesses(t, api, []expectedSuccessCase{
				{
					name:   "deletes existing task",
					method: http.MethodDelete,
					path:   path,
					status: http.StatusNoContent,
				},
			})
		})

		t.Run("ExpectedErrors", func(t *testing.T) {
			runExpectedErrors(t, api, []expectedErrorCase{
				{
					name:   "returns not found for missing task",
					method: http.MethodDelete,
					path:   tasksPath + "/2147483647",
					status: http.StatusNotFound,
				},
			})
		})
	})

	t.Run("PatchTask", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			created := createTask(t)
			path := fmt.Sprintf("%s/%d", tasksPath, created.ID)

			runExpectedSuccesses(t, api, []expectedSuccessCase{
				{
					name:   "updates task title",
					method: http.MethodPatch,
					path:   path,
					body: map[string]string{
						"title": "E2E updated",
					},
					status: http.StatusOK,
					check: func(t *testing.T, result apiResult) {
						t.Helper()

						var got taskResponse
						if err := json.Unmarshal(result.Body, &got); err != nil {
							t.Fatalf("decode task: %v; body: %s", err, result.Body)
						}

						if got.ID != created.ID || got.Title != "E2E updated" {
							t.Fatalf("patched task = %+v", got)
						}
					},
				},
			})
		})

		t.Run("ExpectedErrors", func(t *testing.T) {
			runExpectedErrors(t, api, []expectedErrorCase{
				{
					name:   "rejects empty title",
					method: http.MethodPatch,
					path:   tasksPath + "/2147483647",
					body: map[string]string{
						"title": "",
					},
					status: http.StatusBadRequest,
				},
			})
		})
	})
}

func registerTaskCleanup(t *testing.T, api apiClient, tasksPath string, taskID int) {
	t.Helper()

	t.Cleanup(func() {
		path := fmt.Sprintf("%s/%d", tasksPath, taskID)

		result, err := api.request(http.MethodDelete, path, nil)
		if err != nil {
			t.Errorf("cleanup DELETE %s: %v", path, err)
			return
		}

		if result.StatusCode != http.StatusNoContent &&
			result.StatusCode != http.StatusNotFound {
			t.Errorf(
				"cleanup DELETE %s status = %d, want %d or %d",
				path,
				result.StatusCode,
				http.StatusNoContent,
				http.StatusNotFound,
			)
		}
	})
}
