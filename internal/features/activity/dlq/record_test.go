package activity_dlq

import (
	"bytes"
	"testing"
	"time"
)

func TestMarshalAndUnmarshalPreserveOriginalMessage(t *testing.T) {
	want := Record{
		EventID:         "123e4567-e89b-12d3-a456-426614174000",
		OriginalPayload: []byte{0x00, 0x01, 0x02, 0xff},
		OriginalKey:     []byte{0x10, 0x20, 0xff},
		SourceTopic:     "task-events",
		SourcePartition: 2,
		SourceOffset:    125,
		FailedAt:        time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC),
		Error:           "unsupported schema version",
		AttemptCount:    1,
	}

	data, err := Marshal(want)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	got, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.EventID != want.EventID {
		t.Errorf("EventID = %q, want %q", got.EventID, want.EventID)
	}

	if !bytes.Equal(got.OriginalPayload, want.OriginalPayload) {
		t.Errorf("OriginalPayload = %v, want %v", got.OriginalPayload, want.OriginalPayload)
	}

	if !bytes.Equal(got.OriginalKey, want.OriginalKey) {
		t.Errorf("OriginalKey = %v, want %v", got.OriginalKey, want.OriginalKey)
	}

	if got.SourceTopic != want.SourceTopic {
		t.Errorf("SourceTopic = %q, want %q", got.SourceTopic, want.SourceTopic)
	}

	if got.SourcePartition != want.SourcePartition {
		t.Errorf("SourcePartition = %d, want %d", got.SourcePartition, want.SourcePartition)
	}

	if got.SourceOffset != want.SourceOffset {
		t.Errorf("SourceOffset = %d, want %d", got.SourceOffset, want.SourceOffset)
	}

	if !got.FailedAt.Equal(want.FailedAt) {
		t.Errorf("FailedAt = %v, want %v", got.FailedAt, want.FailedAt)
	}

	if got.Error != want.Error {
		t.Errorf("Error = %q, want %q", got.Error, want.Error)
	}

	if got.AttemptCount != want.AttemptCount {
		t.Errorf("AttemptCount = %d, want %d", got.AttemptCount, want.AttemptCount)
	}
}

func TestRecordValidate(t *testing.T) {
	validRecord := Record{
		OriginalPayload: []byte(`{}`),
		SourceTopic:     "task-events",
		SourcePartition: 0,
		SourceOffset:    0,
		FailedAt:        time.Now().UTC(),
		Error:           "invalid event",
		AttemptCount:    1,
	}

	tests := []struct {
		name   string
		modify func(*Record)
	}{
		{
			name: "missing source topic",
			modify: func(record *Record) {
				record.SourceTopic = ""
			},
		},
		{
			name: "negative partition",
			modify: func(record *Record) {
				record.SourcePartition = -1
			},
		},
		{
			name: "negative offset",
			modify: func(record *Record) {
				record.SourceOffset = -1
			},
		},
		{
			name: "missing failed at",
			modify: func(record *Record) {
				record.FailedAt = time.Time{}
			},
		},
		{
			name: "missing error",
			modify: func(record *Record) {
				record.Error = ""
			},
		},
		{
			name: "zero attempt count",
			modify: func(record *Record) {
				record.AttemptCount = 0
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			record := validRecord
			testCase.modify(&record)

			if err := record.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want validation error")
			}
		})
	}
}
