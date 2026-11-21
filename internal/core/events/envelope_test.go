package core_events

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// validEnvelope returns a minimal valid event used by envelope tests.
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

// TestMarshalAndUnmarshal verifies that a valid envelope survives JSON round-tripping.
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

	if got.EventID != want.EventID {
		t.Errorf("EventID = %q, want %q", got.EventID, want.EventID)
	}

	if got.EventType != want.EventType {
		t.Errorf("EventType = %q, want %q", got.EventType, want.EventType)
	}

	if got.SchemaVersion != want.SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", got.SchemaVersion, want.SchemaVersion)
	}

	if !got.OccurredAt.Equal(want.OccurredAt) {
		t.Errorf("OccurredAt = %v, want %v", got.OccurredAt, want.OccurredAt)
	}

	if got.AggregateType != want.AggregateType {
		t.Errorf("AggregateType = %q, want %q", got.AggregateType, want.AggregateType)
	}

	if got.AggregateID != want.AggregateID {
		t.Errorf("AggregateID = %d, want %d", got.AggregateID, want.AggregateID)
	}

	if got.AggregateVersion != want.AggregateVersion {
		t.Errorf(
			"AggregateVersion = %d, want %d",
			got.AggregateVersion,
			want.AggregateVersion,
		)
	}

	if compactJSON(t, got.Payload) != compactJSON(t, want.Payload) {
		t.Errorf("Payload = %s, want %s", got.Payload, want.Payload)
	}
}

// TestEnvelopeValidate checks required metadata, supported event types, and payload constraints.
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
			name: "zero event ID",
			modify: func(e *Envelope) {
				e.EventID = "00000000-0000-0000-0000-000000000000"
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
			wantErr: ErrUnsupportedEventType,
		},
		{
			name: "missing schema version",
			modify: func(e *Envelope) {
				e.SchemaVersion = 0
			},
			wantErr: ErrInvalidEnvelope,
		},
		{
			name: "unsupported schema version",
			modify: func(e *Envelope) {
				e.SchemaVersion = 2
			},
			wantErr: ErrUnsupportedSchemaVersion,
		},
		{
			name: "missing occurred at",
			modify: func(e *Envelope) {
				e.OccurredAt = time.Time{}
			},
			wantErr: ErrInvalidEnvelope,
		},
		{
			name: "unsupported aggregate type",
			modify: func(e *Envelope) {
				e.AggregateType = "user"
			},
			wantErr: ErrInvalidEnvelope,
		},
		{
			name: "invalid aggregate ID",
			modify: func(e *Envelope) {
				e.AggregateID = 0
			},
			wantErr: ErrInvalidEnvelope,
		},
		{
			name: "invalid aggregate version",
			modify: func(e *Envelope) {
				e.AggregateVersion = 0
			},
			wantErr: ErrInvalidEnvelope,
		},
		{
			name: "missing payload",
			modify: func(e *Envelope) {
				e.Payload = nil
			},
			wantErr: ErrInvalidEnvelope,
		},
		{
			name: "invalid payload JSON",
			modify: func(e *Envelope) {
				e.Payload = json.RawMessage(`{invalid}`)
			},
			wantErr: ErrInvalidEnvelope,
		},
		{
			name: "null payload",
			modify: func(e *Envelope) {
				e.Payload = json.RawMessage(`null`)
			},
			wantErr: ErrInvalidEnvelope,
		},
		{
			name: "array payload",
			modify: func(e *Envelope) {
				e.Payload = json.RawMessage(`[]`)
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

// TestMarshalRejectsInvalidEnvelope ensures invalid event contracts are not serialized.
func TestMarshalRejectsInvalidEnvelope(t *testing.T) {
	envelope := validEnvelope()
	envelope.SchemaVersion = 2

	_, err := Marshal(envelope)

	if !errors.Is(err, ErrUnsupportedSchemaVersion) {
		t.Fatalf(
			"Marshal() error = %v, want ErrUnsupportedSchemaVersion",
			err,
		)
	}
}

// TestUnmarshalRejectsUnsupportedSchemaVersion verifies schema-version compatibility checks.
func TestUnmarshalRejectsUnsupportedSchemaVersion(t *testing.T) {
	data, err := json.Marshal(validEnvelope())
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var rawEnvelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawEnvelope); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	rawEnvelope["schema_version"] = json.RawMessage(`2`)

	data, err = json.Marshal(rawEnvelope)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	_, err = Unmarshal(data)

	if !errors.Is(err, ErrUnsupportedSchemaVersion) {
		t.Fatalf(
			"Unmarshal() error = %v, want ErrUnsupportedSchemaVersion",
			err,
		)
	}
}

// TestUnmarshalRejectsMalformedJSON checks that invalid JSON is classified as malformed input.
func TestUnmarshalRejectsMalformedJSON(t *testing.T) {
	_, err := Unmarshal([]byte(`{"event_id":`))

	if !errors.Is(err, ErrMalformedJSON) {
		t.Fatalf(
			"Unmarshal() error = %v, want ErrMalformedJSON",
			err,
		)
	}
}

// compactJSON normalizes JSON whitespace so test cases can compare serialized payloads.
func compactJSON(t *testing.T, data []byte) string {
	t.Helper()

	var compact bytes.Buffer

	if err := json.Compact(&compact, data); err != nil {
		t.Fatalf("compact JSON: %v", err)
	}

	return compact.String()
}
