package users_transport_http

import (
	"context"
	"net/http"

	"github.com/Reigor1/20h-project/internal/core/domain"
	core_http_server "github.com/Reigor1/20h-project/internal/core/transport/http/server"
)

// UsersHTTPHandler adapts user service operations to HTTP requests and responses.
type UsersHTTPHandler struct {
	usersService UserService
}

// UserService defines the user use cases exposed by the HTTP transport.
type UserService interface {
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
		patch domain.UserPatch,
	) (domain.User, error)
}

// NewUsersHTTPHandler creates the user handler with its service dependency.
func NewUsersHTTPHandler(
	usersService UserService,
) *UsersHTTPHandler {
	return &UsersHTTPHandler{
		usersService: usersService,
	}
}

// Routes returns the create, list, read, delete, and patch endpoints for Users.
func (h *UsersHTTPHandler) Routes() []core_http_server.Route {
	return []core_http_server.Route{
		{
			Method:  http.MethodPost,
			Path:    "/users",
			Handler: h.CreateUser,
		},
		{
			Method:  http.MethodGet,
			Path:    "/users",
			Handler: h.GetUsers,
			// Example of usage Middleware on separate Route
			// Middleware: []core_http_middleware.Middleware{
			// 	core_http_middleware.Dummy("get users middleware"),
			// },
		},
		{
			Method:  http.MethodGet,
			Path:    "/users/{id}",
			Handler: h.GetUser,
		},
		{
			Method:  http.MethodDelete,
			Path:    "/users/{id}",
			Handler: h.DeleteUser,
		},
		{
			Method:  http.MethodPatch,
			Path:    "/users/{id}",
			Handler: h.PatchUser,
		},
	}
}
