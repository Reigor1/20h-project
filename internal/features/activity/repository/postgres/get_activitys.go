package activity_postgres_repository

import (
	"context"
	"fmt"

	activity_service "github.com/Reigor1/20h-project/internal/features/activity/service"
)

func (r *ActivityRepository) GetActivities(
	ctx context.Context,
	taskID int,
	limit *int,
	offset *int,
) ([]activity_service.Activity, error) {
	ctx, cancel := context.WithTimeout(ctx, r.pool.OpTimeout())
	defer cancel()

	query := `
	SELECT
		event_id,
		event_type,
		task_id,
		task_version,
		schema_version,
		occurred_at,
		recorded_at,
		COALESCE(
			(payload #>> '{task,author_user_id}')::BIGINT,
			(payload #>> '{after,author_user_id}')::BIGINT,
			(payload #>> '{before,author_user_id}')::BIGINT
		) AS author_user_id,
		payload
	FROM todoapp.task_activity
	WHERE task_id = $1
	ORDER BY task_version ASC, id ASC
	LIMIT $2
	OFFSET $3;
	`

	rows, err := r.pool.Query(
		ctx,
		query,
		taskID,
		limit,
		offset,
	)
	if err != nil {
		return nil, fmt.Errorf("select task activities: %w", err)
	}
	defer rows.Close()

	activitiesModels := make([]ActivityModel, 0)

	for rows.Next() {
		var model ActivityModel

		err := rows.Scan(
			&model.EventID,
			&model.EventType,
			&model.TaskID,
			&model.AggregateVersion,
			&model.SchemaVersion,
			&model.OccurredAt,
			&model.RecordedAt,
			&model.AuthorUserID,
			&model.Payload,
		)
		if err != nil {
			return nil, fmt.Errorf("scan task activity: %w", err)
		}

		activitiesModels = append(activitiesModels, model)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task activities: %w", err)
	}

	activities := make(
		[]activity_service.Activity,
		len(activitiesModels),
	)

	for i, model := range activitiesModels {
		activities[i] = activityDomainFromModel(model)
	}

	return activities, nil
}
