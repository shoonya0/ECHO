package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/shoonya0/ECHO/internal/config"
	"github.com/shoonya0/ECHO/internal/logger"

	"github.com/redis/go-redis/v9"
)

// Redis is the shared Redis client (token blacklist + WebSocket pub/sub).
var Redis *redis.Client

// ConnectRedis connects to Redis and verifies the connection with a ping.
// REDIS_URI may be a plain host:port or a redis:// / rediss:// URL.
func ConnectRedis(ctx context.Context) error {
	log := logger.WithContext(ctx)
	log.Info("Connecting to Redis...")

	opts, err := redisOptions(config.Cfg.RedisURI, config.Cfg.RedisPass)
	if err != nil {
		return err
	}

	Redis = redis.NewClient(opts)
	if err := Redis.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}

	log.Info("Connected to Redis")
	return nil
}

// CloseRedis closes the Redis connection.
func CloseRedis() error {
	if Redis == nil {
		return nil
	}
	return Redis.Close()
}

func redisOptions(uri, password string) (*redis.Options, error) {
	if strings.HasPrefix(uri, "redis://") || strings.HasPrefix(uri, "rediss://") {
		opts, err := redis.ParseURL(uri)
		if err != nil {
			return nil, fmt.Errorf("invalid REDIS_URI: %w", err)
		}
		return opts, nil
	}
	return &redis.Options{Addr: uri, Password: password}, nil
}
