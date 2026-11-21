package users_postgres_repository

import core_postgres_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool"

// UsersRepository implements user persistence operations using the PostgreSQL pool.
type UsersRepository struct {
	pool core_postgres_pool.Pool
}

// NewUsersRepository creates the user repository with its database pool dependency.
func NewUsersRepository(
	pool core_postgres_pool.Pool,
) *UsersRepository {
	return &UsersRepository{
		pool: pool,
	}
}
