package tasks_service

import (
	"time"

	"github.com/Reigor1/20h-project/internal/core/domain"
)

func sameTaskState(before, after domain.Task) bool {
	return before.ID == after.ID &&
		before.Version == after.Version &&
		before.Title == after.Title &&
		equalStringPointers(before.Description, after.Description) &&
		before.Completed == after.Completed &&
		before.CreatedAt.Equal(after.CreatedAt) &&
		equalTimePointers(before.CompletedAt, after.CompletedAt) &&
		before.AuthorUserID == after.AuthorUserID
}

func equalStringPointers(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}

	return *left == *right
}

func equalTimePointers(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}

	return left.Equal(*right)
}
