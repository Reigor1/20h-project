package users_service

import (
	"context"
	"fmt"

	"github.com/Reigor1/20h-project/internal/core/domain"
	core_errors "github.com/Reigor1/20h-project/internal/core/errors"
)

func (s *UsersService) GetUsers(
	ctx context.Context,
	limit *int,
	offset *int,
) ([]domain.User, error) {
	ctx, span := s.tracer.Start(ctx, "users.list")
	defer span.End()

	if limit != nil && *limit < 0 {
		markSpanError(span, "users.list")
		return nil, fmt.Errorf(
			"limit must be non-negative: %w",
			core_errors.ErrInvalidArgument,
		)
	}

	if offset != nil && *offset < 0 {
		markSpanError(span, "users.list")
		return nil, fmt.Errorf(
			"offset must be non-negative: %w",
			core_errors.ErrInvalidArgument,
		)
	}

	users, err := s.usersRepository.GetUsers(ctx, limit, offset)
	if err != nil {
		markSpanError(span, "users.list")
		return nil, fmt.Errorf("get users from repository: %w", err)
	}

	return users, nil
}
