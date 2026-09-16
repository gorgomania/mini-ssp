package freqcap

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Capper interface {
	IsCapped(ctx context.Context, userID, advertiserID string) bool
	// Record atomically increments the impression counter and returns true if the
	// impression is allowed (counter was below the limit). Returns false if the cap
	// was already reached — the caller should not serve the ad.
	Record(ctx context.Context, userID, advertiserID string) bool
}

type NoopCapper struct{}

func (NoopCapper) IsCapped(_ context.Context, _, _ string) bool { return false }
func (NoopCapper) Record(_ context.Context, _, _ string) bool   { return true }

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

// luaRecord atomically increments the counter, sets TTL on first write, and
// rolls back if the limit is exceeded. Returns 1 if allowed, 0 if capped.
var luaRecord = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then
    redis.call('EXPIRE', KEYS[1], ARGV[1])
end
if count > tonumber(ARGV[2]) then
    redis.call('DECR', KEYS[1])
    return 0
end
return 1
`)

func (c *RedisCapper) IsCapped(ctx context.Context, userID, advertiserID string) bool {
	key := fmt.Sprintf("fc:%s:%s", userID, advertiserID)
	count, err := c.client.Get(ctx, key).Int64()
	if err != nil {
		return false
	}
	return count >= c.limit
}

func (c *RedisCapper) Record(ctx context.Context, userID, advertiserID string) bool {
	key := fmt.Sprintf("fc:%s:%s", userID, advertiserID)
	windowSecs := int64(c.window.Seconds())
	result, err := luaRecord.Run(ctx, c.client, []string{key},
		windowSecs, c.limit).Int64()
	if err != nil {
		return true // fail-open: don't block on Redis errors
	}
	return result == 1
}
