package users_service

import (
	"context"
	"fmt"

	"github.com/Reigor1/20h-project/internal/core/domain"
	core_observability "github.com/Reigor1/20h-project/internal/core/observability"
)

func (s *UsersService) PatchUser(
	ctx context.Context,
	id int,
	patch domain.UserPatch,
) (domain.User, error) {
	ctx, span := s.tracer.Start(ctx, "users.update")
	defer span.End()

	user, err := s.usersRepository.GetUser(ctx, id)
	if err != nil {
		core_observability.MarkSpanError(span, "users.update")
		return domain.User{}, fmt.Errorf("get user: %w", err)
	}

	if err := user.ApplyPatch(patch); err != nil {
		core_observability.MarkSpanError(span, "users.update")
		return domain.User{}, fmt.Errorf("apply patch: %w", err)
	}

	patchedUser, err := s.usersRepository.PatchUser(ctx, id, user)
	if err != nil {
		core_observability.MarkSpanError(span, "users.update")
		return domain.User{}, fmt.Errorf("patch user: %w", err)
	}

	return patchedUser, nil
}
