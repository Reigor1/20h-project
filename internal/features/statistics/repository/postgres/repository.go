package statistics_postgres_repository

import core_postgres_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool"

// StatisticsRepository reads task data from PostgreSQL for statistics calculations.
type StatisticsRepository struct {
	pool core_postgres_pool.Pool
}

// NewStatisticsRepository creates the statistics repository with its PostgreSQL pool.
func NewStatisticsRepository(
	pool core_postgres_pool.Pool,
) *StatisticsRepository {
	return &StatisticsRepository{
		pool: pool,
	}
}
