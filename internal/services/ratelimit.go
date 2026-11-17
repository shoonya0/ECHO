package services

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/logger"
)

// Limit allows Max requests per key in each Window. The window starts with the
// key's first request (a fixed window, kept in Redis so every server instance
// shares the count).
type Limit struct {
	Max    int
	Window time.Duration
}

// Limits on logging in and signing up (#40). They are variables so tests can
// make them small.
var (
	// LoginIPLimit counts every login from one client IP. It is generous because
	// mobile carriers put many users behind one address.
	LoginIPLimit = Limit{Max: 50, Window: 10 * time.Minute}
	// LoginEmailFailLimit counts failed logins for one email, from any IP.
	LoginEmailFailLimit = Limit{Max: 10, Window: 15 * time.Minute}
	// SignupIPLimit counts every signup from one client IP; each one runs bcrypt.
	SignupIPLimit = Limit{Max: 10, Window: time.Hour}
)

// RateLimitKey is the Redis key that counts requests of one kind for one client.
func RateLimitKey(name, id string) string {
	return "ratelimit:" + name + ":" + id
}

// EmailRateLimitID is the id an email counts under. Case and surrounding space
// don't make a new bucket. It is used only for the count, never for the lookup.
func EmailRateLimitID(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// Hit counts one request under key. It reports whether the request is within the
// limit and, if not, how long until the window ends. When Redis fails, the
// request is allowed: an outage shouldn't block every login.
func Hit(ctx context.Context, key string, l Limit) (allowed bool, retryAfter time.Duration) {
	var incr *redis.IntCmd
	var ttl *redis.DurationCmd
	_, err := database.Redis.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		incr = pipe.Incr(ctx, key)
		pipe.ExpireNX(ctx, key, l.Window)
		ttl = pipe.TTL(ctx, key)
		return nil
	})
	if err != nil {
		logger.WithContext(ctx).WithError(err).Warn("Rate limit count failed; allowing the request")
		return true, 0
	}
	if incr.Val() <= int64(l.Max) {
		return true, 0
	}
	return false, waitFor(ttl.Val(), l)
}

// Exceeded reports whether key is already over the limit, without counting a
// request. When Redis fails, it reports false.
func Exceeded(ctx context.Context, key string, l Limit) (exceeded bool, retryAfter time.Duration) {
	var get *redis.StringCmd
	var ttl *redis.DurationCmd
	_, err := database.Redis.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		get = pipe.Get(ctx, key)
		ttl = pipe.TTL(ctx, key)
		return nil
	})
	if errors.Is(err, redis.Nil) {
		return false, 0
	}
	if err != nil {
		logger.WithContext(ctx).WithError(err).Warn("Rate limit check failed; allowing the request")
		return false, 0
	}
	count, convErr := get.Int64()
	if convErr != nil || count <= int64(l.Max) {
		return false, 0
	}
	return true, waitFor(ttl.Val(), l)
}

// waitFor turns a key's TTL into a wait. A key without a TTL (-1) or a missing
// one (-2) waits the whole window.
func waitFor(ttl time.Duration, l Limit) time.Duration {
	if ttl <= 0 {
		return l.Window
	}
	return ttl
}
