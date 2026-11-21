package users_service

import (
	"context"
	"fmt"

	core_observability "github.com/Reigor1/20h-project/internal/core/observability"
)

// DeleteUser removes the user identified by ID and propagates repository failures.
func (s *UsersService) DeleteUser(
	ctx context.Context,
	id int,
) error {
	ctx, span := s.tracer.Start(ctx, "users.delete")
	defer span.End()

	err := s.usersRepository.DeleteUser(ctx, id)
	if err != nil {
		core_observability.MarkSpanError(span, "users.delete")
		return fmt.Errorf("delete user: %w", err)
	}

	return nil
}
