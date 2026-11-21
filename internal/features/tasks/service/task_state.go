package tasks_service

import (
	"time"

	"github.com/Reigor1/20h-project/internal/core/domain"
)

// sameTaskState reports whether the business-visible task fields are unchanged.
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

// equalStringPointers compares optional strings, treating two nil pointers as equal.
func equalStringPointers(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}

	return *left == *right
}

// equalTimePointers compares optional timestamps, treating two nil pointers as equal.
func equalTimePointers(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}

	return left.Equal(*right)
}
