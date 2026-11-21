package core_events

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// EventType identifies the business operation represented by an event.
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
	ErrMalformedJSON            = errors.New("malformed event JSON")
	ErrUnsupportedEventType     = errors.New("unsupported event type")
	ErrUnsupportedSchemaVersion = errors.New("unsupported schema version")
)

// Envelope is the versioned event contract stored in the Outbox and published to Kafka.
// Payload contains the event-specific JSON object while the remaining fields describe its aggregate.
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

// Validate checks the envelope identity, supported event/schema, aggregate metadata,
// timestamp, and the presence of a JSON-object payload.
func (e Envelope) Validate() error {
	eventID, err := uuid.Parse(e.EventID)
	if err != nil || eventID == uuid.Nil {
		return fmt.Errorf(
			"%w: event_id must be a non-zero UUID",
			ErrInvalidEnvelope,
		)
	}

	switch e.EventType {
	case EventTypeTaskCreated,
		EventTypeTaskUpdated,
		EventTypeTaskCompleted,
		EventTypeTaskDeleted:
	default:
		if e.EventType == "" {
			return fmt.Errorf(
				"%w: event_type is required",
				ErrInvalidEnvelope,
			)
		}

		return fmt.Errorf(
			"%w: %q",
			ErrUnsupportedEventType,
			e.EventType,
		)
	}

	if e.SchemaVersion <= 0 {
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

	if len(e.Payload) == 0 {
		return fmt.Errorf(
			"%w: payload is required",
			ErrInvalidEnvelope,
		)
	}

	var payload map[string]json.RawMessage

	if err := json.Unmarshal(e.Payload, &payload); err != nil || payload == nil {
		return fmt.Errorf(
			"%w: payload must be a JSON object",
			ErrInvalidEnvelope,
		)
	}

	return nil
}

// Marshal validates an event envelope before encoding it as JSON.
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

// Unmarshal decodes JSON into an envelope and validates the resulting event contract.
// Malformed JSON and invalid envelope metadata are returned as distinguishable errors.
func Unmarshal(data []byte) (Envelope, error) {
	var envelope Envelope

	if err := json.Unmarshal(data, &envelope); err != nil {
		return Envelope{}, fmt.Errorf(
			"%w: %w",
			ErrMalformedJSON,
			err,
		)
	}

	if err := envelope.Validate(); err != nil {
		return Envelope{}, fmt.Errorf(
			"validate event envelope: %w",
			err,
		)
	}

	return envelope, nil
}
