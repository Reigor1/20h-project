package core_events

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type EventType string

const (
	EventTypeTaskCreated   EventType = "task.created"
	EventTypeTaskUpdated   EventType = "task.updated"
	EventTypeTaskCompleted EventType = "task.completed"
	EventTypeTaskDeleted   EventType = "task.deleted"
)

const (
	AggregateTypeTask = "task"

	CurrentSchemaVersion = 1
)

var (
	ErrInvalidEnvelope          = errors.New("invalid event envelope")
	ErrUnsupportedSchemaVersion = errors.New("unsupported event schema version")
)

type Envelope struct {
	EventID          string          `json:"event_id"`
	EventType        EventType       `json:"event_type"`
	SchemaVersion    int             `json:"schema_version"`
	OccurredAt       time.Time       `json:"occurred_at"`
	AggregateType    string          `json:"aggregate_type"`
	AggregateID      int64           `json:"aggregate_id"`
	AggregateVersion int64           `json:"aggregate_version"`
	Payload          json.RawMessage `json:"payload"`
}

func (e Envelope) Validate() error {
	if _, err := uuid.Parse(e.EventID); err != nil {
		return fmt.Errorf(
			"%w: invalid event_id: %v",
			ErrInvalidEnvelope,
			err,
		)
	}

	switch e.EventType {
	case EventTypeTaskCreated,
		EventTypeTaskUpdated,
		EventTypeTaskCompleted,
		EventTypeTaskDeleted:
	default:
		return fmt.Errorf(
			"%w: unsupported event_type %q",
			ErrInvalidEnvelope,
			e.EventType,
		)
	}

	if e.SchemaVersion < 1 {
		return fmt.Errorf(
			"%w: schema_version must be positive",
			ErrInvalidEnvelope,
		)
	}

	if e.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf(
			"%w: got %d, supported %d",
			ErrUnsupportedSchemaVersion,
			e.SchemaVersion,
			CurrentSchemaVersion,
		)
	}

	if e.OccurredAt.IsZero() {
		return fmt.Errorf(
			"%w: occurred_at is required",
			ErrInvalidEnvelope,
		)
	}

	if e.AggregateType != AggregateTypeTask {
		return fmt.Errorf(
			"%w: unsupported aggregate_type %q",
			ErrInvalidEnvelope,
			e.AggregateType,
		)
	}

	if e.AggregateID <= 0 {
		return fmt.Errorf(
			"%w: aggregate_id must be positive",
			ErrInvalidEnvelope,
		)
	}

	if e.AggregateVersion <= 0 {
		return fmt.Errorf(
			"%w: aggregate_version must be positive",
			ErrInvalidEnvelope,
		)
	}

	var payload map[string]json.RawMessage

	if len(e.Payload) == 0 {
		return fmt.Errorf(
			"%w: payload is required",
			ErrInvalidEnvelope,
		)
	}

	if err := json.Unmarshal(e.Payload, &payload); err != nil || payload == nil {
		return fmt.Errorf(
			"%w: payload must be a JSON object",
			ErrInvalidEnvelope,
		)
	}

	return nil
}

func Marshal(envelope Envelope) ([]byte, error) {
	if err := envelope.Validate(); err != nil {
		return nil, fmt.Errorf("validate event envelope: %w", err)
	}

	data, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("marshal event envelope: %w", err)
	}

	return data, nil
}

func Unmarshal(data []byte) (Envelope, error) {
	var envelope Envelope

	if err := json.Unmarshal(data, &envelope); err != nil {
		return Envelope{}, fmt.Errorf("unmarshal event envelope: %w", err)
	}

	if err := envelope.Validate(); err != nil {
		return Envelope{}, fmt.Errorf("validate event envelope: %w", err)
	}

	return envelope, nil
}
