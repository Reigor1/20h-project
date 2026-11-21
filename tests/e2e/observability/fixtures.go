package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

const (
	usersPath = "/api/v1/users"
	tasksPath = "/api/v1/tasks"
)

// fixtureSet stores the data required by the corresponding test scenario.
type fixtureSet struct {
	runID string
	users []userFixture
}

// userFixture stores the data required by the corresponding test scenario.
type userFixture struct {
	ID      int
	TaskIDs []int
}

// userResponse stores the data required by the corresponding test scenario.
type userResponse struct {
	ID int `json:"id"`
}

// taskResponse stores the data required by the corresponding test scenario.
type taskResponse struct {
	ID int `json:"id"`
}

// createUserRequest stores the data required by the corresponding test scenario.
type createUserRequest struct {
	FullName string `json:"full_name"`
}

// createTaskRequest stores the data required by the corresponding test scenario.
type createTaskRequest struct {
	Title        string `json:"title"`
	AuthorUserID int    `json:"author_user_id"`
}

// newFixtureSet constructs the test client or fixture required by the scenario.
func newFixtureSet() *fixtureSet {
	return &fixtureSet{
		runID: time.Now().UTC().Format("20060102T150405.000000000"),
	}
}

// create supports setup, execution, or verification of the corresponding test scenario.
func (f *fixtureSet) create(
	ctx context.Context,
	client *apiClient,
	config Config,
) error {
	for i := 1; i <= config.UsersCount; i++ {
		user, err := f.createUser(ctx, client, i)
		if err != nil {
			return err
		}

		f.users = append(f.users, user)
	}

	for i := range f.users {
		for j := 1; j <= config.TasksPerUser; j++ {
			taskID, err := f.createTask(
				ctx,
				client,
				f.users[i].ID,
				i+1,
				j,
			)
			if err != nil {
				return err
			}

			f.users[i].TaskIDs = append(
				f.users[i].TaskIDs,
				taskID,
			)
		}
	}

	return nil
}

// createUser supports setup, execution, or verification of the corresponding test scenario.
func (f *fixtureSet) createUser(
	ctx context.Context,
	client *apiClient,
	number int,
) (userFixture, error) {
	request := createUserRequest{
		FullName: fmt.Sprintf(
			"OBS-%s-User-%02d",
			f.runID,
			number,
		),
	}

	response, err := client.do(
		ctx,
		http.MethodPost,
		usersPath,
		request,
	)
	if err != nil {
		return userFixture{}, fmt.Errorf("create user %d: %w", number, err)
	}

	if err := expectStatus(
		response,
		http.MethodPost,
		usersPath,
		http.StatusCreated,
	); err != nil {
		return userFixture{}, fmt.Errorf("create user %d: %w", number, err)
	}

	var created userResponse
	if err := decodeJSON(response, &created); err != nil {
		return userFixture{}, fmt.Errorf("decode created user: %w", err)
	}

	if created.ID <= 0 {
		return userFixture{}, fmt.Errorf(
			"created user has invalid ID: %d",
			created.ID,
		)
	}

	return userFixture{
		ID:      created.ID,
		TaskIDs: make([]int, 0),
	}, nil
}

// createTask supports setup, execution, or verification of the corresponding test scenario.
func (f *fixtureSet) createTask(
	ctx context.Context,
	client *apiClient,
	userID int,
	userNumber int,
	taskNumber int,
) (int, error) {
	request := createTaskRequest{
		Title: fmt.Sprintf(
			"OBS-%s-Task-%02d-%02d",
			f.runID,
			userNumber,
			taskNumber,
		),
		AuthorUserID: userID,
	}

	response, err := client.do(
		ctx,
		http.MethodPost,
		tasksPath,
		request,
	)
	if err != nil {
		return 0, fmt.Errorf(
			"create task for user %d: %w",
			userID,
			err,
		)
	}

	if err := expectStatus(
		response,
		http.MethodPost,
		tasksPath,
		http.StatusCreated,
	); err != nil {
		return 0, fmt.Errorf("create task for user %d: %w", userID, err)
	}

	var created taskResponse
	if err := decodeJSON(response, &created); err != nil {
		return 0, fmt.Errorf("decode created task: %w", err)
	}

	if created.ID <= 0 {
		return 0, fmt.Errorf(
			"created task has invalid ID: %d",
			created.ID,
		)
	}

	return created.ID, nil
}

// cleanup supports setup, execution, or verification of the corresponding test scenario.
func (f *fixtureSet) cleanup(
	ctx context.Context,
	client *apiClient,
) error {
	var cleanupErrors []error

	// Delete tasks before users because tasks reference their authors.
	for i := len(f.users) - 1; i >= 0; i-- {
		user := f.users[i]

		for j := len(user.TaskIDs) - 1; j >= 0; j-- {
			taskID := user.TaskIDs[j]
			path := tasksPath + "/" + strconv.Itoa(taskID)

			if err := deleteFixture(ctx, client, path); err != nil {
				cleanupErrors = append(cleanupErrors, err)
			}
		}
	}

	for i := len(f.users) - 1; i >= 0; i-- {
		userID := f.users[i].ID
		path := usersPath + "/" + strconv.Itoa(userID)

		if err := deleteFixture(ctx, client, path); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	}

	return errors.Join(cleanupErrors...)
}

// deleteFixture removes test data or schema created by the integration scenario.
func deleteFixture(
	ctx context.Context,
	client *apiClient,
	path string,
) error {
	response, err := client.do(
		ctx,
		http.MethodDelete,
		path,
		nil,
	)
	if err != nil {
		return fmt.Errorf("delete fixture %s: %w", path, err)
	}

	// 404 is acceptable: cleanup is idempotent.
	if response.StatusCode == http.StatusNoContent ||
		response.StatusCode == http.StatusNotFound {
		return nil
	}

	return fmt.Errorf(
		"delete fixture %s: unexpected status %d; body: %s",
		path,
		response.StatusCode,
		response.Body,
	)
}

// taskCount supports setup, execution, or verification of the corresponding test scenario.
func (f *fixtureSet) taskCount() int {
	count := 0

	for _, user := range f.users {
		count += len(user.TaskIDs)
	}

	return count
}
