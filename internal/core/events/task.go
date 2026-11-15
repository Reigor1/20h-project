package core_events

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type TaskSnapshot struct {
	ID           int64      `json:"id"`
	Version      int64      `json:"version"`
	Title        string     `json:"title"`
	Description  *string    `json:"description"`
	Completed    bool       `json:"completed"`
	CreatedAt    time.Time  `json:"created_at"`
	CompletedAt  *time.Time `json:"completed_at"`
	AuthorUserID int64      `json:"author_user_id"`
}

type TaskCreatedPayload struct {
	Task TaskSnapshot `json:"task"`
}

type TaskUpdatedPayload struct {
	Before TaskSnapshot `json:"before"`
	After  TaskSnapshot `json:"after"`
}

type TaskDeletedPayload struct {
	Task TaskSnapshot `json:"task"`
}

func NewTaskCreated(task TaskSnapshot) (Envelope, error) {
	return newTaskEnvelope(
		EventTypeTaskCreated,
		task,
		TaskCreatedPayload{
			Task: task,
		},
	)
}

func NewTaskUpdated(
	before TaskSnapshot,
	after TaskSnapshot,
) (Envelope, error) {
	if before.ID != after.ID {
		return Envelope{}, fmt.Errorf(
			"%w: task IDs do not match",
			ErrInvalidEnvelope,
		)
	}

	if after.Version != before.Version+1 {
		return Envelope{}, fmt.Errorf(
			"%w: invalid task version transition %d -> %d",
			ErrInvalidEnvelope,
			before.Version,
			after.Version,
		)
	}

	eventType := EventTypeTaskUpdated

	if !before.Completed && after.Completed {
		eventType = EventTypeTaskCompleted
	}

	return newTaskEnvelope(
		eventType,
		after,
		TaskUpdatedPayload{
			Before: before,
			After:  after,
		},
	)
}

func NewTaskDeleted(task TaskSnapshot) (Envelope, error) {
	return newTaskEnvelope(
		EventTypeTaskDeleted,
		task,
		TaskDeletedPayload{
			Task: task,
		},
	)
}

func newTaskEnvelope(
	eventType EventType,
	task TaskSnapshot,
	payload any,
) (Envelope, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, fmt.Errorf("marshal task event payload: %w", err)
	}

	envelope := Envelope{
		EventID:          uuid.NewString(),
		EventType:        eventType,
		SchemaVersion:    CurrentSchemaVersion,
		OccurredAt:       time.Now().UTC(),
		AggregateType:    AggregateTypeTask,
		AggregateID:      task.ID,
		AggregateVersion: task.Version,
		Payload:          payloadJSON,
	}

	if err := envelope.Validate(); err != nil {
		return Envelope{}, fmt.Errorf("validate task event: %w", err)
	}

	return envelope, nil
}
