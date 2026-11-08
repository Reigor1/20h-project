package statistics_redis_cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Reigor1/20h-project/internal/core/domain"
)

func (c *StatisticsCache) Set(
	ctx context.Context,
	statistics domain.Statistics,
	ttl time.Duration,
) error {
	value, err := json.Marshal(statistics)
	if err != nil {
		return fmt.Errorf("marshal statistics: %w", err)
	}

	if err := c.cache.Set(ctx, globalStatisticsCacheKey, value, ttl); err != nil {
		return fmt.Errorf("set statistics in Redis cache: %w", err)
	}

	return nil
}
