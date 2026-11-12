package core_events

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func validEnvelope() Envelope {
	return Envelope{
		EventID:          "123e4567-e89b-12d3-a456-426614174000",
		EventType:        EventTypeTaskCreated,
		SchemaVersion:    CurrentSchemaVersion,
		OccurredAt:       time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC),
		AggregateType:    AggregateTypeTask,
		AggregateID:      123,
		AggregateVersion: 1,
		Payload: json.RawMessage(`{
			"title": "Prepare report"
		}`),
	}
}

func TestMarshalAndUnmarshal(t *testing.T) {
	want := validEnvelope()

	data, err := Marshal(want)
	if err != nil {
		t.Fatalf("Marshal() error = %v, want nil", err)
	}

	got, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v, want nil", err)
	}

	var gotPayload, wantPayload bytes.Buffer

	if err := json.Compact(&gotPayload, got.Payload); err != nil {
		t.Fatalf("Compact(got.Payload) error = %v", err)
	}

	if err := json.Compact(&wantPayload, want.Payload); err != nil {
		t.Fatalf("Compact(want.Payload) error = %v", err)
	}

	if got.EventID != want.EventID ||
		got.EventType != want.EventType ||
		got.SchemaVersion != want.SchemaVersion ||
		!got.OccurredAt.Equal(want.OccurredAt) ||
		got.AggregateType != want.AggregateType ||
		got.AggregateID != want.AggregateID ||
		got.AggregateVersion != want.AggregateVersion ||
		gotPayload.String() != wantPayload.String() {
		t.Fatalf("Unmarshal() = %+v, want %+v", got, want)
	}
}

func TestEnvelopeValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Envelope)
		wantErr error
	}{
		{
			name: "invalid event ID",
			modify: func(e *Envelope) {
				e.EventID = "invalid"
			},
			wantErr: ErrInvalidEnvelope,
		},
		{
			name: "empty event type",
			modify: func(e *Envelope) {
				e.EventType = ""
			},
			wantErr: ErrInvalidEnvelope,
		},
		{
			name: "unknown event type",
			modify: func(e *Envelope) {
				e.EventType = "task.unknown"
			},
			wantErr: ErrInvalidEnvelope,
		},
		{
			name: "missing schema version",
			modify: func(e *Envelope) {
				e.SchemaVersion = 0
			},
			wantErr: ErrInvalidEnvelope,
		},
		{
			name: "unknown schema version",
			modify: func(e *Envelope) {
				e.SchemaVersion = 2
			},
			wantErr: ErrUnsupportedSchemaVersion,
		},
		{
			name: "invalid payload",
			modify: func(e *Envelope) {
				e.Payload = json.RawMessage(`{invalid}`)
			},
			wantErr: ErrInvalidEnvelope,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			envelope := validEnvelope()
			testCase.modify(&envelope)

			err := envelope.Validate()

			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf(
					"Validate() error = %v, want %v",
					err,
					testCase.wantErr,
				)
			}
		})
	}
}
