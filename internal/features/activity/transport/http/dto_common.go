package activity_transport_http

import (
	"encoding/json"
	"time"

	activity_service "github.com/Reigor1/20h-project/internal/features/activity/service"
)

// ActivityDTOResponse is the JSON representation of one Activity Log entry.
type ActivityDTOResponse struct {
	EventID          string          `json:"event_id"           example:"550e8400-e29b-41d4-a716-446655440000"`
	EventType        string          `json:"event_type"         example:"task.completed"`
	TaskID           int64           `json:"task_id"            example:"123"`
	AuthorUserID     int64           `json:"author_user_id"     example:"456"`
	AggregateVersion int64           `json:"aggregate_version"  example:"4"`
	SchemaVersion    int             `json:"schema_version"     example:"1"`
	OccurredAt       time.Time       `json:"occurred_at"        example:"2026-11-11T10:00:00Z"`
	RecordedAt       time.Time       `json:"recorded_at"        example:"2026-11-11T10:00:01Z"`
	Payload          json.RawMessage `json:"payload"             swaggertype:"object"`
}

// activityDTOFromDomain maps a domain Activity value to its HTTP response DTO.
func activityDTOFromDomain(
	activity activity_service.Activity,
) ActivityDTOResponse {
	return ActivityDTOResponse{
		EventID:          activity.EventID,
		EventType:        activity.EventType,
		TaskID:           activity.TaskID,
		AuthorUserID:     activity.AuthorUserID,
		AggregateVersion: activity.AggregateVersion,
		SchemaVersion:    activity.SchemaVersion,
		OccurredAt:       activity.OccurredAt,
		RecordedAt:       activity.RecordedAt,
		Payload:          json.RawMessage(activity.Payload),
	}
}

// activitiesDTOFromDomains converts a slice of domain activities into response DTOs.
func activitiesDTOFromDomains(
	activities []activity_service.Activity,
) []ActivityDTOResponse {
	activitiesDTO := make(
		[]ActivityDTOResponse,
		len(activities),
	)

	for i, activity := range activities {
		activitiesDTO[i] = activityDTOFromDomain(activity)
	}

	return activitiesDTO
}
