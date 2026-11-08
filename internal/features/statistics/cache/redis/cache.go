package statistics_redis_cache

import core_redis "github.com/Reigor1/20h-project/internal/core/repository/redis"

const globalStatisticsCacheKey = "statistics:global"

type StatisticsCache struct {
    cache core_redis.Cache
}

func NewStatisticsCache(cache core_redis.Cache) *StatisticsCache {
    return &StatisticsCache{
        cache: cache,
    }
}
