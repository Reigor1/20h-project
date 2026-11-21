package statistics_redis_cache

import core_redis "github.com/Reigor1/20h-project/internal/core/repository/redis"

// globalStatisticsCacheKey identifies the unfiltered aggregate statistics entry.
const globalStatisticsCacheKey = "statistics:global"

// StatisticsCache adapts the shared byte-oriented Redis cache to the Statistics domain model.
type StatisticsCache struct {
	cache core_redis.Cache
}

// NewStatisticsCache creates the statistics cache adapter over the shared Redis contract.
func NewStatisticsCache(cache core_redis.Cache) *StatisticsCache {
	return &StatisticsCache{
		cache: cache,
	}
}
