package statistics_redis_cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Reigor1/20h-project/internal/core/domain"
	core_redis "github.com/Reigor1/20h-project/internal/core/repository/redis"
)

func (c *StatisticsCache) Get(ctx context.Context) (domain.Statistics, bool, error) {
	value, err := c.cache.Get(ctx, globalStatisticsCacheKey)
	if err != nil {
		if errors.Is(err, core_redis.ErrCacheMiss) {
			return domain.Statistics{}, false, nil
		}

		return domain.Statistics{}, false, fmt.Errorf("get statistics from Redis cache: %w", err)
	}

	var statistics domain.Statistics
	if err := json.Unmarshal(value, &statistics); err != nil {
		return domain.Statistics{}, false, fmt.Errorf("unmarshal cached statistics: %w", err)
	}

	return statistics, true, nil
}
