package users_service

import (
	"context"

	"github.com/Reigor1/20h-project/internal/core/domain"
	"go.opentelemetry.io/otel/trace"
)

const tracerInstrumentationName = "github.com/Reigor1/20h-project/internal/features/users/service"

// UsersService implements user use cases over the Users repository.
type UsersService struct {
	usersRepository UsersRepository
	tracer          trace.Tracer
}

type UsersRepository interface {
	CreateUser(
		ctx context.Context,
		user domain.User,
	) (domain.User, error)

	GetUsers(
		ctx context.Context,
		limit *int,
		offset *int,
	) ([]domain.User, error)

	GetUser(
		ctx context.Context,
		id int,
	) (domain.User, error)

	DeleteUser(
		ctx context.Context,
		id int,
	) error

	PatchUser(
		ctx context.Context,
		id int,
		user domain.User,
	) (domain.User, error)
}

func NewUsersService(
	usersRepository UsersRepository,
	tracerProvider trace.TracerProvider,
) *UsersService {
	return &UsersService{
		usersRepository: usersRepository,
		tracer:          tracerProvider.Tracer(tracerInstrumentationName),
	}
}
