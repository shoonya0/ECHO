package testutil

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/shoonya0/ECHO/internal/database"

	"github.com/redis/go-redis/v9"
)

// UseTestRedis points database.Redis at logical database 15 of the Redis at
// TEST_REDIS_URI (default: the docker-compose one, localhost:6379), so tests don't
// touch a dev server's keys in database 0. It skips when Redis isn't reachable, or
// fails when TEST_DB_REQUIRED is set (CI). Tests must use keys they own (random IDs)
// and delete them; the database is not flushed.
func UseTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("TEST_REDIS_URI")
	if addr == "" {
		addr = "localhost:6379"
	}

	unavailable := t.Skipf
	if os.Getenv("TEST_DB_REQUIRED") != "" {
		unavailable = t.Fatalf
	}

	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15, DialTimeout: 2 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		unavailable("Redis unavailable: %v", err)
	}

	prev := database.Redis
	database.Redis = client
	t.Cleanup(func() {
		_ = client.Close()
		database.Redis = prev
	})
	return client
}

// UseBrokenRedis points database.Redis at an address nothing listens on, so every
// command fails. It tests what the code does when Redis is down.
func UseBrokenRedis(t *testing.T) {
	t.Helper()
	client := redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:1",
		DialTimeout: 200 * time.Millisecond,
		MaxRetries:  -1,
	})
	prev := database.Redis
	database.Redis = client
	t.Cleanup(func() {
		_ = client.Close()
		database.Redis = prev
	})
}
