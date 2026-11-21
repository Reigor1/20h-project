package statistics_redis_cache

import (
	"context"
	"fmt"
)

// Delete invalidates the cached global statistics entry.
func (c *StatisticsCache) Delete(ctx context.Context) error {
	if err := c.cache.Delete(ctx, globalStatisticsCacheKey); err != nil {
		return fmt.Errorf("delete statistics from Redis cache: %w", err)
	}

	return nil
}
