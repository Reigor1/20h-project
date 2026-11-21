package activity_postgres_repository

import (
	"time"

	activity_service "github.com/Reigor1/20h-project/internal/features/activity/service"
)

// ActivityModel maps the persisted Activity Log row and its event metadata from PostgreSQL.
type ActivityModel struct {
	EventID          string
	EventType        string
	TaskID           int64
	AggregateVersion int64
	SchemaVersion    int
	AuthorUserID     int64
	OccurredAt       time.Time
	RecordedAt       time.Time
	Payload          []byte
}

// activityDomainFromModel converts a database model into the public Activity domain value.
func activityDomainFromModel(
	model ActivityModel,
) activity_service.Activity {
	return activity_service.Activity{
		EventID:          model.EventID,
		EventType:        model.EventType,
		TaskID:           model.TaskID,
		AggregateVersion: model.AggregateVersion,
		SchemaVersion:    model.SchemaVersion,
		AuthorUserID:     model.AuthorUserID,
		OccurredAt:       model.OccurredAt,
		RecordedAt:       model.RecordedAt,
		Payload:          model.Payload,
	}
}
