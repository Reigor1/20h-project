package users_postgres_repository

import "github.com/Reigor1/20h-project/internal/core/domain"

// UserModel represents a PostgreSQL users row before conversion to the domain model.
type UserModel struct {
	ID          int
	Version     int
	FullName    string
	PhoneNumber *string
}

// userDomainsFromModels converts persistence models into domain User values.
func userDomainsFromModels(users []UserModel) []domain.User {
	userDomains := make([]domain.User, len(users))

	for i, user := range users {
		userDomains[i] = domain.User{
			ID:          user.ID,
			Version:     user.Version,
			FullName:    user.FullName,
			PhoneNumber: user.PhoneNumber,
		}
	}

	return userDomains
}
