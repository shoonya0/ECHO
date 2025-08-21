package db

import (
	"context"
	"fmt"
	"gin/logger"
	"gin/objects"

	"github.com/redis/go-redis/v9"
)

// ConnectRedis initializes Redis client connection
func ConnectRedis(ctx context.Context) error {
	log := logger.WithContext(ctx)
	log.Info("Connecting to Redis...")

	// Create Redis client options
	opts := &redis.Options{
		Addr:     objects.MainConfiguration.RedisUri,
		Password: objects.MainConfiguration.RedisPass,
		DB:       0, // Use default DB
	}

	log.WithField("addr", opts.Addr).Debug("Creating Redis client")

	// Create Redis client
	objects.RedisClient = redis.NewClient(opts)

	// Test the connection
	_, err := objects.RedisClient.Ping(ctx).Result()
	if err != nil {
		log.WithError(err).Error("Failed to connect to Redis")
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}

	log.Info("Successfully connected to Redis")
	return nil
}

// CloseRedis closes the Redis connection
func CloseRedis(ctx context.Context) error {
	log := logger.WithContext(ctx)
	if objects.RedisClient != nil {
		if err := objects.RedisClient.Close(); err != nil {
			log.WithError(err).Error("Failed to close Redis connection")
			return fmt.Errorf("failed to close Redis connection: %w", err)
		}
		log.Info("Successfully closed Redis connection")
	}
	return nil
}
