package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"gin/config"
	"gin/internal/db"
	"gin/logger"
	"gin/objects"

	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/v2/bson"
)

var (
	configPath string
	dryRun     bool
)

func init() {
	flag.StringVar(&configPath, "config", "", "Path to the configuration file directory")
	flag.BoolVar(&dryRun, "dry-run", false, "Preview what would be cleaned without actually deleting anything")
}

func main() {
	flag.Parse()

	// Load configuration
	config.New(configPath)

	// Initialize logger for console output
	if err := logger.InitLogger("", logrus.DebugLevel); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	ctx := logger.WithTransactionID(context.Background())
	log := logger.WithContext(ctx)

	log.Info("ECHO Cleanup Tool - Starting database and Redis cleanup")

	// Connect to MongoDB
	if err := db.ConnectDB(ctx); err != nil {
		log.WithError(err).Fatal("Failed to connect to MongoDB")
	}
	log.WithField("database", string(objects.DBName)).Info("Connected to MongoDB")

	// Connect to Redis
	redisAvailable := true
	if err := db.ConnectRedis(ctx); err != nil {
		log.WithError(err).Warn("Failed to connect to Redis - Redis cleanup will be skipped")
		redisAvailable = false
	} else {
		log.Info("Connected to Redis")
	}

	if dryRun {
		log.Info("=== DRY RUN MODE (no data will be deleted) ===")
		printCleanupPlan(ctx, log)
		disconnect(ctx, log, redisAvailable)
		log.Warn("Dry run complete. Run without -dry-run to perform actual cleanup.")
		os.Exit(0)
	}

	log.Warn("=== LIVE MODE: Proceeding with full cleanup of MongoDB and Redis ===")

	// --- Clean MongoDB ---
	mongoDropped := 0
	if err := cleanMongoDB(ctx, log, &mongoDropped); err != nil {
		log.WithError(err).Fatal("MongoDB cleanup failed")
	}
	log.WithField("collections_dropped", mongoDropped).Info("MongoDB cleanup completed")

	// --- Clean Redis ---
	redisFlushed := 0
	if redisAvailable {
		if err := cleanRedis(ctx, log, &redisFlushed); err != nil {
			log.WithError(err).Fatal("Redis cleanup failed")
		}
		log.WithField("keys_flushed", redisFlushed).Info("Redis cleanup completed")
	}

	// Summary
	log.WithFields(map[string]interface{}{
		"mongo_collections_dropped": mongoDropped,
		"redis_keys_flushed":        redisFlushed,
	}).Info("All cleanup operations completed")

	disconnect(ctx, log, redisAvailable)
	log.Info("Cleanup tool finished")
}

// disconnect closes DB and Redis connections
func disconnect(ctx context.Context, log *logrus.Entry, redisAvailable bool) {
	if err := db.DisconnectDB(ctx); err != nil {
		log.WithError(err).Error("Failed to disconnect from MongoDB")
	}
	if redisAvailable {
		if err := db.CloseRedis(ctx); err != nil {
			log.WithError(err).Error("Failed to close Redis connection")
		}
	}
}

// printCleanupPlan lists what would be cleaned without actually doing it
func printCleanupPlan(ctx context.Context, log *logrus.Entry) {
	// MongoDB collections
	collections, err := objects.DB.ListCollectionNames(ctx, bson.M{})
	if err != nil {
		log.WithError(err).Error("Failed to list MongoDB collections")
	} else if len(collections) == 0 {
		log.Info("MongoDB: no collections found (database is already empty)")
	} else {
		log.WithField("total_collections", len(collections)).Info("MongoDB: would drop the following collections")
		for _, name := range collections {
			log.WithField("collection", name).Info("  - would drop")
		}
	}

	// Redis info
	if objects.RedisClient != nil {
		dbsize, err := objects.RedisClient.DBSize(ctx).Result()
		if err != nil {
			log.WithError(err).Error("Failed to get Redis DB size")
		} else if dbsize == 0 {
			log.Info("Redis: no keys found (database is already empty)")
		} else {
			log.WithField("key_count", dbsize).Info("Redis: would flush all keys")
		}
	}

	log.Info("=== END OF DRY RUN ===")
}

// cleanMongoDB drops all collections in the Echo database.
// droppedCount is set to the number of collections dropped.
func cleanMongoDB(ctx context.Context, log *logrus.Entry, droppedCount *int) error {
	collections, err := objects.DB.ListCollectionNames(ctx, bson.M{})
	if err != nil {
		return fmt.Errorf("failed to list collections: %w", err)
	}

	if len(collections) == 0 {
		log.Info("MongoDB: no collections to drop (database is already empty)")
		return nil
	}

	log.WithField("total_collections", len(collections)).Info("MongoDB: dropping all collections")

	for _, name := range collections {
		log.WithField("collection", name).Info("MongoDB: dropping collection")
		if err := objects.DB.Collection(name).Drop(ctx); err != nil {
			return fmt.Errorf("failed to drop collection %q: %w", name, err)
		}
		log.WithField("collection", name).Info("MongoDB: dropped collection")
	}

	*droppedCount = len(collections)
	log.WithField("count", len(collections)).Info("MongoDB: all collections dropped successfully")
	return nil
}

// cleanRedis flushes all keys from Redis DB 0.
// flushedCount is set to the number of keys that were present.
func cleanRedis(ctx context.Context, log *logrus.Entry, flushedCount *int) error {
	keyCount, err := objects.RedisClient.DBSize(ctx).Result()
	if err != nil {
		return fmt.Errorf("failed to get Redis DB size: %w", err)
	}

	if keyCount == 0 {
		log.Info("Redis: no keys to flush (database is already empty)")
		return nil
	}

	log.WithField("key_count", keyCount).Info("Redis: flushing all keys")

	if err := objects.RedisClient.FlushDB(ctx).Err(); err != nil {
		return fmt.Errorf("failed to flush Redis DB: %w", err)
	}

	*flushedCount = int(keyCount)
	log.WithField("key_count", keyCount).Info("Redis: DB flushed successfully")
	return nil
}
