package activity_service

import (
	"context"
	"fmt"

	core_errors "github.com/Reigor1/20h-project/internal/core/errors"
)

func (s *ActivityService) GetActivities(
	ctx context.Context,
	taskID int,
	limit *int,
	offset *int,
) ([]Activity, error) {
	if taskID <= 0 {
		return nil, fmt.Errorf(
			"task ID must be positive: %w",
			core_errors.ErrInvalidArgument,
		)
	}

	if limit != nil && *limit < 0 {
		return nil, fmt.Errorf(
			"limit must be non-negative: %w",
			core_errors.ErrInvalidArgument,
		)
	}

	if offset != nil && *offset < 0 {
		return nil, fmt.Errorf(
			"offset must be non-negative: %w",
			core_errors.ErrInvalidArgument,
		)
	}

	activities, err := s.repository.GetActivities(
		ctx,
		taskID,
		limit,
		offset,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"get activities from repository: %w",
			err,
		)
	}

	return activities, nil
}
