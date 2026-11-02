package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

type routeCoverageUserResponse struct {
	ID       int    `json:"id"`
	FullName string `json:"full_name"`
}

type routeCoverageTaskResponse struct {
	ID        int    `json:"id"`
	Title     string `json:"title"`
	Completed bool   `json:"completed"`
}

type routeCoverageStatisticsResponse struct {
	TasksCreated   int `json:"tasks_created"`
	TasksCompleted int `json:"tasks_completed"`
}

func runRouteCoverage(
	ctx context.Context,
	client *apiClient,
	fixtures *fixtureSet,
) error {
	if len(fixtures.users) == 0 {
		return fmt.Errorf("route coverage requires at least one fixture user")
	}

	fmt.Println("Checking Users routes...")
	if err := coverUsersRoutes(ctx, client, fixtures); err != nil {
		return fmt.Errorf("users route coverage: %w", err)
	}

	fmt.Println("Checking Tasks routes...")
	if err := coverTasksRoutes(ctx, client, fixtures); err != nil {
		return fmt.Errorf("tasks route coverage: %w", err)
	}

	fmt.Println("Checking Statistics routes...")
	if err := coverStatisticsRoutes(ctx, client, fixtures); err != nil {
		return fmt.Errorf("statistics route coverage: %w", err)
	}

	return nil
}

func coverUsersRoutes(
	ctx context.Context,
	client *apiClient,
	fixtures *fixtureSet,
) error {
	// Create a disposable user specifically for route coverage.
	userNumber := len(fixtures.users) + 1

	user, err := fixtures.createUser(ctx, client, userNumber)
	if err != nil {
		return fmt.Errorf("create coverage user: %w", err)
	}

	// Register the user immediately so deferred cleanup can remove it.
	fixtures.users = append(fixtures.users, user)

	// GET /users
	listPath := usersPath + "?" + url.Values{
		"limit":  {"2"},
		"offset": {"0"},
	}.Encode()

	response, err := doRouteRequest(
		ctx,
		client,
		http.MethodGet,
		listPath,
		nil,
		http.StatusOK,
	)
	if err != nil {
		return err
	}

	var users []routeCoverageUserResponse
	if err := decodeJSON(response, &users); err != nil {
		return fmt.Errorf("decode users list: %w", err)
	}

	if len(users) == 0 {
		return fmt.Errorf("GET users returned an empty list")
	}

	// GET /users/{id}
	userPath := usersPath + "/" + strconv.Itoa(user.ID)

	response, err = doRouteRequest(
		ctx,
		client,
		http.MethodGet,
		userPath,
		nil,
		http.StatusOK,
	)
	if err != nil {
		return err
	}

	var receivedUser routeCoverageUserResponse
	if err := decodeJSON(response, &receivedUser); err != nil {
		return fmt.Errorf("decode user: %w", err)
	}

	if receivedUser.ID != user.ID {
		return fmt.Errorf(
			"GET user returned ID %d, want %d",
			receivedUser.ID,
			user.ID,
		)
	}

	// PATCH /users/{id}
	updatedName := fmt.Sprintf(
		"OBS-%s-UpdatedUser",
		fixtures.runID,
	)

	response, err = doRouteRequest(
		ctx,
		client,
		http.MethodPatch,
		userPath,
		map[string]string{
			"full_name": updatedName,
		},
		http.StatusOK,
	)
	if err != nil {
		return err
	}

	if err := decodeJSON(response, &receivedUser); err != nil {
		return fmt.Errorf("decode patched user: %w", err)
	}

	if receivedUser.ID != user.ID {
		return fmt.Errorf(
			"PATCH user returned ID %d, want %d",
			receivedUser.ID,
			user.ID,
		)
	}

	if receivedUser.FullName != updatedName {
		return fmt.Errorf(
			"PATCH user returned full_name %q, want %q",
			receivedUser.FullName,
			updatedName,
		)
	}

	// DELETE /users/{id}
	// This user has no tasks and does not affect the main fixtures.
	_, err = doRouteRequest(
		ctx,
		client,
		http.MethodDelete,
		userPath,
		nil,
		http.StatusNoContent,
	)
	if err != nil {
		return err
	}

	return nil
}

