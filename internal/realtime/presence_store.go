package realtime

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Which servers a user is connected to (#31). Each server marks the users it
// holds in the sorted set online:<userID>: the member is the server's instance
// ID, the score the time the mark expires (Unix ms). The sync loop renews the
// marks, so a server that stops without removing its own stops counting once
// they expire. A sorted set works on every Redis version; per-field hash expiry
// needs Redis 7.4. Scores come from each server's clock, so servers are assumed
// to agree on the time to within a few seconds.

// presenceTTL is how long a mark lasts without renewal. A variable so tests can
// shorten it.
var presenceTTL = 2 * time.Minute

// offlineRecheckMargin is how long after another server's mark expires a
// skipped offline is checked again. A variable so tests can shorten it.
var offlineRecheckMargin = 5 * time.Second

// presenceRedisTimeout bounds each call, so a slow Redis can't hold a user's
// presence lock for long.
const presenceRedisTimeout = time.Second

func onlineKey(userID string) string { return "online:" + userID }

func nowScore() string { return strconv.FormatInt(time.Now().UnixMilli(), 10) }

// markOnline marks userID as connected to this server. Call it under the
// user's presence lock, so it can't land after markOffline's removal.
func (eh *Hub) markOnline(userID string) {
	ctx, cancel := context.WithTimeout(context.Background(), presenceRedisTimeout)
	defer cancel()
	pipe := eh.redisClient.Pipeline()
	eh.queueMark(ctx, pipe, userID)
	if _, err := pipe.Exec(ctx); err != nil {
		eh.logger.WithError(err).Error("Failed to mark a user as online")
	}
}

// markOffline removes this server's mark for userID. If another server holds
// a mark that hasn't expired, it returns when the last of those expires.
// Call it under the user's presence lock.
func (eh *Hub) markOffline(userID string) (heldUntil time.Time, elsewhere bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), presenceRedisTimeout)
	defer cancel()
	key := onlineKey(userID)
	pipe := eh.redisClient.TxPipeline()
	pipe.ZRem(ctx, key, eh.instanceID)
	pipe.ZRemRangeByScore(ctx, key, "-inf", nowScore())
	last := pipe.ZRangeArgsWithScores(ctx, redis.ZRangeArgs{Key: key, Start: 0, Stop: 0, Rev: true})
	if _, err := pipe.Exec(ctx); err != nil {
		return time.Time{}, false, fmt.Errorf("mark offline: %w", err)
	}
	if len(last.Val()) == 0 {
		return time.Time{}, false, nil
	}
	return time.UnixMilli(int64(last.Val()[0].Score)), true, nil
}

// refreshOnline renews this server's marks for userIDs that it still holds.
// Each user is checked and renewed under their presence lock, so a renewal
// can't bring back a mark that a disconnect has just removed.
func (eh *Hub) refreshOnline(userIDs []string) error {
	byStripe := make(map[*sync.Mutex][]string)
	for _, userID := range userIDs {
		lock := eh.presenceLock(userID)
		byStripe[lock] = append(byStripe[lock], userID)
	}
	var firstErr error
	for lock, users := range byStripe {
		if err := eh.refreshStripe(lock, users); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (eh *Hub) refreshStripe(lock *sync.Mutex, userIDs []string) error {
	lock.Lock()
	defer lock.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), presenceRedisTimeout)
	defer cancel()
	pipe := eh.redisClient.Pipeline()
	queued := false
	for _, userID := range userIDs {
		if eh.pubSubManager.userInUse(userID) {
			eh.queueMark(ctx, pipe, userID)
			queued = true
		}
	}
	if !queued {
		return nil
	}
	_, err := pipe.Exec(ctx)
	return err
}

// queueMark adds this server's mark for userID to pipe and drops expired
// marks, so restarted servers don't pile up. The key expires with its newest
// mark, so a user nobody holds leaves nothing behind.
func (eh *Hub) queueMark(ctx context.Context, pipe redis.Pipeliner, userID string) {
	key := onlineKey(userID)
	pipe.ZRemRangeByScore(ctx, key, "-inf", nowScore())
	pipe.ZAdd(ctx, key, redis.Z{Score: float64(time.Now().Add(presenceTTL).UnixMilli()), Member: eh.instanceID})
	pipe.PExpire(ctx, key, presenceTTL)
}
