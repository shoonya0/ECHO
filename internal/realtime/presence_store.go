package realtime

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// Which servers a user is connected to (#31). Each server marks the users it
// holds in the sorted set online:<userID>: the member is the server's instance
// ID, the score the time the mark expires (Unix ms). The sync loop renews the
// marks, so a server that stops without removing its own stops counting once
// they expire. A sorted set works on every Redis version; per-field hash expiry
// needs Redis 7.4.

// presenceTTL is how long a mark lasts without renewal. A variable so tests can
// shorten it.
var presenceTTL = 2 * time.Minute

// presenceRedisTimeout bounds each call, so a slow Redis can't hold a user's
// presence lock for long.
const presenceRedisTimeout = 2 * time.Second

func onlineKey(userID string) string { return "online:" + userID }

// markOnline marks userID as connected to this server.
func (eh *Hub) markOnline(userID string) {
	ctx, cancel := context.WithTimeout(context.Background(), presenceRedisTimeout)
	defer cancel()
	pipe := eh.redisClient.Pipeline()
	eh.queueMark(ctx, pipe, userID)
	if _, err := pipe.Exec(ctx); err != nil {
		eh.logger.WithError(err).Error("Failed to mark a user as online")
	}
}

// markOffline removes this server's mark for userID and reports whether
// another server still holds one that hasn't expired.
func (eh *Hub) markOffline(userID string) (elsewhere bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), presenceRedisTimeout)
	defer cancel()
	key := onlineKey(userID)
	pipe := eh.redisClient.TxPipeline()
	pipe.ZRem(ctx, key, eh.instanceID)
	count := pipe.ZCount(ctx, key, "("+strconv.FormatInt(time.Now().UnixMilli(), 10), "+inf")
	if _, err := pipe.Exec(ctx); err != nil {
		return false, fmt.Errorf("mark offline: %w", err)
	}
	return count.Val() > 0, nil
}

// refreshOnline renews this server's marks for userIDs.
func (eh *Hub) refreshOnline(userIDs []string) error {
	if len(userIDs) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), presenceRedisTimeout)
	defer cancel()
	pipe := eh.redisClient.Pipeline()
	for _, userID := range userIDs {
		eh.queueMark(ctx, pipe, userID)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// queueMark adds this server's mark for userID to pipe. The key expires with
// its newest mark, so a user nobody holds leaves nothing behind.
func (eh *Hub) queueMark(ctx context.Context, pipe redis.Pipeliner, userID string) {
	key := onlineKey(userID)
	pipe.ZAdd(ctx, key, redis.Z{Score: float64(time.Now().Add(presenceTTL).UnixMilli()), Member: eh.instanceID})
	pipe.PExpire(ctx, key, presenceTTL)
}
