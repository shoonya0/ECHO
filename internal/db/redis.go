package db

import (
	"context"
	"fmt"
	"gin/objects"
	"log"

	"github.com/redis/go-redis/v9"
)

// ConnectRedis initializes Redis client connection
func ConnectRedis(ctx context.Context) error {
	// Create Redis client options
	opts := &redis.Options{
		Addr:     objects.MainConfiguration.RedisUri,
		Password: objects.MainConfiguration.RedisPass,
		DB:       0, // Use default DB
	}

	// Create Redis client
	objects.RedisClient = redis.NewClient(opts)

	// Test the connection
	_, err := objects.RedisClient.Ping(ctx).Result()
	if err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}

	log.Println("Successfully connected to Redis")
	return nil
}

// CloseRedis closes the Redis connection
func CloseRedis() error {
	if objects.RedisClient != nil {
		return objects.RedisClient.Close()
	}
	return nil
}
