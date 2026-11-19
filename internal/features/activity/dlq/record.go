package activity_dlq

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrInvalidRecord = errors.New("invalid DLQ record")

type Record struct {
	EventID         string    `json:"event_id,omitempty"`
	OriginalPayload []byte    `json:"original_payload"`
	OriginalKey     []byte    `json:"original_key"`
	SourceTopic     string    `json:"source_topic"`
	SourcePartition int32     `json:"source_partition"`
	SourceOffset    int64     `json:"source_offset"`
	FailedAt        time.Time `json:"failed_at"`
	Error           string    `json:"error"`
	AttemptCount    int       `json:"attempt_count"`
}

func (r Record) Validate() error {
	if strings.TrimSpace(r.SourceTopic) == "" {
		return fmt.Errorf(
			"%w: source_topic is required",
			ErrInvalidRecord,
		)
	}

	if r.SourcePartition < 0 {
		return fmt.Errorf(
			"%w: source_partition must not be negative",
			ErrInvalidRecord,
		)
	}

	if r.SourceOffset < 0 {
		return fmt.Errorf(
			"%w: source_offset must not be negative",
			ErrInvalidRecord,
		)
	}

	if r.FailedAt.IsZero() {
		return fmt.Errorf(
			"%w: failed_at is required",
			ErrInvalidRecord,
		)
	}

	if strings.TrimSpace(r.Error) == "" {
		return fmt.Errorf(
			"%w: error description is required",
			ErrInvalidRecord,
		)
	}

	if r.AttemptCount < 1 {
		return fmt.Errorf(
			"%w: attempt_count must be positive",
			ErrInvalidRecord,
		)
	}

	return nil
}

func Marshal(record Record) ([]byte, error) {
	if err := record.Validate(); err != nil {
		return nil, err
	}

	data, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("marshal DLQ record: %w", err)
	}

	return data, nil
}

func Unmarshal(data []byte) (Record, error) {
	var record Record

	if err := json.Unmarshal(data, &record); err != nil {
		return Record{}, fmt.Errorf(
			"unmarshal DLQ record: %w",
			err,
		)
	}

	if err := record.Validate(); err != nil {
		return Record{}, err
	}

	return record, nil
}
