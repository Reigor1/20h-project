package users_service

import (
	"context"
	"fmt"

	"github.com/Reigor1/20h-project/internal/core/domain"
)

func (s *UsersService) CreateUser(
	ctx context.Context,
	user domain.User,
) (domain.User, error) {
	ctx, span := s.tracer.Start(ctx, "users.create")
	defer span.End()

	if err := user.Validate(); err != nil {
		markSpanError(span, "users.create")
		return domain.User{}, fmt.Errorf("validate user domain: %w", err)
	}

	user, err := s.usersRepository.CreateUser(ctx, user)
	if err != nil {
		markSpanError(span, "users.create")
		return domain.User{}, fmt.Errorf("create user: %w", err)
	}

	return user, nil
}
