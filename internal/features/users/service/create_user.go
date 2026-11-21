package users_service

import (
	"context"
	"fmt"

	"github.com/Reigor1/20h-project/internal/core/domain"
	core_observability "github.com/Reigor1/20h-project/internal/core/observability"
)

// CreateUser validates the new user and persists it through the user repository.
func (s *UsersService) CreateUser(
	ctx context.Context,
	user domain.User,
) (domain.User, error) {
	ctx, span := s.tracer.Start(ctx, "users.create")
	defer span.End()

	if err := user.Validate(); err != nil {
		core_observability.MarkSpanError(span, "users.create")
		return domain.User{}, fmt.Errorf("validate user domain: %w", err)
	}

	user, err := s.usersRepository.CreateUser(ctx, user)
	if err != nil {
		core_observability.MarkSpanError(span, "users.create")
		return domain.User{}, fmt.Errorf("create user: %w", err)
	}

	return user, nil
}