func coverTasksRoutes(
	ctx context.Context,
	client *apiClient,
	fixtures *fixtureSet,
) error {
	// Use the first regular fixture user as the task author.
	user := &fixtures.users[0]

	taskNumber := len(user.TaskIDs) + 1
	userNumber := len(fixtures.users)

	taskID, err := fixtures.createTask(
		ctx,
		client,
		user.ID,
		userNumber,
		taskNumber,
	)
	if err != nil {
		return fmt.Errorf("create coverage task: %w", err)
	}

	// Register the task immediately for deferred cleanup.
	user.TaskIDs = append(user.TaskIDs, taskID)

	// GET /tasks?user_id=...&limit=...&offset=...
	listPath := tasksPath + "?" + url.Values{
		"user_id": {strconv.Itoa(user.ID)},
		"limit":   {"2"},
		"offset":  {"0"},
	}.Encode()

	response, err := doRouteRequest(
		ctx,
		client,
		http.MethodGet,
		listPath,
		nil,
		http.StatusOK,
	)
	if err != nil {
		return err
	}

	var tasks []routeCoverageTaskResponse
	if err := decodeJSON(response, &tasks); err != nil {
		return fmt.Errorf("decode tasks list: %w", err)
	}

	if len(tasks) == 0 {
		return fmt.Errorf("GET tasks returned an empty list")
	}

	// GET /tasks/{id}
	taskPath := tasksPath + "/" + strconv.Itoa(taskID)

	response, err = doRouteRequest(
		ctx,
		client,
		http.MethodGet,
		taskPath,
		nil,
		http.StatusOK,
	)
	if err != nil {
		return err
	}

	var receivedTask routeCoverageTaskResponse
	if err := decodeJSON(response, &receivedTask); err != nil {
		return fmt.Errorf("decode task: %w", err)
	}

	if receivedTask.ID != taskID {
		return fmt.Errorf(
			"GET task returned ID %d, want %d",
			receivedTask.ID,
			taskID,
		)
	}

	// PATCH /tasks/{id}
	updatedTitle := fmt.Sprintf(
		"OBS-%s-UpdatedTask",
		fixtures.runID,
	)

	response, err = doRouteRequest(
		ctx,
		client,
		http.MethodPatch,
		taskPath,
		map[string]any{
			"title":     updatedTitle,
			"completed": true,
		},
		http.StatusOK,
	)
	if err != nil {
		return err
	}

	if err := decodeJSON(response, &receivedTask); err != nil {
		return fmt.Errorf("decode patched task: %w", err)
	}

	if receivedTask.ID != taskID {
		return fmt.Errorf(
			"PATCH task returned ID %d, want %d",
			receivedTask.ID,
			taskID,
		)
	}

	if receivedTask.Title != updatedTitle {
		return fmt.Errorf(
			"PATCH task returned title %q, want %q",
			receivedTask.Title,
			updatedTitle,
		)
	}

	if !receivedTask.Completed {
		return fmt.Errorf("PATCH task did not set completed=true")
	}

	// DELETE /tasks/{id}
	_, err = doRouteRequest(
		ctx,
		client,
		http.MethodDelete,
		taskPath,
		nil,
		http.StatusNoContent,
	)
	if err != nil {
		return err
	}

	return nil
}

func coverStatisticsRoutes(
	ctx context.Context,
	client *apiClient,
	fixtures *fixtureSet,
) error {
	// GET /statistics
	response, err := doRouteRequest(
		ctx,
		client,
		http.MethodGet,
		"/api/v1/statistics",
		nil,
		http.StatusOK,
	)
	if err != nil {
		return err
	}

	var globalStatistics routeCoverageStatisticsResponse
	if err := decodeJSON(response, &globalStatistics); err != nil {
		return fmt.Errorf("decode global statistics: %w", err)
	}

	// GET /statistics?user_id=...&from=...&to=...
	userID := fixtures.users[0].ID

	query := url.Values{
		"user_id": {strconv.Itoa(userID)},
		"from":    {"2000-01-01"},
		"to":      {"2099-12-31"},
	}

	response, err = doRouteRequest(
		ctx,
		client,
		http.MethodGet,
		"/api/v1/statistics?"+query.Encode(),
		nil,
		http.StatusOK,
	)
	if err != nil {
		return err
	}

	var userStatistics routeCoverageStatisticsResponse
	if err := decodeJSON(response, &userStatistics); err != nil {
		return fmt.Errorf("decode user statistics: %w", err)
	}

	return nil
}

func doRouteRequest(
	ctx context.Context,
	client *apiClient,
	method string,
	path string,
	body any,
	expectedStatus int,
) (apiResponse, error) {
	response, err := client.do(ctx, method, path, body)
	if err != nil {
		return apiResponse{}, fmt.Errorf(
			"%s %s request failed: %w",
			method,
			path,
			err,
		)
	}

	if err := expectStatus(
		response,
		method,
		path,
		expectedStatus,
	); err != nil {
		return apiResponse{}, err
	}

	return response, nil
}
