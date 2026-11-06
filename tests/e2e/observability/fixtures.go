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

type fixtureSet struct {
	runID string
	users []userFixture
}

type userFixture struct {
	ID      int
	TaskIDs []int
}

type userResponse struct {
	ID int `json:"id"`
}

type taskResponse struct {
	ID int `json:"id"`
}

type createUserRequest struct {
	FullName string `json:"full_name"`
}

type createTaskRequest struct {
	Title        string `json:"title"`
	AuthorUserID int    `json:"author_user_id"`
}

func newFixtureSet() *fixtureSet {
	return &fixtureSet{
		runID: time.Now().UTC().Format("20060102T150405.000000000"),
	}
}

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

func (f *fixtureSet) taskCount() int {
	count := 0

	for _, user := range f.users {
		count += len(user.TaskIDs)
	}

	return count
}
