package freqcap

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Capper interface {
	IsCapped(ctx context.Context, userID, advertiserID string) bool
	Record(ctx context.Context, userID, advertiserID string)
}

type NoopCapper struct{}

func (NoopCapper) IsCapped(_ context.Context, _, _ string) bool { return false }
func (NoopCapper) Record(_ context.Context, _, _ string)        {}

type RedisCapper struct {
	client *redis.Client
	limit  int64
	window time.Duration
}

func NewRedisCapper(addr string, limit int64, window time.Duration) (*RedisCapper, error) {
	client := redis.NewClient(&redis.Options{Addr: addr})
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, err
	}
	return &RedisCapper{client: client, limit: limit, window: window}, nil
}

func (c *RedisCapper) IsCapped(ctx context.Context, userID, advertiserID string) bool {
	key := fmt.Sprintf("fc:%s:%s", userID, advertiserID)
	count, err := c.client.Get(ctx, key).Int64()
	if err != nil {
		return false
	}
	return count >= c.limit
}

func (c *RedisCapper) Record(ctx context.Context, userID, advertiserID string) {
	key := fmt.Sprintf("fc:%s:%s", userID, advertiserID)
	count, err := c.client.Incr(ctx, key).Result()
	if err != nil {
		return
	}
	if count == 1 {
		c.client.Expire(ctx, key, c.window)
	}
}
