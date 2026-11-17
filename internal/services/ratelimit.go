package services

import (
	"context"
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
	// LoginPairFailLimit counts login attempts on one account from one IP. It
	// stops one address from guessing, without locking the account for others.
	LoginPairFailLimit = Limit{Max: 10, Window: 15 * time.Minute}
	// LoginAccountLimit counts login attempts on one account from all IPs
	// together. It caps guessing spread over many addresses; locking a user out
	// takes several addresses at the pair limit.
	LoginAccountLimit = Limit{Max: 100, Window: time.Hour}
	// SignupIPLimit counts every signup from one client IP; each one runs bcrypt.
	SignupIPLimit = Limit{Max: 10, Window: time.Hour}
)

// RateLimitKey is the Redis key that counts requests of one kind for one client.
func RateLimitKey(name, id string) string {
	return "ratelimit:" + name + ":" + id
}

// hitScript counts one request and starts the window on the first one, in one
// step, so concurrent requests can't all pass before any is counted. It needs no
// Redis 7 command (EXPIRE NX). Returns the count and the window's ms left.
var hitScript = redis.NewScript(`
local count = redis.call("INCR", KEYS[1])
if count == 1 then
  redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
return {count, redis.call("PTTL", KEYS[1])}
`)

// Hit counts one request under key. It reports whether the request is within the
// limit and, if not, how long until the window ends. When Redis fails, the
// request is allowed: an outage shouldn't block every login. That is logged as
// an error, because every limit is off meanwhile.
func Hit(ctx context.Context, key string, l Limit) (allowed bool, retryAfter time.Duration) {
	if database.Redis == nil {
		return true, 0
	}
	res, err := hitScript.Run(ctx, database.Redis, []string{key}, l.Window.Milliseconds()).Int64Slice()
	if err != nil || len(res) != 2 {
		logger.WithContext(ctx).WithError(err).Error("Rate limit count failed; allowing the request")
		return true, 0
	}
	if res[0] <= int64(l.Max) {
		return true, 0
	}
	ttl := time.Duration(res[1]) * time.Millisecond
	if ttl <= 0 {
		ttl = l.Window
	}
	return false, ttl
}

// ClearRateLimits deletes the counts under keys, e.g. after a login succeeds.
// A failure is only logged: the counts end with their window anyway.
func ClearRateLimits(ctx context.Context, keys ...string) {
	if database.Redis == nil || len(keys) == 0 {
		return
	}
	if err := database.Redis.Del(ctx, keys...).Err(); err != nil {
		logger.WithContext(ctx).WithError(err).Warn("Clearing rate limit counts failed")
	}
}
