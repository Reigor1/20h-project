package activity_service

import (
	"context"
	"fmt"
	"time"

	core_events "github.com/Reigor1/20h-project/internal/core/events"
)

type Activity struct {
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

type ActivityRepository interface {
	Record(
		ctx context.Context,
		event core_events.Envelope,
	) error

	GetActivities(
		ctx context.Context,
		taskID int,
		limit *int,
		offset *int,
	) ([]Activity, error)
}

type ActivityService struct {
	repository ActivityRepository
}

func NewActivityService(
	repository ActivityRepository,
) *ActivityService {
	return &ActivityService{
		repository: repository,
	}
}

func (s *ActivityService) Record(
	ctx context.Context,
	event core_events.Envelope,
) error {
	if err := event.Validate(); err != nil {
		return fmt.Errorf("validate Activity event: %w", err)
	}

	if err := s.repository.Record(ctx, event); err != nil {
		return fmt.Errorf("record Activity event: %w", err)
	}

	return nil
}
