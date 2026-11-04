package users_service

import (
	"context"
	"fmt"
)

func (s *UsersService) DeleteUser(
	ctx context.Context,
	id int,
) error {
	ctx, span := s.tracer.Start(ctx, "users.delete")
	defer span.End()

	err := s.usersRepository.DeleteUser(ctx, id)
	if err != nil {
		markSpanError(span, "users.delete")
		return fmt.Errorf("delete user: %w", err)
	}

	return nil
}
